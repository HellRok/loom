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

// fakeGraph stands in for the hierarchy until the store has one.
type fakeGraph struct {
	rel      map[int64]web.Relations
	frontier map[int64][]store.Issue
}

func (g fakeGraph) Relations(_ context.Context, _ string, n int64) (web.Relations, error) {
	return g.rel[n], nil
}

func (g fakeGraph) Frontier(_ context.Context, _ string, n int64) ([]store.Issue, error) {
	return g.frontier[n], nil
}

var actor = store.Actor{Name: "Tess Ter", Session: "test-session-0001"}

type fixture struct {
	t  *testing.T
	st *store.Store
	h  http.Handler
}

// seed builds a Project "loom" with a Map (#1), a Ticket (#2) under it, a
// blocked-by chain and a closed Issue, behind the real handlers.
func seed(t *testing.T) *fixture {
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
	if _, err := st.CreateProject(ctx, "other", "/code/other"); err != nil {
		t.Fatal(err)
	}
	mk := func(kind, title, body string, labels ...string) store.Issue {
		i, err := st.CreateIssue(ctx, "loom", actor, kind, title, body, labels)
		if err != nil {
			t.Fatal(err)
		}
		return i
	}
	mk(store.KindMap, "Ship v1", "The *plan*. See #2.")
	mk(store.KindIssue, "Write the web UI", "Body with **bold**.", "wayfinder:task", "needs-triage")
	mk(store.KindIssue, "Fix the bug", "Details", "bug", "ready-for-agent")
	mk(store.KindIssue, "Old thing", "done")
	if _, _, err := st.CloseIssue(ctx, "loom", 4, "Closing it.", false, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddComment(ctx, "loom", 2, "A *comment* mentioning #3 and other#9.", actor); err != nil {
		t.Fatal(err)
	}
	g := fakeGraph{
		rel: map[int64]web.Relations{
			1: {Children: []web.Ref{{Number: 2, Title: "Write the web UI"}, {Number: 4, Title: "Old thing", Closed: true}}},
			2: {
				Parent:   &web.Ref{Number: 1, Title: "Ship v1"},
				Blockers: []web.Ref{{Number: 3, Title: "Fix the bug"}, {Number: 4, Title: "Old thing", Closed: true}},
			},
			3: {Blocks: []web.Ref{{Number: 2, Title: "Write the web UI"}}},
		},
		frontier: map[int64][]store.Issue{1: {{Number: 3, Title: "Fix the bug"}}},
	}
	mux := http.NewServeMux()
	web.Register(mux, web.Deps{Store: st, Graph: g})
	return &fixture{t: t, st: st, h: mux}
}

// get fetches path, with htmx headers when target is non-empty.
func (f *fixture) get(path, target string) (*httptest.ResponseRecorder, *goquery.Document) {
	f.t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if target != "" {
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", target)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rec.Body.String()))
	if err != nil {
		f.t.Fatal(err)
	}
	return rec, doc
}

func text(doc *goquery.Document, sel string) string {
	return strings.Join(strings.Fields(doc.Find(sel).Text()), " ")
}

func expect(t *testing.T, doc *goquery.Document, sel string, n int) {
	t.Helper()
	if got := doc.Find(sel).Length(); got != n {
		t.Errorf("%q matched %d elements, want %d", sel, got, n)
	}
}

func TestHome(t *testing.T) {
	f := seed(t)

	t.Run("full page", func(t *testing.T) {
		rec, doc := f.get("/", "")
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		expect(t, doc, "html head title", 1)
		expect(t, doc, "script[src='/static/htmx-2.0.4.min.js']", 1)
		expect(t, doc, "main#main article.project", 2)
		p := doc.Find("#project-loom")
		if got := strings.Join(strings.Fields(p.Find(".meta").Text()), " "); !strings.Contains(got, "/code/loom") || !strings.Contains(got, "3 open") || !strings.Contains(got, "last activity") {
			t.Errorf("meta = %q", got)
		}
		if got := p.Find("h2 a").AttrOr("href", ""); got != "/loom" {
			t.Errorf("project link = %q", got)
		}
		if got := p.Find(".map-title").Text(); !strings.Contains(got, "#1 Ship v1") {
			t.Errorf("map title = %q", got)
		}
		if got := p.Find("progress").AttrOr("value", ""); got != "1" {
			t.Errorf("progress value = %q", got)
		}
		if got := p.Find("progress").AttrOr("max", ""); got != "2" {
			t.Errorf("progress max = %q", got)
		}
		if got := p.Find(".next a").AttrOr("href", ""); got != "/loom/issues/3" {
			t.Errorf("next link = %q", got)
		}
		if got := text(doc, "#project-other .meta"); !strings.Contains(got, "0 open") || !strings.Contains(got, "never") {
			t.Errorf("empty project meta = %q", got)
		}
	})

	t.Run("fragment", func(t *testing.T) {
		rec, doc := f.get("/", "main")
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		expect(t, doc, "head title", 0)
		expect(t, doc, "section.home article.project", 2)
	})
}

func TestProjectPage(t *testing.T) {
	f := seed(t)

	t.Run("full page", func(t *testing.T) {
		rec, doc := f.get("/loom", "")
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		expect(t, doc, "head title", 1)
		expect(t, doc, "nav.nav", 1)
		expect(t, doc, "section#list-pane", 1)
		expect(t, doc, "article#issue-pane", 1)
		if got := text(doc, "#issue-pane"); got != "Select an Issue to read it." {
			t.Errorf("empty issue pane = %q", got)
		}
		// Switcher with open counts.
		expect(t, doc, ".switcher li", 2)
		if got := text(doc, ".switcher li.active"); got != "loom 3" {
			t.Errorf("active switcher item = %q", got)
		}
		// Saved views.
		var names []string
		doc.Find(".views li a").Each(func(_ int, s *goquery.Selection) { names = append(names, s.Text()) })
		if got := strings.Join(names, ","); got != "Open,Maps,Needs triage,Ready for agent,Unassigned,Closed" {
			t.Errorf("views = %s", got)
		}
		// Labels with counts.
		if got := text(doc, ".labels li:has(.label-yellow:contains('needs-triage'))"); got != "needs-triage 1" {
			t.Errorf("needs-triage label = %q", got)
		}
		// Open view: #1, #2, #3 oldest first, closed #4 excluded.
		expect(t, doc, ".rows .row", 3)
		r2 := doc.Find("#row-2")
		if got := text(doc, "#row-2 .num"); got != "#2" {
			t.Errorf("row num = %q", got)
		}
		if got := text(doc, "#row-2 .parent"); got != "↑#1" {
			t.Errorf("row parent = %q", got)
		}
		if got := text(doc, "#row-2 .title"); got != "Write the web UI" {
			t.Errorf("row title = %q", got)
		}
		if !r2.HasClass("status-blocked") {
			t.Errorf("#2 should be blocked: %v", r2.AttrOr("class", ""))
		}
		expect(t, doc, "#row-2 .label", 2)
		expect(t, doc, "#row-2 time.updated", 1)
		expect(t, doc, "form.filter input[name=q]", 1)
	})

	t.Run("fragments", func(t *testing.T) {
		_, doc := f.get("/loom", "list-pane")
		expect(t, doc, "head title", 0)
		expect(t, doc, "section#list-pane .row", 3)
		expect(t, doc, "nav", 0)
		_, doc = f.get("/loom", "main")
		expect(t, doc, "head title", 0)
		expect(t, doc, "nav.nav", 1)
		expect(t, doc, ".row", 3)
	})

	t.Run("saved views", func(t *testing.T) {
		for view, want := range map[string]int{"maps": 1, "needs-triage": 1, "ready-for-agent": 1, "unassigned": 3, "closed": 1, "": 3, "bogus": 3} {
			_, doc := f.get("/loom?view="+view, "")
			expect(t, doc, ".rows .row", want)
			if view == "closed" {
				expect(t, doc, "#row-4.status-closed", 1)
			}
		}
	})

	t.Run("filters", func(t *testing.T) {
		_, doc := f.get("/loom?label=bug", "")
		expect(t, doc, ".rows .row", 1)
		expect(t, doc, "#row-3", 1)
		if got := doc.Find(".labels li.active").Length(); got != 1 {
			t.Errorf("active labels = %d", got)
		}
		_, doc = f.get("/loom?q=WEB", "")
		expect(t, doc, ".rows .row", 1)
		expect(t, doc, "#row-2", 1)
		// Filters ride along in the Issue links.
		_, doc = f.get("/loom?view=ready-for-agent&label=bug&q=fix", "")
		href := doc.Find("#row-3 a").AttrOr("href", "")
		if href != "/loom/issues/3?label=bug&q=fix&view=ready-for-agent" {
			t.Errorf("row href = %q", href)
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec, doc := f.get("/nope", "")
		if rec.Code != 404 {
			t.Errorf("status %d", rec.Code)
		}
		expect(t, doc, "head title", 1)
		rec, doc = f.get("/nope", "main")
		if rec.Code != 404 {
			t.Errorf("fragment status %d", rec.Code)
		}
		expect(t, doc, "head title", 0)
	})
}

func TestIssueOverview(t *testing.T) {
	f := seed(t)

	check := func(t *testing.T, doc *goquery.Document) {
		t.Helper()
		if got := text(doc, "#issue-pane h1"); got != "#2 Write the web UI" {
			t.Errorf("heading = %q", got)
		}
		expect(t, doc, "#issue-pane .tabs li.active", 1)
		props := map[string]string{
			".state":        "open · blocked",
			".assignee":     "unassigned",
			".issue-labels": "needs-triage wayfinder:task",
			".parent":       "#1 Ship v1",
			".blockers":     "#3 Fix the bug #4 Old thing",
			".blocks":       "none",
			".children":     "none",
			".version":      "1",
		}
		for sel, want := range props {
			if got := text(doc, "#issue-pane dd"+sel); got != want {
				t.Errorf("dd%s = %q, want %q", sel, got, want)
			}
		}
		expect(t, doc, ".blockers a.ref", 2)
		expect(t, doc, ".blockers a.ref.closed s", 1)
		if got := text(doc, ".blockers a.closed s"); got != "#4 Old thing" {
			t.Errorf("struck blocker = %q", got)
		}
		if got := text(doc, ".body strong"); got != "bold" {
			t.Errorf("rendered body bold = %q", got)
		}
		expect(t, doc, ".comments .comment", 1)
		c := doc.Find(".comment").First()
		if id := c.AttrOr("id", ""); id != "comment-2" {
			t.Errorf("comment id = %q", id)
		}
		if got := c.Find(".byline strong").Text(); got != "Tess Ter" {
			t.Errorf("comment author = %q", got)
		}
		if got := c.Find(".byline a").AttrOr("href", ""); got != "#comment-2" {
			t.Errorf("comment anchor = %q", got)
		}
		if got := c.Find(".comment-body a").Map(func(_ int, s *goquery.Selection) string { return s.AttrOr("href", "") }); strings.Join(got, ",") != "/loom/issues/3,/other/issues/9" {
			t.Errorf("comment ref links = %v", got)
		}
	}

	t.Run("full page", func(t *testing.T) {
		rec, doc := f.get("/loom/issues/2", "")
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		expect(t, doc, "head title", 1)
		expect(t, doc, "nav.nav", 1)
		expect(t, doc, "#list-pane #row-2.selected", 1)
		if got := text(doc, "title"); !strings.Contains(got, "#2 Write the web UI") {
			t.Errorf("title = %q", got)
		}
		check(t, doc)
	})

	t.Run("issue pane fragment", func(t *testing.T) {
		rec, doc := f.get("/loom/issues/2", "issue-pane")
		if rec.Code != 200 {
			t.Fatalf("status %d", rec.Code)
		}
		expect(t, doc, "head title", 0)
		expect(t, doc, "nav", 0)
		expect(t, doc, "#list-pane", 0)
		check(t, doc)
		if got := rec.Header().Get("Vary"); !strings.Contains(got, "HX-Target") {
			t.Errorf("Vary = %q", got)
		}
	})

	t.Run("main fragment keeps filters in links", func(t *testing.T) {
		_, doc := f.get("/loom/issues/2?label=bug", "main")
		expect(t, doc, "head title", 0)
		expect(t, doc, "#list-pane", 1)
		if got := doc.Find(".parent a").AttrOr("href", ""); got != "/loom/issues/1?label=bug" {
			t.Errorf("parent href = %q", got)
		}
	})

	t.Run("map with children", func(t *testing.T) {
		_, doc := f.get("/loom/issues/1", "issue-pane")
		if got := text(doc, "dd.state"); got != "open (Map) · open" {
			t.Errorf("state = %q", got)
		}
		if got := text(doc, "dd.children"); got != "#2 Write the web UI #4 Old thing" {
			t.Errorf("children = %q", got)
		}
		if got := doc.Find(".body a").AttrOr("href", ""); got != "/loom/issues/2" {
			t.Errorf("ref in body = %q", got)
		}
	})

	t.Run("closed issue", func(t *testing.T) {
		_, doc := f.get("/loom/issues/4", "issue-pane")
		if got := text(doc, "dd.state"); got != "closed · closed" {
			t.Errorf("state = %q", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		for _, path := range []string{"/loom/issues/99", "/loom/issues/x", "/nope/issues/1", "/loom/issues/0"} {
			rec, _ := f.get(path, "")
			if rec.Code != 404 {
				t.Errorf("%s: status %d", path, rec.Code)
			}
			rec, doc := f.get(path, "issue-pane")
			if rec.Code != 404 {
				t.Errorf("%s fragment: status %d", path, rec.Code)
			}
			expect(t, doc, "head title", 0)
		}
	})
}

func TestWithoutGraph(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.CreateProject(ctx, "loom", "/code/loom")
	st.CreateIssue(ctx, "loom", actor, store.KindMap, "A map", "", nil)
	mux := http.NewServeMux()
	web.Register(mux, web.Deps{Store: st})
	f := &fixture{t: t, st: st, h: mux}
	for _, path := range []string{"/", "/loom", "/loom/issues/1"} {
		if rec, _ := f.get(path, ""); rec.Code != 200 {
			t.Errorf("%s: status %d", path, rec.Code)
		}
	}
}

func TestStatic(t *testing.T) {
	f := seed(t)
	for _, path := range []string{"/static/loom.css", "/static/htmx-2.0.4.min.js", "/static/htmx-ext-sse-2.2.3.js", "/static/idiomorph-ext-0.3.0.min.js"} {
		rec, _ := f.get(path, "")
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Errorf("%s: status %d, %d bytes", path, rec.Code, rec.Body.Len())
		}
	}
	if rec, _ := f.get("/static/missing.js", ""); rec.Code != 404 {
		t.Errorf("missing asset: status %d", rec.Code)
	}
}
