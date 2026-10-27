//go:build system

package systest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestGlossaryFillsTheSpaceBesideTheNavigation(t *testing.T) {
	l := Start(t, "demo")
	l.Call("set_standalone", map[string]any{"standalone": true})
	l.Call("edit_glossary", map[string]any{"version": 0,
		"body": "# Terms\n\n" + strings.Repeat("**Widget**: a thing.\n\n", 120)})
	l.Call("create_issue", map[string]any{"title": "An Issue"})

	l.Do(chromedp.EmulateViewport(1280, 720))
	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	Get(l, chromedp.Evaluate[any](`document.querySelector("#nav a.reference").click()`))
	pollUntil(l, `location.pathname === "/demo/glossary"`)
	pollUntil(l, `document.querySelector("#glossary-pane")`)

	// The list and Issue panes are gone and the Glossary takes their space.
	if got := Get(l, chromedp.Evaluate[bool](`!document.querySelector("#list-pane, #issue-pane")`)); !got {
		t.Error("the list or Issue pane is still shown")
	}
	rect := func(sel string) (left, right float64) {
		l.t.Helper()
		left = Get(l, chromedp.Evaluate[float64](fmt.Sprintf(`document.querySelector(%q).getBoundingClientRect().left`, sel)))
		right = Get(l, chromedp.Evaluate[float64](fmt.Sprintf(`document.querySelector(%q).getBoundingClientRect().right`, sel)))
		return
	}
	_, navRight := rect("#nav")
	glossaryLeft, glossaryRight := rect("#glossary-pane")
	if glossaryLeft != navRight {
		t.Errorf("the Glossary starts at %v, the navigation ends at %v", glossaryLeft, navRight)
	}
	if glossaryRight != 1280 {
		t.Errorf("the Glossary ends at %v, want the viewport's 1280", glossaryRight)
	}

	// It scrolls on its own, leaving the navigation and the page where they were.
	if scrollTo(l, "#glossary-pane", scrollOffset) == 0 {
		t.Fatal("#glossary-pane doesn't scroll")
	}
	if got := scrollPosition(l, "#nav"); got != 0 {
		t.Errorf("scrolling the Glossary moved #nav to %v", got)
	}
	if got := Get(l, chromedp.Evaluate[float64](`document.scrollingElement.scrollTop`)); got != 0 {
		t.Errorf("scrolling the Glossary moved the page to %v", got)
	}

	// A live update refreshes the Glossary and the navigation without
	// moving where the Glossary is scrolled.
	l.Call("create_issue", map[string]any{"title": "Arrives mid-scroll"})
	pollUntil(l, viewCount(3)+` === "2"`)
	if got := scrollPosition(l, "#glossary-pane"); got != scrollOffset {
		t.Errorf("#glossary-pane after a live update: scrolled to %v, want %d", got, scrollOffset)
	}

	// The navigation still gets back to the Issues.
	Get(l, chromedp.Evaluate[any](`document.querySelector("#nav .views .nav-link:nth-child(3)").click()`))
	pollUntil(l, `document.querySelector("#list-pane") && !document.querySelector("#glossary-pane")`)
}
