package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Event types for hierarchy and blocking.
const (
	EventParentSet      = "parent_set"
	EventParentCleared  = "parent_cleared"
	EventBlockerAdded   = "blocker_added"
	EventBlockerRemoved = "blocker_removed"
)

// WayfinderTypes are the types a Ticket's wayfinder:<type> label can name.
var WayfinderTypes = []string{"research", "prototype", "grilling", "task"}

// wayfinderPrefix starts the name of a Ticket's type label.
const wayfinderPrefix = "wayfinder:"

// InvalidTicketError reports a change that breaks the Map rules.
type InvalidTicketError struct {
	Message string
}

func (e *InvalidTicketError) Error() string { return e.Message }

// Edge names the kind of link a CycleError is about.
type Edge string

const (
	EdgeParent  Edge = "parent"
	EdgeBlocker Edge = "blocker"
)

// CycleError reports a parent or Blocker edge that would close a loop.
type CycleError struct {
	Edge Edge
	// Issue and Other are the numbers the edge would join: Other would be
	// Issue's parent or Blocker.
	Issue, Other int64
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("making #%d the %s of #%d would make a cycle", e.Other, e.Edge, e.Issue)
}

// SelfBlockError reports an Issue made to block itself.
type SelfBlockError struct{ N int64 }

func (e *SelfBlockError) Error() string { return fmt.Sprintf("#%d can't block itself", e.N) }

// MissingIssueError reports a parent or Blocker that doesn't exist in the
// Project.
type MissingIssueError struct{ N int64 }

func (e *MissingIssueError) Error() string { return fmt.Sprintf("no Issue #%d", e.N) }

// issueIDByNumber finds the row id of Issue n in the Project slug, or
// returns a *MissingIssueError.
func issueIDByNumber(ctx context.Context, q querier, slug string, n int64) (int64, error) {
	id, _, err := issue(ctx, q, slug, n)
	if errors.Is(err, ErrNotFound) {
		return 0, &MissingIssueError{N: n}
	}
	return id, err
}

// numberOf is the number of the Issue with row id: the n people see, as
// opposed to the id that keys the table.
func numberOf(ctx context.Context, q querier, id int64) (n int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT number FROM issues WHERE id = ?`, id).Scan(&n)
	return n, err
}

// setParent points Issue id (number n) at parent (0 for none), recording an
// Event if that changed anything.
func setParent(ctx context.Context, tx *sql.Tx, slug string, id, n, parent int64, a Actor, at string) error {
	var current sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT parent_id FROM issues WHERE id = ?`, id).Scan(&current); err != nil {
		return err
	}
	var currentN int64
	if current.Valid {
		var err error
		if currentN, err = numberOf(ctx, tx, current.Int64); err != nil {
			return err
		}
	}
	if parent == 0 {
		if !current.Valid {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE issues SET parent_id = NULL, updated_at = ? WHERE id = ?`, at, id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, a, at, EventParentCleared, map[string]any{"parent": currentN})
	}
	pid, err := issueIDByNumber(ctx, tx, slug, parent)
	if err != nil {
		return err
	}
	// The new parent mustn't be the Issue or one of its descendants.
	var cycle bool
	err = tx.QueryRowContext(ctx, `
		WITH RECURSIVE up(id) AS (
			SELECT ? UNION SELECT i.parent_id FROM issues i JOIN up ON i.id = up.id WHERE i.parent_id IS NOT NULL
		) SELECT EXISTS (SELECT 1 FROM up WHERE id = ?)`, pid, id).Scan(&cycle)
	if err != nil {
		return err
	}
	if cycle {
		return &CycleError{Edge: EdgeParent, Issue: n, Other: parent}
	}
	if current.Valid && current.Int64 == pid {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE issues SET parent_id = ?, updated_at = ? WHERE id = ?`, pid, at, id); err != nil {
		return err
	}
	data := map[string]any{"parent": parent}
	if current.Valid {
		data["previous"] = currentN
	}
	return addEvent(ctx, tx, id, a, at, EventParentSet, data)
}

// addBlockers makes Issue id (number n) blocked by each of blockers,
// recording an Event for each new edge. Existing edges are ignored.
func addBlockers(ctx context.Context, tx *sql.Tx, slug string, id, n int64, blockers []int64, a Actor, at string) error {
	changed := false
	for _, b := range blockers {
		if b == n {
			return &SelfBlockError{N: n}
		}
		bid, err := issueIDByNumber(ctx, tx, slug, b)
		if err != nil {
			return err
		}
		// The Issue mustn't already block the new Blocker, however indirectly.
		var cycle bool
		err = tx.QueryRowContext(ctx, `
			WITH RECURSIVE chain(id) AS (
				SELECT ? UNION SELECT ib.blocker_id FROM issue_blockers ib JOIN chain ON ib.issue_id = chain.id
			) SELECT EXISTS (SELECT 1 FROM chain WHERE id = ?)`, bid, id).Scan(&cycle)
		if err != nil {
			return err
		}
		if cycle {
			return &CycleError{Edge: EdgeBlocker, Issue: n, Other: b}
		}
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO issue_blockers (issue_id, blocker_id) VALUES (?, ?)`, id, bid)
		if err != nil {
			return err
		}
		if k, _ := res.RowsAffected(); k > 0 {
			changed = true
			if err := addEvent(ctx, tx, id, a, at, EventBlockerAdded, map[string]any{"blocker": b}); err != nil {
				return err
			}
		}
	}
	if changed {
		return touch(ctx, tx, id, at)
	}
	return nil
}

// checkTicketRules fails with an *InvalidTicketError if Issue id, as it now
// stands, breaks the Map rules: a Map has no parent, and a Ticket (a child
// of a Map) has exactly one wayfinder type label. Its other Labels are
// unrestricted, and it may have children of its own, which are ordinary Issues.
func checkTicketRules(ctx context.Context, q querier, id int64) error {
	var n int64
	var kind string
	var parent, pn sql.NullInt64
	var pkind sql.NullString
	if err := q.QueryRowContext(ctx, `
		SELECT i.number, i.kind, i.parent_id, par.number, par.kind
		FROM issues i LEFT JOIN issues par ON par.id = i.parent_id
		WHERE i.id = ?`, id).Scan(&n, &kind, &parent, &pn, &pkind); err != nil {
		return err
	}
	if kind == KindMap && parent.Valid {
		return &InvalidTicketError{Message: fmt.Sprintf("#%d is a Map, and a Map can't have a parent.", n)}
	}
	if !parent.Valid || pkind.String != KindMap {
		return nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT l.name FROM issue_labels il JOIN labels l ON l.id = il.label_id
		WHERE il.issue_id = ? ORDER BY l.name`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if t, ok := strings.CutPrefix(name, wayfinderPrefix); ok && slices.Contains(WayfinderTypes, t) {
			types = append(types, name)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(types) != 1 {
		return &InvalidTicketError{Message: fmt.Sprintf("#%d is a Ticket (a child of Map #%d), so it needs exactly one wayfinder:<type> label; it has %d.", n, pn.Int64, len(types))}
	}
	return nil
}

// SetParent makes parent (0 for none) Issue n's parent. A parent that
// doesn't exist is a *MissingIssueError; one that would close a loop a
// *CycleError; one that breaks the Map rules an *InvalidTicketError.
// Setting the parent it already has changes nothing.
func (s *Store) SetParent(ctx context.Context, slug string, n, parent int64, a Actor) (Issue, error) {
	return s.mutate(ctx, slug, n, "setting the parent of", func(tx *sql.Tx, id int64, i Issue) error {
		if err := setParent(ctx, tx, slug, id, n, parent, a, now()); err != nil {
			return err
		}
		return checkTicketRules(ctx, tx, id)
	})
}

// AddBlockers makes Issue n blocked by each of blockers, all or none: one
// that doesn't exist is a *MissingIssueError, Issue n itself a
// *SelfBlockError and one that would close a loop a *CycleError. Blockers
// it already has are ignored.
func (s *Store) AddBlockers(ctx context.Context, slug string, n int64, blockers []int64, a Actor) (Issue, error) {
	return s.mutateAlso(ctx, slug, n, "blocking", blockers, func(tx *sql.Tx, id int64, i Issue) error {
		return addBlockers(ctx, tx, slug, id, n, blockers, a, now())
	})
}

// RemoveBlockers lifts each of blockers from Issue n, ignoring any that
// weren't blocking it.
func (s *Store) RemoveBlockers(ctx context.Context, slug string, n int64, blockers []int64, a Actor) (Issue, error) {
	return s.mutateAlso(ctx, slug, n, "unblocking", blockers, func(tx *sql.Tx, id int64, i Issue) error {
		t := now()
		changed := false
		for _, b := range blockers {
			res, err := tx.ExecContext(ctx, `
				DELETE FROM issue_blockers WHERE issue_id = ? AND blocker_id IN (
					SELECT b.id FROM issues b WHERE b.project_id = (SELECT project_id FROM issues WHERE id = ?) AND b.number = ?)`,
				id, id, b)
			if err != nil {
				return err
			}
			if k, _ := res.RowsAffected(); k > 0 {
				changed = true
				if err := addEvent(ctx, tx, id, a, t, EventBlockerRemoved, map[string]any{"blocker": b}); err != nil {
					return err
				}
			}
		}
		if changed {
			return touch(ctx, tx, id, t)
		}
		return nil
	})
}

// Parent returns Issue n's parent, or nil if it has none.
func (s *Store) Parent(ctx context.Context, slug string, n int64) (*Issue, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+issueCols+` FROM issues c
		JOIN issues i ON i.id = c.parent_id JOIN projects p ON p.id = c.project_id
		WHERE p.slug = ? AND c.number = ?`, slug, n)
	_, i, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the parent of %s#%d: %w", slug, n, err)
	}
	return &i, nil
}

func (s *Store) related(ctx context.Context, what, slug string, n int64, query string) ([]Issue, error) {
	_, is, err := related(ctx, s.db, what, slug, n, query)
	return is, err
}

// related runs query (taking slug and n) on q and returns the Issues it
// selects, with their row ids.
func related(ctx context.Context, q querier, what, slug string, n int64, query string) ([]int64, []Issue, error) {
	rows, err := q.QueryContext(ctx, query, slug, n)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the %s of %s#%d: %w", what, slug, n, err)
	}
	defer rows.Close()
	ids := []int64{}
	is := []Issue{}
	for rows.Next() {
		id, i, err := scanIssue(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("reading the %s of %s#%d: %w", what, slug, n, err)
		}
		ids = append(ids, id)
		is = append(is, i)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading the %s of %s#%d: %w", what, slug, n, err)
	}
	return ids, is, nil
}

const childrenQuery = `
	SELECT ` + issueCols + ` FROM issues i
	JOIN issues par ON par.id = i.parent_id JOIN projects p ON p.id = par.project_id
	WHERE p.slug = ? AND par.number = ? ORDER BY i.number`

// Children lists Issue n's children by number.
func (s *Store) Children(ctx context.Context, slug string, n int64) ([]Issue, error) {
	return s.related(ctx, "children", slug, n, childrenQuery)
}

// Blockers lists every Issue blocking Issue n, open or closed, by number.
func (s *Store) Blockers(ctx context.Context, slug string, n int64) ([]Issue, error) {
	return s.related(ctx, "blockers", slug, n, `
		SELECT `+issueCols+` FROM issue_blockers ib
		JOIN issues i ON i.id = ib.blocker_id
		JOIN issues blocked ON blocked.id = ib.issue_id JOIN projects p ON p.id = blocked.project_id
		WHERE p.slug = ? AND blocked.number = ? ORDER BY i.number`)
}

// OpenBlockers lists the open Issues blocking Issue n: while there are any,
// n is blocked.
func (s *Store) OpenBlockers(ctx context.Context, slug string, n int64) ([]Issue, error) {
	is, err := s.Blockers(ctx, slug, n)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(is, func(i Issue) bool { return i.State != StateOpen }), nil
}

// OpenChildren lists the open children of Issue n, and the open descendants
// below any closed child: while there are any, n can't be taken, as if
// blocked, though no Blocker is recorded. A Map has none that count.
func (s *Store) OpenChildren(ctx context.Context, slug string, n int64) ([]Issue, error) {
	id, i, err := issue(ctx, s.db, slug, n)
	if err != nil {
		return nil, err
	}
	return openChildrenTx(ctx, s.db, id, i.Kind)
}

// Frontier is the open children of an Issue, sorted into those ready to take
// and those skipped.
type Frontier struct {
	// Ready are the unblocked, unclaimed children, in creation order.
	Ready []Issue
	// Claimed are the unblocked children somebody holds.
	Claimed []Issue
	// Blocked are the children with an open Blocker or an open child, claimed
	// or not.
	Blocked []Issue
}

// Frontier sorts the open children of Issue parent, all read from one
// snapshot. An unknown parent is ErrNotFound.
func (s *Store) Frontier(ctx context.Context, slug string, parent int64) (Frontier, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Frontier{}, fmt.Errorf("reading the frontier of %s#%d: %w", slug, parent, err)
	}
	defer tx.Rollback()
	if _, _, err := issue(ctx, tx, slug, parent); err != nil {
		return Frontier{}, err
	}
	ids, kids, err := related(ctx, tx, "children", slug, parent, childrenQuery)
	if err != nil {
		return Frontier{}, err
	}
	f := Frontier{Ready: []Issue{}, Claimed: []Issue{}, Blocked: []Issue{}}
	for j, k := range kids {
		if k.State != StateOpen {
			continue
		}
		open, err := openBlockersTx(ctx, tx, ids[j])
		if err != nil {
			return Frontier{}, fmt.Errorf("reading the frontier of %s#%d: %w", slug, parent, err)
		}
		held, err := openChildrenTx(ctx, tx, ids[j], k.Kind)
		if err != nil {
			return Frontier{}, fmt.Errorf("reading the frontier of %s#%d: %w", slug, parent, err)
		}
		switch {
		case len(open) > 0 || len(held) > 0:
			f.Blocked = append(f.Blocked, k)
		case k.Claimed():
			f.Claimed = append(f.Claimed, k)
		default:
			f.Ready = append(f.Ready, k)
		}
	}
	return f, nil
}
