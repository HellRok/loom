package web

import (
	"context"

	"github.com/HellRok/loom/internal/store"
)

// StoreGraph is the Graph backed by the Store's hierarchy and blocking.
func StoreGraph(st *store.Store) Graph { return storeGraph{st} }

type storeGraph struct{ st *store.Store }

func ref(i store.Issue) Ref {
	return Ref{Number: i.Number, Title: i.Title, Closed: i.State == store.StateClosed}
}

func refs(is []store.Issue) []Ref {
	var out []Ref
	for _, i := range is {
		out = append(out, ref(i))
	}
	return out
}

func (g storeGraph) Relations(ctx context.Context, slug string, n int64) (Relations, error) {
	var r Relations
	p, err := g.st.Parent(ctx, slug, n)
	if err != nil {
		return r, err
	}
	if p != nil {
		pr := ref(*p)
		r.Parent = &pr
	}
	blockers, err := g.st.Blockers(ctx, slug, n)
	if err != nil {
		return r, err
	}
	blocks, err := g.st.Blocking(ctx, slug, n)
	if err != nil {
		return r, err
	}
	kids, err := g.st.Children(ctx, slug, n)
	if err != nil {
		return r, err
	}
	r.Blockers, r.Blocks, r.Children = refs(blockers), refs(blocks), refs(kids)
	return r, nil
}

func (g storeGraph) Frontier(ctx context.Context, slug string, parent int64) ([]store.Issue, error) {
	f, err := g.st.Frontier(ctx, slug, parent)
	return f.Ready, err
}
