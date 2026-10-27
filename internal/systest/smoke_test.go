//go:build system

package systest

import (
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestProjectPageShowsSeededIssue(t *testing.T) {
	l := Start(t, "demo")
	l.Call("create_issue", map[string]any{"title": "Fix the login page"})

	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	if title := Get(l, chromedp.Text(chromedp.CSS("#row-1 .issue-title"))); strings.TrimSpace(title) != "Fix the login page" {
		t.Fatalf("row #1 title: got %q", title)
	}
}
