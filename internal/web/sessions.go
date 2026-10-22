package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HellRok/loom/internal/store"
)

// shortLen is how many trailing characters of a Session id the UI shows.
const shortLen = 8

// HolderView is a Session as shown wherever a claim appears: its last 8
// characters (the full id on hover) linking to its page, and its idle time.
type HolderView struct {
	ID, Short, URL string
	Idle           string // "idle 5m", or "never seen"
}

func sessionURL(id string) string { return "/sessions/" + url.PathEscape(id) }

func shortID(id string) string {
	r := []rune(id)
	if len(r) > shortLen {
		r = r[len(r)-shortLen:]
	}
	return string(r)
}

// idle is how long a Session has been quiet, in the coarse units of ago.
func idle(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "idle <1m"
	case d < time.Hour:
		return fmt.Sprintf("idle %dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("idle %dh", int(d.Hours()))
	}
	return fmt.Sprintf("idle %dd", int(d.Hours()/24))
}

// holders looks Sessions up by id for one request.
type holders struct {
	byID map[string]store.Session
	now  time.Time
}

func (s *server) holders(ctx context.Context) (holders, error) {
	all, err := s.Store.Sessions(ctx)
	if err != nil {
		return holders{}, err
	}
	h := holders{byID: make(map[string]store.Session, len(all)), now: s.now()}
	for _, sess := range all {
		h.byID[sess.ID] = sess
	}
	return h, nil
}

// view is the HolderView of Session id, or nil when id is empty.
func (h holders) view(id string) *HolderView {
	if id == "" {
		return nil
	}
	v := &HolderView{ID: id, Short: shortID(id), URL: sessionURL(id), Idle: "never seen"}
	if sess, ok := h.byID[id]; ok {
		v.Idle = idle(h.now.Sub(sess.LastSeen))
	}
	return v
}

// ---- pages ----

type sessionRow struct {
	HolderView
	FirstSeen, LastSeen time.Time
}

// sessionsRoute serves /sessions and /sessions/<id>. It means no Project can
// have the Slug "sessions".
func (s *server) sessionsRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutPrefix(r.URL.Path, "/sessions/")
	if !ok || id == "" {
		if r.URL.Path != "/sessions" && r.URL.Path != "/sessions/" {
			s.fail(w, r, store.ErrNotFound)
			return
		}
		s.sessionsPage(w, r)
		return
	}
	s.sessionPage(w, r, id)
}

func (s *server) sessionsPage(w http.ResponseWriter, r *http.Request) {
	h, err := s.holders(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var rows []sessionRow
	for _, sess := range h.byID {
		rows = append(rows, sessionRow{HolderView: *h.view(sess.ID), FirstSeen: sess.FirstSeen, LastSeen: sess.LastSeen})
	}
	slices.SortFunc(rows, func(a, b sessionRow) int {
		if c := b.LastSeen.Compare(a.LastSeen); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	s.render(w, r, http.StatusOK, "sessions", map[string]any{"Title": "Sessions", "Sessions": rows})
}

type heldIssue struct {
	store.SessionIssue
	URL string
}

type sessionEventView struct {
	store.SessionEvent
	URL string
}

func (s *server) sessionPage(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	sess, err := s.Store.SessionByID(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	held, err := s.Store.HeldBy(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	events, err := s.Store.SessionEvents(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := holders{byID: map[string]store.Session{id: sess}, now: s.now()}
	issueURL := func(slug string, n int64) string { return "/" + slug + "/issues/" + strconv.FormatInt(n, 10) }
	var hv []heldIssue
	for _, i := range held {
		hv = append(hv, heldIssue{SessionIssue: i, URL: issueURL(i.Slug, i.Number)})
	}
	var ev []sessionEventView
	for _, e := range events {
		ev = append(ev, sessionEventView{SessionEvent: e, URL: issueURL(e.Slug, e.Number)})
	}
	s.render(w, r, http.StatusOK, "session", map[string]any{
		"Title":   "Session " + shortID(id),
		"Session": sess,
		"Holder":  h.view(id),
		"Held":    hv,
		"Events":  ev,
	})
}
