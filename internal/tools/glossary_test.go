package tools_test

import (
	"os"
	"reflect"
	"testing"
)

func TestViewGlossaryIsEmptyWithAVersionUntilOneIsWritten(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "app")
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "app"}))

	out := h.mustOK(h.call(dir, "view_glossary", nil))
	if out["body"] != "" {
		t.Fatalf("want an empty body, got %v", out)
	}
	if _, ok := out["version"].(float64); !ok {
		t.Fatalf("want a version, got %v", out)
	}
}

func TestEditGlossaryReplacesTheWholeDocumentAndReturnsTheNewVersion(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "app")
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "app"}))

	first := h.mustOK(h.call(dir, "view_glossary", nil))
	out := h.mustOK(h.call(dir, "edit_glossary", map[string]any{"version": first["version"], "body": "# App\n\n**Widget**: a thing."}))
	if out["version"] == first["version"] {
		t.Fatalf("version did not move: %v", out)
	}

	view := h.mustOK(h.call(dir, "view_glossary", nil))
	if view["body"] != "# App\n\n**Widget**: a thing." || view["version"] != out["version"] {
		t.Fatalf("got %v, want the written body at %v", view, out["version"])
	}

	again := h.mustOK(h.call(dir, "edit_glossary", map[string]any{"version": out["version"], "body": "# App\n\nreplaced"}))
	view = h.mustOK(h.call(dir, "view_glossary", nil))
	if view["body"] != "# App\n\nreplaced" || view["version"] != again["version"] || again["version"] == out["version"] {
		t.Fatalf("second edit: got %v", view)
	}
}

func TestEditGlossaryRefusesAStaleVersionAndChangesNothing(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "app")
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "app"}))
	first := h.mustOK(h.call(dir, "view_glossary", nil))
	written := h.mustOK(h.call(dir, "edit_glossary", map[string]any{"version": first["version"], "body": "mine"}))

	// A session still holding the empty Glossary's version loses.
	r := h.call(dir, "edit_glossary", map[string]any{"version": first["version"], "body": "clobber"})
	if r.code() != "version_conflict" {
		t.Fatalf("want version_conflict, got %v", r)
	}
	want := map[string]any{"version": written["version"], "body": "mine"}
	if !reflect.DeepEqual(r.Err["current"], want) {
		t.Fatalf("current = %v, want %v", r.Err["current"], want)
	}

	view := h.mustOK(h.call(dir, "view_glossary", nil))
	if view["body"] != "mine" || view["version"] != written["version"] {
		t.Fatalf("a refused edit changed the Glossary: %v", view)
	}
}

func TestGlossaryToolsResolveTheProjectByCwdOrSlug(t *testing.T) {
	h := newHarness(t)
	a := mkdir(t, t.TempDir(), "a")
	b := mkdir(t, t.TempDir(), "b")
	h.mustOK(h.call(a, "create_project", map[string]any{"slug": "ay"}))
	h.mustOK(h.call(b, "create_project", map[string]any{"slug": "bee"}))

	h.mustOK(h.call(a, "edit_glossary", map[string]any{"version": 0, "body": "ay's terms"}))
	h.mustOK(h.call(b, "edit_glossary", map[string]any{"project": "bee", "version": 0, "body": "bee's terms"}))

	if got := h.mustOK(h.call(a, "view_glossary", nil))["body"]; got != "ay's terms" {
		t.Fatalf("by cwd: got %v", got)
	}
	if got := h.mustOK(h.call(b, "view_glossary", nil))["body"]; got != "bee's terms" {
		t.Fatalf("by cwd: got %v", got)
	}
	elsewhere := mkdir(t, t.TempDir(), "elsewhere")
	if got := h.mustOK(h.call(elsewhere, "view_glossary", map[string]any{"project": "ay"}))["body"]; got != "ay's terms" {
		t.Fatalf("by slug: got %v", got)
	}
	if r := h.call(elsewhere, "view_glossary", nil); r.code() != "no_project" {
		t.Fatalf("want no_project, got %v", r)
	}
	if r := h.call(elsewhere, "edit_glossary", map[string]any{"project": "ghost", "version": 0, "body": "x"}); r.code() != "unknown_project" {
		t.Fatalf("want unknown_project, got %v", r)
	}
}

func TestGlossaryFollowsItsProjectThroughMoveProject(t *testing.T) {
	h := newHarness(t)
	old := gitRepo(t, t.TempDir(), "old-name")
	h.mustOK(h.call(old, "create_project", map[string]any{"slug": "app"}))
	h.mustOK(h.call(old, "edit_glossary", map[string]any{"version": 0, "body": "kept"}))
	if err := os.RemoveAll(old); err != nil {
		t.Fatal(err)
	}
	moved := gitRepo(t, t.TempDir(), "new-name")
	h.mustOK(h.call(moved, "move_project", map[string]any{"project": "app"}))

	view := h.mustOK(h.call(moved, "view_glossary", nil))
	if view["body"] != "kept" || view["version"] != float64(1) {
		t.Fatalf("Glossary lost after move: %v", view)
	}
}

func TestDeleteProjectRemovesItsGlossary(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "app")
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "app"}))
	h.mustOK(h.call(dir, "edit_glossary", map[string]any{"version": 0, "body": "doomed"}))
	h.mustOK(h.call(dir, "delete_project", map[string]any{"slug": "app", "confirm": "app"}))

	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "app"}))
	if view := h.mustOK(h.call(dir, "view_glossary", nil)); view["body"] != "" || view["version"] != float64(0) {
		t.Fatalf("a recreated Project inherited the Glossary: %v", view)
	}
}
