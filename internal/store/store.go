// Package store owns Loom's SQLite database: opening it, migrating it, and
// every transactional domain operation on it.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedded embed.FS

// Errors the domain operations return.
var (
	ErrNotFound  = errors.New("not found")
	ErrSlugTaken = errors.New("slug taken")
	ErrRootTaken = errors.New("root taken")
)

// SchemaTooNewError reports a database migrated by a newer Loom binary.
type SchemaTooNewError struct {
	DB, Binary int64
}

func (e *SchemaTooNewError) Error() string {
	return fmt.Sprintf("database schema version %d is newer than this binary's %d; upgrade loom", e.DB, e.Binary)
}

// Store is an open, migrated Loom database.
type Store struct {
	db *sql.DB
}

// Open opens the database at path and brings its schema up to date with the
// migrations built into this binary.
func Open(ctx context.Context, path string) (*Store, error) {
	migrations, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return nil, fmt.Errorf("reading built-in migrations: %w", err)
	}
	return OpenWith(ctx, path, migrations)
}

// OpenWith is Open with an explicit set of migrations.
func OpenWith(ctx context.Context, path string, migrations fs.FS) (*Store, error) {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if err := migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(ctx context.Context, db *sql.DB, migrations fs.FS) error {
	p, err := goose.NewProvider(goose.DialectSQLite3, db, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	current, err := p.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	sources := p.ListSources()
	latest := sources[len(sources)-1].Version
	if current > latest {
		return &SchemaTooNewError{DB: current, Binary: latest}
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrating: %w", err)
	}
	return nil
}

// Close checkpoints the WAL into the main database file and closes it.
func (s *Store) Close() error {
	_, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return errors.Join(err, s.db.Close())
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// Session is an agent process that has called Loom.
type Session struct {
	ID                  string
	FirstSeen, LastSeen time.Time
}

// TouchSession records that a Session made a call.
func (s *Store) TouchSession(ctx context.Context, id string) error {
	t := now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, first_seen, last_seen) VALUES (?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET last_seen = excluded.last_seen`, id, t, t)
	if err != nil {
		return fmt.Errorf("recording session %q: %w", id, err)
	}
	return nil
}

// SessionByID finds one Session, or returns ErrNotFound.
func (s *Store) SessionByID(ctx context.Context, id string) (Session, error) {
	var first, last string
	err := s.db.QueryRowContext(ctx, `SELECT first_seen, last_seen FROM sessions WHERE id = ?`, id).Scan(&first, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("reading session %q: %w", id, err)
	}
	sess := Session{ID: id}
	if sess.FirstSeen, err = time.Parse(time.RFC3339Nano, first); err != nil {
		return Session{}, fmt.Errorf("session %q first_seen: %w", id, err)
	}
	if sess.LastSeen, err = time.Parse(time.RFC3339Nano, last); err != nil {
		return Session{}, fmt.Errorf("session %q last_seen: %w", id, err)
	}
	return sess, nil
}
