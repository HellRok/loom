package daemon_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/config"
	"github.com/HellRok/loom/internal/daemon"
	"github.com/HellRok/loom/internal/store"
)

func handler(t *testing.T, port int) http.Handler {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(home, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return daemon.Handler(daemon.Options{Config: config.Config{Home: home, Port: port}, Version: "v9.9.9"}, st)
}

func TestHealthzIdentifiesLoom(t *testing.T) {
	h := handler(t, 7337)
	req := httptest.NewRequest("GET", "http://127.0.0.1:7337/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if got["name"] != "loom" || got["version"] != "v9.9.9" || got["pid"] != float64(os.Getpid()) || got["home"] == "" {
		t.Fatalf("got %v", got)
	}
}

func TestServesWebUIAlongsideMCP(t *testing.T) {
	h := handler(t, 7337)
	for path, want := range map[string]int{"/": 200, "/static/loom.css": 200, "/nope": 404, "/healthz": 200} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:7337"+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("GET %s: got %d, want %d", path, rec.Code, want)
		}
	}
}

func TestHostCheck(t *testing.T) {
	h := handler(t, 7337)
	for host, want := range map[string]int{
		"127.0.0.1:7337":    200,
		"localhost:7337":    200,
		"127.0.0.1:8080":    http.StatusForbidden,
		"evil.example:7337": http.StatusForbidden,
		"localhost":         http.StatusForbidden,
	} {
		req := httptest.NewRequest("GET", "/healthz", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %q: got %d, want %d", host, rec.Code, want)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestRunServesThenStopsAndASecondDaemonExitsQuietly(t *testing.T) {
	home := t.TempDir()
	cfg := config.Config{Home: home, Port: freePort(t), Assignee: "dev"}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx, daemon.Options{Config: cfg, Version: "v1"}) }()

	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", cfg.Port)
	var resp *http.Response
	for deadline := time.Now().Add(5 * time.Second); ; {
		var err error
		if resp, err = http.Get(url); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon never came up: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	resp.Body.Close()

	lock, _ := os.ReadFile(filepath.Join(home, "loom.lock"))
	if strings.TrimSpace(string(lock)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lockfile holds %q, want our pid", lock)
	}

	projects := callOverHTTP(t, cfg.Port, home)
	if len(projects) != 1 || projects[0].(map[string]any)["root"] != resolved(t, home) {
		t.Fatalf("list_projects over HTTP: got %v", projects)
	}

	err := daemon.Run(context.Background(), daemon.Options{Config: cfg, Version: "v1"})
	if !errors.Is(err, daemon.ErrAlreadyRunning) {
		t.Fatalf("second daemon: want ErrAlreadyRunning, got %v", err)
	}

	stop()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := http.Get(url); err == nil {
		t.Fatal("port still serving after shutdown")
	}
}

func TestRunReportsPortInUse(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	cfg := config.Config{Home: t.TempDir(), Port: taken.Addr().(*net.TCPAddr).Port}

	err = daemon.Run(context.Background(), daemon.Options{Config: cfg, Version: "v1"})
	if !errors.Is(err, daemon.ErrPortInUse) {
		t.Fatalf("want ErrPortInUse, got %v", err)
	}
}

func TestRunReportsOtherBindFailures(t *testing.T) {
	// Port 1 is privileged on Linux, so binding it fails with a permission
	// error, which must not pass for a running daemon.
	if l, err := net.Listen("tcp", "127.0.0.1:1"); err == nil {
		l.Close()
		t.Skip("this user can bind port 1")
	}
	cfg := config.Config{Home: t.TempDir(), Port: 1}

	err := daemon.Run(context.Background(), daemon.Options{Config: cfg, Version: "v1"})
	if err == nil || errors.Is(err, daemon.ErrPortInUse) {
		t.Fatalf("want a bind error other than ErrPortInUse, got %v", err)
	}
}

type headerTransport struct{ cwd string }

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Loom-Session", "http-client-session-1")
	r.Header.Set("X-Loom-Cwd", h.cwd)
	return http.DefaultTransport.RoundTrip(r)
}

// callOverHTTP is a direct-HTTP MCP client: it creates a Project at cwd and
// returns list_projects' projects.
func callOverHTTP(t *testing.T, port int, cwd string) []any {
	t.Helper()
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   fmt.Sprintf("http://127.0.0.1:%d/mcp", port),
		HTTPClient: &http.Client{Transport: headerTransport{cwd: cwd}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "create_project", Arguments: map[string]any{"slug": "home"}})
	if err != nil || res.IsError {
		t.Fatalf("create_project: %v %v", err, res)
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_projects: %v %v", err, res)
	}
	return res.StructuredContent.(map[string]any)["projects"].([]any)
}

func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
