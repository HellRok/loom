//go:build system

package systest

import (
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

// rowStatus is a JavaScript expression for the status class of Issue n's row
// in the list pane, or "" with no such row.
func rowStatus(n int) string {
	return fmt.Sprintf(`(document.querySelector("#list-pane #row-%d")?.className.match(/status-\S+/) || [""])[0]`, n)
}

func TestIssueWithOpenChildrenShowsAsBlocked(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"title": "Parent"})
	l.Call("create_issue", map[string]any{"title": "Child", "parent": "#1"})

	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	poll := func(expr, want string) {
		t.Helper()
		pollUntil(l, fmt.Sprintf(`(%s) === %q`, expr, want))
	}
	waiting := `document.querySelector("#list-pane #row-1 .row-waiting")?.textContent ?? ""`
	poll(rowStatus(1), "status-blocked")
	poll(waiting, "waiting on 1 open child")
	poll(rowStatus(2), "status-open")

	// The Issue pane says so apart from the explicit Blockers.
	l.Do(chromedp.Navigate(l.URL("/demo/issues/1")))
	poll(`document.querySelector("#issue-pane dd.open-children")?.textContent ?? ""`, "waiting on 1 open child")
	poll(`document.querySelector("#issue-pane dd.blockers").textContent.trim()`, "none")

	// Closing the last child lifts it, live.
	l.Call("close", map[string]any{"ref": "#2"})
	poll(`document.querySelector("#issue-pane dd.open-children")?.textContent ?? ""`, "")
	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	poll(rowStatus(1), "status-open")
}
