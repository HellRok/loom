package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HellRok/loom/internal/store"
)

func (h *handlers) actor(c call) store.Actor {
	return store.Actor{Name: h.assignee, Session: c.Session}
}

func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// issueError codes the store errors an Issue operation can return.
func issueError(slug string, n int64, err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return notFound(slug, n)
	}
	if conflict, ok := errors.AsType[*store.VersionConflictError](err); ok {
		cur := conflict.Current
		return fail(codeVersionConflict,
			fmt.Sprintf("#%d changed since you read it; it is now at version %d. Re-apply your change to the current title and body and retry with version %d.", n, cur.Version, cur.Version),
			map[string]any{"current": map[string]any{"version": cur.Version, "title": cur.Title, "body": cur.Body}})
	}
	if undefined, ok := errors.AsType[*store.UndefinedLabelError](err); ok {
		return fail(codeUndefinedLabel,
			fmt.Sprintf("%q isn't a label in this Project. Use one of the valid labels, or define it first with create_label.", undefined.Name),
			map[string]any{"valid": undefined.Valid})
	}
	if invalid, ok := errors.AsType[*store.InvalidTicketError](err); ok {
		return fail(codeInvalidTicket, invalid.Message+" A Ticket's type is one of the valid_types, as the label wayfinder:<type>.",
			map[string]any{"valid_types": store.WayfinderTypes})
	}
	if cycle, ok := errors.AsType[*store.CycleError](err); ok {
		msg := fmt.Sprintf("#%d is already (transitively) blocked by #%d, so it can't also block it.", cycle.Other, cycle.Issue)
		if cycle.Edge == store.EdgeParent {
			msg = fmt.Sprintf("#%d is #%d or one of its descendants, so it can't be its parent.", cycle.Other, cycle.Issue)
		}
		return fail(codeCycle, msg, map[string]any{"edge": string(cycle.Edge)})
	}
	if self, ok := errors.AsType[*store.SelfBlockError](err); ok {
		return fail(codeSelfBlock, fmt.Sprintf("#%d can't block itself.", self.N), nil)
	}
	if missing, ok := errors.AsType[*store.MissingIssueError](err); ok {
		return notFound(slug, missing.N)
	}
	if missing, ok := errors.AsType[*store.NoSectionError](err); ok {
		return fail(codeNoSection, fmt.Sprintf("#%d has no section with that heading.", n),
			map[string]any{"headings": missing.Headings})
	}
	return err
}

type commentOut struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Session   string `json:"session"`
	CreatedAt string `json:"created_at"`
	Body      string `json:"body"`
}

func (h *handlers) comments(ctx context.Context, slug string, n int64) (*[]commentOut, error) {
	cs, err := h.store.Comments(ctx, slug, n)
	if err != nil {
		return nil, err
	}
	out := make([]commentOut, len(cs))
	for k, c := range cs {
		out[k] = commentOut{ID: c.ID, Author: c.Author, Session: c.Session, CreatedAt: timestamp(c.CreatedAt), Body: c.Body}
	}
	return &out, nil
}

type eventOut struct {
	At      string         `json:"at"`
	Actor   string         `json:"actor"`
	Session string         `json:"session"`
	Type    string         `json:"type"`
	Data    map[string]any `json:"data,omitempty"`
}

func (h *handlers) events(ctx context.Context, slug string, n int64) (*[]eventOut, error) {
	es, err := h.store.Events(ctx, slug, n)
	if err != nil {
		return nil, err
	}
	out := make([]eventOut, len(es))
	for k, e := range es {
		out[k] = eventOut{At: timestamp(e.At), Actor: e.Actor, Session: e.Session, Type: e.Type, Data: e.Data}
	}
	return &out, nil
}

type createIssueIn struct {
	Common
	Title string `json:"title"`
	Body  string `json:"body,omitempty" jsonschema:"Markdown"`
	Kind  string `json:"kind,omitempty" jsonschema:"issue (the default) or map; fixed once created"`
	// Labels must all be defined in the Project.
	Labels    []string `json:"labels,omitempty" jsonschema:"Labels to apply, all defined in the Project (see list_labels)"`
	Parent    string   `json:"parent,omitempty" jsonschema:"The parent Issue's ref, in the same Project. A Map's children are Tickets: each needs exactly one wayfinder:<type> label, so pass labels too; other labels are allowed alongside it"`
	BlockedBy []string `json:"blocked_by,omitempty" jsonschema:"Refs of Issues that must close before this one can be taken, in the same Project"`
}

type createIssueOut struct {
	Ref     string `json:"ref"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Version int64  `json:"version"`
}

func (h *handlers) createIssue(ctx context.Context, c call, in createIssueIn) (createIssueOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return createIssueOut{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return createIssueOut{}, fail(codeInvalidArgument, "An Issue needs a `title`.", nil)
	}
	kind := in.Kind
	if kind == "" {
		kind = store.KindIssue
	}
	if kind != store.KindIssue && kind != store.KindMap {
		return createIssueOut{}, fail(codeInvalidArgument, fmt.Sprintf("`kind` must be %q or %q, not %q.", store.KindIssue, store.KindMap, kind), nil)
	}
	n := store.NewIssue{Kind: kind, Title: in.Title, Body: in.Body, Labels: in.Labels}
	if in.Parent != "" {
		if n.Parent, err = h.sameProjectRef(p, in.Parent, "parent"); err != nil {
			return createIssueOut{}, err
		}
	}
	if n.BlockedBy, err = h.sameProjectRefs(p, in.BlockedBy, "blocked_by"); err != nil {
		return createIssueOut{}, err
	}
	i, err := h.store.CreateIssue(ctx, p.Slug, h.actor(c), n)
	if errors.Is(err, store.ErrNotFound) {
		return createIssueOut{}, h.unknownProject(ctx, p.Slug)
	}
	if err != nil {
		return createIssueOut{}, issueError(p.Slug, 0, err)
	}
	return createIssueOut{Ref: refString(i.Number), URL: h.issueURL(p.Slug, i.Number), Title: i.Title, Kind: i.Kind, Version: i.Version}, nil
}

type viewIssueIn struct {
	Common
	Ref      string `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Comments *bool  `json:"comments,omitempty" jsonschema:"Include comments (default true)"`
	Events   bool   `json:"events,omitempty" jsonschema:"Include the Event history (default false)"`
}

type viewIssueOut struct {
	Ref      string  `json:"ref"`
	URL      string  `json:"url"`
	Project  string  `json:"project"`
	Kind     string  `json:"kind"`
	Title    string  `json:"title"`
	State    string  `json:"state"`
	Assignee *string `json:"assignee"`
	// HolderSession and ClaimedAt are set while the Issue is claimed.
	HolderSession *string     `json:"holder_session"`
	ClaimedAt     *string     `json:"claimed_at"`
	Author        string      `json:"author"`
	CreatedAt     string      `json:"created_at"`
	UpdatedAt     string      `json:"updated_at"`
	Version       int64       `json:"version"`
	Labels        []string    `json:"labels"`
	Parent        *issueLink  `json:"parent"`
	BlockedBy     []issueLink `json:"blocked_by" jsonschema:"Every Blocker, open or closed"`
	// Children are summaries, without bodies.
	Children []issueSummary `json:"children"`
	// OpenChildren are the children not yet closed, and the open Issues below closed ones. Unless the Issue is a Map,
	// it can't be claimed while there are any, though they aren't Blockers.
	OpenChildren []string      `json:"open_children" jsonschema:"Children not yet closed, and open Issues below closed children; while there are any, an Issue other than a Map can't be claimed"`
	Body         string        `json:"body"`
	Comments     *[]commentOut `json:"comments,omitempty"`
	Events       *[]eventOut   `json:"events,omitempty"`
}

func (h *handlers) viewIssue(ctx context.Context, c call, in viewIssueIn) (viewIssueOut, error) {
	p, i, err := h.resolveIssue(ctx, c, in.Ref)
	if err != nil {
		return viewIssueOut{}, err
	}
	out := viewIssueOut{
		Ref: refString(i.Number), URL: h.issueURL(p.Slug, i.Number), Project: p.Slug,
		Kind: i.Kind, Title: i.Title, State: i.State, Assignee: nullable(i.Assignee), HolderSession: nullable(i.HolderSession), ClaimedAt: claimedAt(i), Author: i.Author,
		CreatedAt: timestamp(i.CreatedAt), UpdatedAt: timestamp(i.UpdatedAt), Version: i.Version, Body: i.Body,
	}
	if out.Labels, err = h.store.IssueLabels(ctx, p.Slug, i.Number); err != nil {
		return viewIssueOut{}, err
	}
	if err := h.fillRelations(ctx, p.Slug, i.Number, &out); err != nil {
		return viewIssueOut{}, err
	}
	if in.Comments == nil || *in.Comments {
		if out.Comments, err = h.comments(ctx, p.Slug, i.Number); err != nil {
			return viewIssueOut{}, err
		}
	}
	if in.Events {
		if out.Events, err = h.events(ctx, p.Slug, i.Number); err != nil {
			return viewIssueOut{}, err
		}
	}
	return out, nil
}

// versionOut is what a text edit returns: the version to pass next time.
type versionOut struct {
	Ref     string `json:"ref"`
	Version int64  `json:"version"`
}

type editIssueIn struct {
	Common
	Ref     string  `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Version int64   `json:"version" jsonschema:"The version you last read; the edit is refused if the Issue has changed since"`
	Title   *string `json:"title,omitempty" jsonschema:"The new title"`
	Body    *string `json:"body,omitempty" jsonschema:"The new Markdown body, replacing the old one"`
}

func (h *handlers) editIssue(ctx context.Context, c call, in editIssueIn) (versionOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return versionOut{}, err
	}
	if in.Title == nil && in.Body == nil {
		return versionOut{}, fail(codeInvalidArgument, "Pass a new `title`, `body` or both.", nil)
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return versionOut{}, fail(codeInvalidArgument, "An Issue's `title` can't be blank.", nil)
	}
	i, err := h.store.EditIssue(ctx, p.Slug, n, in.Version, in.Title, in.Body, h.actor(c))
	if err != nil {
		return versionOut{}, issueError(p.Slug, n, err)
	}
	return versionOut{Ref: refString(i.Number), Version: i.Version}, nil
}

type appendToSectionIn struct {
	Common
	Ref     string `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Heading string `json:"heading" jsonschema:"The section's heading text, e.g. Decisions so far"`
	Text    string `json:"text" jsonschema:"Markdown to add as the section's last lines"`
}

func (h *handlers) appendToSection(ctx context.Context, c call, in appendToSectionIn) (versionOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return versionOut{}, err
	}
	if strings.TrimSpace(in.Text) == "" {
		return versionOut{}, fail(codeInvalidArgument, "Pass the `text` to append.", nil)
	}
	i, err := h.store.AppendToSection(ctx, p.Slug, n, in.Heading, in.Text, h.actor(c))
	if err != nil {
		return versionOut{}, issueError(p.Slug, n, err)
	}
	return versionOut{Ref: refString(i.Number), Version: i.Version}, nil
}

type commentIn struct {
	Common
	Ref  string `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Body string `json:"body" jsonschema:"Markdown"`
}

type commentResultOut struct {
	Ref       string `json:"ref"`
	CommentID int64  `json:"comment_id"`
	URL       string `json:"url"`
}

func (h *handlers) comment(ctx context.Context, c call, in commentIn) (commentResultOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return commentResultOut{}, err
	}
	if strings.TrimSpace(in.Body) == "" {
		return commentResultOut{}, fail(codeInvalidArgument, "A comment needs a `body`.", nil)
	}
	cm, err := h.store.AddComment(ctx, p.Slug, n, in.Body, h.actor(c))
	if err != nil {
		return commentResultOut{}, issueError(p.Slug, n, err)
	}
	return commentResultOut{Ref: refString(n), CommentID: cm.ID, URL: fmt.Sprintf("%s#comment-%d", h.issueURL(p.Slug, n), cm.ID)}, nil
}

// stateOut is what close and reopen return; already means the Issue was
// in that state before the call, which changed nothing.
type stateOut struct {
	Ref     string `json:"ref"`
	State   string `json:"state"`
	Already bool   `json:"already"`
}

type closeIn struct {
	Common
	Ref     string `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Comment string `json:"comment,omitempty" jsonschema:"Markdown comment posted in the same step, e.g. the resolution"`
	Wontfix bool   `json:"wontfix,omitempty" jsonschema:"Also apply the wontfix label, in the same step"`
}

func (h *handlers) closeIssue(ctx context.Context, c call, in closeIn) (stateOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return stateOut{}, err
	}
	comment := in.Comment
	if strings.TrimSpace(comment) == "" {
		comment = ""
	}
	i, already, err := h.store.CloseIssue(ctx, p.Slug, n, comment, in.Wontfix, h.actor(c))
	if err != nil {
		return stateOut{}, issueError(p.Slug, n, err)
	}
	return stateOut{Ref: refString(n), State: i.State, Already: already}, nil
}

type reopenIn struct {
	Common
	Ref string `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
}

func (h *handlers) reopenIssue(ctx context.Context, c call, in reopenIn) (stateOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return stateOut{}, err
	}
	i, already, err := h.store.ReopenIssue(ctx, p.Slug, n, h.actor(c))
	if err != nil {
		return stateOut{}, issueError(p.Slug, n, err)
	}
	return stateOut{Ref: refString(n), State: i.State, Already: already}, nil
}

type listIssuesIn struct {
	Common
	State    string `json:"state,omitempty" jsonschema:"open (the default), closed or all"`
	Kind     string `json:"kind,omitempty" jsonschema:"issue or map; both by default"`
	Assignee string `json:"assignee,omitempty" jsonschema:"none, any, or an assignee's name"`
	HeldBy   string `json:"held_by,omitempty" jsonschema:"Only Issues claimed by this Session id, or me for your own claims"`
	Detail   string `json:"detail,omitempty" jsonschema:"summary (the default), or full to include bodies and comments"`
	Limit    int    `json:"limit,omitempty" jsonschema:"At most this many Issues (default 50)"`
	// Labels are all required.
	Labels   []string `json:"labels,omitempty" jsonschema:"Only Issues carrying all of these labels"`
	NoLabels bool     `json:"no_labels,omitempty" jsonschema:"Only Issues with no labels"`
	Parent   string   `json:"parent,omitempty" jsonschema:"Only the children of this Issue"`
	Blocked  *bool    `json:"blocked,omitempty" jsonschema:"true: only Issues with an open Blocker; false: only those without"`
}

// issueSummary is an Issue in a list: bodies and comments only with
// detail: full.
type issueSummary struct {
	Ref      string  `json:"ref"`
	Title    string  `json:"title"`
	Kind     string  `json:"kind"`
	State    string  `json:"state"`
	Assignee *string `json:"assignee"`
	// HolderSession is the Session claiming the Issue, if any.
	HolderSession *string  `json:"holder_session"`
	Labels        []string `json:"labels"`
	// OpenBlockers are the open Issues blocking this one, if any.
	OpenBlockers []string      `json:"open_blockers,omitempty"`
	Body         *string       `json:"body,omitempty"`
	Comments     *[]commentOut `json:"comments,omitempty"`
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

type listIssuesOut struct {
	Total  int            `json:"total"`
	Issues []issueSummary `json:"issues"`
}

const defaultLimit = 50

func (h *handlers) listIssues(ctx context.Context, c call, in listIssuesIn) (listIssuesOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return listIssuesOut{}, err
	}
	f := store.IssueFilter{Kind: in.Kind, Assignee: in.Assignee, HeldBy: in.HeldBy, Limit: in.Limit, Labels: in.Labels, NoLabels: in.NoLabels, Blocked: in.Blocked}
	if in.Parent != "" {
		if f.Parent, err = h.sameProjectRef(p, in.Parent, "parent"); err != nil {
			return listIssuesOut{}, err
		}
		if _, err := h.store.Issue(ctx, p.Slug, f.Parent); err != nil {
			return listIssuesOut{}, issueError(p.Slug, f.Parent, err)
		}
	}
	if f.HeldBy == "me" {
		f.HeldBy = c.Session
	}
	if f.NoLabels && len(f.Labels) > 0 {
		return listIssuesOut{}, fail(codeInvalidArgument, "`labels` and `no_labels` can't both be set: no Issue matches.", nil)
	}
	if len(f.Labels) > 0 {
		// A typo'd label would silently match nothing, so check it exists.
		if err := h.checkLabels(ctx, p.Slug, f.Labels); err != nil {
			return listIssuesOut{}, err
		}
	}
	switch in.State {
	case "":
		f.State = store.StateOpen
	case store.StateOpen, store.StateClosed:
		f.State = in.State
	case "all":
	default:
		return listIssuesOut{}, fail(codeInvalidArgument, fmt.Sprintf("`state` must be open, closed or all, not %q.", in.State), nil)
	}
	if f.Kind != "" && f.Kind != store.KindIssue && f.Kind != store.KindMap {
		return listIssuesOut{}, fail(codeInvalidArgument, fmt.Sprintf("`kind` must be %q or %q, not %q.", store.KindIssue, store.KindMap, f.Kind), nil)
	}
	full := false
	switch in.Detail {
	case "", "summary":
	case "full":
		full = true
	default:
		return listIssuesOut{}, fail(codeInvalidArgument, fmt.Sprintf("`detail` must be summary or full, not %q.", in.Detail), nil)
	}
	switch {
	case f.Limit < 0:
		return listIssuesOut{}, fail(codeInvalidArgument, "`limit` can't be negative.", nil)
	case f.Limit == 0:
		f.Limit = defaultLimit
	}

	is, total, err := h.store.Issues(ctx, p.Slug, f)
	if err != nil {
		return listIssuesOut{}, err
	}
	out := listIssuesOut{Total: total, Issues: make([]issueSummary, len(is))}
	for k, i := range is {
		s, err := h.summary(ctx, p.Slug, i)
		if err != nil {
			return listIssuesOut{}, err
		}
		if full {
			s.Body = &i.Body
			if s.Comments, err = h.comments(ctx, p.Slug, i.Number); err != nil {
				return listIssuesOut{}, err
			}
		}
		out.Issues[k] = s
	}
	return out, nil
}

type deleteIssueIn struct {
	Common
	Ref     string `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Confirm string `json:"confirm" jsonschema:"Must be the Issue's #N"`
}

type deleteIssueOut struct {
	Deleted string `json:"deleted"`
}

func (h *handlers) deleteIssue(ctx context.Context, c call, in deleteIssueIn) (deleteIssueOut, error) {
	p, i, err := h.resolveIssue(ctx, c, in.Ref)
	if err != nil {
		return deleteIssueOut{}, err
	}
	ref := refString(i.Number)
	if in.Confirm != ref {
		return deleteIssueOut{}, fail(codeConfirmMismatch, fmt.Sprintf("Pass confirm: %q to hard-delete %q with its comments and history.", ref, i.Title), nil)
	}
	if err := h.store.DeleteIssue(ctx, p.Slug, i.Number, h.actor(c)); err != nil {
		return deleteIssueOut{}, issueError(p.Slug, i.Number, err)
	}
	return deleteIssueOut{Deleted: ref}, nil
}
