//go:build system

package systest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// scrollOffset is how far down each pane is scrolled in the tests below.
const scrollOffset = 300

// scrollPosition returns how far the element is scrolled.
func scrollPosition(l *Loom, sel string) float64 {
	l.t.Helper()
	return Get(l, chromedp.Evaluate[float64](fmt.Sprintf(`document.querySelector(%q).scrollTop`, sel)))
}

// scrollTo scrolls the element to y and returns where it ended up, which is
// 0 if it has nothing to scroll.
func scrollTo(l *Loom, sel string, y int) float64 {
	l.t.Helper()
	return Get(l, chromedp.Evaluate[float64](fmt.Sprintf(
		`(() => { const el = document.querySelector(%q); el.scrollTop = %d; return el.scrollTop })()`, sel, y)))
}

// scrollToBottom scrolls the element as far down as it goes and returns where
// that is.
func scrollToBottom(l *Loom, sel string) float64 {
	l.t.Helper()
	return Get(l, chromedp.Evaluate[float64](fmt.Sprintf(
		`(() => { const el = document.querySelector(%q); el.scrollTop = el.scrollHeight; return el.scrollTop })()`, sel)))
}

func TestPanesScrollIndependently(t *testing.T) {
	const issues = 60
	l := Start(t, "demo")
	seedLabels(l, 40)
	l.Call("create_issue", map[string]any{
		"title": "Long Issue",
		"body":  strings.Repeat("A line of the body.\n\n", 120),
	})
	for i := 2; i <= issues; i++ {
		l.Call("create_issue", map[string]any{"title": fmt.Sprintf("Issue %d", i)})
	}

	l.Do(chromedp.EmulateViewport(1280, 720))
	l.Do(chromedp.Navigate(l.URL("/demo/issues/1?view=open")))

	// At desktop widths the panes fill the viewport and the page itself
	// doesn't scroll.
	overflow := Get(l, chromedp.Evaluate[float64](
		`document.documentElement.scrollHeight - document.documentElement.clientHeight`))
	if overflow > 0 {
		t.Fatalf("the page overflows the viewport by %vpx", overflow)
	}

	panes := []string{"#nav", "#list-pane", "#issue-pane"}
	for _, sel := range panes {
		bottom := Get(l, chromedp.Evaluate[float64](
			fmt.Sprintf(`document.querySelector(%q).getBoundingClientRect().bottom - window.innerHeight`, sel)))
		if bottom != 0 {
			t.Fatalf("%s ends %vpx from the bottom of the viewport, want 0", sel, -bottom)
		}
		if scrollTo(l, sel, scrollOffset) == 0 {
			t.Fatalf("%s doesn't scroll", sel)
		}
		scrollTo(l, sel, 0)
	}

	// Scrolling one pane leaves the others, and the page, where they were.
	for _, scrolled := range panes {
		scrollTo(l, scrolled, scrollOffset)
		for _, other := range panes {
			if other != scrolled {
				if got := scrollPosition(l, other); got != 0 {
					t.Errorf("scrolling %s moved %s to %v", scrolled, other, got)
				}
			}
		}
		if got := Get(l, chromedp.Evaluate[float64](`document.scrollingElement.scrollTop`)); got != 0 {
			t.Errorf("scrolling %s moved the page to %v", scrolled, got)
		}
		scrollTo(l, scrolled, 0)
	}

	// Live updates morph the panes without resetting where they're scrolled.
	for _, sel := range panes {
		scrollTo(l, sel, scrollOffset)
	}
	l.Call("create_issue", map[string]any{"title": "Arrives mid-scroll"})
	l.Call("comment", map[string]any{"ref": "#1", "body": "Arrives mid-scroll"})
	for _, landed := range []string{
		fmt.Sprintf(`document.querySelector("#row-%d")`, issues+1),
		`document.querySelector("#issue-pane .comment")`,
		// The Open view's count in the sidebar, so the nav's morph has landed too.
		fmt.Sprintf(`document.querySelector("#nav .views .nav-link.is-active .count")?.textContent === "%d"`, issues+1),
	} {
		pollUntil(l, landed)
	}
	for _, sel := range panes {
		if got := scrollPosition(l, sel); got != scrollOffset {
			t.Errorf("%s after a live update: scrolled to %v, want %d", sel, got, scrollOffset)
		}
	}
}

func TestNavigationKeepsItsScrollWhenFollowingLinks(t *testing.T) {
	l := Start(t, "demo")
	seedLabels(l, 40)
	l.Call("create_issue", map[string]any{"title": "An Issue"})

	l.Do(chromedp.EmulateViewport(1280, 720))
	l.Do(chromedp.Navigate(l.URL("/demo?view=open")))
	scrolled := scrollToBottom(l, "#nav")
	if scrolled == 0 {
		t.Fatal("#nav doesn't scroll")
	}

	// A label link far down the sidebar.
	Get(l, chromedp.Evaluate[any](`document.querySelector("#nav .labels .nav-link:last-child").click()`))
	pollUntil(l, `location.search.includes("label=")`)
	if got := scrollPosition(l, "#nav"); got != scrolled {
		t.Errorf("#nav after following a label link: scrolled to %v, want %v", got, scrolled)
	}

	// The filter form, once htmx has settled the swap and bound the new form.
	waitForHtmxIdle(l)
	Get(l, chromedp.Evaluate[any](`(() => {
		const form = document.querySelector("#list-pane form.filter");
		form.q.value = "Issue";
		form.requestSubmit();
	})()`))
	pollUntil(l, `location.search.includes("q=Issue")`)
	// Give a full page load time to replace the pane, then check it didn't.
	time.Sleep(300 * time.Millisecond)
	if got := scrollPosition(l, "#nav"); got != scrolled {
		t.Errorf("#nav after filtering: scrolled to %v, want %v", got, scrolled)
	}
}

func TestSelectingARowKeepsTheListAndNavigationScroll(t *testing.T) {
	l := Start(t, "demo")
	seedLabels(l, 40)
	for i := 1; i <= 60; i++ {
		l.Call("create_issue", map[string]any{"title": fmt.Sprintf("Issue %d", i)})
	}

	l.Do(chromedp.EmulateViewport(1280, 720))
	l.Do(chromedp.Navigate(l.URL("/demo/issues/1?view=open")))
	listScrolled := scrollToBottom(l, "#list-pane")
	navScrolled := scrollToBottom(l, "#nav")
	if listScrolled == 0 || navScrolled == 0 {
		t.Fatalf("the panes don't scroll: list %v, nav %v", listScrolled, navScrolled)
	}

	// A row out of view at the top of the list (it sorts oldest first), followed
	// without scrolling it into view.
	Get(l, chromedp.Evaluate[any](`document.querySelector("#row-5 .row-link").click()`))
	pollUntil(l, `document.querySelector("#row-5.is-selected")`)
	waitForHtmxIdle(l)
	if got := Get(l, chromedp.Evaluate[float64](`document.querySelectorAll("#list-pane").length`)); got != 1 {
		t.Errorf("%v list panes after selecting a row, want 1", got)
	}
	if got := scrollPosition(l, "#list-pane"); got != listScrolled {
		t.Errorf("#list-pane after selecting a row: scrolled to %v, want %v", got, listScrolled)
	}
	if got := scrollPosition(l, "#nav"); got != navScrolled {
		t.Errorf("#nav after selecting a row: scrolled to %v, want %v", got, navScrolled)
	}
}
