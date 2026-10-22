package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/store"
	"github.com/HellRok/loom/internal/tools"
)

// harness drives the tool handlers in-process through a real MCP client,
// injecting the HTTP headers the daemon would see on each call.
type harness struct {
	t       *testing.T
	store   *store.Store
	client  *mcp.ClientSession
	headers http.Header
}

const testSession = "test-session-0001"

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	h := &harness{t: t, store: st}
	srv := tools.NewServer(tools.Deps{Store: st})
	srv.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if r, ok := req.(*mcp.CallToolRequest); ok {
				r.Extra = &mcp.RequestExtra{Header: h.headers}
			}
			return next(ctx, method, req)
		}
	})

	ctx := context.Background()
	ct, st2 := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st2, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	h.client = cs
	return h
}

// result is one tool call's outcome: either Out (structured output) or Err
// (the decoded {error: {...}} payload).
type result struct {
	Out map[string]any
	Err map[string]any
}

func (r result) code() string {
	if r.Err == nil {
		return ""
	}
	c, _ := r.Err["code"].(string)
	return c
}

// call invokes a tool as an agent in cwd with the test Session.
func (h *harness) call(cwd, name string, args map[string]any) result {
	h.t.Helper()
	hdr := http.Header{}
	hdr.Set("X-Loom-Session", testSession)
	if cwd != "" {
		hdr.Set("X-Loom-Cwd", cwd)
	}
	return h.callWith(hdr, name, args)
}

func (h *harness) callWith(hdr http.Header, name string, args map[string]any) result {
	h.t.Helper()
	h.headers = hdr
	if args == nil {
		args = map[string]any{}
	}
	res, err := h.client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		h.t.Fatalf("%s: protocol error: %v", name, err)
	}
	if len(res.Content) != 1 {
		h.t.Fatalf("%s: want one content block, got %d", name, len(res.Content))
	}
	text := res.Content[0].(*mcp.TextContent).Text
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		h.t.Fatalf("%s: content is not JSON: %q", name, text)
	}
	if res.IsError {
		e, ok := payload["error"].(map[string]any)
		if !ok {
			h.t.Fatalf("%s: error without {error: ...} shape: %s", name, text)
		}
		return result{Err: e}
	}
	var structured map[string]any
	b, _ := json.Marshal(res.StructuredContent)
	json.Unmarshal(b, &structured)
	if string(mustJSON(structured)) != string(mustJSON(payload)) {
		h.t.Fatalf("%s: text copy %s differs from structured %s", name, text, b)
	}
	return result{Out: payload}
}

// mustOK fails the test unless the call succeeded.
func (h *harness) mustOK(r result) map[string]any {
	h.t.Helper()
	if r.Err != nil {
		h.t.Fatalf("unexpected error: %v", r.Err)
	}
	return r.Out
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// gitRepo makes a git repository at dir/name and returns its resolved path.
func gitRepo(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, p, "init", "-q")
	git(t, p, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	return resolved(t, p)
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// resolved returns p with symlinks evaluated (macOS's /tmp is a symlink).
func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return resolved(t, p)
}
