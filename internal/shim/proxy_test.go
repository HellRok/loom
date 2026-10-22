package shim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/shim"
)

// daemonStub is a Loom-shaped daemon: /healthz plus a stateless /mcp whose
// `echo` tool returns the Loom headers it saw. `drop` hangs up mid-call.
type daemonStub struct {
	srv  *httptest.Server
	home string

	mu   sync.Mutex
	seen []http.Header
}

func newDaemonStub(t *testing.T, home, version string) *daemonStub {
	t.Helper()
	d := &daemonStub{home: home}
	server := mcp.NewServer(&mcp.Implementation{Name: "loom"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, in struct {
		Say string `json:"say"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		d.mu.Lock()
		d.seen = append(d.seen, req.Extra.Header.Clone())
		d.mu.Unlock()
		return nil, map[string]string{"said": in.Say}, nil
	})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(loomHealth(version, home))
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(peekBody(r), `"drop"`) {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "drop"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return nil, nil, errors.New("unreachable")
	})
	d.srv = httptest.NewServer(mux)
	t.Cleanup(d.srv.Close)
	return d
}

func (d *daemonStub) port() int { return d.srv.Listener.Addr().(*net.TCPAddr).Port }

// peekBody reads r's body and puts it back.
func peekBody(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(b))
	return string(b)
}

// connect runs the shim against target and returns an agent's session to
// it. spawn stands in for starting a daemon process.
func connect(t *testing.T, target shim.Target, spawn func(context.Context) (<-chan error, error)) *mcp.ClientSession {
	t.Helper()
	s, err := shim.NewWithSpawn(target, spawn)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := s.Server().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func noSpawn(t *testing.T) func(context.Context) (<-chan error, error) {
	return func(context.Context) (<-chan error, error) {
		t.Error("spawned a daemon")
		return nil, errors.New("no spawning here")
	}
}

// toolError decodes a tool error's {error: {...}} payload.
func toolError(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if !res.IsError {
		t.Fatalf("want a tool error, got %+v", res)
	}
	var payload struct{ Error map[string]any }
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &payload); err != nil || payload.Error == nil {
		t.Fatalf("error is not {error: ...}: %v", res.Content[0])
	}
	return payload.Error
}

func TestShimForwardsToolsAndCallsLiveWithSessionAndCwd(t *testing.T) {
	home := t.TempDir()
	d := newDaemonStub(t, home, "v1.0.0")
	cs := connect(t, shim.Target{Home: home, Port: d.port(), Version: "v1.0.0"}, noSpawn(t))
	ctx := context.Background()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if strings.Join(names, ",") != "drop,echo" {
		t.Fatalf("tools/list: got %v", names)
	}

	for _, say := range []string{"one", "two"} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"say": say}})
		if err != nil || res.IsError {
			t.Fatalf("echo: %v %+v", err, res)
		}
		if got := res.StructuredContent.(map[string]any)["said"]; got != say {
			t.Fatalf("echo returned %v, want %q", got, say)
		}
	}

	cwd, _ := os.Getwd()
	if len(d.seen) != 2 {
		t.Fatalf("daemon saw %d calls", len(d.seen))
	}
	first := d.seen[0].Get("X-Loom-Session")
	if id, err := uuid.Parse(first); err != nil || id.Version() != 7 {
		t.Fatalf("X-Loom-Session %q is not a UUIDv7", first)
	}
	for _, h := range d.seen {
		if h.Get("X-Loom-Session") != first {
			t.Fatalf("Session changed between calls: %q then %q", first, h.Get("X-Loom-Session"))
		}
		if h.Get("X-Loom-Cwd") != cwd {
			t.Fatalf("X-Loom-Cwd %q, want %q", h.Get("X-Loom-Cwd"), cwd)
		}
	}
}

func TestShimReportsAForwardingFailureAsOutcomeUnknownThenRedetects(t *testing.T) {
	home := t.TempDir()
	d := newDaemonStub(t, home, "v1.0.0")
	spawned := 0
	cs := connect(t, shim.Target{Home: home, Port: d.port(), Version: "v1.0.0"}, func(context.Context) (<-chan error, error) {
		spawned++
		exited := make(chan error, 1)
		exited <- errors.New("exit status 1")
		return exited, nil
	})
	ctx := context.Background()
	logPath := filepath.Join(home, "serve.log")

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "drop", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	e := toolError(t, res)
	msg, _ := e["message"].(string)
	if e["code"] != "outcome_unknown" || !strings.Contains(msg, "verify") || !strings.Contains(msg, logPath) || e["log"] != logPath {
		t.Fatalf("got %v", e)
	}
	if spawned != 0 {
		t.Fatal("a forwarding failure must not respawn or retry")
	}

	// The daemon dies; the next call re-runs detect-and-spawn, and the new
	// daemon's startup failure points at serve.log.
	d.srv.Close()
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"say": "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	e = toolError(t, res)
	msg, _ = e["message"].(string)
	if spawned != 1 || e["code"] != "daemon_unavailable" || !strings.Contains(msg, logPath) || e["log"] != logPath {
		t.Fatalf("spawned %d, got %v", spawned, e)
	}
}

func TestShimRefusesAForeignLoomHomeAsAToolError(t *testing.T) {
	d := newDaemonStub(t, "/somebody/elses/loom", "v1.0.0")
	home := t.TempDir()
	cs := connect(t, shim.Target{Home: home, Port: d.port(), Version: "v1.0.0"}, noSpawn(t))

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	e := toolError(t, res)
	if msg, _ := e["message"].(string); e["code"] != "daemon_unavailable" || !strings.Contains(msg, "/somebody/elses/loom") {
		t.Fatalf("got %v", e)
	}
	if len(d.seen) != 0 {
		t.Fatal("call reached a foreign daemon")
	}
}
