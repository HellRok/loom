package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// standalone marks Project slug Standalone, the only kind the Glossary page
// is shown for.
func standalone(t *testing.T, f *fixture, slug string) {
	t.Helper()
	if _, err := f.st.SetStandalone(context.Background(), slug, true); err != nil {
		t.Fatal(err)
	}
}

func TestGlossaryPageRendersTheStoredGlossaryAsMarkdown(t *testing.T) {
	f := seed(t)
	standalone(t, f, "loom")
	if _, err := f.st.EditGlossary(context.Background(), "loom", 0,
		"# Loom\n\n**Widget**: a *thing*. See #2.\n\n<script>alert(1)</script>\n"); err != nil {
		t.Fatal(err)
	}

	rec, doc := f.get("/loom/glossary", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := text(doc, "title"); got != "Glossary · loom" {
		t.Errorf("title = %q", got)
	}
	body := doc.Find("main .glossary .body")
	if got := text(doc, "main .glossary .body h1"); got != "Loom" {
		t.Errorf("heading = %q", got)
	}
	if got := body.Find("strong").Text(); got != "Widget" {
		t.Errorf("bold = %q", got)
	}
	if got := body.Find("em").Text(); got != "thing" {
		t.Errorf("emphasis = %q", got)
	}
	if got := body.Find("a").AttrOr("href", ""); got != "/loom/issues/2" {
		t.Errorf("#2 links to %q", got)
	}
	expect(t, doc, "main .glossary script", 0)
	if !strings.Contains(rec.Body.String(), "&lt;script&gt;") {
		t.Errorf("raw HTML was not shown escaped")
	}
}

func TestGlossaryPageShowsAnEmptyStateWhenThereIsNoGlossary(t *testing.T) {
	f := seed(t)
	standalone(t, f, "loom")
	ctx := context.Background()

	check := func(t *testing.T) {
		t.Helper()
		rec, doc := f.get("/loom/glossary", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if got := text(doc, "main .glossary .empty"); got != "No Glossary yet." {
			t.Errorf("empty state = %q", got)
		}
		expect(t, doc, "main .glossary .body", 0)
	}
	t.Run("never written", check)

	// A Glossary blanked out by an agent is as empty as one never written.
	g, err := f.st.EditGlossary(ctx, "loom", 0, "something")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.EditGlossary(ctx, "loom", g.Version, " \n"); err != nil {
		t.Fatal(err)
	}
	t.Run("blanked", check)
}

func TestGlossaryPageIsReadOnly(t *testing.T) {
	f := seed(t)
	standalone(t, f, "loom")
	if _, err := f.st.EditGlossary(context.Background(), "loom", 0, "# Loom"); err != nil {
		t.Fatal(err)
	}
	_, doc := f.get("/loom/glossary", "")
	expect(t, doc, "main form, main textarea, main input, main button, main [contenteditable]", 0)
}

func TestGlossaryPageBelongsToItsProject(t *testing.T) {
	f := seed(t)
	standalone(t, f, "loom")
	standalone(t, f, "other")
	ctx := context.Background()
	if _, err := f.st.EditGlossary(ctx, "loom", 0, "loom terms"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.EditGlossary(ctx, "other", 0, "other terms"); err != nil {
		t.Fatal(err)
	}

	_, doc := f.get("/other/glossary", "")
	if got := text(doc, "main .glossary .body"); got != "other terms" {
		t.Errorf("/other/glossary shows %q", got)
	}
	if rec, _ := f.get("/ghost/glossary", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown Project: status %d, want 404", rec.Code)
	}
	if rec, _ := f.get("/loom/glossary/extra", ""); rec.Code != http.StatusNotFound {
		t.Errorf("deeper path: status %d, want 404", rec.Code)
	}
}

func TestGlossaryPageIsReachableFromTheProjectPages(t *testing.T) {
	f := seed(t)
	standalone(t, f, "loom")
	_, doc := f.get("/loom", "")
	link := doc.Find("#nav a[href='/loom/glossary']")
	if link.Length() != 1 || link.Text() != "Glossary" {
		t.Fatalf("project nav lacks a Glossary link: %d matches, %q", link.Length(), link.Text())
	}
	if got := link.AttrOr("hx-target", ""); got != "#main" {
		t.Errorf("link swaps %q, want #main", got)
	}

	// Followed with htmx, the page comes back as a fragment.
	rec, frag := f.get("/loom/glossary", "main")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "<html") || text(frag, ".glossary h1") != "Glossary" {
		t.Errorf("fragment: status %d, body %q", rec.Code, rec.Body.String())
	}
}

func TestGlossaryPageIsNotFoundForAPlainProject(t *testing.T) {
	f := seed(t)
	// A Glossary kept from when the Project was Standalone stays stored but
	// is not shown.
	if _, err := f.st.EditGlossary(context.Background(), "loom", 0, "# Kept"); err != nil {
		t.Fatal(err)
	}
	rec, doc := f.get("/loom/glossary", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	expect(t, doc, "main .glossary", 0)
	if strings.Contains(rec.Body.String(), "Kept") {
		t.Errorf("the stored Glossary leaked into the 404 page")
	}
}

func TestGlossaryLinkIsMissingFromAPlainProjectsPages(t *testing.T) {
	f := seed(t)
	for _, path := range []string{"/loom", "/loom/issues/2", "/loom/issues/2?tab=comments"} {
		_, doc := f.get(path, "")
		expect(t, doc, "#nav a[href='/loom/glossary']", 0)
	}
}

func TestGlossaryPageFollowsTheStandaloneFlagWithoutARestart(t *testing.T) {
	f := seed(t)
	ctx := context.Background()
	for _, path := range []string{"/loom", "/loom/issues/2"} {
		_, doc := f.get(path, "")
		expect(t, doc, "#nav a[href='/loom/glossary']", 0)
	}

	standalone(t, f, "loom")
	if rec, _ := f.get("/loom/glossary", ""); rec.Code != http.StatusOK {
		t.Errorf("Standalone: status %d, want 200", rec.Code)
	}
	for _, path := range []string{"/loom", "/loom/issues/2"} {
		_, doc := f.get(path, "")
		expect(t, doc, "#nav a[href='/loom/glossary']", 1)
	}

	if _, err := f.st.SetStandalone(ctx, "loom", false); err != nil {
		t.Fatal(err)
	}
	if rec, _ := f.get("/loom/glossary", ""); rec.Code != http.StatusNotFound {
		t.Errorf("plain again: status %d, want 404", rec.Code)
	}
	_, doc := f.get("/loom", "")
	expect(t, doc, "#nav a[href='/loom/glossary']", 0)
}

func TestGlossaryPageKeepsTheNavigationBesideIt(t *testing.T) {
	f := seed(t)
	standalone(t, f, "loom")
	if _, err := f.st.EditGlossary(context.Background(), "loom", 0, "# Loom"); err != nil {
		t.Fatal(err)
	}

	_, doc := f.get("/loom/glossary", "")
	// The Glossary takes the place of the list and Issue panes.
	expect(t, doc, ".panes > #nav", 1)
	expect(t, doc, ".panes > #glossary-pane", 1)
	expect(t, doc, "#list-pane, #issue-pane", 0)
	_, project := f.get("/loom", "")
	if got, want := doc.Find("#nav .views li").Length(), project.Find("#nav .views li").Length(); got == 0 || got != want {
		t.Errorf("Views = %d, want the Project page's %d", got, want)
	}
	if got, want := doc.Find("#nav .labels").Text(), project.Find("#nav .labels").Text(); got == "" || got != want {
		t.Errorf("Labels = %q, want the Project page's %q", got, want)
	}

	// The Glossary is the current page, not a View.
	expect(t, doc, "#nav .views li.active", 0)
	expect(t, doc, "#nav .reference li.active a[href='/loom/glossary']", 1)
	expect(t, doc, "#nav .switcher li.active", 1)

	// The navigation refreshes from the Glossary page's address.
	rec, frag := f.get("/loom/glossary", "nav")
	if rec.Code != http.StatusOK {
		t.Fatalf("nav fragment status %d", rec.Code)
	}
	expect(t, frag, "nav#nav", 1)
	expect(t, frag, "#glossary-pane", 0)
	expect(t, frag, "#nav .reference li.active", 1)
	expect(t, frag, "#nav .views li.active", 0)

	// A Project page marks a View, not the Glossary.
	_, doc = f.get("/loom", "")
	expect(t, doc, "#nav .views li.active", 1)
	expect(t, doc, "#nav .reference li.active", 0)
}
