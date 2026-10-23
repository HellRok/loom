package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Glossary is a Project's domain glossary. Version 0 with an empty Body
// means none has been written yet.
type Glossary struct {
	Body    string
	Version int64
}

// GlossaryConflictError reports an edit based on a stale Glossary version.
type GlossaryConflictError struct {
	Current Glossary
}

func (e *GlossaryConflictError) Error() string {
	return fmt.Sprintf("glossary is at version %d", e.Current.Version)
}

// Glossary reads Project slug's Glossary, or returns ErrNotFound if there is
// no such Project.
func (s *Store) Glossary(ctx context.Context, slug string) (Glossary, error) {
	var g Glossary
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(g.body, ''), COALESCE(g.version, 0)
		FROM projects p LEFT JOIN glossaries g ON g.project_id = p.id
		WHERE p.slug = ?`, slug).Scan(&g.Body, &g.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Glossary{}, ErrNotFound
	}
	if err != nil {
		return Glossary{}, fmt.Errorf("reading glossary of %q: %w", slug, err)
	}
	return g, nil
}

// EditGlossary replaces Project slug's whole Glossary with body and returns
// it at its new version, provided version is the current one (0 when none
// has been written); otherwise it changes nothing and returns a
// *GlossaryConflictError carrying the current Glossary. An unknown Project is
// ErrNotFound.
func (s *Store) EditGlossary(ctx context.Context, slug string, version int64, body string) (Glossary, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Glossary{}, fmt.Errorf("editing glossary of %q: %w", slug, err)
	}
	defer tx.Rollback()

	var id int64
	var cur Glossary
	err = tx.QueryRowContext(ctx, `
		SELECT p.id, COALESCE(g.body, ''), COALESCE(g.version, 0)
		FROM projects p LEFT JOIN glossaries g ON g.project_id = p.id
		WHERE p.slug = ?`, slug).Scan(&id, &cur.Body, &cur.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Glossary{}, ErrNotFound
	}
	if err != nil {
		return Glossary{}, fmt.Errorf("editing glossary of %q: %w", slug, err)
	}
	if version != cur.Version {
		return Glossary{}, &GlossaryConflictError{Current: cur}
	}
	g := Glossary{Body: body, Version: cur.Version + 1}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO glossaries (project_id, body, version, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (project_id) DO UPDATE SET body = excluded.body, version = excluded.version, updated_at = excluded.updated_at`,
		id, g.Body, g.Version, now())
	if err != nil {
		return Glossary{}, fmt.Errorf("editing glossary of %q: %w", slug, err)
	}
	if err := tx.Commit(); err != nil {
		return Glossary{}, fmt.Errorf("editing glossary of %q: %w", slug, err)
	}
	s.notify(slug)
	return g, nil
}
