package tools

import (
	"context"
	"time"
)

type listSessionsIn struct{ Common }

type sessionOut struct {
	ID        string    `json:"id"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

type listSessionsOut struct {
	Sessions []sessionOut `json:"sessions"`
}

func (h *handlers) listSessions(ctx context.Context, c call, in listSessionsIn) (listSessionsOut, error) {
	ss, err := h.store.Sessions(ctx)
	if err != nil {
		return listSessionsOut{}, err
	}
	out := listSessionsOut{Sessions: make([]sessionOut, len(ss))}
	for i, s := range ss {
		out.Sessions[i] = sessionOut{ID: s.ID, FirstSeen: s.FirstSeen, LastSeen: s.LastSeen}
	}
	return out, nil
}
