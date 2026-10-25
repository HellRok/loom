package tools_test

import (
	"reflect"
	"testing"
)

// issue creates an Issue and returns its ref.
func (h *harness) issue(dir string, args map[string]any) string {
	h.t.Helper()
	if args["title"] == nil {
		args["title"] = "an issue"
	}
	return h.mustOK(h.call(dir, "create_issue", args))["ref"].(string)
}

func eventTypes(t *testing.T, h *harness, dir, ref string) []string {
	t.Helper()
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ref, "events": true}))
	var types []string
	for _, e := range view["events"].([]any) {
		types = append(types, e.(map[string]any)["type"].(string))
	}
	return types
}

func refsOf(v any) []string {
	rows, _ := v.([]any)
	out := []string{}
	for _, r := range rows {
		switch r := r.(type) {
		case string:
			out = append(out, r)
		case map[string]any:
			out = append(out, r["ref"].(string))
		}
	}
	return out
}

func TestParentOnCreateAndSetParentWithEvents(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	spec := h.issue(dir, map[string]any{"title": "Spec"})
	b := h.issue(dir, map[string]any{"title": "B", "parent": spec})
	a := h.issue(dir, map[string]any{"title": "A"})
	h.mustOK(h.call(dir, "set_parent", map[string]any{"ref": a, "parent": spec}))

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": spec}))
	if got := refsOf(view["children"]); !reflect.DeepEqual(got, []string{b, a}) {
		t.Fatalf("children = %v, want %v ordered by #N", got, []string{b, a})
	}
	child := view["children"].([]any)[0].(map[string]any)
	if _, has := child["body"]; has || child["title"] != "B" {
		t.Fatalf("children are summaries without bodies, got %v", child)
	}
	av := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": a}))
	if p, _ := av["parent"].(map[string]any); p["ref"] != spec || p["title"] != "Spec" {
		t.Fatalf("parent = %v", av["parent"])
	}

	// Re-setting the same parent changes nothing; clearing records an Event.
	h.mustOK(h.call(dir, "set_parent", map[string]any{"ref": a, "parent": spec}))
	out := h.mustOK(h.call(dir, "set_parent", map[string]any{"ref": a, "parent": nil}))
	if out["parent"] != nil {
		t.Fatalf("cleared parent = %v", out["parent"])
	}
	h.mustOK(h.call(dir, "set_parent", map[string]any{"ref": a, "parent": nil}))
	if got, want := eventTypes(t, h, dir, a), []string{"created", "parent_set", "parent_cleared"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestParentCyclesAndCrossProjectParentsAreRejected(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	other := h.project("other")
	a := h.issue(dir, map[string]any{"title": "A"})
	b := h.issue(dir, map[string]any{"title": "B", "parent": a})
	c := h.issue(dir, map[string]any{"title": "C", "parent": b})
	foreign := h.issue(other, map[string]any{"title": "Foreign"})

	for _, tc := range []struct{ ref, parent string }{{a, a}, {a, c}, {a, b}} {
		if r := h.call(dir, "set_parent", map[string]any{"ref": tc.ref, "parent": tc.parent}); r.code() != "cycle" {
			t.Fatalf("set_parent %v: want cycle, got %v", tc, r)
		}
	}
	if r := h.call(dir, "set_parent", map[string]any{"ref": c, "parent": "other" + foreign}); r.code() != "invalid_argument" {
		t.Fatalf("cross-Project parent: got %v", r)
	}
	if r := h.call(dir, "set_parent", map[string]any{"ref": c, "parent": "#99"}); r.code() != "not_found" {
		t.Fatalf("missing parent: got %v", r)
	}
	if r := h.call(dir, "create_issue", map[string]any{"title": "x", "parent": "#99"}); r.code() != "not_found" {
		t.Fatalf("create with a missing parent: got %v", r)
	}
	if v := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": a})); v["parent"] != nil {
		t.Fatalf("a rejected set_parent changed #1: %v", v["parent"])
	}
}

func TestDeletingAParentLeavesChildrenParentless(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	p := h.issue(dir, map[string]any{"title": "P"})
	c := h.issue(dir, map[string]any{"title": "C", "parent": p})
	h.mustOK(h.call(dir, "delete_issue", map[string]any{"ref": p, "confirm": p}))

	if v := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": c})); v["parent"] != nil {
		t.Fatalf("parent = %v, want none", v["parent"])
	}
	if got, want := eventTypes(t, h, dir, c), []string{"created", "parent_set", "parent_cleared"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestBlockersAreAddedRemovedAndRecorded(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	a := h.issue(dir, map[string]any{"title": "A"})
	b := h.issue(dir, map[string]any{"title": "B"})
	c := h.issue(dir, map[string]any{"title": "C", "blocked_by": []string{a}})

	out := h.mustOK(h.call(dir, "add_blockers", map[string]any{"ref": c, "blocked_by": []string{b, a, b}}))
	if got := strs(out["blocked_by"]); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("blocked_by = %v (duplicates ignored)", got)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": c}))
	bb := view["blocked_by"].([]any)[0].(map[string]any)
	if bb["ref"] != a || bb["state"] != "open" || bb["title"] != "A" {
		t.Fatalf("view blocked_by = %v", view["blocked_by"])
	}

	out = h.mustOK(h.call(dir, "remove_blockers", map[string]any{"ref": c, "blocked_by": []string{a, "#77"}}))
	if got := strs(out["blocked_by"]); !reflect.DeepEqual(got, []string{b}) {
		t.Fatalf("blocked_by after remove = %v", got)
	}
	want := []string{"created", "blocker_added", "blocker_added", "blocker_removed"}
	if got := eventTypes(t, h, dir, c); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if r := h.call(dir, "add_blockers", map[string]any{"ref": c, "blocked_by": []string{}}); r.code() != "invalid_argument" {
		t.Fatalf("no blockers: got %v", r)
	}
}

func TestSelfBlocksCyclesAndMissingBlockersChangeNothing(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	other := h.project("other")
	a := h.issue(dir, map[string]any{"title": "A"})
	b := h.issue(dir, map[string]any{"title": "B", "blocked_by": []string{a}})
	c := h.issue(dir, map[string]any{"title": "C", "blocked_by": []string{b}})
	foreign := h.issue(other, map[string]any{"title": "F"})

	if r := h.call(dir, "add_blockers", map[string]any{"ref": a, "blocked_by": []string{a}}); r.code() != "self_block" {
		t.Fatalf("self block: got %v", r)
	}
	if r := h.call(dir, "add_blockers", map[string]any{"ref": a, "blocked_by": []string{c}}); r.code() != "cycle" {
		t.Fatalf("cycle: got %v", r)
	}
	// One bad blocker undoes the good one before it.
	if r := h.call(dir, "add_blockers", map[string]any{"ref": c, "blocked_by": []string{a, "#99"}}); r.code() != "not_found" {
		t.Fatalf("missing blocker: got %v", r)
	}
	if r := h.call(dir, "add_blockers", map[string]any{"ref": c, "blocked_by": []string{"other" + foreign}}); r.code() != "invalid_argument" {
		t.Fatalf("cross-Project blocker: got %v", r)
	}
	if r := h.call(dir, "create_issue", map[string]any{"title": "x", "blocked_by": []string{c, "#99"}}); r.code() != "not_found" {
		t.Fatalf("create with a missing blocker: got %v", r)
	}
	if got := strs(h.mustOK(h.call(dir, "remove_blockers", map[string]any{"ref": c, "blocked_by": []string{"#99"}}))["blocked_by"]); !reflect.DeepEqual(got, []string{b}) {
		t.Fatalf("#3 blockers = %v, want only %s", got, b)
	}
	// The failed create took no number.
	if next := h.issue(dir, map[string]any{"title": "next"}); next != "#4" {
		t.Fatalf("next ref = %s, want #4", next)
	}
}

func TestDeletingAnIssueRemovesItsBlockingEdges(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	a := h.issue(dir, map[string]any{"title": "A"})
	b := h.issue(dir, map[string]any{"title": "B", "blocked_by": []string{a}})
	h.mustOK(h.call(dir, "delete_issue", map[string]any{"ref": a, "confirm": a}))

	if v := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": b})); len(v["blocked_by"].([]any)) != 0 {
		t.Fatalf("blocked_by = %v, want none", v["blocked_by"])
	}
	if got := refsOf(h.mustOK(h.call(dir, "list_issues", map[string]any{"blocked": true}))["issues"]); len(got) != 0 {
		t.Fatalf("blocked = %v, want none", got)
	}
}

func TestFrontierIsOpenUnblockedUnassignedChildrenInCreationOrder(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	spec := h.issue(dir, map[string]any{"title": "Spec"})
	first := h.issue(dir, map[string]any{"title": "first", "parent": spec})
	blocked := h.issue(dir, map[string]any{"title": "blocked", "parent": spec, "blocked_by": []string{first}})
	third := h.issue(dir, map[string]any{"title": "third", "parent": spec})
	done := h.issue(dir, map[string]any{"title": "done", "parent": spec})
	h.issue(dir, map[string]any{"title": "not a child"})
	h.mustOK(h.call(dir, "close", map[string]any{"ref": done}))
	claimed := h.issue(dir, map[string]any{"title": "claimed", "parent": spec})
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": claimed}))

	out := h.mustOK(h.call(dir, "frontier", map[string]any{"parent": spec}))
	if out["parent"] != spec {
		t.Fatalf("parent = %v", out["parent"])
	}
	if got, want := refsOf(out["frontier"]), []string{first, third}; !reflect.DeepEqual(got, want) {
		t.Fatalf("frontier = %v, want %v", got, want)
	}
	skipped := out["skipped"].(map[string]any)
	if got := strs(skipped["claimed"]); !reflect.DeepEqual(got, []string{claimed}) {
		t.Fatalf("skipped.claimed = %v", got)
	}
	if got := strs(skipped["blocked"]); !reflect.DeepEqual(got, []string{blocked}) {
		t.Fatalf("skipped.blocked = %v", got)
	}

	// Closing the Blocker unblocks the child, which joins the frontier in
	// creation order.
	h.mustOK(h.call(dir, "close", map[string]any{"ref": first}))
	out = h.mustOK(h.call(dir, "frontier", map[string]any{"parent": spec}))
	if got, want := refsOf(out["frontier"]), []string{blocked, third}; !reflect.DeepEqual(got, want) {
		t.Fatalf("frontier after close = %v, want %v", got, want)
	}
	if got := strs(out["skipped"].(map[string]any)["blocked"]); len(got) != 0 {
		t.Fatalf("skipped.blocked = %v, want none", got)
	}

	if r := h.call(dir, "frontier", map[string]any{"parent": "#99"}); r.code() != "not_found" {
		t.Fatalf("unknown parent: got %v", r)
	}
	if empty := h.mustOK(h.call(dir, "frontier", map[string]any{"parent": third})); len(empty["frontier"].([]any)) != 0 {
		t.Fatalf("a childless Issue has an empty frontier, got %v", empty)
	}
}

func TestListIssuesFiltersByParentAndBlocked(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	spec := h.issue(dir, map[string]any{"title": "Spec"})
	a := h.issue(dir, map[string]any{"title": "A", "parent": spec})
	b := h.issue(dir, map[string]any{"title": "B", "parent": spec, "blocked_by": []string{a}})
	loose := h.issue(dir, map[string]any{"title": "loose"})

	list := func(args map[string]any) []string {
		return refsOf(h.mustOK(h.call(dir, "list_issues", args))["issues"])
	}
	if got := list(map[string]any{"parent": spec}); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("parent filter = %v", got)
	}
	if got := list(map[string]any{"blocked": true}); !reflect.DeepEqual(got, []string{b}) {
		t.Fatalf("blocked = %v", got)
	}
	if got := list(map[string]any{"blocked": false}); !reflect.DeepEqual(got, []string{spec, a, loose}) {
		t.Fatalf("unblocked = %v", got)
	}
	if got := list(map[string]any{"parent": spec, "blocked": false}); !reflect.DeepEqual(got, []string{a}) {
		t.Fatalf("parent and unblocked = %v", got)
	}
	rows := h.mustOK(h.call(dir, "list_issues", map[string]any{"blocked": true}))["issues"].([]any)
	if got := strs(rows[0].(map[string]any)["open_blockers"]); !reflect.DeepEqual(got, []string{a}) {
		t.Fatalf("open_blockers = %v", got)
	}
	if r := h.call(dir, "list_issues", map[string]any{"parent": "#99"}); r.code() != "not_found" {
		t.Fatalf("unknown parent: got %v", r)
	}

	h.mustOK(h.call(dir, "close", map[string]any{"ref": a}))
	if got := list(map[string]any{"blocked": true}); len(got) != 0 {
		t.Fatalf("blocked after the Blocker closed = %v", got)
	}
}

func validTypes(t *testing.T, r result) {
	t.Helper()
	if r.code() != "invalid_ticket" {
		t.Fatalf("want invalid_ticket, got %v", r)
	}
	if got := strs(r.Err["valid_types"]); !reflect.DeepEqual(got, []string{"research", "prototype", "grilling", "task"}) {
		t.Fatalf("valid_types = %v", got)
	}
}

func TestMapRulesForTickets(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	m := h.issue(dir, map[string]any{"title": "Map", "kind": "map"})

	// A Ticket needs exactly one wayfinder type; any other label is fine.
	validTypes(t, h.call(dir, "create_issue", map[string]any{"title": "t", "parent": m}))
	validTypes(t, h.call(dir, "create_issue", map[string]any{"title": "t", "parent": m, "labels": []string{"bug"}}))
	validTypes(t, h.call(dir, "create_issue", map[string]any{"title": "t", "parent": m, "labels": []string{"wayfinder:task", "wayfinder:research"}}))
	// None of those took a number.
	ticket := h.issue(dir, map[string]any{"title": "Ticket", "parent": m, "labels": []string{"wayfinder:grilling", "needs-triage"}})
	if ticket != "#2" {
		t.Fatalf("ticket = %s, want #2", ticket)
	}

	// Maps have no parent; Tickets may have children, which are plain Issues.
	plain := h.issue(dir, map[string]any{"title": "plain"})
	validTypes(t, h.call(dir, "set_parent", map[string]any{"ref": m, "parent": plain}))
	validTypes(t, h.call(dir, "create_issue", map[string]any{"title": "m2", "kind": "map", "parent": plain}))
	h.issue(dir, map[string]any{"title": "grandchild", "parent": ticket})
	h.mustOK(h.call(dir, "set_parent", map[string]any{"ref": plain, "parent": ticket}))

	// An Issue with children can become a Ticket, but needs its label first.
	h.issue(dir, map[string]any{"title": "kid", "parent": plain})
	validTypes(t, h.call(dir, "set_parent", map[string]any{"ref": plain, "parent": m}))
	h.mustOK(h.call(dir, "label", map[string]any{"ref": plain, "add": []string{"wayfinder:task"}}))
	h.mustOK(h.call(dir, "set_parent", map[string]any{"ref": plain, "parent": m}))
	loose := h.issue(dir, map[string]any{"title": "loose"})
	validTypes(t, h.call(dir, "set_parent", map[string]any{"ref": loose, "parent": m}))
	h.mustOK(h.call(dir, "label", map[string]any{"ref": loose, "add": []string{"wayfinder:research"}}))
	h.mustOK(h.call(dir, "set_parent", map[string]any{"ref": loose, "parent": m}))

	// Labelling a Ticket keeps it well formed, as one step.
	h.mustOK(h.call(dir, "label", map[string]any{"ref": ticket, "add": []string{"ready-for-agent", "bug"}, "remove": []string{"needs-triage"}}))
	validTypes(t, h.call(dir, "label", map[string]any{"ref": ticket, "remove": []string{"wayfinder:grilling"}}))
	validTypes(t, h.call(dir, "label", map[string]any{"ref": ticket, "add": []string{"wayfinder:task"}}))
	h.mustOK(h.call(dir, "label", map[string]any{"ref": ticket, "add": []string{"wayfinder:task"}, "remove": []string{"wayfinder:grilling"}}))
	if v := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ticket})); !reflect.DeepEqual(strs(v["labels"]), []string{"bug", "ready-for-agent", "wayfinder:task"}) {
		t.Fatalf("labels = %v", v["labels"])
	}
}

func TestSpecChildrenArePlainIssues(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	spec := h.issue(dir, map[string]any{"title": "Spec"})
	slice := h.issue(dir, map[string]any{"title": "Slice", "parent": spec, "labels": []string{"ready-for-agent"}})
	h.issue(dir, map[string]any{"title": "Sub-slice", "parent": slice})
	h.mustOK(h.call(dir, "close", map[string]any{"ref": slice, "wontfix": true}))
}

func TestTicketCanBeClosedAsWontfix(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	m := h.issue(dir, map[string]any{"title": "Map", "kind": "map"})
	ticket := h.issue(dir, map[string]any{"title": "T", "parent": m, "labels": []string{"wayfinder:task"}})

	h.mustOK(h.call(dir, "close", map[string]any{"ref": ticket, "comment": "no", "wontfix": true}))
	v := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ticket}))
	if v["state"] != "closed" || !reflect.DeepEqual(strs(v["labels"]), []string{"wayfinder:task", "wontfix"}) {
		t.Fatalf("a won't-fix close left the Ticket as %v", v)
	}
}

func TestCreateIssueIsAtomicAcrossParentLabelsAndBlockers(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	m := h.issue(dir, map[string]any{"title": "Map", "kind": "map"})
	blocker := h.issue(dir, map[string]any{"title": "Blocker"})

	// The Ticket rule fails after the parent and blockers were applied.
	r := h.call(dir, "create_issue", map[string]any{"title": "T", "parent": m, "blocked_by": []string{blocker}})
	validTypes(t, r)
	// An undefined label fails after them too.
	if r := h.call(dir, "create_issue", map[string]any{"title": "T", "parent": m, "blocked_by": []string{blocker}, "labels": []string{"nope"}}); r.code() != "undefined_label" {
		t.Fatalf("got %v", r)
	}
	if got := refsOf(h.mustOK(h.call(dir, "list_issues", map[string]any{"parent": m, "state": "all"}))["issues"]); len(got) != 0 {
		t.Fatalf("children = %v, want none", got)
	}
	if got := refsOf(h.mustOK(h.call(dir, "list_issues", map[string]any{"state": "all"}))["issues"]); !reflect.DeepEqual(got, []string{m, blocker}) {
		t.Fatalf("issues = %v", got)
	}

	ok := h.issue(dir, map[string]any{"title": "T", "parent": m, "blocked_by": []string{blocker}, "labels": []string{"wayfinder:research"}})
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ok}))
	if view["parent"].(map[string]any)["ref"] != m || !reflect.DeepEqual(refsOf(view["blocked_by"]), []string{blocker}) {
		t.Fatalf("view = %v", view)
	}
	if ok != "#3" {
		t.Fatalf("ref = %s, want #3 (rejected creates take no number)", ok)
	}
}

func TestUnblockedMeansEveryBlockerClosedOrDeleted(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	spec := h.issue(dir, map[string]any{"title": "Spec"})
	a := h.issue(dir, map[string]any{"title": "A"})
	b := h.issue(dir, map[string]any{"title": "B"})
	c := h.issue(dir, map[string]any{"title": "C", "parent": spec, "blocked_by": []string{a, b}})

	ready := func() []string {
		return refsOf(h.mustOK(h.call(dir, "frontier", map[string]any{"parent": spec}))["frontier"])
	}
	if got := ready(); len(got) != 0 {
		t.Fatalf("frontier = %v with two open Blockers", got)
	}
	h.mustOK(h.call(dir, "close", map[string]any{"ref": a}))
	if got := ready(); len(got) != 0 {
		t.Fatalf("frontier = %v with one open Blocker", got)
	}
	h.mustOK(h.call(dir, "delete_issue", map[string]any{"ref": b, "confirm": b}))
	if got := ready(); !reflect.DeepEqual(got, []string{c}) {
		t.Fatalf("frontier = %v, want %v", got, []string{c})
	}
	// Reopening a Blocker blocks again.
	h.mustOK(h.call(dir, "reopen", map[string]any{"ref": a}))
	if got := ready(); len(got) != 0 {
		t.Fatalf("frontier = %v after reopening a Blocker", got)
	}
}
