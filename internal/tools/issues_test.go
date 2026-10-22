package tools_test

import (
	"fmt"
	"reflect"
	"testing"
)

// project creates a Project named slug in a fresh directory and returns
// that directory, the cwd for calls acting on it.
func (h *harness) project(slug string) string {
	h.t.Helper()
	dir := mkdir(h.t, h.t.TempDir(), slug)
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": slug}))
	return dir
}

func TestCreateIssueThenViewIt(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")

	out := h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "Fix login", "body": "It breaks."}))
	want := map[string]any{
		"ref": "#1", "url": "http://127.0.0.1:7337/app/issues/1",
		"title": "Fix login", "kind": "issue", "version": float64(1),
	}
	for k, v := range want {
		if out[k] != v {
			t.Fatalf("create: %s = %v, want %v (got %v)", k, out[k], v, out)
		}
	}

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1"}))
	for k, v := range map[string]any{
		"ref": "#1", "url": "http://127.0.0.1:7337/app/issues/1", "project": "app",
		"title": "Fix login", "body": "It breaks.", "kind": "issue", "state": "open",
		"author": testAssignee, "version": float64(1),
	} {
		if view[k] != v {
			t.Fatalf("view: %s = %v, want %v (got %v)", k, view[k], v, view)
		}
	}
	if view["assignee"] != nil {
		t.Fatalf("new Issue has assignee %v", view["assignee"])
	}
	if c, ok := view["comments"].([]any); !ok || len(c) != 0 {
		t.Fatalf("want empty comments by default, got %v", view["comments"])
	}
	if _, ok := view["events"]; ok {
		t.Fatalf("events are off by default, got %v", view["events"])
	}
}

func TestRefForms(t *testing.T) {
	h := newHarness(t)
	app := h.project("app")
	other := h.project("other")
	h.mustOK(h.call(app, "create_issue", map[string]any{"title": "in app"}))
	h.mustOK(h.call(other, "create_issue", map[string]any{"title": "in other"}))

	for ref, title := range map[string]string{
		"#1":                                    "in app",
		"1":                                     "in app",
		"app#1":                                 "in app",
		"other#1":                               "in other",
		"http://127.0.0.1:7337/other/issues/1":  "in other",
		"http://localhost:7337/other/issues/1":  "in other",
		"http://127.0.0.1:7337/other/issues/1/": "in other",
	} {
		// slug#N and URLs name their own Project, whatever the cwd.
		out := h.mustOK(h.call(app, "view_issue", map[string]any{"ref": ref}))
		if out["title"] != title {
			t.Errorf("%s: got %v, want %q", ref, out["title"], title)
		}
	}
	for _, ref := range []string{"", "#0", "#x", "App#1", "http://127.0.0.1:9999/app/issues/1", "http://example.com/app/issues/1", "http://127.0.0.1:7337/app/pulls/1"} {
		if r := h.call(app, "view_issue", map[string]any{"ref": ref}); r.code() != "bad_ref" {
			t.Errorf("%q: want bad_ref, got %v", ref, r)
		}
	}
	if r := h.call(app, "view_issue", map[string]any{"ref": "#9"}); r.code() != "not_found" {
		t.Errorf("want not_found, got %v", r)
	}
	if r := h.call(app, "view_issue", map[string]any{"ref": "ghost#1"}); r.code() != "unknown_project" {
		t.Errorf("want unknown_project, got %v", r)
	}
}

func TestIssueNeedsAProject(t *testing.T) {
	h := newHarness(t)
	nowhere := mkdir(t, t.TempDir(), "nowhere")
	if r := h.call(nowhere, "create_issue", map[string]any{"title": "x"}); r.code() != "no_project" {
		t.Fatalf("want no_project, got %v", r)
	}
}

func TestCreateIssueValidatesTitleAndKind(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	if r := h.call(dir, "create_issue", map[string]any{"title": "  "}); r.code() != "invalid_argument" {
		t.Fatalf("blank title: want invalid_argument, got %v", r)
	}
	if r := h.call(dir, "create_issue", map[string]any{"title": "x", "kind": "epic"}); r.code() != "invalid_argument" {
		t.Fatalf("bad kind: want invalid_argument, got %v", r)
	}
	out := h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "Route", "kind": "map"}))
	if out["kind"] != "map" || out["ref"] != "#1" {
		t.Fatalf("got %v", out)
	}
}

func TestEditIssueChecksVersionAndKeepsPreviousText(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "Old title", "body": "old body"}))

	out := h.mustOK(h.call(dir, "edit_issue", map[string]any{"ref": "#1", "version": 1, "title": "New title"}))
	if out["ref"] != "#1" || out["version"] != float64(2) {
		t.Fatalf("got %v, want #1 at version 2", out)
	}

	// Another session's stale edit is refused with the current text.
	r := h.call(dir, "edit_issue", map[string]any{"ref": "#1", "version": 1, "body": "clobber"})
	if r.code() != "version_conflict" {
		t.Fatalf("want version_conflict, got %v", r)
	}
	want := map[string]any{"version": float64(2), "title": "New title", "body": "old body"}
	if !reflect.DeepEqual(r.Err["current"], want) {
		t.Fatalf("current = %v, want %v", r.Err["current"], want)
	}

	h.mustOK(h.call(dir, "edit_issue", map[string]any{"ref": "#1", "version": 2, "body": "new body"}))
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	if view["title"] != "New title" || view["body"] != "new body" || view["version"] != float64(3) {
		t.Fatalf("got %v", view)
	}
	evs := eventsOf(t, view)
	if got := types(evs); !reflect.DeepEqual(got, []string{"created", "title_edited", "body_edited"}) {
		t.Fatalf("event types %v", got)
	}
	if prev := evs[1]["data"].(map[string]any)["previous"]; prev != "Old title" {
		t.Fatalf("title edit kept %v", prev)
	}
	if prev := evs[2]["data"].(map[string]any)["previous"]; prev != "old body" {
		t.Fatalf("body edit kept %v", prev)
	}
	for _, e := range evs {
		if e["actor"] != testAssignee || e["session"] != testSession || e["at"] == "" {
			t.Fatalf("event missing actor, Session or time: %v", e)
		}
	}
}

func TestEditIssueNeedsSomethingToChange(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "t"}))
	if r := h.call(dir, "edit_issue", map[string]any{"ref": "#1", "version": 1}); r.code() != "invalid_argument" {
		t.Fatalf("want invalid_argument, got %v", r)
	}
	if r := h.call(dir, "edit_issue", map[string]any{"ref": "#1", "version": 1, "title": " "}); r.code() != "invalid_argument" {
		t.Fatalf("blank title: want invalid_argument, got %v", r)
	}
}

func eventsOf(t *testing.T, view map[string]any) []map[string]any {
	t.Helper()
	raw, ok := view["events"].([]any)
	if !ok {
		t.Fatalf("no events in %v", view)
	}
	evs := make([]map[string]any, len(raw))
	for k, e := range raw {
		evs[k] = e.(map[string]any)
	}
	return evs
}

func types(evs []map[string]any) []string {
	ts := make([]string, len(evs))
	for k, e := range evs {
		ts[k] = e["type"].(string)
	}
	return ts
}

const mapBody = `# Route

## Decisions so far

- first

### Detail

- nested

## Fog

` + "````" + `
` + "```" + `
## Not a heading
` + "````" + `
`

func TestAppendToSectionNeedsNoVersion(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "Map", "body": mapBody, "kind": "map"}))

	// Two sessions append without reading the version first; both land.
	h.mustOK(h.call(dir, "append_to_section", map[string]any{"ref": "#1", "heading": "Decisions so far", "text": "- second"}))
	out := h.mustOK(h.call(dir, "append_to_section", map[string]any{"ref": "#1", "heading": "## decisions SO far", "text": "- third\n"}))
	if out["ref"] != "#1" || out["version"] != float64(3) {
		t.Fatalf("got %v, want #1 at version 3", out)
	}
	h.mustOK(h.call(dir, "append_to_section", map[string]any{"ref": "#1", "heading": "Fog", "text": "- unknown"}))

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	want := "# Route\n\n## Decisions so far\n\n- first\n\n### Detail\n\n- nested\n- second\n- third\n\n## Fog\n\n````\n```\n## Not a heading\n````\n- unknown\n"
	if view["body"] != want {
		t.Fatalf("body:\n%s\nwant:\n%s", view["body"], want)
	}
	evs := eventsOf(t, view)
	if last := evs[len(evs)-1]; last["type"] != "body_edited" {
		t.Fatalf("last event %v", last)
	}
}

func TestAppendToSectionMissingHeading(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "Map", "body": mapBody}))

	r := h.call(dir, "append_to_section", map[string]any{"ref": "#1", "heading": "Not a heading", "text": "x"})
	if r.code() != "no_section" {
		t.Fatalf("want no_section, got %v", r)
	}
	want := []any{"Route", "Decisions so far", "Detail", "Fog"}
	if !reflect.DeepEqual(r.Err["headings"], want) {
		t.Fatalf("headings %v, want %v", r.Err["headings"], want)
	}
	if view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1"})); view["body"] != mapBody || view["version"] != float64(1) {
		t.Fatalf("a failed append changed the Issue: %v", view)
	}
}

func TestCommentIsRecordedWithAuthorAndSession(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "t"}))

	out := h.mustOK(h.call(dir, "comment", map[string]any{"ref": "#1", "body": "Looks good."}))
	id := out["comment_id"]
	if out["ref"] != "#1" || out["url"] != fmt.Sprintf("http://127.0.0.1:7337/app/issues/1#comment-%v", id) {
		t.Fatalf("got %v", out)
	}
	if r := h.call(dir, "comment", map[string]any{"ref": "#1", "body": ""}); r.code() != "invalid_argument" {
		t.Fatalf("empty comment: want invalid_argument, got %v", r)
	}

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	cs := view["comments"].([]any)
	if len(cs) != 1 {
		t.Fatalf("comments %v", cs)
	}
	c := cs[0].(map[string]any)
	if c["id"] != id || c["body"] != "Looks good." || c["author"] != testAssignee || c["session"] != testSession || c["created_at"] == "" {
		t.Fatalf("comment %v", c)
	}
	if got := types(eventsOf(t, view)); !reflect.DeepEqual(got, []string{"created", "commented"}) {
		t.Fatalf("events %v", got)
	}
	if v := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "comments": false})); v["comments"] != nil {
		t.Fatalf("comments: false still returned %v", v["comments"])
	}
}

func TestCloseAndReopenAreIdempotent(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "t"}))

	out := h.mustOK(h.call(dir, "close", map[string]any{"ref": "#1", "comment": "Done in abc123."}))
	if out["state"] != "closed" || out["already"] != false {
		t.Fatalf("close: %v", out)
	}
	// A retry after an uncertain outcome changes nothing, not even a second comment.
	out = h.mustOK(h.call(dir, "close", map[string]any{"ref": "#1", "comment": "Done in abc123."}))
	if out["state"] != "closed" || out["already"] != true {
		t.Fatalf("second close: %v", out)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	if view["state"] != "closed" || len(view["comments"].([]any)) != 1 {
		t.Fatalf("closed Issue %v", view)
	}

	out = h.mustOK(h.call(dir, "reopen", map[string]any{"ref": "#1"}))
	if out["state"] != "open" || out["already"] != false {
		t.Fatalf("reopen: %v", out)
	}
	out = h.mustOK(h.call(dir, "reopen", map[string]any{"ref": "#1"}))
	if out["state"] != "open" || out["already"] != true {
		t.Fatalf("second reopen: %v", out)
	}
	view = h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1", "events": true}))
	if got := types(eventsOf(t, view)); !reflect.DeepEqual(got, []string{"created", "commented", "closed", "reopened"}) {
		t.Fatalf("events %v", got)
	}
}

func TestCloseWithoutComment(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "t"}))
	h.mustOK(h.call(dir, "close", map[string]any{"ref": "#1"}))
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": "#1"}))
	if view["state"] != "closed" || len(view["comments"].([]any)) != 0 {
		t.Fatalf("got %v", view)
	}
	if r := h.call(dir, "close", map[string]any{"ref": "#2"}); r.code() != "not_found" {
		t.Fatalf("want not_found, got %v", r)
	}
}

func TestListIssuesFiltersOldestFirstWithTotal(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	for _, a := range []map[string]any{
		{"title": "one"},
		{"title": "two", "kind": "map"},
		{"title": "three"},
		{"title": "four", "body": "the body"},
	} {
		h.mustOK(h.call(dir, "create_issue", a))
	}
	h.mustOK(h.call(dir, "close", map[string]any{"ref": "#3"}))
	h.mustOK(h.call(dir, "comment", map[string]any{"ref": "#4", "body": "hi"}))

	refs := func(args map[string]any) ([]string, float64) {
		t.Helper()
		out := h.mustOK(h.call(dir, "list_issues", args))
		var rs []string
		for _, i := range out["issues"].([]any) {
			rs = append(rs, i.(map[string]any)["ref"].(string))
		}
		return rs, out["total"].(float64)
	}
	for _, c := range []struct {
		args  map[string]any
		want  []string
		total float64
	}{
		{map[string]any{}, []string{"#1", "#2", "#4"}, 3},
		{map[string]any{"state": "closed"}, []string{"#3"}, 1},
		{map[string]any{"state": "all"}, []string{"#1", "#2", "#3", "#4"}, 4},
		{map[string]any{"kind": "map"}, []string{"#2"}, 1},
		{map[string]any{"kind": "issue", "state": "all"}, []string{"#1", "#3", "#4"}, 3},
		{map[string]any{"assignee": "none"}, []string{"#1", "#2", "#4"}, 3},
		{map[string]any{"assignee": "any"}, nil, 0},
		{map[string]any{"assignee": testAssignee}, nil, 0},
		{map[string]any{"limit": 2}, []string{"#1", "#2"}, 3},
	} {
		got, total := refs(c.args)
		if !reflect.DeepEqual(got, c.want) || total != c.total {
			t.Errorf("%v: got %v (total %v), want %v (total %v)", c.args, got, total, c.want, c.total)
		}
	}

	summary := h.mustOK(h.call(dir, "list_issues", map[string]any{"limit": 1}))["issues"].([]any)[0].(map[string]any)
	want := map[string]any{"ref": "#1", "title": "one", "kind": "issue", "state": "open", "assignee": nil, "holder_session": nil, "labels": []any{}}
	if !reflect.DeepEqual(summary, want) {
		t.Fatalf("summary %v, want %v", summary, want)
	}

	full := h.mustOK(h.call(dir, "list_issues", map[string]any{"detail": "full"}))["issues"].([]any)
	last := full[len(full)-1].(map[string]any)
	if last["body"] != "the body" || len(last["comments"].([]any)) != 1 {
		t.Fatalf("full detail %v", last)
	}

	for _, bad := range []map[string]any{{"state": "done"}, {"kind": "epic"}, {"detail": "everything"}, {"limit": -1}} {
		if r := h.call(dir, "list_issues", bad); r.code() != "invalid_argument" {
			t.Errorf("%v: want invalid_argument, got %v", bad, r)
		}
	}
}

func TestDeleteIssueCascadesAndNeverReusesItsNumber(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "one"}))
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "two"}))
	h.mustOK(h.call(dir, "comment", map[string]any{"ref": "#2", "body": "c"}))

	if r := h.call(dir, "delete_issue", map[string]any{"ref": "#2", "confirm": "2"}); r.code() != "confirm_mismatch" {
		t.Fatalf("want confirm_mismatch, got %v", r)
	}
	if r := h.call(dir, "delete_issue", map[string]any{"ref": "#9", "confirm": "#9"}); r.code() != "not_found" {
		t.Fatalf("want not_found, got %v", r)
	}
	out := h.mustOK(h.call(dir, "delete_issue", map[string]any{"ref": "app#2", "confirm": "#2"}))
	if out["deleted"] != "#2" {
		t.Fatalf("got %v", out)
	}
	if r := h.call(dir, "view_issue", map[string]any{"ref": "#2"}); r.code() != "not_found" {
		t.Fatalf("deleted Issue: want not_found, got %v", r)
	}
	if out := h.mustOK(h.call(dir, "list_issues", map[string]any{"state": "all"})); out["total"] != float64(1) {
		t.Fatalf("list after delete %v", out)
	}
	// The highest number was deleted, and still isn't reused.
	if out := h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "three"})); out["ref"] != "#3" {
		t.Fatalf("got %v, want #3", out)
	}
}

func TestIssueNumbersArePerProject(t *testing.T) {
	h := newHarness(t)
	a, b := h.project("a"), h.project("b")
	h.mustOK(h.call(a, "create_issue", map[string]any{"title": "a1"}))
	if out := h.mustOK(h.call(b, "create_issue", map[string]any{"title": "b1"})); out["ref"] != "#1" {
		t.Fatalf("got %v", out)
	}
}

func TestDeleteProjectTakesItsIssues(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "one"}))
	h.mustOK(h.call(dir, "comment", map[string]any{"ref": "#1", "body": "c"}))
	h.mustOK(h.call("", "delete_project", map[string]any{"slug": "app", "confirm": "app"}))
	h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "app"}))
	if out := h.mustOK(h.call(dir, "create_issue", map[string]any{"title": "fresh"})); out["ref"] != "#1" {
		t.Fatalf("got %v", out)
	}
}
