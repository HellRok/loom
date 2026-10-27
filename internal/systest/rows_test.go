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
		pollUntil(l, fmt.Sprintf(`[...document.querySelectorAll(".row.is-selected")].map(r => r.id).join() === %q`, want))
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

// seedFullRow creates #1 and its child #2, which has every meta line in its
// list row: Labels, a parent, a holder, a waiting line (its own child, #3)
// and the timestamp. #2 sits between a row above and below, so the dividers
// on both sides can be told apart.
func seedFullRow(l *Loom) {
	l.t.Helper()
	l.Call("create_label", map[string]any{"name": "bug"})
	l.Call("create_issue", map[string]any{"title": "Grandparent"})
	l.Call("create_issue", map[string]any{"title": "Every meta line", "parent": "#1", "labels": []string{"bug"}})
	l.Call("claim", map[string]any{"ref": "#2"})
	l.Call("create_issue", map[string]any{"title": "Its child", "parent": "#2"})
}

// rowBox is where a list row, its last visible content and its neighbour sit,
// in viewport pixels.
type rowBox struct {
	Top, Bottom, Left, Right  float64
	ContentTop, ContentBottom float64 // the extent of the row's descendants
	Background                string
	BorderBottom              float64 // the row's own bottom border, in pixels
	DescendantBorders         int     // descendants, bar Label chips, with a bottom border of their own
	NextTop                   float64 // the next row's top, or -1 with none
}

func measureRow(l *Loom, id string) rowBox {
	l.t.Helper()
	return Get(l, chromedp.Evaluate[rowBox](fmt.Sprintf(`(() => {
		const li = document.querySelector("#list-pane #%s");
		const r = li.getBoundingClientRect(), cs = getComputedStyle(li);
		const kids = [...li.querySelectorAll("*")].map(e => e.getBoundingClientRect()).filter(k => k.height > 0);
		const next = li.nextElementSibling;
		return {
			Top: r.top, Bottom: r.bottom, Left: r.left, Right: r.right,
			ContentTop: Math.min(...kids.map(k => k.top)), ContentBottom: Math.max(...kids.map(k => k.bottom)),
			Background: cs.backgroundColor,
			BorderBottom: cs.borderBottomStyle === "none" ? 0 : parseFloat(cs.borderBottomWidth),
			DescendantBorders: [...li.querySelectorAll("*:not(.label-chip)")].filter(e => {
				const s = getComputedStyle(e);
				return s.borderBottomStyle !== "none" && parseFloat(s.borderBottomWidth) > 0;
			}).length,
			NextTop: next ? next.getBoundingClientRect().top : -1,
		};
	})()`, id)))
}

func TestSelectedRowHighlightCoversTheWholeRow(t *testing.T) {
	l := Start(t, "demo")
	seedFullRow(l)
	l.Do(chromedp.EmulateViewport(1280, 720))
	l.Do(chromedp.Navigate(l.URL("/demo/issues/2?view=open")))
	pollUntil(l, `document.querySelector("#list-pane #row-2.is-selected")`)

	selected, other := measureRow(l, "row-2"), measureRow(l, "row-1")
	const transparent = "rgba(0, 0, 0, 0)"
	if selected.Background == transparent {
		t.Error("the selected row has no background: the highlight is not on the row")
	}
	if other.Background != transparent {
		t.Errorf("an unselected row's background = %s, want transparent", other.Background)
	}
	// Every line, down to the timestamp, sits inside the highlighted box.
	if selected.ContentTop < selected.Top || selected.ContentBottom > selected.Bottom {
		t.Errorf("the row's content spans %v-%v, outside its box %v-%v",
			selected.ContentTop, selected.ContentBottom, selected.Top, selected.Bottom)
	}
	if got := Get(l, chromedp.Evaluate[float64](`document.querySelectorAll("#row-2 > *").length`)); got < 5 {
		t.Errorf("the row has %v lines, want a title and every meta line", got)
	}
}

func TestRowHasASingleDivider(t *testing.T) {
	l := Start(t, "demo")
	seedFullRow(l)
	l.Do(chromedp.EmulateViewport(1280, 720))
	l.Do(chromedp.Navigate(l.URL("/demo/issues/2?view=open")))
	pollUntil(l, `document.querySelector("#list-pane #row-2.is-selected")`)

	for _, id := range []string{"row-1", "row-2"} {
		row := measureRow(l, id)
		if row.BorderBottom != 1 {
			t.Errorf("%s: bottom border = %vpx, want 1", id, row.BorderBottom)
		}
		if row.DescendantBorders != 0 {
			t.Errorf("%s: %d descendants draw a bottom border of their own", id, row.DescendantBorders)
		}
		// The divider is the last thing in the row: the content ends above it.
		if row.ContentBottom > row.Bottom-row.BorderBottom {
			t.Errorf("%s: content ends at %v, below the divider at %v", id, row.ContentBottom, row.Bottom-row.BorderBottom)
		}
	}
	// No second divider or gap between neighbours.
	if first, second := measureRow(l, "row-1"), measureRow(l, "row-2"); first.NextTop != second.Top {
		t.Errorf("rows are %vpx apart, want them to touch", second.Top-first.Bottom)
	}
}

func TestClickingARowsParentLinkOpensTheParent(t *testing.T) {
	l := Start(t, "demo")
	seedFullRow(l)
	l.Do(chromedp.EmulateViewport(1280, 720))
	l.Do(chromedp.Navigate(l.URL("/demo/issues/3?view=open")))
	pollUntil(l, `document.querySelector("#list-pane #row-3.is-selected")`)

	// #2's row is another row's neighbour, so the click can only hit the link.
	l.Do(chromedp.Click(chromedp.CSS("#row-2 .row-parent a")))
	pollUntil(l, `location.pathname === "/demo/issues/1"`)
	pollUntil(l, `document.querySelector("#list-pane #row-1.is-selected")`)
}

func TestClickingTheTitleLineOrBlankSpaceOpensTheRowsIssue(t *testing.T) {
	l := Start(t, "demo")
	seedFullRow(l)
	l.Do(chromedp.EmulateViewport(1280, 720))

	open := func(name string, click func()) {
		t.Helper()
		l.Do(chromedp.Navigate(l.URL("/demo/issues/1?view=open")))
		pollUntil(l, `document.querySelector("#list-pane #row-1.is-selected")`)
		click()
		pollUntil(l, `location.pathname === "/demo/issues/2"`)
		pollUntil(l, `document.querySelector("#list-pane #row-2.is-selected")`)
		t.Logf("%s opened #2", name)
	}
	open("the title", func() {
		l.Do(chromedp.Click(chromedp.CSS("#row-2 .row-link .issue-title")))
	})
	open("blank space", func() {
		// Beside the title, past the end of every line.
		row := measureRow(l, "row-2")
		l.Do(chromedp.MouseClickXY(row.Right-10, (row.Top+row.Bottom)/2))
	})
}
