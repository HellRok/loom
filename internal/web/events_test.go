package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/HellRok/loom/internal/web"
)

// TestLiveMarkup checks the wiring the browser needs for live updates: each
// page connects to its stream, the sidebar, list pane and home refresh on changed,
// the Issue pane only on its own Issue, and every pane refetches when the
// stream (re)opens.
func TestLiveMarkup(t *testing.T) {
	f := seed(t)
	attr := func(doc *goquery.Document, sel, name string) string {
		v, _ := doc.Find(sel).Attr(name)
		return v
	}
	const refresh = "sse:changed, htmx:sseOpen from:body"

	_, doc := f.get("/", "")
	if got := attr(doc, "[sse-connect]", "sse-connect"); got != "/events" {
		t.Errorf("home connects to %q", got)
	}
	if got := attr(doc, "[sse-connect]", "hx-ext"); got != "sse,morph" {
		t.Errorf("hx-ext = %q", got)
	}
	if got := attr(doc, "#home-pane", "hx-trigger"); got != refresh {
		t.Errorf("home refreshes on %q", got)
	}
	if got := attr(doc, "#home-pane", "hx-swap"); got != "morph:outerHTML" {
		t.Errorf("home swaps with %q", got)
	}

	_, doc = f.get("/loom/issues/2", "")
	if got := attr(doc, "[sse-connect]", "sse-connect"); got != "/loom/events" {
		t.Errorf("project connects to %q", got)
	}
	if got := attr(doc, "#nav", "hx-trigger"); got != refresh {
		t.Errorf("sidebar refreshes on %q", got)
	}
	// The list also refreshes after a link swaps the Issue pane and pushes its
	// URL, so its selected row follows.
	const listRefresh = refresh + ", htmx:afterSwap from:body [detail.target.id=='issue-pane' && detail.requestConfig.elt.hasAttribute('hx-push-url')]"
	if got := attr(doc, "#list-pane", "hx-trigger"); got != listRefresh {
		t.Errorf("list refreshes on %q", got)
	}
	if got := attr(doc, "#issue-pane", "hx-trigger"); got != "sse:issue-2, htmx:sseOpen from:body" {
		t.Errorf("Issue pane refreshes on %q", got)
	}
	for _, id := range []string{"#nav", "#list-pane", "#issue-pane"} {
		if got := attr(doc, id, "hx-swap"); got != "morph:outerHTML" {
			t.Errorf("%s swaps with %q", id, got)
		}
	}

	// With no Issue selected the Issue pane has nothing to refresh.
	_, doc = f.get("/loom", "")
	if _, ok := doc.Find("#issue-pane").Attr("hx-trigger"); ok {
		t.Error("an empty Issue pane should not refresh")
	}

	// Fragments carry the same wiring but not the stream connection, so a
	// morph never reconnects it.
	_, doc = f.get("/loom/issues/2", "issue-pane")
	if got := attr(doc, "#issue-pane", "hx-trigger"); got != "sse:issue-2, htmx:sseOpen from:body" {
		t.Errorf("Issue pane fragment refreshes on %q", got)
	}
	expect(t, doc, "[sse-connect]", 0)
	_, doc = f.get("/loom", "nav")
	expect(t, doc, "nav#nav .labels", 1)
	expect(t, doc, "#list-pane", 0)
	expect(t, doc, "[sse-connect]", 0)
	_, doc = f.get("/", "home-pane")
	expect(t, doc, "#home-pane", 1)
	expect(t, doc, "[sse-connect]", 0)
}

func TestEventsForUnknownProject(t *testing.T) {
	f := seed(t)
	if rec, _ := f.get("/nope/events", ""); rec.Code != 404 {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

// TestClosingStreamsEndsThem is what lets the daemon shut down while
// browsers still have the page open.
func TestClosingStreamsEndsThem(t *testing.T) {
	f := seed(t)
	mux := http.NewServeMux()
	closeStreams := web.Register(mux, web.Deps{Store: f.st})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	closeStreams()
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := resp.Body.Read(buf); err != nil {
				close(done)
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream outlived closeStreams")
	}
	if resp2, err := http.Get(ts.URL + "/events"); err == nil {
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("a closed broker answered %d", resp2.StatusCode)
		}
	}
}

// TestLiveSessionsAndGlossaryMarkup checks the Glossary page follows its
// Project's stream, and the Sessions pages the global one, each refreshing
// its pane with the morph swap and still rendering as a page and a fragment.
func TestLiveSessionsAndGlossaryMarkup(t *testing.T) {
	f := seedSessions(t)
	standalone(t, f, "loom") // the Glossary page is 404 for a plain Project
	const refresh = "sse:changed, htmx:sseOpen from:body"
	for _, tc := range []struct {
		path, pane, stream string
	}{
		{"/loom/glossary", "glossary-pane", "/loom/events"},
		{"/sessions", "sessions-pane", "/events"},
		{"/sessions/" + sessA.Session, "session-pane", "/events"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			sel := "#" + tc.pane
			rec, doc := f.get(tc.path, "")
			if rec.Code != 200 {
				t.Fatalf("status %d", rec.Code)
			}
			expect(t, doc, "header.top", 1)
			if got := doc.Find("[sse-connect]").AttrOr("sse-connect", ""); got != tc.stream {
				t.Errorf("connects to %q, want %q", got, tc.stream)
			}
			if got := doc.Find("[sse-connect]").AttrOr("hx-ext", ""); got != "sse,morph" {
				t.Errorf("hx-ext = %q", got)
			}
			expect(t, doc, "[sse-connect] "+sel, 1)
			pane := doc.Find(sel)
			if _, ok := pane.Attr("data-live"); !ok {
				t.Error("pane is not data-live")
			}
			if got := pane.AttrOr("hx-trigger", ""); got != refresh {
				t.Errorf("refreshes on %q", got)
			}
			if got := pane.AttrOr("hx-swap", ""); got != "morph:outerHTML" {
				t.Errorf("swaps with %q", got)
			}
			if got := pane.AttrOr("hx-target", ""); got != "this" {
				t.Errorf("targets %q", got)
			}
			if got := pane.AttrOr("hx-get", ""); got != tc.path {
				t.Errorf("fallback hx-get = %q, want %q", got, tc.path)
			}

			// As a fragment: the pane alone, without the stream connection.
			rec, doc = f.get(tc.path, tc.pane)
			if rec.Code != 200 {
				t.Fatalf("fragment status %d", rec.Code)
			}
			expect(t, doc, "header.top", 0)
			expect(t, doc, sel, 1)
			expect(t, doc, "[sse-connect]", 0)
			if got := doc.Find(sel).AttrOr("hx-trigger", ""); got != refresh {
				t.Errorf("fragment refreshes on %q", got)
			}

			// Navigating to the page into #main brings the connection too.
			_, doc = f.get(tc.path, "main")
			expect(t, doc, "[sse-connect] "+sel, 1)
		})
	}
}
