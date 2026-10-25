//go:build system

// Package systest boots a real Loom daemon and drives its web UI in a
// headless Chrome. System tests are behind the `system` build tag:
//
//	go test -tags system ./internal/systest/
package systest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/config"
	"github.com/HellRok/loom/internal/daemon"
)

// Loom is a running daemon, a working directory registered as a Project's
// root, and a browser pointed at its web UI.
type Loom struct {
	t       *testing.T
	Port    int
	Root    string
	tools   *mcp.ClientSession
	Browser context.Context
}

// Start boots a daemon on a temp Loom home and a free port, creates a
// Project with this slug, and opens a headless Chrome. Everything is torn
// down when the test ends.
func Start(t *testing.T, slug string) *Loom {
	t.Helper()
	chrome := findChrome(t)
	l := &Loom{t: t, Port: freePort(t), Root: t.TempDir()}

	cfg := config.Config{Home: t.TempDir(), Port: l.Port, Assignee: "dev"}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx, daemon.Options{Config: cfg, Version: "system-test"}) }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Errorf("daemon: %v", err)
		}
	})
	l.waitHealthy(done)

	l.tools = l.connect()
	l.Call("create_project", map[string]any{"slug": slug})

	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome))...)
	t.Cleanup(cancelAlloc)
	browser, cancelBrowser := chromedp.NewContext(alloc)
	t.Cleanup(cancelBrowser)
	// Launch Chrome now, so a broken install fails here rather than in the
	// test's first action.
	if err := chromedp.Do(browser); err != nil {
		t.Fatalf("starting Chrome at %s: %v", chrome, err)
	}
	l.Browser = browser
	l.collectConsoleErrors()
	return l
}

// collectConsoleErrors records every console error the page reports:
// console.error calls, uncaught exceptions and browser log errors such as
// failed requests. The test fails at cleanup if there were any, so a page
// that spews errors cannot pass.
func (l *Loom) collectConsoleErrors() {
	var mu sync.Mutex
	var errs []string
	messages := chromedp.Console(l.Browser)
	go func() {
		for m, err := range messages {
			if err != nil {
				return
			}
			if m.Type == chromedp.ConsoleError || m.IsException() {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s (%s:%d)", m, m.URL, m.Line))
				mu.Unlock()
			}
		}
	}()
	l.t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if len(errs) > 0 {
			l.t.Errorf("browser console errors:\n%s", strings.Join(errs, "\n"))
		}
	})
}

// URL is the web UI's address for path.
func (l *Loom) URL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", l.Port, path)
}

// Do runs browser steps in order, failing the test on error or after 10
// seconds.
func (l *Loom) Do(steps ...chromedp.Action[chromedp.Void]) {
	l.t.Helper()
	Get(l, chromedp.Steps(steps...))
}

// Get runs a browser action and returns its value, failing the test on error
// or after 10 seconds.
func Get[T any](l *Loom, a chromedp.Action[T]) T {
	l.t.Helper()
	ctx, cancel := context.WithTimeout(l.Browser, 10*time.Second)
	defer cancel()
	v, err := chromedp.Run(ctx, a)
	if err != nil {
		l.t.Fatalf("browser: %v", err)
	}
	return v
}

// Call calls a Loom tool from the Project's root, failing the test if it
// errors, and returns its structured result.
func (l *Loom) Call(tool string, args map[string]any) map[string]any {
	l.t.Helper()
	res, err := l.tools.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		l.t.Fatalf("%s: %v", tool, err)
	}
	if res.IsError {
		l.t.Fatalf("%s: %v", tool, res.Content)
	}
	out, _ := res.StructuredContent.(map[string]any)
	return out
}

func (l *Loom) connect() *mcp.ClientSession {
	l.t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "systest"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   l.URL("/mcp"),
		HTTPClient: &http.Client{Transport: headers{cwd: l.Root}},
	}, nil)
	if err != nil {
		l.t.Fatalf("connecting to /mcp: %v", err)
	}
	l.t.Cleanup(func() { cs.Close() })
	return cs
}

func (l *Loom) waitHealthy(done <-chan error) {
	l.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		select {
		case err := <-done:
			l.t.Fatalf("daemon exited during startup: %v", err)
		default:
		}
		resp, err := http.Get(l.URL("/healthz"))
		if err == nil {
			resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			l.t.Fatalf("daemon never came up: %v", err)
		}
	}
}

// headers identifies the harness to the daemon as a direct-HTTP client
// working in the Project's root.
type headers struct{ cwd string }

func (h headers) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Loom-Session", "system-test-session")
	r.Header.Set("X-Loom-Cwd", h.cwd)
	return http.DefaultTransport.RoundTrip(r)
}

// findChrome returns the locally installed Chrome or Chromium, failing the
// test if there isn't one: the system tests were asked for, so not running
// them must not pass quietly.
func findChrome(t *testing.T) string {
	t.Helper()
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			filepath.Join(os.Getenv("HOME"), "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
		}
	case "linux":
		candidates = []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	t.Fatalf("system tests need Chrome or Chromium installed; looked for %q", candidates)
	return ""
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
