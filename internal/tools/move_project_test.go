package tools_test

import (
	"os"
	"reflect"
	"testing"
)

func TestNoProjectFlagsProjectsWhoseRootIsGone(t *testing.T) {
	h := newHarness(t)
	gone := mkdir(t, t.TempDir(), "gone")
	here := mkdir(t, t.TempDir(), "here")
	h.mustOK(h.call(gone, "create_project", map[string]any{"slug": "gone"}))
	h.mustOK(h.call(here, "create_project", map[string]any{"slug": "here"}))
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	r := h.call(mkdir(t, t.TempDir(), "elsewhere"), "list_issues", nil)
	if r.code() != "no_project" {
		t.Fatalf("want no_project, got %v", r)
	}
	want := []any{
		map[string]any{"slug": "gone", "root": gone, "root_missing": true},
		map[string]any{"slug": "here", "root": here},
	}
	if !reflect.DeepEqual(r.Err["projects"], want) {
		t.Fatalf("got %v, want %v", r.Err["projects"], want)
	}
}

func TestMoveProjectRebindsRootKeepingSlugAndIssues(t *testing.T) {
	h := newHarness(t)
	old := gitRepo(t, t.TempDir(), "old-name")
	h.mustOK(h.call(old, "create_project", map[string]any{"slug": "app"}))
	h.mustOK(h.call(old, "create_issue", map[string]any{"title": "keep me", "body": "b", "kind": "issue"}))
	if err := os.RemoveAll(old); err != nil {
		t.Fatal(err)
	}
	moved := gitRepo(t, t.TempDir(), "new-name")
	sub := mkdir(t, moved, "pkg")

	out := h.mustOK(h.call(sub, "move_project", map[string]any{"project": "app"}))
	if out["slug"] != "app" || out["root"] != moved {
		t.Fatalf("got %v, want app at %s", out, moved)
	}
	view := h.mustOK(h.call(sub, "view_issue", map[string]any{"ref": "#1"}))
	if view["title"] != "keep me" {
		t.Fatalf("Issue lost after move: %v", view)
	}
}

func TestMoveProjectOutsideGitUsesCwd(t *testing.T) {
	h := newHarness(t)
	a := mkdir(t, t.TempDir(), "a")
	b := mkdir(t, t.TempDir(), "b")
	h.mustOK(h.call(a, "create_project", map[string]any{"slug": "notes"}))

	out := h.mustOK(h.call(b, "move_project", map[string]any{"project": "notes"}))
	if out["root"] != b {
		t.Fatalf("got %v, want root %s", out, b)
	}
}

func TestMoveProjectRejectsTakenRootButAllowsNesting(t *testing.T) {
	h := newHarness(t)
	a := mkdir(t, t.TempDir(), "a")
	b := mkdir(t, t.TempDir(), "b")
	h.mustOK(h.call(a, "create_project", map[string]any{"slug": "ay"}))
	h.mustOK(h.call(b, "create_project", map[string]any{"slug": "bee"}))

	r := h.call(b, "move_project", map[string]any{"project": "ay"})
	if r.code() != "root_taken" || len(r.Err["projects"].([]any)) != 2 {
		t.Fatalf("want root_taken listing projects, got %v", r)
	}
	inner := mkdir(t, b, "inner")
	if out := h.mustOK(h.call(inner, "move_project", map[string]any{"project": "ay"})); out["root"] != inner {
		t.Fatalf("got %v", out)
	}
	// Moving a Project to where it already is changes nothing.
	if out := h.mustOK(h.call(inner, "move_project", map[string]any{"project": "ay"})); out["root"] != inner {
		t.Fatalf("got %v", out)
	}
}

func TestMoveProjectErrors(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "d")
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "d"}))

	if r := h.call("", "move_project", map[string]any{"project": "d"}); r.code() != "no_cwd" {
		t.Fatalf("want no_cwd, got %v", r)
	}
	if r := h.call(dir, "move_project", map[string]any{"project": "ghost"}); r.code() != "unknown_project" {
		t.Fatalf("want unknown_project, got %v", r)
	}
	// Without `project`, the Project containing the caller is the one moved.
	if out := h.mustOK(h.call(dir, "move_project", nil)); out["slug"] != "d" || out["root"] != dir {
		t.Fatalf("no project arg: got %v", out)
	}
}
