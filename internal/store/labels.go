package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

// Label is a named tag defined on a Project.
type Label struct {
	Name        string
	Description string
	Colour      string
	// Count is how many Issues, open or closed, carry it.
	Count int
}

// colourPool is the fixed set of label colours; a new Label takes the next
// one in turn.
var colourPool = []string{"red", "orange", "yellow", "green", "teal", "blue", "purple", "pink"}

// defaultLabels are the Labels every new Project starts with: the ones the
// skills use. The migration that introduced Labels seeds older Projects
// with the same list.
var defaultLabels = []string{
	"bug", "enhancement",
	"needs-triage", "needs-info", "ready-for-agent", "ready-for-human", "wontfix",
	"wayfinder:research", "wayfinder:prototype", "wayfinder:grilling", "wayfinder:task",
}

// LabelWontfix is applied by closing an Issue as won't-fix.
const LabelWontfix = "wontfix"

// UndefinedLabelError reports a Label the Project doesn't define.
type UndefinedLabelError struct {
	Name  string
	Valid []string
}

func (e *UndefinedLabelError) Error() string {
	return fmt.Sprintf("label %q is not defined; valid labels are %q", e.Name, e.Valid)
}

func (*UndefinedLabelError) documented() {}

// LabelInUseError reports a Label that Issues still carry.
type LabelInUseError struct {
	Name   string
	Issues []int64
}

func (e *LabelInUseError) Error() string {
	return fmt.Sprintf("label %q is used by %d Issues", e.Name, len(e.Issues))
}

func addLabel(ctx context.Context, q querier, projectID int64, name, description string) error {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM labels WHERE project_id = ?`, projectID).Scan(&n); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO labels (project_id, name, description, colour) VALUES (?, ?, ?, ?)`,
		projectID, name, description, colourPool[n%len(colourPool)])
	return err
}

func seedLabels(ctx context.Context, q querier, projectID int64) error {
	for _, name := range defaultLabels {
		if err := addLabel(ctx, q, projectID, name, ""); err != nil {
			return err
		}
	}
	return nil
}

func projectID(ctx context.Context, q querier, slug string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM projects WHERE slug = ?`, slug).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func definedLabels(ctx context.Context, q querier, projectID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT name FROM labels WHERE project_id = ? ORDER BY name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		names = append(names, s)
	}
	return names, rows.Err()
}

// labelIDs maps names to their Label ids, or returns an
// *UndefinedLabelError for the first the Project doesn't define.
func labelIDs(ctx context.Context, q querier, projectID int64, names []string) (map[string]int64, error) {
	ids := make(map[string]int64, len(names))
	for _, name := range names {
		var id int64
		err := q.QueryRowContext(ctx, `SELECT id FROM labels WHERE project_id = ? AND name = ?`, projectID, name).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			valid, verr := definedLabels(ctx, q, projectID)
			if verr != nil {
				return nil, verr
			}
			return nil, &UndefinedLabelError{Name: name, Valid: valid}
		}
		if err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, nil
}

// applyLabels adds and removes Labels on an Issue, recording an Event for
// each one that actually changed.
func applyLabels(ctx context.Context, e edit, add, remove []string) error {
	pid, err := projectID(ctx, e.tx, e.slug)
	if err != nil {
		return err
	}
	ids, err := labelIDs(ctx, e.tx, pid, slices.Concat(add, remove))
	if err != nil {
		return err
	}
	changed := false
	for _, name := range add {
		res, err := e.tx.ExecContext(ctx, `INSERT OR IGNORE INTO issue_labels (issue_id, label_id) VALUES (?, ?)`, e.id, ids[name])
		if err != nil {
			return err
		}
		if k, _ := res.RowsAffected(); k > 0 {
			changed = true
			if err := e.event(ctx, EventLabelled, map[string]any{"label": name}); err != nil {
				return err
			}
		}
	}
	for _, name := range remove {
		res, err := e.tx.ExecContext(ctx, `DELETE FROM issue_labels WHERE issue_id = ? AND label_id = ?`, e.id, ids[name])
		if err != nil {
			return err
		}
		if k, _ := res.RowsAffected(); k > 0 {
			changed = true
			if err := e.event(ctx, EventUnlabelled, map[string]any{"label": name}); err != nil {
				return err
			}
		}
	}
	if changed {
		return e.touch(ctx)
	}
	return nil
}

// SetLabels adds and removes Labels on Issue n in one transaction: if any
// name is undefined (an *UndefinedLabelError) nothing changes. Adding a
// Label the Issue has, or removing one it lacks, is a no-op.
func (s *Store) SetLabels(ctx context.Context, slug string, n int64, add, remove []string, a Actor) (Issue, error) {
	return s.mutate(ctx, slug, n, "labelling", func(tx *sql.Tx, id int64, i Issue) error {
		if err := applyLabels(ctx, edit{tx, slug, id, n, a, now()}, add, remove); err != nil {
			return err
		}
		return checkTicketRules(ctx, tx, id)
	})
}

// IssueLabels lists the names of the Labels on Issue n, sorted.
func (s *Store) IssueLabels(ctx context.Context, slug string, n int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.name FROM issue_labels il
		JOIN labels l ON l.id = il.label_id
		JOIN issues i ON i.id = il.issue_id
		JOIN projects p ON p.id = i.project_id
		WHERE p.slug = ? AND i.number = ? ORDER BY l.name`, slug, n)
	if err != nil {
		return nil, fmt.Errorf("reading labels on %s#%d: %w", slug, n, err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("reading labels on %s#%d: %w", slug, n, err)
		}
		names = append(names, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading labels on %s#%d: %w", slug, n, err)
	}
	return names, nil
}

// Labels lists the Project's Labels by name, with usage counts.
func (s *Store) Labels(ctx context.Context, slug string) ([]Label, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.name, l.description, l.colour, count(il.issue_id)
		FROM labels l JOIN projects p ON p.id = l.project_id
		LEFT JOIN issue_labels il ON il.label_id = l.id
		WHERE p.slug = ? GROUP BY l.id ORDER BY l.name`, slug)
	if err != nil {
		return nil, fmt.Errorf("listing labels in %s: %w", slug, err)
	}
	defer rows.Close()
	ls := []Label{}
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.Name, &l.Description, &l.Colour, &l.Count); err != nil {
			return nil, fmt.Errorf("listing labels in %s: %w", slug, err)
		}
		ls = append(ls, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing labels in %s: %w", slug, err)
	}
	return ls, nil
}

// CreateLabel defines a Label in the Project slug. A name already defined
// changes nothing, whatever the description, and reports already. It
// returns ErrNotFound for an unknown Project.
func (s *Store) CreateLabel(ctx context.Context, slug, name, description string) (l Label, already bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Label{}, false, fmt.Errorf("creating label %q in %s: %w", name, slug, err)
	}
	defer tx.Rollback()
	pid, err := projectID(ctx, tx, slug)
	if err != nil {
		return Label{}, false, err
	}
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM labels WHERE project_id = ? AND name = ?`, pid, name).Scan(new(int))
	switch {
	case err == nil:
		already = true
	case errors.Is(err, sql.ErrNoRows):
		if err := addLabel(ctx, tx, pid, name, description); err != nil {
			return Label{}, false, fmt.Errorf("creating label %q in %s: %w", name, slug, err)
		}
	default:
		return Label{}, false, fmt.Errorf("creating label %q in %s: %w", name, slug, err)
	}
	if err := tx.Commit(); err != nil {
		return Label{}, false, fmt.Errorf("creating label %q in %s: %w", name, slug, err)
	}
	if !already {
		s.notify(slug)
	}
	ls, err := s.Labels(ctx, slug)
	if err != nil {
		return Label{}, false, err
	}
	for _, l := range ls {
		if l.Name == name {
			return l, already, nil
		}
	}
	return Label{}, false, fmt.Errorf("label %q vanished from %s", name, slug)
}

// DeleteLabel removes an unused Label. One that Issues carry is a
// *LabelInUseError listing them; an unknown one is ErrNotFound.
func (s *Store) DeleteLabel(ctx context.Context, slug, name string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting label %q in %s: %w", name, slug, err)
	}
	defer tx.Rollback()
	pid, err := projectID(ctx, tx, slug)
	if err != nil {
		return err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM labels WHERE project_id = ? AND name = ?`, pid, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("deleting label %q in %s: %w", name, slug, err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT i.number FROM issue_labels il JOIN issues i ON i.id = il.issue_id
		WHERE il.label_id = ? ORDER BY i.number`, id)
	if err != nil {
		return fmt.Errorf("deleting label %q in %s: %w", name, slug, err)
	}
	var users []int64
	for rows.Next() {
		var n int64
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return fmt.Errorf("deleting label %q in %s: %w", name, slug, err)
		}
		users = append(users, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("deleting label %q in %s: %w", name, slug, err)
	}
	if len(users) > 0 {
		return &LabelInUseError{Name: name, Issues: users}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM labels WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting label %q in %s: %w", name, slug, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("deleting label %q in %s: %w", name, slug, err)
	}
	s.notify(slug)
	return nil
}
