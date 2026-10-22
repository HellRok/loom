package tools_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/HellRok/loom/internal/tools"
)

func (h *harness) resolve(cwd, project string) (string, *tools.ToolError) {
	h.t.Helper()
	slug, err := tools.ResolveProject(h.store, cwd, project)
	te, ok := errors.AsType[*tools.ToolError](err)
	if err != nil && !ok {
		h.t.Fatalf("resolve: %v", err)
	}
	return slug, te
}

func TestResolveProjectFromSubdirectory(t *testing.T) {
	h := newHarness(t)
	repo := gitRepo(t, t.TempDir(), "app")
	h.mustOK(h.call(repo, "create_project", nil))

	if slug, err := h.resolve(mkdir(t, repo, "pkg", "x"), ""); slug != "app" {
		t.Fatalf("got %q, %v", slug, err)
	}
}

func TestResolveProjectFromLinkedWorktree(t *testing.T) {
	h := newHarness(t)
	repo := gitRepo(t, t.TempDir(), "app")
	h.mustOK(h.call(repo, "create_project", nil))
	wt := filepath.Join(t.TempDir(), "feature")
	git(t, repo, "worktree", "add", "-q", wt)

	if slug, err := h.resolve(wt, ""); slug != "app" {
		t.Fatalf("got %q, %v", slug, err)
	}
}

func TestResolveProjectDeepestRootWins(t *testing.T) {
	h := newHarness(t)
	outer := mkdir(t, t.TempDir(), "outer")
	inner := mkdir(t, outer, "vendor", "inner")
	h.mustOK(h.call(outer, "create_project", map[string]any{"slug": "outer"}))
	h.mustOK(h.call(inner, "create_project", map[string]any{"slug": "inner"}))

	cases := map[string]string{
		outer:                           "outer",
		mkdir(t, outer, "vendor"):       "outer",
		inner:                           "inner",
		mkdir(t, inner, "src"):          "inner",
		mkdir(t, outer, "vendor", "in"): "outer", // a name prefix is not a path prefix
	}
	for cwd, want := range cases {
		if slug, err := h.resolve(cwd, ""); slug != want {
			t.Errorf("%s: got %q, %v; want %q", cwd, slug, err, want)
		}
	}
}

func TestResolveProjectExplicitSlugOverridesCwd(t *testing.T) {
	h := newHarness(t)
	a := mkdir(t, t.TempDir(), "a")
	b := mkdir(t, t.TempDir(), "b")
	h.mustOK(h.call(a, "create_project", map[string]any{"slug": "ay"}))
	h.mustOK(h.call(b, "create_project", map[string]any{"slug": "bee"}))

	if slug, err := h.resolve(a, "bee"); slug != "bee" {
		t.Fatalf("got %q, %v", slug, err)
	}
	if slug, err := h.resolve("", "bee"); slug != "bee" {
		t.Fatalf("without cwd: got %q, %v", slug, err)
	}
	if _, err := h.resolve(a, "nope"); err == nil || err.Code != "unknown_project" {
		t.Fatalf("want unknown_project, got %v", err)
	}
}

func TestResolveProjectNoCwdNoSlug(t *testing.T) {
	h := newHarness(t)
	if _, err := h.resolve("", ""); err == nil || err.Code != "no_project" {
		t.Fatalf("want no_project, got %v", err)
	}
}

func TestResolveProjectNoMatchListsProjects(t *testing.T) {
	h := newHarness(t)
	a := mkdir(t, t.TempDir(), "a")
	h.mustOK(h.call(a, "create_project", map[string]any{"slug": "ay"}))

	_, err := h.resolve(mkdir(t, t.TempDir(), "elsewhere"), "")
	if err == nil || err.Code != "no_project" {
		t.Fatalf("want no_project, got %v", err)
	}
	var payload struct {
		Error struct {
			Projects []map[string]string `json:"projects"`
		} `json:"error"`
	}
	json.Unmarshal([]byte(err.Error()), &payload)
	if got := payload.Error.Projects; len(got) != 1 || got[0]["slug"] != "ay" || got[0]["root"] != a {
		t.Fatalf("want projects [{ay %s}], got %v", a, got)
	}
}
