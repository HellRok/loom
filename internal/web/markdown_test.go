package web

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	body := strings.Join([]string{
		"# Title",
		"",
		"Raw <script>alert(1)</script> and <b>bold</b> inline, see #12 and other#3.",
		"",
		"<div onclick=\"x()\">block</div>",
		"",
		"~~gone~~ and a table:",
		"",
		"| a | b |",
		"|---|---|",
		"| 1 | 2 |",
		"",
		"`code #5` stays, [link #6](https://example.com) too, and path/#7 or &#35;8 are not refs.",
		"",
		"[bad](javascript:alert(1))",
	}, "\n")
	got := string(renderMarkdown("loom", body))

	for _, bad := range []string{"<script", "<b>", "<div", "onclick=\"", "href=\"javascript:"} {
		if strings.Contains(got, bad) {
			t.Errorf("output contains %q:\n%s", bad, got)
		}
	}
	for _, want := range []string{
		"&lt;script&gt;", "&lt;b&gt;", "&lt;div onclick",
		`<a href="/loom/issues/12">#12</a>`,
		`<a href="/other/issues/3">other#3</a>`,
		"<del>gone</del>", "<table>",
		"<code>code #5</code>",
		`<a href="https://example.com">link #6</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `/issues/7"`) || strings.Contains(got, `/issues/8"`) {
		t.Errorf("linked something that is not a ref:\n%s", got)
	}
}
