package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// SessionIssue is an Issue with the Project it belongs to.
type SessionIssue struct {
	Slug string
	Issue
}

// SessionEvent is an Event with the Issue it happened to.
type SessionEvent struct {
	Slug   string
	Number int64
	Title  string
	Event
}

// withSlug reads a leading slug column before the issue columns.
type withSlug struct {
	r    scanner
	slug *string
}

func (w withSlug) Scan(dest ...any) error { return w.r.Scan(append([]any{w.slug}, dest...)...) }

// HeldBy lists the open Issues the Session holds, across Projects, by
// Project then number.
func (s *Store) HeldBy(ctx context.Context, session string) ([]SessionIssue, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.slug, `+issueCols+` FROM issues i JOIN projects p ON p.id = i.project_id
		WHERE i.holder_session = ? AND i.state = ? ORDER BY p.slug, i.number`, session, StateOpen)
	if err != nil {
		return nil, fmt.Errorf("listing issues held by %q: %w", session, err)
	}
	defer rows.Close()
	out := []SessionIssue{}
	for rows.Next() {
		var si SessionIssue
		if _, si.Issue, err = scanIssue(withSlug{rows, &si.Slug}); err != nil {
			return nil, fmt.Errorf("listing issues held by %q: %w", session, err)
		}
		out = append(out, si)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing issues held by %q: %w", session, err)
	}
	return out, nil
}

// SessionEvents lists the Events the Session recorded across Projects,
// newest first.
func (s *Store) SessionEvents(ctx context.Context, session string) ([]SessionEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.slug, i.number, i.title, e.at, e.actor, e.session, e.type, e.data
		FROM events e JOIN issues i ON i.id = e.issue_id JOIN projects p ON p.id = i.project_id
		WHERE e.session = ? ORDER BY e.id DESC`, session)
	if err != nil {
		return nil, fmt.Errorf("listing events of session %q: %w", session, err)
	}
	defer rows.Close()
	out := []SessionEvent{}
	for rows.Next() {
		var e SessionEvent
		var at, data string
		if err := rows.Scan(&e.Slug, &e.Number, &e.Title, &at, &e.Actor, &e.Event.Session, &e.Type, &data); err != nil {
			return nil, fmt.Errorf("listing events of session %q: %w", session, err)
		}
		if e.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("listing events of session %q: %w", session, err)
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			return nil, fmt.Errorf("listing events of session %q: %w", session, err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing events of session %q: %w", session, err)
	}
	return out, nil
}
