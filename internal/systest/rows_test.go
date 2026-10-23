//go:build system

package systest

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestClickingARowOpensTheIssueExceptOnTheSessionLink(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"title": "Beep the boop"})
	l.Call("claim", map[string]any{"ref": "#1"})

	path := func(prefix string) string {
		t.Helper()
		return Get(l, chromedp.Poll[string](`location.pathname.startsWith(`+`"`+prefix+`"`+`) && location.pathname`,
			chromedp.WithPollingTimeout(5*time.Second)))
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

// selectedRows is a JavaScript expression for the ids of the list's selected rows.
const selectedRows = `[...document.querySelectorAll(".row.selected")].map(r => r.id).join()`

func TestClickingARowMovesTheSelectedHighlight(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"title": "First"})
	l.Call("create_issue", map[string]any{"title": "Second"})

	selected := func(want string) {
		t.Helper()
		got := Get(l, chromedp.Poll[string](`(`+selectedRows+` === `+`"`+want+`") && "`+want+`"`,
			chromedp.WithPollingTimeout(5*time.Second)))
		if got != want {
			t.Fatalf("selected rows = %q, want %q", got, want)
		}
	}

	l.Do(chromedp.Navigate(l.URL("/demo/issues/1?view=open")))
	selected("row-1")

	l.Do(chromedp.Click(chromedp.CSS("#row-2 .updated")))
	Get(l, chromedp.Poll[string](`location.pathname === "/demo/issues/2" && "ok"`,
		chromedp.WithPollingTimeout(5*time.Second)))
	selected("row-2")

	// The list was morphed in place, not nested inside the old one.
	if got := Get(l, chromedp.Evaluate[float64](`document.querySelectorAll("#list-pane").length`)); got != 1 {
		t.Errorf("%v list panes, want 1", got)
	}
}
