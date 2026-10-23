package store_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/HellRok/loom/internal/store"
)

var changeActor = store.Actor{Name: "Tess Ter", Session: "test-session-0001"}

// TestChangesReportedAfterCommit checks which Issues each kind of write
// reports changed: the Issue first, then its parents.
func TestChangesReportedAfterCommit(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateProject(ctx, "app", "/code/app"); err != nil {
		t.Fatal(err)
	}
	mk := func(parent int64) int64 {
		i, err := st.CreateIssue(ctx, "app", changeActor, store.NewIssue{Kind: store.KindIssue, Title: "t", Parent: parent})
		if err != nil {
			t.Fatal(err)
		}
		return i.Number
	}
	a, b := mk(0), mk(0)
	child := mk(a)

	var got []store.Change
	st.OnChange(func(c store.Change) { got = append(got, c) })
	check := func(what string, want ...store.Change) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: changes = %v, want %v", what, got, want)
		}
		got = nil
	}

	mk(a)
	check("create under a parent", store.Change{Slug: "app", Issues: []int64{4, a}})

	if _, err := st.AddComment(ctx, "app", child, "hi", changeActor); err != nil {
		t.Fatal(err)
	}
	check("comment", store.Change{Slug: "app", Issues: []int64{child, a}})

	if _, err := st.SetParent(ctx, "app", child, b, changeActor); err != nil {
		t.Fatal(err)
	}
	check("reparent reports both parents", store.Change{Slug: "app", Issues: []int64{child, b, a}})

	if _, err := st.AddBlockers(ctx, "app", a, []int64{b}, changeActor); err != nil {
		t.Fatal(err)
	}
	check("blocker", store.Change{Slug: "app", Issues: []int64{a, b}})

	if _, err := st.AddComment(ctx, "app", 99, "hi", changeActor); err == nil {
		t.Fatal("commenting on a missing Issue should fail")
	}
	check("a failed write reports nothing")

	if err := st.DeleteIssue(ctx, "app", b, changeActor); err != nil {
		t.Fatal(err)
	}
	check("delete reports its children", store.Change{Slug: "app", Issues: []int64{b, child}})

	if _, _, err := st.CreateLabel(ctx, "app", "mine", ""); err != nil {
		t.Fatal(err)
	}
	check("label", store.Change{Slug: "app"})
	if _, already, err := st.CreateLabel(ctx, "app", "mine", ""); err != nil || !already {
		t.Fatal(err)
	}
	check("an existing label changes nothing")
}

// TestTouchSessionReportsOnlyAFirstSighting checks that a newly seen Session
// is reported as a Change in no Project, and later touches are not, so tool
// calls don't make every open page refresh.
func TestTouchSessionReportsOnlyAFirstSighting(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var got []store.Change
	st.OnChange(func(c store.Change) { got = append(got, c) })

	if err := st.TouchSession(ctx, "sess-1"); err != nil {
		t.Fatal(err)
	}
	if want := []store.Change{{}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first touch: changes = %v, want %v", got, want)
	}
	got = nil

	if err := st.TouchSession(ctx, "sess-1"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("repeat touch: changes = %v, want none", got)
	}

	if err := st.TouchSession(ctx, "sess-2"); err != nil {
		t.Fatal(err)
	}
	if want := []store.Change{{}}; !reflect.DeepEqual(got, want) {
		t.Errorf("another Session: changes = %v, want %v", got, want)
	}
}
