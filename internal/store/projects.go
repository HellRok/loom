package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Project is one tracked codebase, anchored at its Root.
type Project struct {
	Slug       string
	Root       string
	Standalone bool
}

// CreateProject registers a Project. Slugs and Roots are unique.
func (s *Store) CreateProject(ctx context.Context, slug, root string) (Project, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, fmt.Errorf("creating project %q: %w", slug, err)
	}
	defer tx.Rollback()

	var slugTaken, rootTaken bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM projects WHERE slug = ?),
		       EXISTS (SELECT 1 FROM projects WHERE root = ?)`, slug, root).Scan(&slugTaken, &rootTaken)
	switch {
	case err != nil:
		return Project{}, fmt.Errorf("creating project %q: %w", slug, err)
	case slugTaken:
		return Project{}, ErrSlugTaken
	case rootTaken:
		return Project{}, ErrRootTaken
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO projects (slug, root, created_at) VALUES (?, ?, ?)`, slug, root, now())
	if err != nil {
		return Project{}, fmt.Errorf("creating project %q: %w", slug, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Project{}, fmt.Errorf("creating project %q: %w", slug, err)
	}
	if err := seedLabels(ctx, tx, id); err != nil {
		return Project{}, fmt.Errorf("creating project %q: %w", slug, err)
	}
	if err := tx.Commit(); err != nil {
		return Project{}, fmt.Errorf("creating project %q: %w", slug, err)
	}
	s.notify(slug)
	return Project{Slug: slug, Root: root}, nil
}

// Projects lists every Project, ordered by Slug.
func (s *Store) Projects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slug, root, standalone FROM projects ORDER BY slug`)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	defer rows.Close()
	ps := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Slug, &p.Root, &p.Standalone); err != nil {
			return nil, fmt.Errorf("listing projects: %w", err)
		}
		ps = append(ps, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	return ps, nil
}

// ProjectBySlug finds one Project, or returns ErrNotFound.
func (s *Store) ProjectBySlug(ctx context.Context, slug string) (Project, error) {
	p := Project{Slug: slug}
	err := s.db.QueryRowContext(ctx, `SELECT root, standalone FROM projects WHERE slug = ?`, slug).Scan(&p.Root, &p.Standalone)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("reading project %q: %w", slug, err)
	}
	return p, nil
}

// SetStandalone marks a Project Standalone or not, reporting already when it
// was set that way. An unknown Slug is ErrNotFound.
func (s *Store) SetStandalone(ctx context.Context, slug string, standalone bool) (already bool, err error) {
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET standalone = ? WHERE slug = ? AND standalone != ?`, standalone, slug, standalone)
	if err != nil {
		return false, fmt.Errorf("updating project %q: %w", slug, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("updating project %q: %w", slug, err)
	}
	if n == 0 {
		if _, err := s.ProjectBySlug(ctx, slug); err != nil {
			return false, err
		}
		return true, nil
	}
	s.notify(slug)
	return false, nil
}

// DeleteProject hard-deletes a Project and, by cascade, everything in it.
func (s *Store) DeleteProject(ctx context.Context, slug string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE slug = ?`, slug)
	if err != nil {
		return fmt.Errorf("deleting project %q: %w", slug, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("deleting project %q: %w", slug, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	s.notify(slug)
	return nil
}

// MoveProject rebinds a Project's Root, leaving its Slug and everything in it
// alone. Another Project's Root is refused with ErrRootTaken; the Project's
// own Root is a no-op.
func (s *Store) MoveProject(ctx context.Context, slug, root string) (Project, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, fmt.Errorf("moving project %q: %w", slug, err)
	}
	defer tx.Rollback()

	var holder string
	err = tx.QueryRowContext(ctx, `SELECT slug FROM projects WHERE root = ?`, root).Scan(&holder)
	switch {
	case err == nil && holder != slug:
		return Project{}, ErrRootTaken
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return Project{}, fmt.Errorf("moving project %q: %w", slug, err)
	}
	res, err := tx.ExecContext(ctx, `UPDATE projects SET root = ? WHERE slug = ?`, root, slug)
	if err != nil {
		return Project{}, fmt.Errorf("moving project %q: %w", slug, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Project{}, fmt.Errorf("moving project %q: %w", slug, err)
	} else if n == 0 {
		return Project{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return Project{}, fmt.Errorf("moving project %q: %w", slug, err)
	}
	s.notify(slug)
	return Project{Slug: slug, Root: root}, nil
}
