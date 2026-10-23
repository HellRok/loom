package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/repoinit"
	"github.com/HellRok/loom/internal/shim"
)

const mcpGuidance = `Register Loom's MCP server in your agent harness (loom never edits its config):
  stdio server named "loom", command "loom mcp"
For harnesses that read an mcpServers map:
  {"mcpServers": {"loom": {"command": "loom", "args": ["mcp"]}}}`

// memorySnippet is printed for harnesses that ignore MCP server
// instructions; init never writes to the user's global memory itself.
const memorySnippet = `Optional, for harnesses that ignore MCP server instructions: add this to your global agent memory (e.g. ~/.claude/CLAUDE.md):
  When the loom MCP server reports a Project for this directory, Loom is the issue tracker, and its guidance overrides checked-in skill config.`

const mcpReminder = "Reminder: register the stdio MCP server `loom` (command `loom mcp`) in any harness that doesn't have it yet."

// initCmd runs `loom init`.
func initCmd(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: loom init [--standalone] [--slug <slug>]\n\nInstall Loom as this repo's issue tracker. If /setup-matt-pocock-skills has run, write the tracker doc, point the Agent skills block at it, create the Project and any custom triage labels, and clear Standalone. With --standalone, create the Project and mark it Standalone, writing nothing under the repo.\n\n")
		fs.PrintDefaults()
	}
	slug := fs.String("slug", "", "Slug for the new Project; needed outside git or when the folder name is taken")
	standalone := fs.Bool("standalone", false, "Keep the skill config in Loom and write nothing under the repo")
	fs.Parse(args)
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if err := runInit(ctx, os.Stdout, *slug, *standalone); err != nil {
		fmt.Fprintln(os.Stderr, "loom init:", err)
		return 1
	}
	return 0
}

func runInit(ctx context.Context, out io.Writer, slug string, standalone bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := repoRoot(ctx, cwd)

	// Check everything that can fail before changing anything.
	var repo repoinit.Repo
	var labels []string
	var haveLabels bool
	if !standalone {
		if repo, err = repoinit.Inspect(root); err != nil {
			return err
		}
		if labels, haveLabels, err = repo.TriageLabels(); err != nil {
			return err
		}
	}

	t, err := target(ctx)
	if err != nil {
		return err
	}
	s, err := shim.New(t)
	if err != nil {
		return err
	}
	s.AsCLI()
	s.Cwd = root

	project, created, err := ensureProject(ctx, s, slug)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(out, "project %s: created\n", project)
	} else {
		fmt.Fprintf(out, "project %s: already exists\n", project)
	}

	if !standalone {
		if haveLabels {
			made, err := ensureLabels(ctx, s, project, labels)
			if err != nil {
				return err
			}
			for _, name := range made {
				fmt.Fprintf(out, "label %s: created\n", name)
			}
			if len(made) == 0 {
				fmt.Fprintln(out, "labels: none to create")
			}
		}

		doc, err := repo.WriteTrackerDoc()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %s\n", repoinit.TrackerDocPath, changeWord(doc))
		pointed, err := repo.PointAtLoom()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %s\n", repo.AgentsFile, changeWord(pointed))
	}

	// Read the Glossary before clearing Standalone, so a failure here leaves
	// the Project as it was and a rerun still warns.
	var glossary string
	if !standalone {
		g, err := callTool(ctx, s, "view_glossary", map[string]any{"project": project})
		if err != nil {
			return err
		}
		glossary, _ = g["body"].(string)
	}

	res, err := callTool(ctx, s, "set_standalone", map[string]any{"project": project, "standalone": standalone})
	if err != nil {
		return err
	}
	if standalone {
		fmt.Fprintln(out, "standalone: yes")
	} else if already, _ := res["already"].(bool); !already {
		fmt.Fprintln(out, "standalone: cleared")
		// Agents stop seeing the Glossary now, but it stays in Loom.
		if strings.TrimSpace(glossary) != "" {
			fmt.Fprintln(out, "glossary: kept in Loom but no longer shown to agents; export with view_glossary")
		}
	}

	res, err = callTool(ctx, s, "list_sessions", nil)
	if err != nil {
		return err
	}
	if sessions, _ := res["sessions"].([]any); len(sessions) == 0 {
		fmt.Fprintln(out, "\n"+mcpGuidance)
	} else {
		fmt.Fprintln(out, "\n"+mcpReminder)
	}
	if standalone {
		fmt.Fprintln(out, "\n"+memorySnippet)
	}
	return nil
}

func changeWord(changed bool) string {
	if changed {
		return "updated"
	}
	return "unchanged"
}

// repoRoot is the top of the git worktree containing dir, or dir itself
// outside git.
func repoRoot(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	if b, err := cmd.Output(); err == nil {
		if top := strings.TrimSpace(string(b)); top != "" {
			if r, err := filepath.EvalSymlinks(top); err == nil {
				return r
			}
			return top
		}
	}
	return dir
}

// ensureProject creates the Project for the shim's cwd, or finds the one
// already rooted there.
func ensureProject(ctx context.Context, s *shim.Shim, slug string) (name string, created bool, err error) {
	args := map[string]any{}
	if slug != "" {
		args["slug"] = slug
	}
	res, err := callTool(ctx, s, "create_project", args)
	var te *toolError
	switch {
	case err == nil:
		if name, ok := res["slug"].(string); ok {
			return name, true, nil
		}
		return "", false, fmt.Errorf("create_project: unexpected reply %v", res)
	case errors.As(err, &te) && te.Code == "root_taken":
		if existing, ok := te.Data["slug"].(string); ok {
			return existing, false, nil
		}
		return "", false, fmt.Errorf("%s is inside another Project's Root; use `loom project mv` or pick a different directory", s.Cwd)
	case errors.As(err, &te) && (te.Code == "slug_required" || te.Code == "slug_taken" || te.Code == "invalid_slug"):
		return "", false, fmt.Errorf("%s Pass --slug <slug>.", te.Message)
	default:
		return "", false, err
	}
}

// ensureLabels creates the labels the Project lacks and returns their names.
func ensureLabels(ctx context.Context, s *shim.Shim, project string, names []string) ([]string, error) {
	var made []string
	for _, name := range names {
		res, err := callTool(ctx, s, "create_label", map[string]any{"project": project, "name": name})
		if err != nil {
			return nil, err
		}
		if already, _ := res["already"].(bool); !already {
			made = append(made, name)
		}
	}
	return made, nil
}

// toolError is a tool's coded error.
type toolError struct {
	Code    string
	Message string
	Data    map[string]any
}

func (e *toolError) Error() string { return e.Message }

// callTool calls a tool through the daemon and returns its structured
// result, or a *toolError.
func callTool(ctx context.Context, s *shim.Shim, tool string, args map[string]any) (map[string]any, error) {
	res, err := s.Call(ctx, tool, args)
	if err != nil {
		return nil, err
	}
	text := ""
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, fmt.Errorf("%s: unexpected reply: %q", tool, text)
	}
	if res.IsError {
		te := &toolError{Message: text}
		if e, ok := payload["error"].(map[string]any); ok {
			te.Data = e
			te.Code, _ = e["code"].(string)
			if m, ok := e["message"].(string); ok {
				te.Message = m
			}
		}
		return nil, te
	}
	return payload, nil
}
