package tools_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestClaimingAnIssueWithOpenChildrenIsRefused(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	parent := h.issue(dir, map[string]any{"title": "parent"})
	a := h.issue(dir, map[string]any{"title": "a", "parent": parent})
	b := h.issue(dir, map[string]any{"title": "b", "parent": parent})

	r := h.call(dir, "claim", map[string]any{"ref": parent})
	if r.code() != "issue_has_open_children" || !reflect.DeepEqual(strs(r.Err["open_children"]), []string{a, b}) {
		t.Fatalf("claim = %v", r)
	}
	if msg, _ := r.Err["message"].(string); !strings.Contains(msg, "has 2 open children") || strings.Contains(msg, "blocked by") || strings.Contains(msg, "remove_blockers") {
		t.Fatalf("message = %q", msg)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": parent}))
	if view["assignee"] != nil {
		t.Fatalf("a refused claim must change nothing, view = %v", view)
	}

	// A won't-fix child counts as closed; the block lifts with the last child.
	h.mustOK(h.call(dir, "close", map[string]any{"ref": a, "wontfix": true}))
	if r := h.call(dir, "claim", map[string]any{"ref": parent}); r.code() != "issue_has_open_children" || !reflect.DeepEqual(strs(r.Err["open_children"]), []string{b}) {
		t.Fatalf("claim with one open child = %v", r)
	}
	h.mustOK(h.call(dir, "close", map[string]any{"ref": b}))
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": parent}))
}

func TestOpenChildrenBlockTransitively(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	grand := h.issue(dir, map[string]any{"title": "grandparent"})
	parent := h.issue(dir, map[string]any{"title": "parent", "parent": grand})
	leaf := h.issue(dir, map[string]any{"title": "leaf", "parent": parent})

	for _, ref := range []string{grand, parent} {
		if r := h.call(dir, "claim", map[string]any{"ref": ref}); r.code() != "issue_has_open_children" {
			t.Fatalf("claim %s = %v", ref, r)
		}
	}
	h.mustOK(h.call(dir, "close", map[string]any{"ref": leaf}))
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": parent}))
	if r := h.call(dir, "claim", map[string]any{"ref": grand}); r.code() != "issue_has_open_children" {
		t.Fatalf("the open parent still blocks the grandparent: %v", r)
	}
	h.mustOK(h.call(dir, "close", map[string]any{"ref": parent}))
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": grand}))
}

func TestMapsAreNotBlockedByOpenChildren(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	m := h.issue(dir, map[string]any{"title": "map", "kind": "map"})
	h.issue(dir, map[string]any{"title": "ticket", "parent": m, "labels": []string{"wayfinder:task"}})

	h.mustOK(h.call(dir, "claim", map[string]any{"ref": m}))
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": m}))
	if got := strs(view["open_children"]); len(got) != 0 {
		t.Fatalf("a Map reports no open children, got %v", got)
	}
}

func TestAClaimHeldSurvivesAChildBeingAttached(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	parent := h.issue(dir, map[string]any{"title": "parent"})
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": parent}))
	h.issue(dir, map[string]any{"title": "child", "parent": parent})

	if out := h.mustOK(h.call(dir, "claim", map[string]any{"ref": parent})); out["already"] != true {
		t.Fatalf("re-claim = %v", out)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": parent}))
	if view["holder_session"] != testSession {
		t.Fatalf("view = %v", view)
	}
}

func TestFrontierSkipsParentsWithOpenChildrenAsBlocked(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	spec := h.issue(dir, map[string]any{"title": "Spec"})
	parent := h.issue(dir, map[string]any{"title": "parent", "parent": spec})
	leaf := h.issue(dir, map[string]any{"title": "leaf", "parent": parent})
	free := h.issue(dir, map[string]any{"title": "free", "parent": spec})

	out := h.mustOK(h.call(dir, "frontier", map[string]any{"parent": spec}))
	if got := refsOf(out["frontier"]); !reflect.DeepEqual(got, []string{free}) {
		t.Fatalf("frontier = %v", got)
	}
	if got := strs(out["skipped"].(map[string]any)["blocked"]); !reflect.DeepEqual(got, []string{parent}) {
		t.Fatalf("skipped.blocked = %v", got)
	}

	h.mustOK(h.call(dir, "close", map[string]any{"ref": leaf, "wontfix": true}))
	out = h.mustOK(h.call(dir, "frontier", map[string]any{"parent": spec}))
	if got := refsOf(out["frontier"]); !reflect.DeepEqual(got, []string{parent, free}) {
		t.Fatalf("frontier after the child closes = %v", got)
	}
}

func TestViewIssueReportsOpenChildrenApartFromBlockers(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	parent := h.issue(dir, map[string]any{"title": "parent"})
	a := h.issue(dir, map[string]any{"title": "a", "parent": parent})
	b := h.issue(dir, map[string]any{"title": "b", "parent": parent})
	h.mustOK(h.call(dir, "close", map[string]any{"ref": a}))

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": parent}))
	if got := strs(view["open_children"]); !reflect.DeepEqual(got, []string{b}) {
		t.Fatalf("open_children = %v", got)
	}
	if got := view["blocked_by"].([]any); len(got) != 0 {
		t.Fatalf("no Blocker rows are written, blocked_by = %v", got)
	}
	summary := h.mustOK(h.call(dir, "list_issues", map[string]any{"parent": parent}))["issues"].([]any)
	if _, ok := summary[0].(map[string]any)["open_blockers"]; ok {
		t.Fatalf("open_blockers stays explicit edges only, got %v", summary[0])
	}
	h.mustOK(h.call(dir, "close", map[string]any{"ref": b}))
	view = h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": parent}))
	if got := strs(view["open_children"]); len(got) != 0 {
		t.Fatalf("open_children after closing = %v", got)
	}
}

func TestListIssuesBlockedFilterIgnoresOpenChildren(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	parent := h.issue(dir, map[string]any{"title": "parent"})
	child := h.issue(dir, map[string]any{"title": "child", "parent": parent})

	list := func(args map[string]any) []string {
		return refsOf(h.mustOK(h.call(dir, "list_issues", args))["issues"])
	}
	if got := list(map[string]any{"blocked": true}); len(got) != 0 {
		t.Fatalf("blocked = %v", got)
	}
	if got := list(map[string]any{"blocked": false}); !reflect.DeepEqual(got, []string{parent, child}) {
		t.Fatalf("blocked: false = %v", got)
	}
}
