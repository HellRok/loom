package web

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HellRok/loom/internal/store"
)

// Tabs of the Issue pane.
const (
	tabBoard    = "board"
	tabDecided  = "decided"
	tabOverview = "overview"
	tabHistory  = "history"
)

var tabNames = map[string]string{
	tabBoard: "Board", tabDecided: "Decided", tabOverview: "Overview", tabHistory: "History",
}

type tabLink struct {
	Name, URL string
	Active    bool
}

// tabsFor are the tabs an Issue of kind has; the first is the default.
func tabsFor(kind string) []string {
	if kind == store.KindMap {
		return []string{tabBoard, tabDecided, tabOverview, tabHistory}
	}
	return []string{tabOverview, tabHistory}
}

// tabURL is Issue n's page on tab, keeping the list filters; a default tab
// is left out.
func tabURL(slug string, n int64, f filters, tab, def string) string {
	v, _ := url.ParseQuery(strings.TrimPrefix(f.query(), "?"))
	if tab != def {
		v.Set("tab", tab)
	}
	u := issueURL(slug, n, "")
	if len(v) > 0 {
		u += "?" + v.Encode()
	}
	return u
}

type column struct {
	Key, Name string
	Cards     []card
}

// shortSession is the last 8 characters of a Session, as shown in lists.
func shortSession(s string) string {
	if r := []rune(s); len(r) > 8 {
		return string(r[len(r)-8:])
	}
	return s
}

// card is one Ticket on the Board.
type card struct {
	store.Issue
	Type     string // its wayfinder:<type> label
	URL      string
	Blockers []refLink   // the open ones
	Holder   *HolderView // the claiming Session, if any
}

type board struct {
	Frontier, Claimed, Blocked []card
}

// decision is a closed Ticket with the gist of its resolution.
type decision struct {
	store.Issue
	URL  string
	Gist string
}

type eventView struct {
	store.Event
	Summary    string
	Ref        *refLink // the Issue an event is about, if any
	CommentURL string
	Diff       []diffLine
}

// loadTab fills in iv for the requested tab, falling back to the Issue's
// default for an unknown one, and builds the tab links.
func (s *server) loadTab(ctx context.Context, iv *issueView, requested string, f filters) error {
	avail := tabsFor(iv.Kind)
	def := avail[0]
	iv.Tab = def
	if slices.Contains(avail, requested) {
		iv.Tab = requested
	}
	for _, t := range avail {
		iv.Tabs = append(iv.Tabs, tabLink{Name: tabNames[t], URL: tabURL(iv.Slug, iv.Number, f, t, def), Active: t == iv.Tab})
	}
	iv.Self = tabURL(iv.Slug, iv.Number, f, iv.Tab, def)

	link := func(n int64) string { return issueURL(iv.Slug, n, iv.Query) }
	switch iv.Tab {
	case tabBoard:
		fr, err := s.Store.Frontier(ctx, iv.Slug, iv.Number)
		if err != nil {
			return err
		}
		types, err := s.Store.LabelsByIssue(ctx, iv.Slug)
		if err != nil {
			return err
		}
		hs, err := s.holders(ctx)
		if err != nil {
			return err
		}
		mk := func(is []store.Issue) ([]card, error) {
			var out []card
			for _, i := range is {
				c := card{Issue: i, URL: link(i.Number), Holder: hs.view(i.HolderSession)}
				for _, l := range types[i.Number] {
					if strings.HasPrefix(l, "wayfinder:") {
						c.Type = l
					}
				}
				open, err := s.Store.OpenBlockers(ctx, iv.Slug, i.Number)
				if err != nil {
					return nil, err
				}
				for _, b := range open {
					c.Blockers = append(c.Blockers, refLink{Ref: ref(b), URL: link(b.Number)})
				}
				out = append(out, c)
			}
			return out, nil
		}
		var mkErr error
		if iv.Board.Frontier, mkErr = mk(fr.Ready); mkErr != nil {
			return mkErr
		}
		if iv.Board.Claimed, mkErr = mk(fr.Claimed); mkErr != nil {
			return mkErr
		}
		if iv.Board.Blocked, mkErr = mk(fr.Blocked); mkErr != nil {
			return mkErr
		}
	case tabDecided:
		kids, err := s.Store.Children(ctx, iv.Slug, iv.Number)
		if err != nil {
			return err
		}
		type closed struct {
			d  decision
			at time.Time
		}
		var cs []closed
		for _, k := range kids {
			if k.State != store.StateClosed {
				continue
			}
			gist, at, err := s.resolution(ctx, iv.Slug, k)
			if err != nil {
				return err
			}
			cs = append(cs, closed{decision{Issue: k, URL: link(k.Number), Gist: gist}, at})
		}
		// The route walked: in the order the decisions were made.
		sort.SliceStable(cs, func(a, b int) bool { return cs[a].at.Before(cs[b].at) })
		for _, c := range cs {
			iv.Decided = append(iv.Decided, c.d)
		}
	case tabHistory:
		var err error
		iv.History, err = s.history(ctx, iv, link)
		return err
	}
	return nil
}

// resolution is the gist of the comment posted as Ticket k closed, and when
// it closed.
func (s *server) resolution(ctx context.Context, slug string, k store.Issue) (string, time.Time, error) {
	evs, err := s.Store.Events(ctx, slug, k.Number)
	if err != nil {
		return "", time.Time{}, err
	}
	at := k.UpdatedAt
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type != store.EventClosed {
			continue
		}
		at = evs[i].At
		// Closing posts its comment in the same instant, just before.
		if i > 0 && evs[i-1].Type == store.EventCommented && evs[i-1].At.Equal(at) {
			id, _ := evs[i-1].Data["comment_id"].(float64)
			comments, err := s.Store.Comments(ctx, slug, k.Number)
			if err != nil {
				return "", at, err
			}
			for _, c := range comments {
				if c.ID == int64(id) {
					return gist(c.Body), at, nil
				}
			}
		}
		break
	}
	return "", at, nil
}

const gistLen = 200

// gist is the first paragraph of Markdown body as one plain line.
func gist(body string) string {
	var lines []string
	for l := range strings.SplitSeq(strings.TrimSpace(body), "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "#> "))
		if l == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		lines = append(lines, l)
	}
	g := []rune(strings.Join(lines, " "))
	if len(g) > gistLen {
		return string(g[:gistLen-1]) + "…"
	}
	return string(g)
}

// history lists iv's Events, newest first. An edit shows a diff from the
// value it replaced to the one the next edit of the same field replaced, or
// the current value for the last.
func (s *server) history(ctx context.Context, iv *issueView, link func(int64) string) ([]eventView, error) {
	evs, err := s.Store.Events(ctx, iv.Slug, iv.Number)
	if err != nil {
		return nil, err
	}
	next := map[string]string{store.EventTitleEdited: iv.Title, store.EventBodyEdited: iv.Issue.Body}
	out := make([]eventView, 0, len(evs))
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		v := eventView{Event: e}
		num := func(key string) int64 { n, _ := e.Data[key].(float64); return int64(n) }
		refTo := func(key string) {
			if n := num(key); n != 0 {
				v.Ref = &refLink{Ref: Ref{Number: n}, URL: link(n)}
			}
		}
		switch e.Type {
		case store.EventCreated:
			v.Summary = "created this"
		case store.EventTitleEdited, store.EventBodyEdited:
			old, _ := e.Data["previous"].(string)
			v.Summary = "edited the " + strings.TrimSuffix(e.Type, "_edited")
			v.Diff = lineDiff(old, next[e.Type])
			next[e.Type] = old
		case store.EventCommented:
			v.Summary = "added a"
			v.CommentURL = tabURLKeep(iv, tabOverview) + "#comment-" + strconv.FormatInt(num("comment_id"), 10)
		case store.EventClosed:
			v.Summary = "closed this"
		case store.EventReopened:
			v.Summary = "reopened this"
		case store.EventLabelled, store.EventUnlabelled:
			label, _ := e.Data["label"].(string)
			v.Summary = fmt.Sprintf("%s %q", map[string]string{store.EventLabelled: "added label", store.EventUnlabelled: "removed label"}[e.Type], label)
		case store.EventClaimed:
			v.Summary = "claimed this"
		case store.EventReleased:
			v.Summary = "released the claim"
		case store.EventParentSet:
			v.Summary = "set the parent to"
			refTo("parent")
		case store.EventParentCleared:
			v.Summary = "cleared the parent"
			refTo("parent")
		case store.EventBlockerAdded:
			v.Summary = "added blocker"
			refTo("blocker")
		case store.EventBlockerRemoved:
			v.Summary = "removed blocker"
			refTo("blocker")
		default:
			v.Summary = strings.ReplaceAll(e.Type, "_", " ")
		}
		out = append(out, v)
	}
	return out, nil
}

// tabURLKeep is iv's URL on tab, with the filters it was opened with. The
// Overview is every kind's tab, and a Map's default is the Board.
func tabURLKeep(iv *issueView, tab string) string {
	u := issueURL(iv.Slug, iv.Number, "")
	v, _ := url.ParseQuery(strings.TrimPrefix(iv.Query, "?"))
	if tab != tabsFor(iv.Kind)[0] {
		v.Set("tab", tab)
	}
	if len(v) > 0 {
		u += "?" + v.Encode()
	}
	return u
}

// ---- diff ----

func (d diffLine) Removed() bool { return d.Op == '-' }
func (d diffLine) Added() bool   { return d.Op == '+' }

type diffLine struct {
	Op   byte // ' ' unchanged, '-' removed, '+' added
	Text string
}

// maxDiffCells bounds the line diff's table; larger edits show as a
// wholesale replacement.
const maxDiffCells = 250_000

// lineDiff is a line-based diff of old into new from their longest common
// subsequence.
func lineDiff(old, new string) []diffLine {
	a, b := splitLines(old), splitLines(new)
	if len(a)*len(b) > maxDiffCells {
		var d []diffLine
		for _, l := range a {
			d = append(d, diffLine{'-', l})
		}
		for _, l := range b {
			d = append(d, diffLine{'+', l})
		}
		return d
	}
	// lcs[i][j] is the common subsequence length of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var d []diffLine
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			d = append(d, diffLine{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			d = append(d, diffLine{'-', a[i]})
			i++
		default:
			d = append(d, diffLine{'+', b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		d = append(d, diffLine{'-', a[i]})
	}
	for ; j < len(b); j++ {
		d = append(d, diffLine{'+', b[j]})
	}
	return d
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n"), "\n")
}
