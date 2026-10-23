//go:build system

package systest

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// pollUntil waits up to 5 seconds for the JavaScript condition to be truthy,
// failing the test if it never is.
func pollUntil(l *Loom, cond string) {
	l.t.Helper()
	ctx, cancel := context.WithTimeout(l.Browser, 10*time.Second)
	defer cancel()
	_, err := chromedp.Run(ctx, chromedp.Poll[string](`(`+cond+`) && "ok"`, chromedp.WithPollingTimeout(5*time.Second)))
	if err != nil {
		l.t.Fatalf("never true: %s: %v", cond, err)
	}
}

// waitForHtmxIdle waits for htmx to settle its swaps and bind the new
// elements.
func waitForHtmxIdle(l *Loom) {
	l.t.Helper()
	pollUntil(l, `!document.querySelector(".htmx-request, .htmx-swapping, .htmx-settling")`)
}

// seedLabels creates n Labels, enough to make the sidebar scroll.
func seedLabels(l *Loom, n int) {
	l.t.Helper()
	for i := range n {
		l.Call("create_label", map[string]any{"name": fmt.Sprintf("label-%02d", i)})
	}
}
