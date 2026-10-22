package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
)

// Change reports a committed write: the Project it was in and the Issues it
// changed, each listed once in the order the write touched them (the Issue,
// then its parents). Writes that touch no Issue, such as defining a Label,
// list none.
type Change struct {
	Slug   string
	Issues []int64
}

// OnChange sets fn to be called after every committed write, replacing any
// earlier one; nil stops the calls. fn runs on the writer's goroutine, so it
// must not block.
func (s *Store) OnChange(fn func(Change)) {
	if fn == nil {
		s.onChange.Store(nil)
		return
	}
	s.onChange.Store(&fn)
}

// notify reports a committed write to the OnChange hook, ignoring the
// zero numbers that stand for "no such Issue".
func (s *Store) notify(slug string, issues ...int64) {
	fn := s.onChange.Load()
	if fn == nil {
		return
	}
	c := Change{Slug: slug}
	for _, n := range issues {
		if n != 0 && !slices.Contains(c.Issues, n) {
			c.Issues = append(c.Issues, n)
		}
	}
	(*fn)(c)
}

// parentNumber is the number of the Issue with row id's parent, 0 if it has
// none.
func parentNumber(ctx context.Context, q querier, id int64) (int64, error) {
	var n sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT p.number FROM issues c JOIN issues p ON p.id = c.parent_id WHERE c.id = ?`, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n.Int64, err
}
