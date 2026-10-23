package shim_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/repoinit"
	"github.com/HellRok/loom/internal/shim"
)

// withProjects makes the stub answer list_projects as the daemon does:
// every Project, and `current`, the Slug of the one covering the caller.
func (d *daemonStub) withProjects(current string, projects ...map[string]any) {
	d.withProjectsAfter(nil, current, projects...)
}

// withHangingProjects is withProjects with a daemon that doesn't answer
// until the test ends.
func (d *daemonStub) withHangingProjects(t *testing.T, current string, projects ...map[string]any) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	d.withProjectsAfter(release, current, projects...)
}

// withProjectsAfter answers list_projects once wait (if any) is closed.
func (d *daemonStub) withProjectsAfter(wait <-chan struct{}, current string, projects ...map[string]any) {
	mcp.AddTool(d.server, &mcp.Tool{Name: "list_projects"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, map[string]any, error) {
		if wait != nil {
			<-wait
		}
		out := map[string]any{"projects": projects}
		for _, p := range projects {
			if p["slug"] == current {
				out["current"] = p
			}
		}
		return nil, out, nil
	})
}

// handshakes are the ways an agent opens a session: the legacy initialize
// and the stateless server/discover. Instructions must reach both.
var handshakes = map[string]*mcp.ClientSessionOptions{
	"initialize":      {ProtocolVersion: "2025-11-25"},
	"server/discover": nil,
}

// agentIn connects an agent, over the handshake opts selects, to a daemon
// that reports current (a Slug, or empty for none) among projects.
func agentIn(t *testing.T, opts *mcp.ClientSessionOptions, current string, projects ...map[string]any) *mcp.ClientSession {
	t.Helper()
	home := t.TempDir()
	d := newDaemonStub(t, home, "v1.0.0")
	d.withProjects(current, projects...)
	return connectWith(t, shim.Target{Home: home, Port: d.port(), Version: "v1.0.0"}, noSpawn(t), opts)
}

func TestInstructionsForAPlainProjectNameItAndCarryTheConventions(t *testing.T) {
	for name, opts := range handshakes {
		t.Run(name, func(t *testing.T) {
			cs := agentIn(t, opts, "app", map[string]any{"slug": "app", "root": "/work/app"})
			checkPlainInstructions(t, cs.InitializeResult().Instructions)
		})
	}
}

func checkPlainInstructions(t *testing.T, got string) {
	t.Helper()
	for _, want := range []string{
		"`app`",
		"overrides",
		"needs-triage", "needs-info", "ready-for-agent", "ready-for-human", "wontfix",
		repoinit.Conventions,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("instructions lack %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Written by `loom init`", "Register the server once per machine"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("instructions include %q:\n%s", unwanted, got)
		}
	}
}

func TestInstructionsForAStandaloneProjectNameItToo(t *testing.T) {
	for name, opts := range handshakes {
		t.Run(name, func(t *testing.T) {
			cs := agentIn(t, opts, "app", map[string]any{"slug": "app", "root": "/work/app", "standalone": true})
			checkPlainInstructions(t, cs.InitializeResult().Instructions)
		})
	}
}

func TestNoInstructionsWhenNoProjectCoversTheCwd(t *testing.T) {
	for name, opts := range handshakes {
		t.Run(name, func(t *testing.T) {
			cs := agentIn(t, opts, "", map[string]any{"slug": "elsewhere", "root": "/work/elsewhere"})
			if got := cs.InitializeResult().Instructions; got != "" {
				t.Fatalf("want no instructions, got:\n%s", got)
			}
		})
	}
}

func TestStaticInstructionsWhenTheDaemonIsSlowOrUnavailable(t *testing.T) {
	shim.SetInstructionsTimeout(t, 100*time.Millisecond)
	for name, opts := range handshakes {
		t.Run(name+"/slow", func(t *testing.T) {
			home := t.TempDir()
			d := newDaemonStub(t, home, "v1.0.0")
			d.withHangingProjects(t, "app", map[string]any{"slug": "app", "root": "/work/app"})
			cs := connectWith(t, shim.Target{Home: home, Port: d.port(), Version: "v1.0.0"}, noSpawn(t), opts)
			checkStaticInstructions(t, cs)
		})
		t.Run(name+"/unavailable", func(t *testing.T) {
			// Nothing listens on the port and spawning a daemon fails.
			target := shim.Target{Home: t.TempDir(), Port: freePort(t), Version: "v1.0.0"}
			cs := connectWith(t, target, func(context.Context) (<-chan error, error) {
				return nil, errors.New("no daemon here")
			}, opts)
			checkStaticInstructions(t, cs)
		})
	}
}

func checkStaticInstructions(t *testing.T, cs *mcp.ClientSession) {
	t.Helper()
	if got := cs.InitializeResult().Instructions; got != repoinit.StaticInstructions {
		t.Fatalf("want the static instructions, got:\n%s", got)
	}
}
