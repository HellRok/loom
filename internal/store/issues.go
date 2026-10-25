package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Issue kinds and states.
const (
	KindIssue = "issue"
	KindMap   = "map"

	StateOpen   = "open"
	StateClosed = "closed"
)

// Actor is who makes a change: the configured human, acting through a
// Session.
type Actor struct {
	Name    string
	Session string
}

// Issue is one tracked piece of work or decision in a Project.
type Issue struct {
	Number   int64
	Kind     string
	Title    string
	Body     string
	State    string
	Author   string
	Assignee string // empty when unassigned
	// HolderSession and ClaimedAt record the Claim; empty and zero when
	// the Issue is unclaimed.
	HolderSession string
	ClaimedAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// Version counts edits to the title and body.
	Version int64
}

// Comment is an immutable remark on an Issue.
type Comment struct {
	ID        int64
	Author    string
	Session   string
	Body      string
	CreatedAt time.Time
}

// Event is one recorded change to an Issue.
type Event struct {
	At      time.Time
	Actor   string
	Session string
	Type    string
	// Data holds the type's own fields, e.g. "previous" for an edit.
	Data map[string]any
}

// Event types.
const (
	EventCreated     = "created"
	EventTitleEdited = "title_edited"
	EventBodyEdited  = "body_edited"
	EventCommented   = "commented"
	EventClosed      = "closed"
	EventReopened    = "reopened"
	EventLabelled    = "labelled"
	EventUnlabelled  = "unlabelled"
	EventClaimed     = "claimed"
	EventReleased    = "released"
)

// VersionConflictError reports an edit based on a stale version.
type VersionConflictError struct {
	Current Issue
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("#%d is at version %d", e.Current.Number, e.Current.Version)
}

func (*VersionConflictError) documented() {}

// NoSectionError reports a heading the body doesn't have.
type NoSectionError struct {
	Headings []string
}

func (e *NoSectionError) Error() string {
	return fmt.Sprintf("no such section; headings are %q", e.Headings)
}

func (*NoSectionError) documented() {}

// Assignee filters for IssueFilter.
const (
	AssigneeNone = "none"
	AssigneeAny  = "any"
)

// IssueFilter selects Issues for Issues. Zero fields don't filter.
type IssueFilter struct {
	State string
	Kind  string
	// NotKind excludes Issues of that kind.
	NotKind string
	// Assignee is AssigneeNone, AssigneeAny or a name.
	Assignee string
	// HeldBy selects the Issues claimed by that Session.
	HeldBy string
	// Claimed selects Issues somebody holds (true) or nobody does (false), as
	// Issue.Claimed reports.
	Claimed *bool
	// Labels are all required; NoLabels selects Issues with none.
	Labels   []string
	NoLabels bool
	// Parent selects the children of that Issue number, when above zero.
	Parent int64
	// Blocked selects Issues with (true) or without (false) an open Blocker.
	Blocked *bool
	// NoOpenChildren excludes the Issues other than Maps that have an open
	// descendant.
	NoOpenChildren bool
	Limit          int
}

// NewIssue is what CreateIssue makes an Issue from.
type NewIssue struct {
	Kind, Title, Body string
	// Labels, Parent (zero for none) and BlockedBy are applied in the same
	// transaction as the Issue.
	Labels    []string
	Parent    int64
	BlockedBy []int64
}

// querier is what both *sql.DB and *sql.Tx offer.
type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

const issueCols = `i.id, i.number, i.kind, i.title, i.body, i.state, i.author, i.assignee, i.holder_session, i.claimed_at, i.created_at, i.updated_at, i.version`

type scanner interface{ Scan(...any) error }

func scanIssue(r scanner) (id int64, i Issue, err error) {
	var assignee, holder, claimed sql.NullString
	var created, updated string
	err = r.Scan(&id, &i.Number, &i.Kind, &i.Title, &i.Body, &i.State, &i.Author, &assignee, &holder, &claimed, &created, &updated, &i.Version)
	if err != nil {
		return 0, Issue{}, err
	}
	i.Assignee = assignee.String
	i.HolderSession = holder.String
	if claimed.Valid {
		if i.ClaimedAt, err = parseTime(claimed.String); err != nil {
			return 0, Issue{}, err
		}
	}
	if i.CreatedAt, err = parseTime(created); err != nil {
		return 0, Issue{}, err
	}
	if i.UpdatedAt, err = parseTime(updated); err != nil {
		return 0, Issue{}, err
	}
	return id, i, nil
}

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

// issue finds Issue n in the Project slug, returning its row id too.
func issue(ctx context.Context, q querier, slug string, n int64) (int64, Issue, error) {
	row := q.QueryRowContext(ctx, `
		SELECT `+issueCols+` FROM issues i JOIN projects p ON p.id = i.project_id
		WHERE p.slug = ? AND i.number = ?`, slug, n)
	id, i, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Issue{}, ErrNotFound
	}
	if err != nil {
		return 0, Issue{}, fmt.Errorf("reading %s#%d: %w", slug, n, err)
	}
	return id, i, nil
}

// edit is one change to an Issue inside a transaction: the Issue (by row id
// and number, in the Project slug), who is making it and when.
type edit struct {
	tx    *sql.Tx
	slug  string
	id, n int64
	actor Actor
	at    string
}

// event records an Event of type typ for the Issue.
func (e edit) event(ctx context.Context, typ string, data map[string]any) error {
	return addEvent(ctx, e.tx, e.id, e.actor, e.at, typ, data)
}

// touch stamps the Issue as updated at the time of the edit.
func (e edit) touch(ctx context.Context) error { return touch(ctx, e.tx, e.id, e.at) }

func addEvent(ctx context.Context, tx *sql.Tx, issueID int64, a Actor, at, typ string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events (issue_id, at, actor, session, type, data) VALUES (?, ?, ?, ?, ?, ?)`,
		issueID, at, a.Name, a.Session, typ, string(b))
	return err
}

// mutate runs fn in a transaction on Issue n of the Project slug and
// returns the Issue as fn left it.
func (s *Store) mutate(ctx context.Context, slug string, n int64, what string, fn func(tx *sql.Tx, id int64, i Issue) error) (Issue, error) {
	return s.mutateAlso(ctx, slug, n, what, nil, fn)
}

// mutateAlso is mutate for a change that also touches the Issues fn adds to
// also: each is reported changed along with Issue n and its parents.
func (s *Store) mutateAlso(ctx context.Context, slug string, n int64, what string, also []int64, fn func(tx *sql.Tx, id int64, i Issue) error) (Issue, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("%s %s#%d: %w", what, slug, n, err)
	}
	defer tx.Rollback()
	id, i, err := issue(ctx, tx, slug, n)
	if err != nil {
		return Issue{}, err
	}
	before, err := parentNumber(ctx, tx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("%s %s#%d: %w", what, slug, n, err)
	}
	if err := fn(tx, id, i); err != nil {
		return Issue{}, err
	}
	if _, i, err = issue(ctx, tx, slug, n); err != nil {
		return Issue{}, err
	}
	after, err := parentNumber(ctx, tx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("%s %s#%d: %w", what, slug, n, err)
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("%s %s#%d: %w", what, slug, n, err)
	}
	changed := []int64{n, after, before}
	changed = append(changed, also...)
	s.notify(slug, changed...)
	return i, nil
}

// CreateIssue adds an open Issue to the Project slug with the next number.
//
// Its parent, Labels and Blockers are applied in the same transaction, and
// if any is refused (an *UndefinedLabelError, *InvalidTicketError,
// *MissingIssueError and so on) no Issue is created.
func (s *Store) CreateIssue(ctx context.Context, slug string, a Actor, in NewIssue) (Issue, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Issue{}, fmt.Errorf("creating issue in %s: %w", slug, err)
	}
	defer tx.Rollback()

	var projectID, n int64
	err = tx.QueryRowContext(ctx, `
		UPDATE projects SET next_issue = next_issue + 1 WHERE slug = ?
		RETURNING id, next_issue - 1`, slug).Scan(&projectID, &n)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	if err != nil {
		return Issue{}, fmt.Errorf("creating issue in %s: %w", slug, err)
	}
	t := now()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO issues (project_id, number, kind, title, body, state, author, created_at, updated_at, version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`, projectID, n, in.Kind, in.Title, in.Body, StateOpen, a.Name, t, t)
	if err != nil {
		return Issue{}, fmt.Errorf("creating issue in %s: %w", slug, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Issue{}, fmt.Errorf("creating issue in %s: %w", slug, err)
	}
	if err := addEvent(ctx, tx, id, a, t, EventCreated, nil); err != nil {
		return Issue{}, fmt.Errorf("creating issue in %s: %w", slug, err)
	}
	e := edit{tx, slug, id, n, a, t}
	if err := setParent(ctx, e, in.Parent); err != nil {
		return Issue{}, createError(slug, err)
	}
	if err := addBlockers(ctx, e, in.BlockedBy); err != nil {
		return Issue{}, createError(slug, err)
	}
	if err := applyLabels(ctx, e, in.Labels, nil); err != nil {
		return Issue{}, createError(slug, err)
	}
	if err := checkTicketRules(ctx, tx, id); err != nil {
		return Issue{}, createError(slug, err)
	}
	_, i, err := issue(ctx, tx, slug, n)
	if err != nil {
		return Issue{}, err
	}
	parent, err := parentNumber(ctx, tx, id)
	if err != nil {
		return Issue{}, fmt.Errorf("creating issue in %s: %w", slug, err)
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("creating issue in %s: %w", slug, err)
	}
	s.notify(slug, n, parent)
	return i, nil
}

// DocumentedError is implemented by the errors an Issue operation returns
// for the caller to act on, as opposed to an unexpected failure. Every one of
// them marks itself, so a function passing documented errors on, like
// createError, and one coding them, like the tools' issueError, share one
// list: the types with this method.
type DocumentedError interface {
	error
	documented()
}

// Documented reports whether err is, or wraps, a documented error.
func Documented(err error) bool {
	_, ok := errors.AsType[DocumentedError](err)
	return ok
}

// createError passes on the errors CreateIssue documents and wraps the rest.
func createError(slug string, err error) error {
	if Documented(err) {
		return err
	}
	return fmt.Errorf("creating issue in %s: %w", slug, err)
}

// Issue finds Issue n in the Project slug, or returns ErrNotFound.
func (s *Store) Issue(ctx context.Context, slug string, n int64) (Issue, error) {
	_, i, err := issue(ctx, s.db, slug, n)
	return i, err
}

// Issues lists the Issues in the Project slug matching f, oldest first, with
// the total number matching before the limit.
func (s *Store) Issues(ctx context.Context, slug string, f IssueFilter) ([]Issue, int, error) {
	where := []string{"p.slug = ?"}
	args := []any{slug}
	if f.State != "" {
		where = append(where, "i.state = ?")
		args = append(args, f.State)
	}
	if f.Kind != "" {
		where = append(where, "i.kind = ?")
		args = append(args, f.Kind)
	}
	if f.NotKind != "" {
		where = append(where, "i.kind <> ?")
		args = append(args, f.NotKind)
	}
	if f.Claimed != nil {
		not := ""
		if !*f.Claimed {
			not = "NOT "
		}
		where = append(where, not+"(i.assignee IS NOT NULL OR i.holder_session IS NOT NULL)")
	}
	switch f.Assignee {
	case "":
	case AssigneeNone:
		where = append(where, "i.assignee IS NULL")
	case AssigneeAny:
		where = append(where, "i.assignee IS NOT NULL")
	default:
		where = append(where, "i.assignee = ?")
		args = append(args, f.Assignee)
	}
	if f.HeldBy != "" {
		where = append(where, "i.holder_session = ?")
		args = append(args, f.HeldBy)
	}
	if f.NoLabels {
		where = append(where, "NOT EXISTS (SELECT 1 FROM issue_labels il WHERE il.issue_id = i.id)")
	}
	for _, name := range f.Labels {
		where = append(where, `EXISTS (SELECT 1 FROM issue_labels il JOIN labels l ON l.id = il.label_id
			WHERE il.issue_id = i.id AND l.name = ?)`)
		args = append(args, name)
	}
	if f.Parent > 0 {
		where = append(where, "i.parent_id = (SELECT id FROM issues WHERE project_id = p.id AND number = ?)")
		args = append(args, f.Parent)
	}
	if f.Blocked != nil {
		not := ""
		if !*f.Blocked {
			not = "NOT "
		}
		where = append(where, not+`EXISTS (SELECT 1 FROM issue_blockers ib JOIN issues b ON b.id = ib.blocker_id
			WHERE ib.issue_id = i.id AND b.state = 'open')`)
	}
	if f.NoOpenChildren {
		where = append(where, `(i.kind = 'map' OR NOT EXISTS (
			WITH RECURSIVE below(id, state) AS (
				SELECT c.id, c.state FROM issues c WHERE c.parent_id = i.id
				UNION SELECT c.id, c.state FROM issues c JOIN below ON c.parent_id = below.id)
			SELECT 1 FROM below WHERE state = 'open'))`)
	}
	from := ` FROM issues i JOIN projects p ON p.id = i.project_id WHERE ` + strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("listing issues in %s: %w", slug, err)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = -1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+issueCols+from+` ORDER BY i.number LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing issues in %s: %w", slug, err)
	}
	defer rows.Close()
	is := []Issue{}
	for rows.Next() {
		_, i, err := scanIssue(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("listing issues in %s: %w", slug, err)
		}
		is = append(is, i)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listing issues in %s: %w", slug, err)
	}
	return is, total, nil
}

// Comments lists Issue n's comments, oldest first.
func (s *Store) Comments(ctx context.Context, slug string, n int64) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.author, c.session, c.body, c.created_at
		FROM comments c JOIN issues i ON i.id = c.issue_id JOIN projects p ON p.id = i.project_id
		WHERE p.slug = ? AND i.number = ? ORDER BY c.id`, slug, n)
	if err != nil {
		return nil, fmt.Errorf("reading comments on %s#%d: %w", slug, n, err)
	}
	defer rows.Close()
	cs := []Comment{}
	for rows.Next() {
		var c Comment
		var at string
		if err := rows.Scan(&c.ID, &c.Author, &c.Session, &c.Body, &at); err != nil {
			return nil, fmt.Errorf("reading comments on %s#%d: %w", slug, n, err)
		}
		if c.CreatedAt, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("reading comments on %s#%d: %w", slug, n, err)
		}
		cs = append(cs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading comments on %s#%d: %w", slug, n, err)
	}
	return cs, nil
}

// Events lists Issue n's Events, oldest first.
func (s *Store) Events(ctx context.Context, slug string, n int64) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.at, e.actor, e.session, e.type, e.data
		FROM events e JOIN issues i ON i.id = e.issue_id JOIN projects p ON p.id = i.project_id
		WHERE p.slug = ? AND i.number = ? ORDER BY e.id`, slug, n)
	if err != nil {
		return nil, fmt.Errorf("reading events on %s#%d: %w", slug, n, err)
	}
	defer rows.Close()
	es := []Event{}
	for rows.Next() {
		var e Event
		var at, data string
		if err := rows.Scan(&at, &e.Actor, &e.Session, &e.Type, &data); err != nil {
			return nil, fmt.Errorf("reading events on %s#%d: %w", slug, n, err)
		}
		if e.At, err = parseTime(at); err != nil {
			return nil, fmt.Errorf("reading events on %s#%d: %w", slug, n, err)
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			return nil, fmt.Errorf("reading events on %s#%d: %w", slug, n, err)
		}
		es = append(es, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading events on %s#%d: %w", slug, n, err)
	}
	return es, nil
}

// EditIssue replaces Issue n's title and/or body (nil leaves one as is),
// provided version is the Issue's current version; otherwise it returns a
// *VersionConflictError carrying the current Issue. Each changed field is
// recorded with its previous value, and the version goes up when anything
// changed.
func (s *Store) EditIssue(ctx context.Context, slug string, n, version int64, title, body *string, a Actor) (Issue, error) {
	return s.mutate(ctx, slug, n, "editing", func(tx *sql.Tx, id int64, i Issue) error {
		if version != i.Version {
			return &VersionConflictError{Current: i}
		}
		t := now()
		newTitle, newBody := i.Title, i.Body
		if title != nil {
			newTitle = *title
		}
		if body != nil {
			newBody = *body
		}
		return setText(ctx, tx, id, i, newTitle, newBody, a, t)
	})
}

// AppendToSection appends text to the end of the section under heading in
// Issue n's body, whatever its version. A heading the body lacks is a
// *NoSectionError listing those it has.
func (s *Store) AppendToSection(ctx context.Context, slug string, n int64, heading, text string, a Actor) (Issue, error) {
	return s.mutate(ctx, slug, n, "appending to", func(tx *sql.Tx, id int64, i Issue) error {
		body, err := appendToSection(i.Body, heading, text)
		if err != nil {
			return err
		}
		return setText(ctx, tx, id, i, i.Title, body, a, now())
	})
}

// setText records and saves changes to an Issue's title and body, bumping
// its version if either changed.
func setText(ctx context.Context, tx *sql.Tx, id int64, i Issue, title, body string, a Actor, at string) error {
	if title == i.Title && body == i.Body {
		return nil
	}
	if title != i.Title {
		if err := addEvent(ctx, tx, id, a, at, EventTitleEdited, map[string]any{"previous": i.Title}); err != nil {
			return err
		}
	}
	if body != i.Body {
		if err := addEvent(ctx, tx, id, a, at, EventBodyEdited, map[string]any{"previous": i.Body}); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE issues SET title = ?, body = ?, version = version + 1, updated_at = ? WHERE id = ?`,
		title, body, at, id)
	return err
}

// AddComment posts an immutable comment on Issue n.
func (s *Store) AddComment(ctx context.Context, slug string, n int64, body string, a Actor) (Comment, error) {
	var c Comment
	_, err := s.mutate(ctx, slug, n, "commenting on", func(tx *sql.Tx, id int64, i Issue) error {
		var err error
		c, err = addComment(ctx, tx, id, body, a, now())
		return err
	})
	return c, err
}

func addComment(ctx context.Context, tx *sql.Tx, issueID int64, body string, a Actor, at string) (Comment, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO comments (issue_id, author, session, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		issueID, a.Name, a.Session, body, at)
	if err != nil {
		return Comment{}, err
	}
	c := Comment{Author: a.Name, Session: a.Session, Body: body}
	if c.ID, err = res.LastInsertId(); err != nil {
		return Comment{}, err
	}
	if c.CreatedAt, err = parseTime(at); err != nil {
		return Comment{}, err
	}
	if err := addEvent(ctx, tx, issueID, a, at, EventCommented, map[string]any{"comment_id": c.ID}); err != nil {
		return Comment{}, err
	}
	return c, touch(ctx, tx, issueID, at)
}

func touch(ctx context.Context, tx *sql.Tx, issueID int64, at string) error {
	_, err := tx.ExecContext(ctx, `UPDATE issues SET updated_at = ? WHERE id = ?`, at, issueID)
	return err
}

// CloseIssue closes Issue n, first posting comment if it isn't empty and,
// with wontfix, applying the wontfix Label. Closing a closed Issue changes
// nothing, comment and Label included, and reports already.
func (s *Store) CloseIssue(ctx context.Context, slug string, n int64, comment string, wontfix bool, a Actor) (i Issue, already bool, err error) {
	i, err = s.mutate(ctx, slug, n, "closing", func(tx *sql.Tx, id int64, i Issue) error {
		if i.State == StateClosed {
			already = true
			return nil
		}
		t := now()
		if wontfix {
			if err := applyLabels(ctx, edit{tx, slug, id, n, a, t}, []string{LabelWontfix}, nil); err != nil {
				return err
			}
			if err := checkTicketRules(ctx, tx, id); err != nil {
				return err
			}
		}
		if comment != "" {
			if _, err := addComment(ctx, tx, id, comment, a, t); err != nil {
				return err
			}
		}
		return setState(ctx, tx, id, StateClosed, EventClosed, a, t)
	})
	return i, already, err
}

// ReopenIssue reopens Issue n. Reopening an open Issue changes nothing and
// reports already.
func (s *Store) ReopenIssue(ctx context.Context, slug string, n int64, a Actor) (i Issue, already bool, err error) {
	i, err = s.mutate(ctx, slug, n, "reopening", func(tx *sql.Tx, id int64, i Issue) error {
		if i.State == StateOpen {
			already = true
			return nil
		}
		return setState(ctx, tx, id, StateOpen, EventReopened, a, now())
	})
	return i, already, err
}

func setState(ctx context.Context, tx *sql.Tx, issueID int64, state, event string, a Actor, at string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE issues SET state = ?, updated_at = ? WHERE id = ?`, state, at, issueID); err != nil {
		return err
	}
	return addEvent(ctx, tx, issueID, a, at, event, nil)
}

// DeleteIssue hard-deletes Issue n with its comments, Events and blocking
// edges; its children are left parentless, each with an Event. Its number is
// never reused.
func (s *Store) DeleteIssue(ctx context.Context, slug string, n int64, a Actor) error {
	wrap := func(err error) error { return fmt.Errorf("deleting %s#%d: %w", slug, n, err) }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback()
	id, _, err := issue(ctx, tx, slug, n)
	if err != nil {
		return err
	}
	parent, err := parentNumber(ctx, tx, id)
	if err != nil {
		return wrap(err)
	}
	t := now()
	rows, err := tx.QueryContext(ctx, `SELECT id, number FROM issues WHERE parent_id = ?`, id)
	if err != nil {
		return wrap(err)
	}
	var kids, kidNumbers []int64
	for rows.Next() {
		var k, kn int64
		if err := rows.Scan(&k, &kn); err != nil {
			rows.Close()
			return wrap(err)
		}
		kids = append(kids, k)
		kidNumbers = append(kidNumbers, kn)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return wrap(err)
	}
	for _, k := range kids {
		if err := addEvent(ctx, tx, k, a, t, EventParentCleared, map[string]any{"parent": n}); err != nil {
			return wrap(err)
		}
		if err := touch(ctx, tx, k, t); err != nil {
			return wrap(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM issues WHERE id = ?`, id); err != nil {
		return wrap(err)
	}
	if err := tx.Commit(); err != nil {
		return wrap(err)
	}
	s.notify(slug, append([]int64{n, parent}, kidNumbers...)...)
	return nil
}
