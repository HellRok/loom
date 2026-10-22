package tools_test

import (
	"net/http"
	"reflect"
	"testing"
)

const otherSession = "other-session-0002"

// asOther calls a tool as a second Session.
func (h *harness) asOther(cwd, name string, args map[string]any) result {
	h.t.Helper()
	hdr := http.Header{}
	hdr.Set("X-Loom-Session", otherSession)
	hdr.Set("X-Loom-Cwd", cwd)
	return h.callWith(hdr, name, args)
}

func TestClaimAssignsTheHumanAndRecordsTheSession(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	ref := h.issue(dir, map[string]any{})

	out := h.mustOK(h.call(dir, "claim", map[string]any{"ref": ref}))
	if out["ref"] != ref || out["assignee"] != testAssignee || out["holder_session"] != testSession || out["already"] != false || out["claimed_at"] == nil {
		t.Fatalf("claim = %v", out)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ref}))
	if view["assignee"] != testAssignee || view["holder_session"] != testSession || view["claimed_at"] != out["claimed_at"] {
		t.Fatalf("view = %v", view)
	}
	if got, want := eventTypes(t, h, dir, ref), []string{"created", "claimed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestSameSessionReclaimIsAlreadyAndAnotherSessionIsRefused(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	ref := h.issue(dir, map[string]any{})
	first := h.mustOK(h.call(dir, "claim", map[string]any{"ref": ref}))

	again := h.mustOK(h.call(dir, "claim", map[string]any{"ref": ref}))
	if again["already"] != true || again["holder_session"] != testSession || again["claimed_at"] != first["claimed_at"] {
		t.Fatalf("re-claim = %v", again)
	}
	if got := eventTypes(t, h, dir, ref); len(got) != 2 {
		t.Fatalf("a re-claim records no Event, got %v", got)
	}

	r := h.asOther(dir, "claim", map[string]any{"ref": ref})
	if r.code() != "already_claimed" || r.Err["holder"] != testAssignee || r.Err["holder_session"] != testSession || r.Err["claimed_at"] != first["claimed_at"] {
		t.Fatalf("other session's claim = %v", r)
	}
	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ref}))
	if view["holder_session"] != testSession {
		t.Fatalf("a refused claim must change nothing, view = %v", view)
	}
}

func TestClaimingClosedOrBlockedIssuesIsRefused(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	blocker := h.issue(dir, map[string]any{"title": "blocker"})
	blocked := h.issue(dir, map[string]any{"title": "blocked", "blocked_by": []string{blocker}})
	done := h.issue(dir, map[string]any{"title": "done"})
	h.mustOK(h.call(dir, "close", map[string]any{"ref": done}))

	if r := h.call(dir, "claim", map[string]any{"ref": done}); r.code() != "issue_closed" {
		t.Fatalf("closed: got %v", r)
	}
	r := h.call(dir, "claim", map[string]any{"ref": blocked})
	if r.code() != "issue_blocked" || !reflect.DeepEqual(strs(r.Err["open_blockers"]), []string{blocker}) {
		t.Fatalf("blocked: got %v", r)
	}
	if r := h.call(dir, "claim", map[string]any{"ref": "#99"}); r.code() != "not_found" {
		t.Fatalf("unknown: got %v", r)
	}

	// Once the Blocker closes the Issue can be claimed.
	h.mustOK(h.call(dir, "close", map[string]any{"ref": blocker}))
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": blocked}))
}

func TestReleaseIsIdempotentFromAnySessionAndRecordsWho(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	ref := h.issue(dir, map[string]any{})

	if out := h.mustOK(h.call(dir, "release", map[string]any{"ref": ref})); out["already"] != true {
		t.Fatalf("releasing an unclaimed Issue = %v", out)
	}
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": ref}))
	if out := h.mustOK(h.asOther(dir, "release", map[string]any{"ref": ref})); out["already"] != false || out["ref"] != ref {
		t.Fatalf("release = %v", out)
	}
	if out := h.mustOK(h.call(dir, "release", map[string]any{"ref": ref})); out["already"] != true {
		t.Fatalf("second release = %v", out)
	}

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ref, "events": true}))
	if view["assignee"] != nil || view["holder_session"] != nil || view["claimed_at"] != nil {
		t.Fatalf("released Issue still claimed: %v", view)
	}
	events := view["events"].([]any)
	if len(events) != 3 {
		t.Fatalf("events = %v", events)
	}
	rel := events[2].(map[string]any)
	if rel["type"] != "released" || rel["session"] != otherSession || rel["actor"] != testAssignee ||
		rel["data"].(map[string]any)["holder_session"] != testSession {
		t.Fatalf("release event = %v", rel)
	}

	// Anyone can claim it again.
	h.mustOK(h.asOther(dir, "claim", map[string]any{"ref": ref}))
}

func TestListIssuesHeldBy(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	mine := h.issue(dir, map[string]any{"title": "mine"})
	theirs := h.issue(dir, map[string]any{"title": "theirs"})
	h.issue(dir, map[string]any{"title": "free"})
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": mine}))
	h.mustOK(h.asOther(dir, "claim", map[string]any{"ref": theirs}))

	held := func(by string) []string {
		return refsOf(h.mustOK(h.call(dir, "list_issues", map[string]any{"held_by": by}))["issues"])
	}
	if got := held("me"); !reflect.DeepEqual(got, []string{mine}) {
		t.Fatalf("held_by me = %v", got)
	}
	if got := held(otherSession); !reflect.DeepEqual(got, []string{theirs}) {
		t.Fatalf("held_by %s = %v", otherSession, got)
	}
	if got := held("nobody-session-0003"); len(got) != 0 {
		t.Fatalf("held_by unknown = %v", got)
	}
	out := h.mustOK(h.call(dir, "list_issues", map[string]any{"held_by": "me"}))
	if out["total"] != float64(1) || out["issues"].([]any)[0].(map[string]any)["holder_session"] != testSession {
		t.Fatalf("list = %v", out)
	}
}

func TestEveryEventAndCommentRecordsItsSession(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	ref := h.issue(dir, map[string]any{})
	h.mustOK(h.call(dir, "comment", map[string]any{"ref": ref, "body": "mine"}))
	h.mustOK(h.asOther(dir, "comment", map[string]any{"ref": ref, "body": "theirs"}))
	h.mustOK(h.asOther(dir, "claim", map[string]any{"ref": ref}))
	h.mustOK(h.call(dir, "close", map[string]any{"ref": ref}))

	view := h.mustOK(h.call(dir, "view_issue", map[string]any{"ref": ref, "events": true}))
	var got []string
	for _, e := range view["events"].([]any) {
		e := e.(map[string]any)
		got = append(got, e["type"].(string)+"@"+e["session"].(string))
	}
	want := []string{"created@" + testSession, "commented@" + testSession, "commented@" + otherSession, "claimed@" + otherSession, "closed@" + testSession}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	var sessions []string
	for _, c := range view["comments"].([]any) {
		sessions = append(sessions, c.(map[string]any)["session"].(string))
	}
	if !reflect.DeepEqual(sessions, []string{testSession, otherSession}) {
		t.Fatalf("comment sessions = %v", sessions)
	}
}

func TestFrontierSkipsClaimedIssuesAndReleaseRestoresThem(t *testing.T) {
	h := newHarness(t)
	dir := h.project("app")
	spec := h.issue(dir, map[string]any{"title": "Spec"})
	a := h.issue(dir, map[string]any{"title": "A", "parent": spec})
	b := h.issue(dir, map[string]any{"title": "B", "parent": spec})
	h.mustOK(h.call(dir, "claim", map[string]any{"ref": a}))

	out := h.mustOK(h.call(dir, "frontier", map[string]any{"parent": spec}))
	if got := refsOf(out["frontier"]); !reflect.DeepEqual(got, []string{b}) {
		t.Fatalf("frontier = %v", got)
	}
	if got := strs(out["skipped"].(map[string]any)["claimed"]); !reflect.DeepEqual(got, []string{a}) {
		t.Fatalf("skipped.claimed = %v", got)
	}
	h.mustOK(h.asOther(dir, "release", map[string]any{"ref": a}))
	out = h.mustOK(h.call(dir, "frontier", map[string]any{"parent": spec}))
	if got := refsOf(out["frontier"]); !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("frontier after release = %v", got)
	}
}
