//go:build system

package systest

import (
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

// frontierRows is a JavaScript expression for the Issue numbers listed in the
// Frontier view's ready rows, space-separated.
const frontierRows = `[...document.querySelectorAll("#list-pane .rows .row .num")].map(n => n.textContent).join(" ")`

// frontierIs waits for the ready rows to read want.
func frontierIs(t *testing.T, l *Loom, want string) {
	t.Helper()
	pollUntil(l, fmt.Sprintf(`%s === %q`, frontierRows, want))
}

func TestFrontierUpdatesLive(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"title": "First"})
	l.Call("create_issue", map[string]any{"title": "Second", "blocked_by": []string{"#1"}})

	l.Do(chromedp.Navigate(l.URL("/demo?view=frontier")))
	frontierIs(t, l, "#1")

	// Claiming #1 takes it out of the Frontier; releasing returns it.
	l.Call("claim", map[string]any{"ref": "#1"})
	frontierIs(t, l, "")
	l.Call("release", map[string]any{"ref": "#1"})
	frontierIs(t, l, "#1")

	// Closing the Blocker brings #2 in.
	l.Call("close", map[string]any{"ref": "#1"})
	frontierIs(t, l, "#2")

	// Tagging it reaches the sidebar's Label count.
	l.Call("label", map[string]any{"ref": "#2", "add": []string{"ready-for-agent"}})
	pollUntil(l, labelCount("ready-for-agent")+` === "1"`)
}
