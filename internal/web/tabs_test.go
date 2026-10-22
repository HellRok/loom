package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/HellRok/loom/internal/store"
	"github.com/HellRok/loom/internal/web"
)

// seedMap builds a Project "loom" on the real hierarchy: Map #1 with Tickets
// #2 (frontier), #3 (claimed), #4 (blocked by #2), #5 (decided), and a plain
// Issue #6 whose title and body have been edited.
func seedMap(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateProject(ctx, "loom", "/code/loom"); err != nil {
		t.Fatal(err)
	}
	mk := func(n store.NewIssue) {
		t.Helper()
		if _, err := st.CreateIssue(ctx, "loom", actor, withKind(n)); err != nil {
			t.Fatal(err)
		}
	}
	mk(store.NewIssue{Kind: store.KindMap, Title: "Ship v1", Body: "The plan."})
	mk(store.NewIssue{Title: "Pick a database", Parent: 1, Labels: []string{"wayfinder:research"}})
	mk(store.NewIssue{Title: "Design the board", Parent: 1, Labels: []string{"wayfinder:grilling"}})
	mk(store.NewIssue{Title: "Build the board", Parent: 1, Labels: []string{"wayfinder:task"}, BlockedBy: []int64{2}})
	mk(store.NewIssue{Title: "Choose a router", Parent: 1, Labels: []string{"wayfinder:research"}})
	mk(store.NewIssue{Title: "Plain issue", Body: "line one\nline two\nline three"})
	other := store.Actor{Name: "Agent Smith", Session: "abcdefgh-session-claim-9999"}
	if _, _, err := st.ClaimIssue(ctx, "loom", 3, other); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CloseIssue(ctx, "loom", 5, "Use net/http.\n\nIt has everything we need, and **no** deps.", false, actor); err != nil {
		t.Fatal(err)
	}
	title, body := "Plain issue, renamed", "line one\nline 2\nline three"
	if _, err := st.EditIssue(ctx, "loom", 6, 1, &title, &body, actor); err != nil {
		t.Fatal(err)
	}
	title2 := "Plain issue, renamed again"
	if _, err := st.EditIssue(ctx, "loom", 6, 2, &title2, nil, actor); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	web.Register(mux, web.Deps{Store: st, Graph: web.StoreGraph(st)})
	return &fixture{t: t, st: st, h: mux}
}

func withKind(n store.NewIssue) store.NewIssue {
	if n.Kind == "" {
		n.Kind = store.KindIssue
	}
	return n
}

// both runs check against the full page and the issue pane fragment.
func both(t *testing.T, f *fixture, path string, check func(t *testing.T, doc *goquery.Document)) {
	t.Helper()
	t.Run("full page", func(t *testing.T) {
		rec, doc := f.get(path, "")
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		expect(t, doc, "nav.nav", 1)
		expect(t, doc, "#issue-pane", 1)
		check(t, doc)
	})
	t.Run("issue pane fragment", func(t *testing.T) {
		rec, doc := f.get(path, "issue-pane")
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		expect(t, doc, "nav.nav", 0)
		check(t, doc)
	})
}

func tabLabels(doc *goquery.Document) string {
	return strings.Join(doc.Find("#issue-pane .tabs li").Map(func(_ int, s *goquery.Selection) string {
		return strings.Fields(s.Text())[0]
	}), ",")
}

func TestMapBoard(t *testing.T) {
	f := seedMap(t)
	check := func(t *testing.T, doc *goquery.Document) {
		t.Helper()
		if got := tabLabels(doc); got != "Board,Decided,Overview,History" {
			t.Errorf("tabs = %q", got)
		}
		if got := text(doc, "#issue-pane .tabs li.active"); got != "Board" {
			t.Errorf("active tab = %q", got)
		}
		expect(t, doc, ".board .column", 3)
		cols := doc.Find(".board .column").Map(func(_ int, s *goquery.Selection) string {
			return strings.TrimSpace(s.Find("h2").Contents().First().Text())
		})
		if strings.Join(cols, ",") != "Frontier,Claimed,Blocked" {
			t.Errorf("columns = %v", cols)
		}
		expect(t, doc, ".column.frontier .card", 1)
		expect(t, doc, ".column.claimed .card", 1)
		expect(t, doc, ".column.blocked .card", 1)
		fr := doc.Find(".column.frontier .card")
		if got := text(doc, ".column.frontier .card .title"); got != "Pick a database" {
			t.Errorf("frontier card title = %q", got)
		}
		if got := fr.Find(".type").Text(); got != "wayfinder:research" {
			t.Errorf("frontier type = %q", got)
		}
		if got := fr.Find(".num").Text(); got != "#2" {
			t.Errorf("frontier num = %q", got)
		}
		if got := fr.Find("a").AttrOr("href", ""); got != "/loom/issues/2" {
			t.Errorf("card href = %q", got)
		}
		cl := doc.Find(".column.claimed .card")
		if got := cl.Find(".assignee").Text(); got != "Agent Smith" {
			t.Errorf("claimed assignee = %q", got)
		}
		if got := cl.Find(".num").Text(); got != "#3" {
			t.Errorf("claimed num = %q", got)
		}
		bl := doc.Find(".column.blocked .card")
		if got := bl.Find(".num").Text(); got != "#4" {
			t.Errorf("blocked num = %q", got)
		}
		if got := bl.Find(".blockers").Text(); !strings.Contains(got, "#2") {
			t.Errorf("blocked card blockers = %q", got)
		}
		expect(t, doc, ".board .card.closed", 0)
	}
	both(t, f, "/loom/issues/1", check)
	both(t, f, "/loom/issues/1?tab=board", check)
}

func TestMapDecided(t *testing.T) {
	f := seedMap(t)
	both(t, f, "/loom/issues/1?tab=decided", func(t *testing.T, doc *goquery.Document) {
		if got := text(doc, "#issue-pane .tabs li.active"); got != "Decided" {
			t.Errorf("active tab = %q", got)
		}
		expect(t, doc, ".board", 0)
		expect(t, doc, ".decided .decision", 1)
		d := doc.Find(".decided .decision")
		if got := d.Find(".num").Text(); got != "#5" {
			t.Errorf("num = %q", got)
		}
		if got := d.Find(".title").Text(); got != "Choose a router" {
			t.Errorf("title = %q", got)
		}
		if got := d.Find(".gist").Text(); got != "Use net/http." {
			t.Errorf("gist = %q", got)
		}
	})
}

func TestMapOverviewTab(t *testing.T) {
	f := seedMap(t)
	both(t, f, "/loom/issues/1?tab=overview", func(t *testing.T, doc *goquery.Document) {
		if got := text(doc, "#issue-pane .tabs li.active"); got != "Overview" {
			t.Errorf("active tab = %q", got)
		}
		expect(t, doc, ".board", 0)
		if got := text(doc, "dd.version"); got != "1" {
			t.Errorf("version = %q", got)
		}
	})
}

// The daemon's Graph is the store's: Overview shows real relations.
func TestStoreGraphRelations(t *testing.T) {
	f := seedMap(t)
	_, doc := f.get("/loom/issues/2?tab=overview", "")
	if got := text(doc, "dd.blocks"); got != "#4 Build the board" {
		t.Errorf("blocks = %q", got)
	}
	if got := text(doc, "dd.parent"); got != "#1 Ship v1" {
		t.Errorf("parent = %q", got)
	}
	_, doc = f.get("/loom/issues/4?tab=overview", "")
	if got := text(doc, "dd.blockers"); got != "#2 Pick a database" {
		t.Errorf("blockers = %q", got)
	}
}

func TestIssueTabs(t *testing.T) {
	f := seedMap(t)
	both(t, f, "/loom/issues/6", func(t *testing.T, doc *goquery.Document) {
		if got := tabLabels(doc); got != "Overview,History" {
			t.Errorf("tabs = %q", got)
		}
		if got := text(doc, "#issue-pane .tabs li.active"); got != "Overview" {
			t.Errorf("active tab = %q", got)
		}
	})
	// A Map-only tab on a plain Issue falls back to its default.
	both(t, f, "/loom/issues/6?tab=board", func(t *testing.T, doc *goquery.Document) {
		if got := text(doc, "#issue-pane .tabs li.active"); got != "Overview" {
			t.Errorf("active tab = %q", got)
		}
		expect(t, doc, ".board", 0)
	})
	// Tab links keep the list filters and select the tab.
	_, doc := f.get("/loom/issues/6?view=closed", "")
	var hrefs []string
	doc.Find("#issue-pane .tabs a").Each(func(_ int, s *goquery.Selection) { hrefs = append(hrefs, s.AttrOr("href", "")) })
	if strings.Join(hrefs, " ") != "/loom/issues/6?view=closed /loom/issues/6?tab=history&view=closed" {
		t.Errorf("tab links = %v", hrefs)
	}
}

func TestHistory(t *testing.T) {
	f := seedMap(t)
	check := func(t *testing.T, doc *goquery.Document) {
		t.Helper()
		if got := text(doc, "#issue-pane .tabs li.active"); got != "History" {
			t.Errorf("active tab = %q", got)
		}
		evs := doc.Find(".history .event")
		var kinds []string
		evs.Each(func(_ int, s *goquery.Selection) { kinds = append(kinds, s.AttrOr("data-type", "")) })
		if got := strings.Join(kinds, ","); got != "title_edited,body_edited,title_edited,created" {
			t.Errorf("events newest first = %s", got)
		}
		first := evs.First()
		if got := first.Find(".actor").Text(); got != "Tess Ter" {
			t.Errorf("actor = %q", got)
		}
		expect(t, doc, ".event details", 3)
		// Newest title edit: "renamed" -> "renamed again".
		d := first.Find("details")
		if got := d.Find("del").Text(); got != "Plain issue, renamed" {
			t.Errorf("newest title del = %q", got)
		}
		if got := d.Find("ins").Text(); got != "Plain issue, renamed again" {
			t.Errorf("newest title ins = %q", got)
		}
		// The body edit changed one line of three.
		body := doc.Find(`.event[data-type="body_edited"] details`)
		if got := body.Find("del").Text(); got != "line two" {
			t.Errorf("body del = %q", got)
		}
		if got := body.Find("ins").Text(); got != "line 2" {
			t.Errorf("body ins = %q", got)
		}
		if got := body.Find(".same").Length(); got != 2 {
			t.Errorf("unchanged lines = %d", got)
		}
		// The first title edit's new value is the next edit's previous.
		old := doc.Find(`.event[data-type="title_edited"]`).Last().Find("details")
		if got := old.Find("del").Text(); got != "Plain issue" {
			t.Errorf("first title del = %q", got)
		}
		if got := old.Find("ins").Text(); got != "Plain issue, renamed" {
			t.Errorf("first title ins = %q", got)
		}
	}
	both(t, f, "/loom/issues/6?tab=history", check)

	t.Run("map events", func(t *testing.T) {
		_, doc := f.get("/loom/issues/1?tab=history", "")
		expect(t, doc, ".history .event", 1)
	})
	t.Run("ticket events describe relations, claims and closing", func(t *testing.T) {
		_, doc := f.get("/loom/issues/5?tab=history", "")
		var kinds []string
		doc.Find(".history .event").Each(func(_ int, s *goquery.Selection) { kinds = append(kinds, s.AttrOr("data-type", "")) })
		if got := strings.Join(kinds, ","); got != "closed,commented,labelled,parent_set,created" && got != "closed,commented,parent_set,labelled,created" {
			t.Errorf("events = %s", got)
		}
		if got := text(doc, `.event[data-type="commented"] a`); got != "comment" {
			t.Errorf("comment link text = %q", got)
		}
		_, doc = f.get("/loom/issues/3?tab=history", "")
		if got := text(doc, `.event[data-type="claimed"] .actor`); got != "Agent Smith" {
			t.Errorf("claim actor = %q", got)
		}
		_, doc = f.get("/loom/issues/4?tab=history", "")
		if got := text(doc, `.event[data-type="blocker_added"]`); !strings.Contains(got, "#2") {
			t.Errorf("blocker event = %q", got)
		}
	})
}

func TestTabNotFound(t *testing.T) {
	f := seedMap(t)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest("GET", "/loom/issues/99?tab=history", nil))
	if rec.Code != 404 {
		t.Errorf("status %d", rec.Code)
	}
}
