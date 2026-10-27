package web_test

import (
	"context"
	"strings"
	"testing"

	"github.com/HellRok/loom/internal/store"
)

var (
	sessA = store.Actor{Name: "Tess Ter", Session: "0190aaaa-0000-7000-8000-aaaa11112222"}
	sessB = store.Actor{Name: "Tess Ter", Session: "0190bbbb-0000-7000-8000-bbbb33334444"}
)

// seedSessions has A claim loom#3 and file an other#1, and B only be seen.
// A was seen last.
func seedSessions(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := seed(t)
	for _, s := range []string{sessB.Session, sessA.Session} {
		if err := f.st.TouchSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := f.st.ClaimIssue(ctx, "loom", 3, sessA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.CreateIssue(ctx, "other", sessA, store.NewIssue{Kind: store.KindIssue, Title: "Elsewhere"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.AddComment(ctx, "loom", 3, "on it", sessB); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestClaimsShowHolderSession(t *testing.T) {
	f := seedSessions(t)

	t.Run("list row", func(t *testing.T) {
		_, doc := f.get("/loom?view=open", "")
		row := doc.Find("#row-3")
		link := row.Find(".held-by a.session-link")
		if link.Length() != 1 || link.Text() != "11112222" {
			t.Fatalf("holder link = %q", link.Text())
		}
		if got := link.AttrOr("title", ""); got != sessA.Session {
			t.Errorf("title = %q, want the full Session", got)
		}
		if got := link.AttrOr("href", ""); got != "/sessions/"+sessA.Session {
			t.Errorf("href = %q", got)
		}
		if got := text(doc, "#row-3 .held-by .idle"); got != "idle <1m" {
			t.Errorf("idle = %q", got)
		}
		if got := text(doc, "#row-3 .held-by"); got != "held by: 11112222 idle <1m" {
			t.Errorf("held-by line = %q", got)
		}
		expect(t, doc, "#row-2 .held-by", 0)
	})

	t.Run("issue overview", func(t *testing.T) {
		_, doc := f.get("/loom/issues/3", "issue-pane")
		if got := doc.Find("#issue-pane .held-by a.session-link").Text(); got != "11112222" {
			t.Errorf("holder = %q", got)
		}
		if got := text(doc, "#issue-pane .held-by .idle"); got != "idle <1m" {
			t.Errorf("idle = %q", got)
		}
		_, doc = f.get("/loom/issues/2", "issue-pane")
		expect(t, doc, "#issue-pane .held-by", 0)
	})
}

func TestSessionsPage(t *testing.T) {
	f := seedSessions(t)
	rec, doc := f.get("/sessions", "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	expect(t, doc, "main#main section.sessions", 1)
	expect(t, doc, "a[href='/sessions']", 1)
	rows := doc.Find(".session-list tbody tr")
	if rows.Length() != 2 {
		t.Fatalf("%d rows", rows.Length())
	}
	first := rows.Eq(0).Find("a.session-link")
	if first.Text() != "11112222" || first.AttrOr("title", "") != sessA.Session {
		t.Errorf("first row = %q (%q), want the most recently seen Session A", first.Text(), first.AttrOr("title", ""))
	}
	if got := rows.Eq(1).Find("a.session-link").Text(); got != "33334444" {
		t.Errorf("second row = %q", got)
	}

	_, doc = f.get("/sessions", "main")
	expect(t, doc, "head title", 0)
	expect(t, doc, "section.sessions", 1)
}

func TestSessionPage(t *testing.T) {
	f := seedSessions(t)
	rec, doc := f.get("/sessions/"+sessA.Session, "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if got := doc.Find("title").Text(); got != "Session 11112222" {
		t.Errorf("title = %q", got)
	}
	if got := doc.Find(".full-id").Text(); got != sessA.Session {
		t.Errorf("full id = %q", got)
	}
	held := doc.Find("ul.held li")
	if held.Length() != 1 || !strings.Contains(held.Text(), "loom#3 Fix the bug") || held.Find("a").AttrOr("href", "") != "/loom/issues/3" {
		t.Errorf("held = %q", held.Text())
	}
	// A created other#1 then claimed loom#3, so newest first.
	evs := doc.Find("ul.events li")
	if evs.Length() != 2 {
		t.Fatalf("%d events", evs.Length())
	}
	if got := evs.Eq(0).Find(".type").Text(); got != store.EventCreated {
		t.Errorf("newest event = %q, want %q (other#1 was created after the claim)", got, store.EventCreated)
	}
	if got := evs.Eq(0).Find("a").AttrOr("href", ""); got != "/other/issues/1" {
		t.Errorf("newest event links %q", got)
	}
	if got := evs.Eq(1).Find(".type").Text(); got != store.EventClaimed {
		t.Errorf("older event = %q", got)
	}

	_, doc = f.get("/sessions/"+sessB.Session, "")
	expect(t, doc, "ul.held li.empty", 1)
	if got := doc.Find("ul.events li").Length(); got != 1 || doc.Find("ul.events .type").Text() != store.EventCommented {
		t.Errorf("B's events = %q", doc.Find("ul.events").Text())
	}

	if rec, _ := f.get("/sessions/no-such-session-0000", ""); rec.Code != 404 {
		t.Errorf("unknown Session: status %d", rec.Code)
	}
}
