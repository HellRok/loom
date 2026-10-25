package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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

	for name, in := range map[string]store.NewIssue{
		"undefined label": {Kind: store.KindIssue, Title: "t", Labels: []string{"nope"}},
		"missing parent":  {Kind: store.KindIssue, Title: "t", Parent: 99},
		"map with parent": {Kind: store.KindMap, Title: "t", Parent: 1},
		"self block":      {Kind: store.KindIssue, Title: "t", BlockedBy: []int64{2}}, // the new Issue is #2
	} {
		_, err := st.CreateIssue(ctx, "app", actor, in)
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		if !store.Documented(err) {
			t.Errorf("%s: %T is not a documented error: %v", name, err, err)
		}
		if strings.Contains(err.Error(), "creating issue in") {
			t.Errorf("%s: error was wrapped: %v", name, err)
		}
		if errors.Unwrap(err) != nil {
			t.Errorf("%s: error wraps another: %v", name, err)
		}
	}
}
