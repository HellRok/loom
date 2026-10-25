package tools

import (
	"testing"

	"github.com/HellRok/loom/internal/store"
)

// Every store error issueError turns into a coded tool error must be one the
// store documents, or CreateIssue would wrap it and issueError would miss it.
// Add a new type here when issueError learns to map it.
func TestEveryErrorIssueErrorMapsIsDocumentedByTheStore(t *testing.T) {
	for _, err := range []error{
		&store.UndefinedLabelError{},
		&store.InvalidTicketError{},
		&store.CycleError{},
		&store.SelfBlockError{},
		&store.MissingIssueError{},
		&store.VersionConflictError{},
		&store.NoSectionError{},
	} {
		if !store.Documented(err) {
			t.Errorf("%T is mapped by issueError but not documented by the store", err)
		}
	}
}
