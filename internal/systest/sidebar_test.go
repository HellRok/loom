//go:build system

package systest

import (
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// labelCount is a JavaScript expression for the count shown beside label in
// the sidebar.
func labelCount(label string) string {
	return fmt.Sprintf(`[...document.querySelectorAll("#nav .labels li")]
		.find(li => li.querySelector(".label").textContent === %q)
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
	got := Get(l, chromedp.Poll[string](labelCount("bug")+` === "1" && "1"`,
		chromedp.WithPollingTimeout(5*time.Second)))
	if got != "1" {
		t.Fatalf("bug count after labelling: got %q, want 1", got)
	}

	// Closing the Issue moves it out of the Open view the counts follow.
	l.Call("close", map[string]any{"ref": "#1"})
	got = Get(l, chromedp.Poll[string](labelCount("bug")+` === "0" && "0"`,
		chromedp.WithPollingTimeout(5*time.Second)))
	if got != "0" {
		t.Fatalf("bug count after closing: got %q, want 0", got)
	}
}
