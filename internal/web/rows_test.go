package web_test

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HellRok/loom/internal/store"
	"github.com/HellRok/loom/internal/web"
	"github.com/PuerkitoBio/goquery"
)

// seedStackedRow has #2 carry every fact a list row can show: Labels, a
// parent, an open child and a holding Session. #3 carries none of them.
func seedStackedRow(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateProject(ctx, "loom", "/code/loom"); err != nil {
		t.Fatal(err)
	}
	mk := func(in store.NewIssue) {
		if _, err := st.CreateIssue(ctx, "loom", actor, in); err != nil {
			t.Fatal(err)
		}
	}
	mk(store.NewIssue{Kind: store.KindIssue, Title: "Top"})
	mk(store.NewIssue{Kind: store.KindIssue, Title: "Beep the boop", Parent: 1, Labels: []string{"ready-for-agent", "needs-triage"}})
	if err := st.TouchSession(ctx, sessA.Session); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ClaimIssue(ctx, "loom", 2, sessA); err != nil {
		t.Fatal(err)
	}
	mk(store.NewIssue{Kind: store.KindIssue, Title: "Sub", Parent: 2})
	mk(store.NewIssue{Kind: store.KindIssue, Title: "Plain"})
	mux := http.NewServeMux()
	web.Register(mux, web.Deps{Store: st, Graph: web.StoreGraph(st)})
	return &fixture{t: t, st: st, h: mux}
}

// lines is the first class of each direct child of the row, top to bottom.
func lines(doc *goquery.Document, sel string) string {
	var out []string
	doc.Find(sel).Children().Each(func(_ int, s *goquery.Selection) {
		out = append(out, strings.Fields(s.AttrOr("class", ""))[0])
	})
	return strings.Join(out, " ")
}

func TestListRowStacksMetaLines(t *testing.T) {
	f := seedStackedRow(t)
	_, doc := f.get("/loom?view=open", "")

	t.Run("lines run title, labels, parent, waiting, held by, timestamp", func(t *testing.T) {
		want := "row-title row-labels row-parent row-waiting row-held-by updated"
		if got := lines(doc, "#row-2"); got != want {
			t.Errorf("#2 lines = %q, want %q", got, want)
		}
	})

	t.Run("absent facts leave no line", func(t *testing.T) {
		if got := lines(doc, "#row-4"); got != "row-title updated" {
			t.Errorf("#4 lines = %q", got)
		}
	})

	t.Run("title line", func(t *testing.T) {
		if got := text(doc, "#row-2 .row-title"); got != "⊘ #2 Beep the boop" {
			t.Errorf("title line = %q", got)
		}
	})

	t.Run("labels are pills on their own line", func(t *testing.T) {
		expect(t, doc, "#row-2 .row-labels .label", 2)
		if got := text(doc, "#row-2 .row-labels"); got != "needs-triage ready-for-agent" {
			t.Errorf("labels line = %q", got)
		}
	})

	t.Run("parent line links to the parent", func(t *testing.T) {
		if got := text(doc, "#row-2 .row-parent"); got != "parent: #1" {
			t.Errorf("parent line = %q", got)
		}
		link := doc.Find("#row-2 .row-parent a")
		if got := link.AttrOr("href", ""); got != "/loom/issues/1?view=open" {
			t.Errorf("parent href = %q", got)
		}
		if got := link.AttrOr("title", ""); got != "Top" {
			t.Errorf("parent tooltip = %q", got)
		}
	})

	t.Run("waiting line", func(t *testing.T) {
		if got := text(doc, "#row-2 .row-waiting"); got != "waiting on 1 open child" {
			t.Errorf("waiting line = %q", got)
		}
	})

	t.Run("held-by line keeps the session link and idle text", func(t *testing.T) {
		if got := text(doc, "#row-2 .row-held-by"); got != "held by: 11112222 idle <1m" {
			t.Errorf("held-by line = %q", got)
		}
		if got := doc.Find("#row-2 .row-held-by a.session-link").AttrOr("href", ""); got != "/sessions/"+sessA.Session {
			t.Errorf("session href = %q", got)
		}
	})
}

func TestListRowOpensIssueFromAnywhere(t *testing.T) {
	f := seedStackedRow(t)
	_, doc := f.get("/loom?view=open", "")

	// The title is the row's one link to the Issue; its box is stretched over
	// the whole row in CSS, so no link nests inside another.
	links := doc.Find("#row-2 a.row-link")
	if links.Length() != 1 || links.AttrOr("href", "") != "/loom/issues/2?view=open" {
		t.Fatalf("row link = %q", links.AttrOr("href", ""))
	}
	if links.ParentsFiltered(".row-title").Length() == 0 {
		t.Errorf("the row link should sit in the title line")
	}
	expect(t, doc, "#row-2 a a", 0)
	// The selection highlight is on the row, not just the title link.
	_, doc = f.get("/loom/issues/2?view=open", "")
	expect(t, doc, "#list-pane #row-2.selected", 1)
}
