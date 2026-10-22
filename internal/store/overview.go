package store

import (
	"context"
	"fmt"
	"time"
)

// ProjectSummary is a Project with what the home page shows about it.
type ProjectSummary struct {
	Project
	Open int
	// LastActivity is when any of its Issues last changed; zero if it has
	// none.
	LastActivity time.Time
}

// ProjectSummaries lists every Project with its open Issue count and last
// activity, ordered by Slug.
func (s *Store) ProjectSummaries(ctx context.Context) ([]ProjectSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.slug, p.root,
		       count(i.id) FILTER (WHERE i.state = 'open'),
		       max(i.updated_at)
		FROM projects p LEFT JOIN issues i ON i.project_id = p.id
		GROUP BY p.id ORDER BY p.slug`)
	if err != nil {
		return nil, fmt.Errorf("summarising projects: %w", err)
	}
	defer rows.Close()
	ps := []ProjectSummary{}
	for rows.Next() {
		var p ProjectSummary
		var last *string
		if err := rows.Scan(&p.Slug, &p.Root, &p.Open, &last); err != nil {
			return nil, fmt.Errorf("summarising projects: %w", err)
		}
		if last != nil {
			if p.LastActivity, err = parseTime(*last); err != nil {
				return nil, fmt.Errorf("summarising projects: %w", err)
			}
		}
		ps = append(ps, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("summarising projects: %w", err)
	}
	return ps, nil
}

// LabelsByIssue maps each Issue number in the Project slug to the names of
// its Labels, sorted. Issues without Labels are absent.
func (s *Store) LabelsByIssue(ctx context.Context, slug string) (map[int64][]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.number, l.name FROM issue_labels il
		JOIN labels l ON l.id = il.label_id
		JOIN issues i ON i.id = il.issue_id
		JOIN projects p ON p.id = i.project_id
		WHERE p.slug = ? ORDER BY i.number, l.name`, slug)
	if err != nil {
		return nil, fmt.Errorf("reading labels in %s: %w", slug, err)
	}
	defer rows.Close()
	m := map[int64][]string{}
	for rows.Next() {
		var n int64
		var name string
		if err := rows.Scan(&n, &name); err != nil {
			return nil, fmt.Errorf("reading labels in %s: %w", slug, err)
		}
		m[n] = append(m[n], name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading labels in %s: %w", slug, err)
	}
	return m, nil
}
