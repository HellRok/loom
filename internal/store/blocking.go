package store

import "context"

// Blocking lists every Issue that Issue n blocks, open or closed, by number.
func (s *Store) Blocking(ctx context.Context, slug string, n int64) ([]Issue, error) {
	return s.related(ctx, "blocked issues", slug, n, `
		SELECT `+issueCols+` FROM issue_blockers ib
		JOIN issues i ON i.id = ib.issue_id
		JOIN issues blocker ON blocker.id = ib.blocker_id JOIN projects p ON p.id = blocker.project_id
		WHERE p.slug = ? AND blocker.number = ? ORDER BY i.number`)
}
