package store

import (
	"context"
	"database/sql"
	"fmt"
)

// ClosedError reports a Claim on a closed Issue.
type ClosedError struct{ N int64 }

func (e *ClosedError) Error() string { return fmt.Sprintf("#%d is closed", e.N) }

// BlockedError reports a Claim on an Issue with open Blockers.
type BlockedError struct {
	N        int64
	Blockers []Issue
}

func (e *BlockedError) Error() string { return fmt.Sprintf("#%d is blocked", e.N) }

// OpenChildrenError reports a Claim on an Issue with open children.
type OpenChildrenError struct {
	N        int64
	Children []Issue
}

func (e *OpenChildrenError) Error() string {
	return fmt.Sprintf("#%d has %d open children", e.N, len(e.Children))
}

// AlreadyClaimedError reports a Claim on an Issue another Session holds.
type AlreadyClaimedError struct {
	Issue Issue
}

func (e *AlreadyClaimedError) Error() string {
	return fmt.Sprintf("#%d is held by session %q", e.Issue.Number, e.Issue.HolderSession)
}

// Claimed reports whether somebody holds the Issue.
func (i Issue) Claimed() bool { return i.Assignee != "" || i.HolderSession != "" }

// ClaimIssue assigns Issue n to a.Name and records a.Session as its holder,
// atomically. The Session that already holds it gets the Issue back with
// already set and nothing changes; another holder is an *AlreadyClaimedError.
// A closed Issue is a *ClosedError, one with an open Blocker a *BlockedError
// and one other than a Map with an open child an *OpenChildrenError.
func (s *Store) ClaimIssue(ctx context.Context, slug string, n int64, a Actor) (i Issue, already bool, err error) {
	i, err = s.mutate(ctx, slug, n, "claiming", func(tx *sql.Tx, id int64, i Issue) error {
		if i.Claimed() {
			if i.HolderSession == a.Session {
				already = true
				return nil
			}
			return &AlreadyClaimedError{Issue: i}
		}
		if i.State == StateClosed {
			return &ClosedError{N: n}
		}
		open, err := openBlockersTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if len(open) > 0 {
			return &BlockedError{N: n, Blockers: open}
		}
		kids, err := openChildrenTx(ctx, tx, id, i.Kind)
		if err != nil {
			return err
		}
		if len(kids) > 0 {
			return &OpenChildrenError{N: n, Children: kids}
		}
		t := now()
		if _, err := tx.ExecContext(ctx, `UPDATE issues SET assignee = ?, holder_session = ?, claimed_at = ?, updated_at = ? WHERE id = ?`,
			a.Name, a.Session, t, t, id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, a, t, EventClaimed, nil)
	})
	return i, already, err
}

// ReleaseIssue clears Issue n's Claim, whoever holds it; the Event records
// the releasing Session and the one that held it. Releasing an unclaimed
// Issue changes nothing and reports already.
func (s *Store) ReleaseIssue(ctx context.Context, slug string, n int64, a Actor) (i Issue, already bool, err error) {
	i, err = s.mutate(ctx, slug, n, "releasing", func(tx *sql.Tx, id int64, i Issue) error {
		if !i.Claimed() {
			already = true
			return nil
		}
		t := now()
		if _, err := tx.ExecContext(ctx, `UPDATE issues SET assignee = NULL, holder_session = NULL, claimed_at = NULL, updated_at = ? WHERE id = ?`, t, id); err != nil {
			return err
		}
		return addEvent(ctx, tx, id, a, t, EventReleased, map[string]any{"holder_session": i.HolderSession})
	})
	return i, already, err
}

// openBlockersTx lists the open Issues blocking the Issue with row id, by number.
func openBlockersTx(ctx context.Context, q querier, id int64) ([]Issue, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT `+issueCols+` FROM issue_blockers ib JOIN issues i ON i.id = ib.blocker_id
		WHERE ib.issue_id = ? AND i.state = ? ORDER BY i.number`, id, StateOpen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		_, i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// openChildrenTx lists the open children of the Issue of that kind with row
// id, by number. A closed child doesn't hide its own open descendants: they
// count too, so closing a parent with open children doesn't free its parent.
// A Map has none that count: it closes on judgement.
func openChildrenTx(ctx context.Context, q querier, id int64, kind string) ([]Issue, error) {
	if kind == KindMap {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx, `
		WITH RECURSIVE closed(id) AS (
			SELECT id FROM issues WHERE parent_id = ?1 AND state = 'closed'
			UNION SELECT c.id FROM issues c JOIN closed ON c.parent_id = closed.id WHERE c.state = 'closed')
		SELECT `+issueCols+` FROM issues i
		WHERE i.state = 'open' AND (i.parent_id = ?1 OR i.parent_id IN (SELECT id FROM closed))
		ORDER BY i.number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		_, i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
