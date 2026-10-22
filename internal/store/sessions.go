package store

import (
	"context"
	"fmt"
	"time"
)

// Sessions lists every recorded Session, oldest first.
func (s *Store) Sessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, first_seen, last_seen FROM sessions ORDER BY first_seen, id`)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var sess Session
		var first, last string
		if err := rows.Scan(&sess.ID, &first, &last); err != nil {
			return nil, fmt.Errorf("listing sessions: %w", err)
		}
		if sess.FirstSeen, err = time.Parse(time.RFC3339Nano, first); err != nil {
			return nil, fmt.Errorf("session %q first_seen: %w", sess.ID, err)
		}
		if sess.LastSeen, err = time.Parse(time.RFC3339Nano, last); err != nil {
			return nil, fmt.Errorf("session %q last_seen: %w", sess.ID, err)
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	return out, nil
}
