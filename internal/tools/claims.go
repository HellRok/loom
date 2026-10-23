package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/HellRok/loom/internal/store"
)

const (
	codeAlreadyClaimed Code = "already_claimed"
	codeIssueClosed    Code = "issue_closed"
	codeIssueBlocked   Code = "issue_blocked"
	codeOpenChildren   Code = "issue_has_open_children"
)

// claimError codes the store errors a Claim can return.
func claimError(slug string, n int64, err error) error {
	if held, ok := errors.AsType[*store.AlreadyClaimedError](err); ok {
		i := held.Issue
		return fail(codeAlreadyClaimed,
			fmt.Sprintf("#%d is held by another Session. Take a different Issue (see frontier); anyone may free it with release if its holder is gone.", n),
			map[string]any{"holder": i.Assignee, "holder_session": i.HolderSession, "claimed_at": claimedAt(i)})
	}
	if _, ok := errors.AsType[*store.ClosedError](err); ok {
		return fail(codeIssueClosed, fmt.Sprintf("#%d is closed, so it can't be claimed. Reopen it first if it needs more work.", n), nil)
	}
	if blocked, ok := errors.AsType[*store.BlockedError](err); ok {
		return fail(codeIssueBlocked, fmt.Sprintf("#%d is blocked by open %v, so it can't be claimed yet.", n, refs(blocked.Blockers)),
			map[string]any{"open_blockers": refs(blocked.Blockers)})
	}
	if open, ok := errors.AsType[*store.OpenChildrenError](err); ok {
		return fail(codeOpenChildren, fmt.Sprintf("#%d has %d open children %v, so it can't be claimed yet. Take one of them, or wait until they close.", n, len(open.Children), refs(open.Children)),
			map[string]any{"open_children": refs(open.Children)})
	}
	return issueError(slug, n, err)
}

// claimedAt is when i was claimed, or nil if it wasn't.
func claimedAt(i store.Issue) *string {
	if i.ClaimedAt.IsZero() {
		return nil
	}
	s := timestamp(i.ClaimedAt)
	return &s
}

type claimIn struct {
	Common
	Ref string `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
}

type claimOut struct {
	Ref           string  `json:"ref"`
	Assignee      string  `json:"assignee"`
	HolderSession string  `json:"holder_session"`
	ClaimedAt     *string `json:"claimed_at"`
	// Already means your Session already held the Issue, so nothing changed.
	Already bool `json:"already"`
}

func (h *handlers) claim(ctx context.Context, c call, in claimIn) (claimOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return claimOut{}, err
	}
	i, already, err := h.store.ClaimIssue(ctx, p.Slug, n, h.actor(c))
	if err != nil {
		return claimOut{}, claimError(p.Slug, n, err)
	}
	return claimOut{Ref: refString(n), Assignee: i.Assignee, HolderSession: i.HolderSession, ClaimedAt: claimedAt(i), Already: already}, nil
}

type releaseOut struct {
	Ref string `json:"ref"`
	// Already means the Issue wasn't claimed, so nothing changed.
	Already bool `json:"already"`
}

func (h *handlers) release(ctx context.Context, c call, in claimIn) (releaseOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return releaseOut{}, err
	}
	_, already, err := h.store.ReleaseIssue(ctx, p.Slug, n, h.actor(c))
	if err != nil {
		return releaseOut{}, issueError(p.Slug, n, err)
	}
	return releaseOut{Ref: refString(n), Already: already}, nil
}
