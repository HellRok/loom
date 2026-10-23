package store_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/HellRok/loom/internal/store"
)

var filterActor = store.Actor{Name: "Tess Ter", Session: "test-session-0001"}

func numbers(is []store.Issue) []int64 {
	out := []int64{}
	for _, i := range is {
		out = append(out, i.Number)
	}
	return out
}

// filterFixture seeds Project "app": a Map (#1), a ready Issue (#2), an Issue
// blocked by it (#3), a claimed Issue (#4), a closed Issue (#5), a Ticket
// under the Map (#6) and a ready-for-agent Issue (#7).
func filterFixture(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateProject(ctx, "app", "/code/app"); err != nil {
		t.Fatal(err)
	}
	mk := func(ni store.NewIssue) {
		if ni.Kind == "" {
			ni.Kind = store.KindIssue
		}
		ni.Title = "t"
		if _, err := st.CreateIssue(ctx, "app", filterActor, ni); err != nil {
			t.Fatal(err)
		}
	}
	mk(store.NewIssue{Kind: store.KindMap})
	mk(store.NewIssue{})
	mk(store.NewIssue{BlockedBy: []int64{2}})
	mk(store.NewIssue{})
	mk(store.NewIssue{})
	mk(store.NewIssue{Parent: 1, Labels: []string{"wayfinder:task"}})
	mk(store.NewIssue{Labels: []string{"ready-for-agent"}})
	if _, _, err := st.ClaimIssue(ctx, "app", 4, filterActor); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CloseIssue(ctx, "app", 5, "", false, filterActor); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestIssueFilterFrontier(t *testing.T) {
	ctx := context.Background()
	st := filterFixture(t)
	no := false

	frontier := store.IssueFilter{State: store.StateOpen, NotKind: store.KindMap, Blocked: &no, Claimed: &no}
	got, total, err := st.Issues(ctx, "app", frontier)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{2, 6, 7}; !reflect.DeepEqual(numbers(got), want) || total != 3 {
		t.Errorf("project Frontier = %v (total %d), want %v", numbers(got), total, want)
	}

	// Scoped to a parent, the filter agrees with Store.Frontier.
	frontier.Parent = 1
	got, _, err = st.Issues(ctx, "app", frontier)
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.Frontier(ctx, "app", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(numbers(got), numbers(f.Ready)) {
		t.Errorf("filter under #1 = %v, Store.Frontier ready = %v", numbers(got), numbers(f.Ready))
	}
}

func TestIssueFilterNotLabels(t *testing.T) {
	st := filterFixture(t)
	got, _, err := st.Issues(context.Background(), "app", store.IssueFilter{
		State: store.StateOpen, NotKind: store.KindMap, NotLabels: []string{"ready-for-agent", "ready-for-human"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{2, 3, 4, 6}; !reflect.DeepEqual(numbers(got), want) {
		t.Errorf("without the ready labels = %v, want %v", numbers(got), want)
	}
}

func TestIssueFilterNoOpenChildren(t *testing.T) {
	ctx := context.Background()
	st := filterFixture(t)
	if _, err := st.CreateIssue(ctx, "app", filterActor, store.NewIssue{Kind: store.KindIssue, Title: "child", Parent: 2}); err != nil {
		t.Fatal(err)
	}
	f := store.IssueFilter{State: store.StateOpen, NoOpenChildren: true}
	got, _, err := st.Issues(ctx, "app", f)
	if err != nil {
		t.Fatal(err)
	}
	// #2 waits on #8; Map #1 is exempt from its open Ticket #6.
	if want := []int64{1, 3, 4, 6, 7, 8}; !reflect.DeepEqual(numbers(got), want) {
		t.Errorf("open without open children = %v, want %v", numbers(got), want)
	}

	if _, _, err := st.CloseIssue(ctx, "app", 8, "", true, filterActor); err != nil {
		t.Fatal(err)
	}
	got, _, err = st.Issues(ctx, "app", f)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{1, 2, 3, 4, 6, 7}; !reflect.DeepEqual(numbers(got), want) {
		t.Errorf("after the child closes = %v, want %v", numbers(got), want)
	}
}

func TestIssueFilterNoOpenChildrenCountsDescendants(t *testing.T) {
	ctx := context.Background()
	st := filterFixture(t)
	mk := func(parent int64) {
		if _, err := st.CreateIssue(ctx, "app", filterActor, store.NewIssue{Kind: store.KindIssue, Title: "t", Parent: parent}); err != nil {
			t.Fatal(err)
		}
	}
	mk(2) // #8
	mk(8) // #9
	if _, _, err := st.CloseIssue(ctx, "app", 8, "", false, filterActor); err != nil {
		t.Fatal(err)
	}
	got, _, err := st.Issues(ctx, "app", store.IssueFilter{State: store.StateOpen, NoOpenChildren: true})
	if err != nil {
		t.Fatal(err)
	}
	// #2's only child is closed, but its grandchild #9 is open.
	if want := []int64{1, 3, 4, 6, 7, 9}; !reflect.DeepEqual(numbers(got), want) {
		t.Errorf("open without open descendants = %v, want %v", numbers(got), want)
	}
}
