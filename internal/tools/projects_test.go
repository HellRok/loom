package tools_test

import (
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/HellRok/loom/internal/shim"
)

func TestCreateProjectInsideGitUsesGitRootAndBasename(t *testing.T) {
	h := newHarness(t)
	repo := gitRepo(t, t.TempDir(), "widget")
	sub := mkdir(t, repo, "cmd", "deep")

	out := h.mustOK(h.call(sub, "create_project", nil))

	if out["slug"] != "widget" || out["root"] != repo {
		t.Fatalf("got %v, want slug widget root %s", out, repo)
	}
}

func TestCreateProjectOutsideGitNeedsSlugAndUsesCwd(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "notes")

	if r := h.call(dir, "create_project", nil); r.code() != "slug_required" {
		t.Fatalf("want slug_required, got %v", r)
	}
	out := h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "my-notes"}))
	if out["slug"] != "my-notes" || out["root"] != dir {
		t.Fatalf("got %v", out)
	}
}

func TestCreateProjectWithoutCwd(t *testing.T) {
	h := newHarness(t)
	if r := h.call("", "create_project", map[string]any{"slug": "x"}); r.code() != "no_cwd" {
		t.Fatalf("want no_cwd, got %v", r)
	}
}

func TestCreateProjectRejectsInvalidSlug(t *testing.T) {
	h := newHarness(t)
	repo := gitRepo(t, t.TempDir(), "My Repo")

	r := h.call(repo, "create_project", nil)
	if r.code() != "invalid_slug" || r.Err["suggestion"] != "my-repo" {
		t.Fatalf("want invalid_slug suggesting my-repo, got %v", r)
	}

	// Uppercase is invalid, but the suggestion keeps the underscore.
	upper := gitRepo(t, t.TempDir(), "My_Repo")
	r = h.call(upper, "create_project", nil)
	if r.code() != "invalid_slug" || r.Err["suggestion"] != "my_repo" {
		t.Fatalf("want invalid_slug suggesting my_repo, got %v", r)
	}
}

func TestCreateProjectRejectsSlugsOutsideTheAllowedCharacters(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "notes")

	for _, slug := range []string{"Upper", "has space", "a#b", "a/b", "a.b"} {
		if r := h.call(dir, "create_project", map[string]any{"slug": slug}); r.code() != "invalid_slug" {
			t.Errorf("slug %q: want invalid_slug, got %v", slug, r)
		}
	}
}

func TestCreateProjectRejectsSlugsTheWebUIClaims(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "notes")

	for _, slug := range []string{"static", "events", "sessions"} {
		r := h.call(dir, "create_project", map[string]any{"slug": slug})
		if r.code() != "reserved_slug" {
			t.Fatalf("slug %q: want reserved_slug, got %v", slug, r)
		}
		if reserved, _ := r.Err["reserved"].([]any); len(reserved) != 3 {
			t.Errorf("slug %q: want the three reserved names listed, got %v", slug, r.Err["reserved"])
		}
	}
	if out := h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "notes"})); out["slug"] != "notes" {
		t.Fatalf("an ordinary slug must still work, got %v", out)
	}
}

func TestSlugWithUnderscoreResolvesByRefAndURL(t *testing.T) {
	h := newHarness(t)
	repo := gitRepo(t, t.TempDir(), "my_repo")
	elsewhere := mkdir(t, t.TempDir(), "elsewhere")

	if out := h.mustOK(h.call(repo, "create_project", nil)); out["slug"] != "my_repo" {
		t.Fatalf("got %v, want slug my_repo", out)
	}
	created := h.mustOK(h.call(repo, "create_issue", map[string]any{"title": "x"}))

	// From outside the Project, only the ref's own slug says which one.
	for _, ref := range []string{"my_repo#1", created["url"].(string)} {
		view := h.mustOK(h.call(elsewhere, "view_issue", map[string]any{"ref": ref}))
		if view["project"] != "my_repo" || view["ref"] != "#1" {
			t.Errorf("ref %q: got %v", ref, view)
		}
	}
}

func TestCreateProjectRejectsSlugCollision(t *testing.T) {
	h := newHarness(t)
	h.mustOK(h.call(gitRepo(t, t.TempDir(), "app"), "create_project", nil))
	other := gitRepo(t, t.TempDir(), "app")

	if r := h.call(other, "create_project", nil); r.code() != "slug_taken" {
		t.Fatalf("want slug_taken, got %v", r)
	}
	out := h.mustOK(h.call(other, "create_project", map[string]any{"slug": "app-2"}))
	if out["root"] != other {
		t.Fatalf("got %v", out)
	}
}

func TestCreateProjectRejectsDuplicateRootButAllowsNesting(t *testing.T) {
	h := newHarness(t)
	outer := mkdir(t, t.TempDir(), "outer")
	h.mustOK(h.call(outer, "create_project", map[string]any{"slug": "outer"}))

	if r := h.call(outer, "create_project", map[string]any{"slug": "again"}); r.code() != "root_taken" {
		t.Fatalf("want root_taken, got %v", r)
	}
	inner := mkdir(t, outer, "inner")
	out := h.mustOK(h.call(inner, "create_project", map[string]any{"slug": "inner"}))
	if out["root"] != inner {
		t.Fatalf("got %v", out)
	}
}

func TestCreateProjectAtAnExistingRootNamesItsSlugBeforeAnySlugComplaint(t *testing.T) {
	h := newHarness(t)
	outside := mkdir(t, t.TempDir(), "plain")
	h.mustOK(h.call(outside, "create_project", map[string]any{"slug": "plain"}))

	// No slug outside git would be slug_required, but the Root is taken.
	r := h.call(outside, "create_project", nil)
	if r.code() != "root_taken" || r.Err["slug"] != "plain" {
		t.Fatalf("want root_taken naming plain, got %v", r)
	}
}

func TestListSessionsSkipsCLISessions(t *testing.T) {
	h := newHarness(t)
	cli := http.Header{}
	cli.Set("X-Loom-Session", shim.CLISessionPrefix+"0123456789abcdef")
	h.mustOK(h.callWith(cli, "list_projects", nil))
	if got := h.mustOK(h.call("", "list_sessions", nil))["sessions"].([]any); len(got) != 1 || got[0].(map[string]any)["id"] != testSession {
		t.Fatalf("want only the agent Session, got %v", got)
	}
}

func TestCreateProjectFromLinkedWorktreeUsesMainWorktreeRoot(t *testing.T) {
	h := newHarness(t)
	repo := gitRepo(t, t.TempDir(), "main-repo")
	wt := filepath.Join(t.TempDir(), "elsewhere")
	git(t, repo, "worktree", "add", "-q", wt)

	out := h.mustOK(h.call(resolved(t, wt), "create_project", nil))
	if out["slug"] != "main-repo" || out["root"] != repo {
		t.Fatalf("got %v, want main-repo at %s", out, repo)
	}
}

func TestListProjects(t *testing.T) {
	h := newHarness(t)
	if out := h.mustOK(h.call("", "list_projects", nil)); len(out["projects"].([]any)) != 0 {
		t.Fatalf("want no projects, got %v", out)
	}
	a := mkdir(t, t.TempDir(), "a")
	b := mkdir(t, t.TempDir(), "b")
	h.mustOK(h.call(b, "create_project", map[string]any{"slug": "bee"}))
	h.mustOK(h.call(a, "create_project", map[string]any{"slug": "ay"}))

	out := h.mustOK(h.call("", "list_projects", nil))
	want := []any{
		map[string]any{"slug": "ay", "root": a},
		map[string]any{"slug": "bee", "root": b},
	}
	if !reflect.DeepEqual(out["projects"], want) {
		t.Fatalf("got %v, want %v", out["projects"], want)
	}
}

func TestDeleteProject(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "doomed")
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "doomed"}))

	if r := h.call("", "delete_project", map[string]any{"slug": "doomed", "confirm": "nope"}); r.code() != "confirm_mismatch" {
		t.Fatalf("want confirm_mismatch, got %v", r)
	}
	r := h.call("", "delete_project", map[string]any{"slug": "ghost", "confirm": "ghost"})
	if r.code() != "unknown_project" || len(r.Err["projects"].([]any)) != 1 {
		t.Fatalf("want unknown_project listing projects, got %v", r)
	}
	if r := h.call(dir, "delete_project", map[string]any{"slug": "", "confirm": ""}); r.code() != "unknown_project" {
		t.Fatalf("empty slug: want unknown_project, got %v", r)
	}
	out := h.mustOK(h.call("", "delete_project", map[string]any{"slug": "doomed", "confirm": "doomed"}))
	if out["deleted"] != "doomed" {
		t.Fatalf("got %v", out)
	}
	// The Slug and Root are free again.
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "doomed"}))
}

func TestListProjectsNamesTheProjectCoveringTheCallersCwd(t *testing.T) {
	h := newHarness(t)
	outer := gitRepo(t, t.TempDir(), "outer")
	inner := gitRepo(t, mkdir(t, outer, "pkg"), "inner")
	other := mkdir(t, t.TempDir(), "other")
	h.mustOK(h.call(outer, "create_project", map[string]any{"slug": "outer"}))
	h.mustOK(h.call(inner, "create_project", map[string]any{"slug": "inner"}))
	h.mustOK(h.call(other, "create_project", map[string]any{"slug": "other"}))

	current := func(cwd string) any {
		out := h.mustOK(h.call(cwd, "list_projects", nil))
		if c, ok := out["current"].(map[string]any); ok {
			return c["slug"]
		}
		return nil
	}
	if got := current(outer); got != "outer" {
		t.Errorf("at the Root: got %v, want outer", got)
	}
	if got := current(mkdir(t, outer, "cmd")); got != "outer" {
		t.Errorf("below the Root: got %v, want outer", got)
	}
	if got := current(inner); got != "inner" {
		t.Errorf("nested Roots: got %v, want the deepest, inner", got)
	}
	if got := current(mkdir(t, t.TempDir(), "elsewhere")); got != nil {
		t.Errorf("outside every Root: got %v, want none", got)
	}
	if got := current(""); got != nil {
		t.Errorf("no cwd sent: got %v, want none", got)
	}
}
