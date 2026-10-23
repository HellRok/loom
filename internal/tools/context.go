package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/HellRok/loom/internal/store"
)

const (
	headerSession = "X-Loom-Session"
	headerCwd     = "X-Loom-Cwd"
)

// Common holds the arguments every tool accepts.
type Common struct {
	Project string `json:"project,omitempty" jsonschema:"The Slug of the Project to act on; defaults to the Project containing your working directory"`
	Session string `json:"session,omitempty" jsonschema:"Your Session id, for direct-HTTP clients; the X-Loom-Session header wins"`
}

func (c Common) common() Common { return c }

// call is the request context a tool handler runs in.
type call struct {
	Session string
	// Cwd is the caller's working directory, cleaned and with symlinks
	// resolved where possible; empty if the caller sent none.
	Cwd string
	// ProjectSlug is the explicit `project` argument; empty if none was
	// passed.
	ProjectSlug string
}

// newCall resolves the Session and cwd from the request headers and the
// common arguments.
func newCall(h http.Header, args Common) (call, error) {
	s := h.Get(headerSession)
	if s == "" {
		s = args.Session
	}
	if s == "" {
		return call{}, fail(codeNoSession, "Every call needs a Session: send the X-Loom-Session header or a `session` argument (use `loom mcp` to have one sent for you).", nil)
	}
	if n := utf8.RuneCountInString(s); n < 16 || n > 256 {
		return call{}, fail(codeInvalidSession, fmt.Sprintf("A Session must be 16 to 256 characters; got %d.", n), nil)
	}
	c := call{Session: s, ProjectSlug: args.Project}
	if cwd := h.Get(headerCwd); cwd != "" {
		c.Cwd = canonical(cwd)
	}
	return c, nil
}

func canonical(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// mainWorktreeRoot returns the root of the main worktree of the git
// repository containing dir, so linked worktrees resolve to the same place.
func mainWorktreeRoot(ctx context.Context, dir string) (string, bool) {
	if _, err := os.Stat(dir); err != nil {
		return "", false
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	common := canonical(strings.TrimSpace(string(out)))
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common), true
	}
	// A submodule keeps its git dir under the superproject's .git, so fall
	// back to its own top level. This fails in a bare repository.
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	if out, err = cmd.Output(); err != nil {
		return "", false
	}
	return canonical(strings.TrimSpace(string(out))), true
}

// location is where the caller is working: the main worktree root inside
// git, else the cwd itself.
func (c call) location(ctx context.Context) (path string, inGit bool) {
	if root, ok := mainWorktreeRoot(ctx, c.Cwd); ok {
		return root, true
	}
	return c.Cwd, false
}

// resolveProject picks the Project a call acts on: the explicit `project`
// Slug if given, else the Project with the longest Root containing the
// caller's location. Tools that act within a Project call it; the Project
// tools themselves don't.
func (h *handlers) resolveProject(ctx context.Context, c call) (store.Project, error) {
	if c.ProjectSlug != "" {
		p, err := h.store.ProjectBySlug(ctx, c.ProjectSlug)
		if errors.Is(err, store.ErrNotFound) {
			return p, h.unknownProject(ctx, c.ProjectSlug)
		}
		return p, err
	}
	if c.Cwd == "" {
		return store.Project{}, h.projectError(ctx, codeNoProject, "No working directory was sent (X-Loom-Cwd), so pass `project` with a Slug.")
	}
	best, loc, err := h.projectAt(ctx, c)
	if err != nil {
		return store.Project{}, err
	}
	if best.Slug == "" {
		return best, h.projectError(ctx, codeNoProject, fmt.Sprintf("No Project's Root contains %s. Pass `project` with a Slug, or call create_project here.", loc))
	}
	return best, nil
}

// projectAt finds the Project with the longest Root containing the caller's
// location, which it also returns. The Project is zero if none does.
func (h *handlers) projectAt(ctx context.Context, c call) (best store.Project, loc string, err error) {
	loc, _ = c.location(ctx)
	ps, err := h.store.Projects(ctx)
	if err != nil {
		return store.Project{}, loc, err
	}
	for _, p := range ps {
		if within(loc, p.Root) && len(p.Root) > len(best.Root) {
			best = p
		}
	}
	return best, loc, nil
}

func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}

// projectError is a coded error listing every Project with its Root.
func (h *handlers) projectError(ctx context.Context, code Code, message string) error {
	ps, err := h.store.Projects(ctx)
	if err != nil {
		return err
	}
	return fail(code, message, map[string]any{"projects": toProjectOuts(ps)})
}

// unknownProject reports that no Project has slug, listing those that exist.
func (h *handlers) unknownProject(ctx context.Context, slug string) error {
	return h.projectError(ctx, codeUnknownProject, fmt.Sprintf("No Project has the Slug %q.", slug))
}
