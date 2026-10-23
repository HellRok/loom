//go:build system

package systest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestClickingARowOpensTheIssueExceptOnTheSessionLink(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"title": "Beep the boop"})
	l.Call("claim", map[string]any{"ref": "#1"})

	path := func(prefix string) string {
		t.Helper()
		pollUntil(l, fmt.Sprintf(`location.pathname.startsWith(%q)`, prefix))
		return Get(l, chromedp.Evaluate[string](`location.pathname`))
	}

	// The timestamp is a meta line, away from the title link.
	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	l.Do(chromedp.Click(chromedp.CSS("#row-1 .updated")))
	if got := path("/demo/issues/1"); got != "/demo/issues/1" {
		t.Fatalf("clicking the timestamp: got %q", got)
	}

	// The Session ID link goes to its session page instead.
	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	l.Do(chromedp.Click(chromedp.CSS("#row-1 .session-link")))
	if got := path("/sessions/"); !strings.HasPrefix(got, "/sessions/") {
		t.Fatalf("clicking the Session ID: got %q", got)
	}
}

func TestClickingARowMovesTheSelectedHighlight(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"title": "First"})
	l.Call("create_issue", map[string]any{"title": "Second"})

	// selected waits for exactly the row with this id to be the selected one.
	selected := func(want string) {
		t.Helper()
		pollUntil(l, fmt.Sprintf(`[...document.querySelectorAll(".row.selected")].map(r => r.id).join() === %q`, want))
	}

	l.Do(chromedp.Navigate(l.URL("/demo/issues/1?view=open")))
	selected("row-1")

	l.Do(chromedp.Click(chromedp.CSS("#row-2 .updated")))
	pollUntil(l, `location.pathname === "/demo/issues/2"`)
	selected("row-2")

	// The list was morphed in place, not nested inside the old one.
	if got := Get(l, chromedp.Evaluate[float64](`document.querySelectorAll("#list-pane").length`)); got != 1 {
		t.Errorf("%v list panes, want 1", got)
	}
}
