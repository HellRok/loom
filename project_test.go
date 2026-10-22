package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProjectMvAndRmGoThroughTheDaemon(t *testing.T) {
	in := newInstall(t)
	old, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	moved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Register a Project at old, from there, the way an agent would.
	cmd := in.command("mcp")
	cmd.Dir = old
	cs := shimIn(t, cmd)
	call(t, cs, "create_project", map[string]any{"slug": "widget"})

	if out := in.loom("project", "mv", "widget", moved); out != "widget -> "+moved {
		t.Fatalf("project mv: %q", out)
	}
	list := call(t, cs, "list_projects", nil)["projects"].([]any)
	if root := list[0].(map[string]any)["root"]; len(list) != 1 || root != moved {
		t.Fatalf("after mv: %v", list)
	}

	// Default dir is the current directory.
	mv := in.command("project", "mv", "widget")
	mv.Dir = old
	if out, err := mv.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "widget -> "+old {
		t.Fatalf("project mv without dir: %q %v", out, err)
	}

	// Failures exit non-zero with the tool's message.
	if out, err := in.command("project", "mv", "ghost", moved).CombinedOutput(); err == nil || !strings.Contains(string(out), "ghost") {
		t.Fatalf("mv of an unknown Project: %q %v", out, err)
	}
	if _, err := in.command("project", "mv", "widget", filepath.Join(moved, "nope")).CombinedOutput(); err == nil {
		t.Fatal("mv to a missing directory succeeded")
	}
	if _, err := in.command("project", "rm").CombinedOutput(); err == nil {
		t.Fatal("rm without a slug succeeded")
	}

	if out := in.loom("project", "rm", "widget"); out != "deleted widget" {
		t.Fatalf("project rm: %q", out)
	}
	if got := projectSlugs(call(t, cs, "list_projects", nil)); len(got) != 0 {
		t.Fatalf("after rm: %v", got)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("rm touched the directory: %v", err)
	}
}

// shimIn starts cmd (a loom mcp command) as an agent harness would.
func shimIn(t *testing.T, cmd *exec.Cmd) *mcp.ClientSession {
	t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent"}, nil).
		Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("starting loom mcp: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}
