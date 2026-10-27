//go:build system

package systest

import (
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestBoardCardLinksToTheClaimingSession(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"kind": "map", "title": "Plan"})
	l.Call("create_issue", map[string]any{"title": "Ticket", "parent": "#1", "labels": []string{"wayfinder:task"}})
	l.Call("claim", map[string]any{"ref": "#2"})

	l.Do(chromedp.Navigate(l.URL("/demo/issues/1?tab=board")))
	l.Do(chromedp.Click(chromedp.CSS(".board-column.claimed .board-card .session-link")))

	pollUntil(l, `location.pathname.startsWith("/sessions/")`)
	if got := Get(l, chromedp.Evaluate[string](`location.pathname`)); !strings.HasPrefix(got, "/sessions/") || got == "/sessions/" {
		t.Fatalf("clicking the card's Session link: got %q", got)
	}
}
