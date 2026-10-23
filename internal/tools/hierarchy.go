package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/HellRok/loom/internal/store"
)

// sameProjectRef reads s, the argument field's Issue ref, as an Issue number
// in Project p: a ref naming another Project can't be a parent or Blocker.
func (h *handlers) sameProjectRef(p store.Project, s, field string) (int64, error) {
	r, err := h.parseRef(s)
	if err != nil {
		return 0, err
	}
	if r.Slug != "" && r.Slug != p.Slug {
		return 0, fail(codeInvalidArgument, fmt.Sprintf("`%s` %q is in Project %q, but this call acts on %q; hierarchy and blocking stay within one Project.", field, s, r.Slug, p.Slug), nil)
	}
	return r.N, nil
}

func (h *handlers) sameProjectRefs(p store.Project, refs []string, field string) ([]int64, error) {
	ns := make([]int64, len(refs))
	for k, s := range refs {
		n, err := h.sameProjectRef(p, s, field)
		if err != nil {
			return nil, err
		}
		ns[k] = n
	}
	return ns, nil
}

// issueLink points at another Issue, with enough to tell if it's done.
type issueLink struct {
	Ref   string `json:"ref"`
	Title string `json:"title"`
	State string `json:"state"`
}

func link(i store.Issue) issueLink {
	return issueLink{Ref: refString(i.Number), Title: i.Title, State: i.State}
}

func refs(is []store.Issue) []string {
	out := make([]string, len(is))
	for k, i := range is {
		out[k] = refString(i.Number)
	}
	return out
}

// summary is Issue i as listed: no body or comments.
func (h *handlers) summary(ctx context.Context, slug string, i store.Issue) (issueSummary, error) {
	s := issueSummary{Ref: refString(i.Number), Title: i.Title, Kind: i.Kind, State: i.State, Assignee: nullable(i.Assignee), HolderSession: nullable(i.HolderSession)}
	var err error
	if s.Labels, err = h.store.IssueLabels(ctx, slug, i.Number); err != nil {
		return s, err
	}
	open, err := h.store.OpenBlockers(ctx, slug, i.Number)
	if err != nil {
		return s, err
	}
	if len(open) > 0 {
		s.OpenBlockers = refs(open)
	}
	return s, nil
}

// fillRelations adds Issue n's parent, Blockers, children and open children
// to out.
func (h *handlers) fillRelations(ctx context.Context, slug string, n int64, out *viewIssueOut) error {
	parent, err := h.store.Parent(ctx, slug, n)
	if err != nil {
		return err
	}
	if parent != nil {
		l := link(*parent)
		out.Parent = &l
	}
	blockers, err := h.store.Blockers(ctx, slug, n)
	if err != nil {
		return err
	}
	out.BlockedBy = make([]issueLink, len(blockers))
	for k, b := range blockers {
		out.BlockedBy[k] = link(b)
	}
	kids, err := h.store.Children(ctx, slug, n)
	if err != nil {
		return err
	}
	out.Children = make([]issueSummary, len(kids))
	for k, c := range kids {
		if out.Children[k], err = h.summary(ctx, slug, c); err != nil {
			return err
		}
	}
	open, err := h.store.OpenChildren(ctx, slug, n)
	if err != nil {
		return err
	}
	out.OpenChildren = refs(open)
	return nil
}

type frontierIn struct {
	Common
	Parent string `json:"parent" jsonschema:"The Issue whose children to sort: a Map, or a spec"`
}

type skippedOut struct {
	Claimed []string `json:"claimed" jsonschema:"Open, unblocked children somebody holds"`
	Blocked []string `json:"blocked" jsonschema:"Open children with an open Blocker or an open child of their own"`
}

type frontierOut struct {
	Parent   string         `json:"parent"`
	Frontier []issueSummary `json:"frontier" jsonschema:"Open, unblocked, unassigned children in creation order"`
	Skipped  skippedOut     `json:"skipped"`
}

func (h *handlers) frontier(ctx context.Context, c call, in frontierIn) (frontierOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Parent)
	if err != nil {
		return frontierOut{}, err
	}
	f, err := h.store.Frontier(ctx, p.Slug, n)
	if err != nil {
		return frontierOut{}, issueError(p.Slug, n, err)
	}
	out := frontierOut{Parent: refString(n), Frontier: make([]issueSummary, len(f.Ready)),
		Skipped: skippedOut{Claimed: refs(f.Claimed), Blocked: refs(f.Blocked)}}
	for k, i := range f.Ready {
		if out.Frontier[k], err = h.summary(ctx, p.Slug, i); err != nil {
			return frontierOut{}, err
		}
	}
	return out, nil
}

type setParentIn struct {
	Common
	Ref    string  `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Parent *string `json:"parent" jsonschema:"The new parent's ref, or null to make the Issue parentless"`
}

type setParentOut struct {
	Ref    string  `json:"ref"`
	Parent *string `json:"parent"`
}

func (h *handlers) setParent(ctx context.Context, c call, in setParentIn) (setParentOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return setParentOut{}, err
	}
	var parent int64
	if in.Parent != nil {
		if parent, err = h.sameProjectRef(p, *in.Parent, "parent"); err != nil {
			return setParentOut{}, err
		}
	}
	if _, err := h.store.SetParent(ctx, p.Slug, n, parent, h.actor(c)); err != nil {
		return setParentOut{}, issueError(p.Slug, n, err)
	}
	out := setParentOut{Ref: refString(n)}
	if parent != 0 {
		r := refString(parent)
		out.Parent = &r
	}
	return out, nil
}

type blockersIn struct {
	Common
	Ref       string   `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	BlockedBy []string `json:"blocked_by" jsonschema:"Refs of the Blockers, in the same Project"`
}

type blockersOut struct {
	Ref       string   `json:"ref"`
	BlockedBy []string `json:"blocked_by" jsonschema:"All of the Issue's Blockers after the change"`
}

func (h *handlers) addBlockers(ctx context.Context, c call, in blockersIn) (blockersOut, error) {
	return h.changeBlockers(ctx, c, in, h.store.AddBlockers)
}

func (h *handlers) removeBlockers(ctx context.Context, c call, in blockersIn) (blockersOut, error) {
	return h.changeBlockers(ctx, c, in, h.store.RemoveBlockers)
}

func (h *handlers) changeBlockers(ctx context.Context, c call, in blockersIn, change func(context.Context, string, int64, []int64, store.Actor) (store.Issue, error)) (blockersOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return blockersOut{}, err
	}
	if len(in.BlockedBy) == 0 {
		return blockersOut{}, fail(codeInvalidArgument, "Pass the Issues in `blocked_by`.", nil)
	}
	ns, err := h.sameProjectRefs(p, in.BlockedBy, "blocked_by")
	if err != nil {
		return blockersOut{}, err
	}
	if _, err := change(ctx, p.Slug, n, ns, h.actor(c)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return blockersOut{}, notFound(p.Slug, n)
		}
		return blockersOut{}, issueError(p.Slug, n, err)
	}
	all, err := h.store.Blockers(ctx, p.Slug, n)
	if err != nil {
		return blockersOut{}, err
	}
	return blockersOut{Ref: refString(n), BlockedBy: refs(all)}, nil
}
