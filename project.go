package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/shim"
)

const projectUsage = `usage: loom project <command>

commands:
  mv <slug> [dir]   rebind a Project's Root to dir (default: the current directory)
  rm <slug>         delete a Project and everything in it
`

// projectCmd runs `loom project`. Both commands go through the daemon's
// tools, so the CLI and agents share one set of rules.
func projectCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, projectUsage)
		return 2
	}
	switch args[0] {
	case "mv":
		return projectMv(args[1:])
	case "rm":
		return projectRm(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "loom project: unknown command %q\n\n%s", args[0], projectUsage)
		return 2
	}
}

func projectMv(args []string) int {
	fs := flag.NewFlagSet("project mv", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: loom project mv <slug> [dir]\n\nRebind a Project's Root to dir (default: the current directory), or, inside git, to the main worktree's root (the daemon resolves it). The Slug and every Issue are kept.\n")
	}
	fs.Parse(args)
	if fs.NArg() < 1 || fs.NArg() > 2 {
		fs.Usage()
		return 2
	}
	slug, dir := fs.Arg(0), "."
	if fs.NArg() == 2 {
		dir = fs.Arg(1)
	}
	dir, err := filepath.Abs(dir)
	if err == nil {
		_, err = os.Stat(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "loom project mv:", err)
		return 1
	}
	out, code := projectCall("project mv", dir, "move_project", map[string]any{"project": slug})
	if code == 0 {
		fmt.Printf("%v -> %v\n", out["slug"], out["root"])
	}
	return code
}

func projectRm(args []string) int {
	fs := flag.NewFlagSet("project rm", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: loom project rm <slug>\n\nHard-delete a Project and every Issue in it, freeing its Slug and Root.\n")
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	slug := fs.Arg(0)
	// Typing the Slug on the command line is the confirmation.
	out, code := projectCall("project rm", "", "delete_project", map[string]any{"slug": slug, "confirm": slug})
	if code == 0 {
		fmt.Printf("deleted %v\n", out["deleted"])
	}
	return code
}

// projectCall calls a tool as a CLI Session working in cwd (the process's
// own if empty). On failure it prints the tool's error message and returns a
// non-zero exit code.
func projectCall(name, cwd, tool string, args map[string]any) (map[string]any, int) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	t, err := target(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loom %s: %v\n", name, err)
		return nil, 1
	}
	s, err := shim.New(t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loom %s: %v\n", name, err)
		return nil, 1
	}
	s.Cwd = cwd
	res, err := s.Call(ctx, tool, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loom %s: %v\n", name, err)
		return nil, 1
	}
	text := ""
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		fmt.Fprintf(os.Stderr, "loom %s: unexpected reply: %q\n", name, text)
		return nil, 1
	}
	if res.IsError {
		if e, ok := payload["error"].(map[string]any); ok {
			fmt.Fprintf(os.Stderr, "loom %s: %v\n", name, e["message"])
		} else {
			fmt.Fprintf(os.Stderr, "loom %s: %s\n", name, text)
		}
		return nil, 1
	}
	return payload, 0
}
