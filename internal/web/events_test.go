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
	if got := attr(doc, "#list-pane", "hx-trigger"); got != refresh {
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
