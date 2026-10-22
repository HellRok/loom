package tools_test

import (
	"reflect"
	"slices"
	"testing"
)

var seededLabels = []string{
	"bug", "enhancement", "needs-triage", "needs-info", "ready-for-agent", "ready-for-human",
	"wontfix", "wayfinder:research", "wayfinder:prototype", "wayfinder:grilling", "wayfinder:task",
}

func labelRows(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, _ := out["labels"].([]any)
	rows := make([]map[string]any, len(raw))
	for k, r := range raw {
		rows[k] = r.(map[string]any)
	}
	return rows
}

func strs(v any) []string {
	raw, _ := v.([]any)
	out := make([]string, len(raw))
	for k, s := range raw {
		out[k] = s.(string)
	}
	return out
}

func TestNewProjectIsSeededWithTheElevenLabels(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")

	rows := labelRows(t, h.mustOK(h.call(dir, "list_labels", nil)))
	var names []string
	for _, r := range rows {
		names = append(names, r["name"].(string))
		if r["colour"] == "" || r["count"] != float64(0) {
			t.Errorf("seeded label %v wants a colour and a zero count", r)
		}
	}
	want := slices.Sorted(slices.Values(seededLabels))
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("labels = %v, want %v", names, want)
	}

	// Labels are per Project.
	other := h.project("other")
	h.mustOK(h.call(other, "create_label", map[string]any{"name": "only-here"}))
	if got := len(labelRows(t, h.mustOK(h.call(dir, "list_labels", nil)))); got != 11 {
		t.Fatalf("app has %d labels, want 11", got)
	}
}

func TestCreateLabelIsIdempotentAndPicksAColourFromThePool(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")

	out := h.mustOK(h.call(dir, "create_label", map[string]any{"name": "perf", "description": "Slow things"}))
	if out["name"] != "perf" || out["description"] != "Slow things" || out["already"] != false || out["colour"] == "" {
		t.Fatalf("got %v", out)
	}
	again := h.mustOK(h.call(dir, "create_label", map[string]any{"name": "perf", "description": "ignored"}))
	if again["already"] != true || again["description"] != "Slow things" || again["colour"] != out["colour"] {
		t.Fatalf("second create got %v", again)
	}
	if r := h.call(dir, "create_label", map[string]any{"name": " "}); r.code() != "invalid_argument" {
		t.Fatalf("blank name: want invalid_argument, got %v", r)
	}

	// Colours come from a fixed pool, so many labels reuse them.
	colours := map[any]bool{}
	for _, r := range labelRows(t, h.mustOK(h.call(dir, "list_labels", nil))) {
		colours[r["colour"]] = true
	}
	if len(colours) < 2 || len(colours) > 8 {
		t.Fatalf("%d distinct colours, want 2..8", len(colours))
	}
}

func TestLabelAppliesAndRemovesAtomicallyWithEvents(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "t", "labels": []string{"needs-triage"}}))

	out := h.mustOK(h.call(dir, "label", map[string]any{
		"ref": "#1", "add": []string{"bug", "ready-for-agent"}, "remove": []string{"needs-triage"},
	}))
	if out["ref"] != "#1" || !reflect.DeepEqual(strs(out["labels"]), []string{"bug", "ready-for-agent"}) {
		t.Fatalf("got %v", out)
	}

	// A no-op records no Event.
	h.mustOK(h.call(dir, "label", map[string]any{"ref": "#1", "add": []string{"bug"}, "remove": []string{"wontfix"}}))

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	if !reflect.DeepEqual(strs(view["labels"]), []string{"bug", "ready-for-agent"}) {
		t.Fatalf("view labels %v", view["labels"])
	}
	evs := eventsOf(t, view)
	if got := types(evs); !reflect.DeepEqual(got, []string{"created", "labelled", "labelled", "labelled", "unlabelled"}) {
		t.Fatalf("event types %v", got)
	}
	var seen []string
	for _, e := range evs[1:] {
		if e["actor"] != testAssignee || e["session"] != testSession {
			t.Fatalf("event missing actor or Session: %v", e)
		}
		seen = append(seen, e["data"].(map[string]any)["label"].(string))
	}
	if want := []string{"needs-triage", "bug", "ready-for-agent", "needs-triage"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("event labels %v, want %v", seen, want)
	}
}

func TestUndefinedLabelIsRejectedListingValidOnesAndChangesNothing(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "t"}))

	r := h.call(dir, "label", map[string]any{"ref": "#1", "add": []string{"bug", "bgu"}})
	if r.code() != "undefined_label" {
		t.Fatalf("want undefined_label, got %v", r)
	}
	if got := strs(r.Err["valid"]); !reflect.DeepEqual(got, slices.Sorted(slices.Values(seededLabels))) {
		t.Fatalf("valid = %v", got)
	}
	if r := h.call(dir, "label", map[string]any{"ref": "#1", "remove": []string{"nope"}}); r.code() != "undefined_label" {
		t.Fatalf("remove: want undefined_label, got %v", r)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1"}))
	if len(strs(view["labels"])) != 0 {
		t.Fatalf("a rejected call applied %v", view["labels"])
	}

	// create_issue is atomic too: no Issue is left behind.
	if r := h.call(dir, "create_issue", map[string]any{"title": "u", "labels": []string{"bug", "nope"}}); r.code() != "undefined_label" {
		t.Fatalf("create: want undefined_label, got %v", r)
	}
	if out := h.mustOK(h.call(dir, "list_issues", nil)); out["total"] != float64(1) {
		t.Fatalf("total = %v, want 1", out["total"])
	}
}

func TestLabelArgumentChecks(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "t"}))
	if r := h.call(dir, "label", map[string]any{"ref": "#1"}); r.code() != "invalid_argument" {
		t.Fatalf("nothing to do: want invalid_argument, got %v", r)
	}
	if r := h.call(dir, "label", map[string]any{"ref": "#1", "add": []string{"bug"}, "remove": []string{"bug"}}); r.code() != "invalid_argument" {
		t.Fatalf("add and remove: want invalid_argument, got %v", r)
	}
	if r := h.call(dir, "label", map[string]any{"ref": "#9", "add": []string{"bug"}}); r.code() != "not_found" {
		t.Fatalf("want not_found, got %v", r)
	}
}

func TestListLabelsCountsUsageAcrossOpenAndClosedIssues(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "a", "labels": []string{"bug"}}))
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "b", "labels": []string{"bug", "wontfix"}}))
	h.mustOK(h.call(dir, "close", map[string]any{"ref": "#2"}))

	counts := map[string]any{}
	for _, r := range labelRows(t, h.mustOK(h.call(dir, "list_labels", nil))) {
		counts[r["name"].(string)] = r["count"]
	}
	if counts["bug"] != float64(2) || counts["wontfix"] != float64(1) || counts["enhancement"] != float64(0) {
		t.Fatalf("counts %v", counts)
	}
}

func TestDeleteLabelOnlyWhenUnused(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "a", "labels": []string{"bug"}}))
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "b"}))
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "c", "labels": []string{"bug"}}))

	r := h.call(dir, "delete_label", map[string]any{"name": "bug"})
	if r.code() != "label_in_use" || !reflect.DeepEqual(strs(r.Err["issues"]), []string{"#1", "#3"}) {
		t.Fatalf("want label_in_use listing #1 and #3, got %v", r)
	}

	out := h.mustOK(h.call(dir, "delete_label", map[string]any{"name": "enhancement"}))
	if out["deleted"] != "enhancement" {
		t.Fatalf("got %v", out)
	}
	if r := h.call(dir, "label", map[string]any{"ref": "#1", "add": []string{"enhancement"}}); r.code() != "undefined_label" {
		t.Fatalf("deleted label still applies: %v", r)
	}
	if r := h.call(dir, "delete_label", map[string]any{"name": "enhancement"}); r.code() != "undefined_label" {
		t.Fatalf("deleting twice: want undefined_label, got %v", r)
	}

	// Deleting the Issues frees the label.
	h.mustOK(h.call(dir, "delete_issue", map[string]any{"ref": "#1", "confirm": "#1"}))
	h.mustOK(h.call(dir, "delete_issue", map[string]any{"ref": "#3", "confirm": "#3"}))
	h.mustOK(h.call(dir, "delete_label", map[string]any{"name": "bug"}))
}

func TestListIssuesFiltersByLabels(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "a", "labels": []string{"bug", "needs-triage"}}))
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "b", "labels": []string{"bug"}}))
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "c"}))

	refs := func(args map[string]any) []string {
		out := h.mustOK(h.call(dir, "list_issues", args))
		var rs []string
		for _, i := range out["issues"].([]any) {
			rs = append(rs, i.(map[string]any)["ref"].(string))
		}
		return rs
	}
	if got := refs(map[string]any{"labels": []string{"bug"}}); !reflect.DeepEqual(got, []string{"#1", "#2"}) {
		t.Errorf("bug: %v", got)
	}
	if got := refs(map[string]any{"labels": []string{"bug", "needs-triage"}}); !reflect.DeepEqual(got, []string{"#1"}) {
		t.Errorf("all-of: %v", got)
	}
	if got := refs(map[string]any{"no_labels": true}); !reflect.DeepEqual(got, []string{"#3"}) {
		t.Errorf("no_labels: %v", got)
	}
	out := h.mustOK(h.call(dir, "list_issues", map[string]any{"labels": []string{"bug"}, "limit": 1}))
	if out["total"] != float64(2) {
		t.Errorf("total = %v, want 2", out["total"])
	}
	first := out["issues"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(strs(first["labels"]), []string{"bug", "needs-triage"}) {
		t.Errorf("summary labels %v", first["labels"])
	}
	if r := h.call(dir, "list_issues", map[string]any{"labels": []string{"typo"}}); r.code() != "undefined_label" {
		t.Errorf("typo: want undefined_label, got %v", r)
	}
	if r := h.call(dir, "list_issues", map[string]any{"labels": []string{"bug"}, "no_labels": true}); r.code() != "invalid_argument" {
		t.Errorf("both: want invalid_argument, got %v", r)
	}
}

func TestCloseWontfixLabelsCommentsAndClosesAtomically(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "a", "labels": []string{"needs-triage"}}))

	out := h.mustOK(h.call(dir, "close", map[string]any{"ref": "#1", "comment": "Out of scope.", "wontfix": true}))
	if out["state"] != "closed" || out["already"] != false {
		t.Fatalf("got %v", out)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	if !reflect.DeepEqual(strs(view["labels"]), []string{"needs-triage", "wontfix"}) {
		t.Fatalf("labels %v", view["labels"])
	}
	if cs := view["comments"].([]any); len(cs) != 1 || cs[0].(map[string]any)["body"] != "Out of scope." {
		t.Fatalf("comments %v", view["comments"])
	}
	if got := types(eventsOf(t, view)); !reflect.DeepEqual(got, []string{"created", "labelled", "labelled", "commented", "closed"}) {
		t.Fatalf("event types %v", got)
	}

	// Closing again changes nothing.
	if again := h.mustOK(h.call(dir, "close", map[string]any{"ref": "#1", "wontfix": true})); again["already"] != true {
		t.Fatalf("got %v", again)
	}
}

func TestCloseWontfixWithTheLabelDeletedChangesNothing(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "a"}))
	h.mustOK(h.call(dir, "delete_label", map[string]any{"name": "wontfix"}))

	r := h.call(dir, "close", map[string]any{"ref": "#1", "comment": "no", "wontfix": true})
	if r.code() != "undefined_label" {
		t.Fatalf("want undefined_label, got %v", r)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1"}))
	if view["state"] != "open" || len(view["comments"].([]any)) != 0 {
		t.Fatalf("a failed close changed the Issue: %v", view)
	}
}

func TestDeleteProjectWithLabelledIssues(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "a", "labels": []string{"bug"}}))
	h.mustOK(h.call("", "delete_project", map[string]any{"slug": "app", "confirm": "app"}))
}
