package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HellRok/loom/internal/store"
)

// CreateIssue's documented errors reach the caller as they are: not wrapped
// with the "creating issue in" context that marks an unexpected failure.
func TestCreateIssuePassesOnDocumentedErrorsUnwrapped(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateProject(ctx, "app", "/code/app"); err != nil {
		t.Fatal(err)
	}
	actor := store.Actor{Name: "Tess Ter", Session: "test-session-0001"}
	if _, err := st.CreateIssue(ctx, "app", actor, store.NewIssue{Kind: store.KindIssue, Title: "existing"}); err != nil {
		t.Fatal(err)
	}

	// Each case names the concrete error CreateIssue must return itself: a
	// type assertion, not errors.As, so an error wrapped on the way out fails.
	for _, tc := range []struct {
		name string
		in   store.NewIssue
		is   func(error) bool
	}{
		{"undefined label", store.NewIssue{Kind: store.KindIssue, Title: "t", Labels: []string{"nope"}},
			func(err error) bool { _, ok := err.(*store.UndefinedLabelError); return ok }},
		{"missing parent", store.NewIssue{Kind: store.KindIssue, Title: "t", Parent: 99},
			func(err error) bool { _, ok := err.(*store.MissingIssueError); return ok }},
		{"map with parent", store.NewIssue{Kind: store.KindMap, Title: "t", Parent: 1},
			func(err error) bool { _, ok := err.(*store.InvalidTicketError); return ok }},
		{"self block", store.NewIssue{Kind: store.KindIssue, Title: "t", BlockedBy: []int64{2}}, // the new Issue is #2
			func(err error) bool { _, ok := err.(*store.SelfBlockError); return ok }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := st.CreateIssue(ctx, "app", actor, tc.in)
			if !tc.is(err) {
				t.Fatalf("got %T (%v), want the documented error itself", err, err)
			}
			if !store.Documented(err) {
				t.Errorf("%T is not documented", err)
			}
		})
	}
}
