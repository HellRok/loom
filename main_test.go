package main_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"
)

// loomBin is the binary under test, built once by TestMain.
var loomBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "loom-bin")
	if err != nil {
		panic(err)
	}
	loomBin = filepath.Join(dir, "loom")
	if out, err := exec.Command("go", "build", "-o", loomBin, ".").CombinedOutput(); err != nil {
		panic(fmt.Sprintf("building loom: %v\n%s", err, out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// install is one Loom home with its own free port. Its daemon is stopped
// when the test ends.
type install struct {
	t    *testing.T
	home string
	port int
}

func newInstall(t *testing.T) *install {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), fmt.Appendf(nil, "port = %d\nassignee = \"dev\"\n", port), 0o600); err != nil {
		t.Fatal(err)
	}
	in := &install{t: t, home: home, port: port}
	t.Cleanup(func() { in.loom("stop") })
	return in
}

func (in *install) command(args ...string) *exec.Cmd {
	cmd := exec.Command(loomBin, args...)
	cmd.Env = append(os.Environ(), "LOOM_HOME="+in.home)
	cmd.Dir = in.home
	return cmd
}

// loom runs a loom subcommand to completion and returns its output.
func (in *install) loom(args ...string) string {
	in.t.Helper()
	out, err := in.command(args...).CombinedOutput()
	if err != nil {
		in.t.Fatalf("loom %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// shim starts `loom mcp` as an agent harness would.
func (in *install) shim() *mcp.ClientSession {
	in.t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent"}, nil).
		Connect(context.Background(), &mcp.CommandTransport{Command: in.command("mcp")}, nil)
	if err != nil {
		in.t.Fatalf("starting loom mcp: %v", err)
	}
	in.t.Cleanup(func() { cs.Close() })
	return cs
}

func (in *install) healthz() (map[string]any, error) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", in.port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var h map[string]any
	return h, json.NewDecoder(resp.Body).Decode(&h)
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s: %s", name, res.Content[0].(*mcp.TextContent).Text)
	}
	return res.StructuredContent.(map[string]any)
}

func projectSlugs(out map[string]any) []string {
	var slugs []string
	for _, p := range out["projects"].([]any) {
		slugs = append(slugs, p.(map[string]any)["slug"].(string))
	}
	return slugs
}

func TestShimSpawnsTheDaemonASecondShimReusesItAndStopStopsIt(t *testing.T) {
	in := newInstall(t)

	first := in.shim()
	tools, err := first.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) == 0 {
		t.Fatalf("tools/list: %v %v", tools, err)
	}
	call(t, first, "create_project", map[string]any{"slug": "smoke"})
	h, err := in.healthz()
	if err != nil {
		t.Fatalf("no daemon after the first call: %v", err)
	}
	pid := h["pid"]
	if _, err := os.Stat(filepath.Join(in.home, "serve.log")); err != nil {
		t.Fatalf("daemon output not in serve.log: %v", err)
	}

	second := in.shim()
	if got := projectSlugs(call(t, second, "list_projects", nil)); strings.Join(got, ",") != "smoke" {
		t.Fatalf("second shim sees %v", got)
	}
	if h, _ := in.healthz(); h["pid"] != pid {
		t.Fatalf("second shim replaced the daemon: pid %v, was %v", h["pid"], pid)
	}

	if out := in.loom("stop"); out != "stopped" {
		t.Fatalf("loom stop: %q", out)
	}
	if _, err := in.healthz(); err == nil {
		t.Fatal("daemon still answering after loom stop")
	}
	if out := in.loom("stop"); out != "not running" {
		t.Fatalf("second loom stop: %q", out)
	}

	// The next call respawns the daemon.
	if got := projectSlugs(call(t, first, "list_projects", nil)); strings.Join(got, ",") != "smoke" {
		t.Fatalf("after respawn: %v", got)
	}
}

func TestRacingShimsEndUpWithOneDaemon(t *testing.T) {
	in := newInstall(t)
	a, b := in.shim(), in.shim()

	var wg sync.WaitGroup
	for _, cs := range []*mcp.ClientSession{a, b} {
		wg.Go(func() { call(t, cs, "list_projects", nil) })
	}
	wg.Wait()

	h, err := in.healthz()
	if err != nil {
		t.Fatal(err)
	}
	lock, _ := os.ReadFile(filepath.Join(in.home, "loom.lock"))
	if strings.TrimSpace(string(lock)) != fmt.Sprint(h["pid"]) {
		t.Fatalf("lock holder %q is not the daemon answering (%v)", lock, h["pid"])
	}
	if out := in.loom("stop"); out != "stopped" {
		t.Fatalf("loom stop: %q", out)
	}
	if _, err := in.healthz(); err == nil {
		t.Fatal("a second daemon is still answering")
	}
}

// A daemon that can't bring the database up to date exits non-zero, says why
// in its log, and never serves.
func TestServeExitsWhenTheDatabaseCannotBeUsed(t *testing.T) {
	snaps, err := filepath.Glob("internal/store/testdata/snapshots/*.db")
	if err != nil || len(snaps) == 0 {
		t.Fatalf("no snapshots (err %v)", err)
	}
	cases := map[string]struct {
		damage string // run against the copied snapshot
		want   string // in the daemon's log
	}{
		"newer than the binary": {"INSERT INTO goose_db_version (version_id, is_applied) VALUES (9999, 1)", "9999"},
		"failed migration":      {"DELETE FROM goose_db_version WHERE version_id = (SELECT MAX(version_id) FROM goose_db_version)", "migrat"},
	}
	for name, c := range cases {
		for _, snap := range snaps {
			t.Run(name+"/"+filepath.Base(snap), func(t *testing.T) {
				in := newInstall(t)
				b, err := os.ReadFile(snap)
				if err != nil {
					t.Fatal(err)
				}
				dbPath := filepath.Join(in.home, "loom.db")
				if err := os.WriteFile(dbPath, b, 0o600); err != nil {
					t.Fatal(err)
				}
				db, err := sql.Open("sqlite", "file:"+dbPath)
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec(c.damage)
				db.Close()
				if err != nil {
					t.Fatal(err)
				}

				out, err := in.command("serve").CombinedOutput()
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() == 0 {
					t.Fatalf("loom serve should exit non-zero, got %v\n%s", err, out)
				}
				if !strings.Contains(string(out), c.want) {
					t.Errorf("log should mention %q:\n%s", c.want, out)
				}
				if _, err := in.healthz(); err == nil {
					t.Error("daemon answered /healthz despite the database failure")
				}
			})
		}
	}
}
