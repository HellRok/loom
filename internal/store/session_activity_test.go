package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HellRok/loom/internal/store"
)

func TestSessionActivityAcrossProjects(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := store.Actor{Name: "Tess Ter", Session: "session-aaaaaaaaaa"}
	b := store.Actor{Name: "Tess Ter", Session: "session-bbbbbbbbbb"}
	for _, slug := range []string{"one", "two"} {
		if _, err := st.CreateProject(ctx, slug, "/code/"+slug); err != nil {
			t.Fatal(err)
		}
		for _, who := range []store.Actor{a, b} {
			if _, err := st.CreateIssue(ctx, slug, who, store.NewIssue{Kind: store.KindIssue, Title: "by " + who.Session}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// a claims one#1 and two#1; b claims one#2 then closes it.
	for _, c := range []struct {
		slug string
		n    int64
		who  store.Actor
	}{{"one", 1, a}, {"two", 1, a}, {"one", 2, b}} {
		if _, _, err := st.ClaimIssue(ctx, c.slug, c.n, c.who); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := st.CloseIssue(ctx, "one", 2, "done", false, b); err != nil {
		t.Fatal(err)
	}

	held, err := st.HeldBy(ctx, a.Session)
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 2 || held[0].Slug != "one" || held[0].Number != 1 || held[1].Slug != "two" || held[1].Number != 1 {
		t.Errorf("held by a = %+v", held)
	}
	if held, _ := st.HeldBy(ctx, b.Session); len(held) != 0 {
		t.Errorf("closed Issue still held: %+v", held)
	}

	evs, err := st.SessionEvents(ctx, a.Session)
	if err != nil {
		t.Fatal(err)
	}
	// created x2, claimed x2, newest first.
	if len(evs) != 4 || evs[0].Type != store.EventClaimed || evs[0].Slug != "two" || evs[3].Type != store.EventCreated || evs[3].Slug != "one" {
		t.Errorf("events of a = %+v", evs)
	}
	for _, e := range evs {
		if e.Event.Session != a.Session {
			t.Errorf("foreign event %+v", e)
		}
	}
}
