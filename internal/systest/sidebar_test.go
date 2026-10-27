//go:build system

package systest

import (
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

// labelCount is a JavaScript expression for the count shown beside label in
// the sidebar.
func labelCount(label string) string {
	return fmt.Sprintf(`[...document.querySelectorAll("#nav .labels .nav-link")]
		.find(a => a.querySelector(".label-chip").textContent === %q)
		?.querySelector(".count").textContent`, label)
}

func TestSidebarLabelCountsUpdateLive(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_label", map[string]any{"name": "bug"})
	l.Call("create_issue", map[string]any{"title": "Fix the login page"})

	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	if got := Get(l, chromedp.Evaluate[string](labelCount("bug"))); got != "0" {
		t.Fatalf("bug count before labelling: got %q, want 0", got)
	}

	l.Call("label", map[string]any{"ref": "#1", "add": []string{"bug"}})
	pollUntil(l, labelCount("bug")+` === "1"`)

	// Closing the Issue moves it out of the Open view the counts follow.
	l.Call("close", map[string]any{"ref": "#1"})
	pollUntil(l, labelCount("bug")+` === "0"`)
}

// viewCount is a JavaScript expression for the count beside the nth (1-based)
// View in the sidebar.
func viewCount(n int) string {
	return fmt.Sprintf(`document.querySelector("#nav .views .nav-link:nth-child(%d) .count")?.textContent`, n)
}

func TestSelectedLabelNarrowsTheViewCountsAndSurvivesSwitchingView(t *testing.T) {
	const open, closed = 3, 7 // positions of the Open and Closed Views
	l := Start(t, "demo")
	l.Call("create_label", map[string]any{"name": "bug"})
	l.Call("create_issue", map[string]any{"title": "Bug one", "labels": []string{"bug"}})
	l.Call("create_issue", map[string]any{"title": "Bug two", "labels": []string{"bug"}})
	l.Call("create_issue", map[string]any{"title": "Plain"})

	poll := func(expr string) {
		t.Helper()
		pollUntil(l, expr)
	}
	eq := func(expr, want string) string { return fmt.Sprintf(`%s === %q`, expr, want) }

	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	poll(eq(viewCount(open), "3"))

	// Selecting the label narrows the View counts and leaves its own alone.
	poll(eq(labelCount("bug"), "2"))
	Get(l, chromedp.Evaluate[any](`[...document.querySelectorAll("#nav .labels .nav-link")]
		.find(a => a.querySelector(".label-chip").textContent === "bug").click()`))
	poll(`location.search.includes("label=bug")`)
	poll(eq(viewCount(open), "2"))
	poll(eq(labelCount("bug"), "2"))

	// A live update moves the counts with the label still selected.
	l.Call("close", map[string]any{"ref": "#1"})
	poll(eq(viewCount(open), "1"))
	poll(eq(viewCount(closed), "1"))
	poll(eq(labelCount("bug"), "1"))

	// Following a View keeps the label selected.
	Get(l, chromedp.Evaluate[any](fmt.Sprintf(`document.querySelector("#nav .views .nav-link:nth-child(%d)").click()`, closed)))
	poll(`location.search.includes("view=closed") && location.search.includes("label=bug")`)
	poll(`document.querySelector("#nav .labels .nav-link.is-active") && document.querySelector("#row-1")`)
	poll(eq(labelCount("bug"), "1"))
	poll(eq(viewCount(open), "1"))
	poll(eq(viewCount(closed), "1"))

	// And the counts keep following live updates there.
	l.Call("close", map[string]any{"ref": "#2"})
	poll(eq(viewCount(open), "0"))
	poll(eq(viewCount(closed), "2"))
	poll(eq(labelCount("bug"), "2"))
	poll(`location.search.includes("label=bug")`)
}
