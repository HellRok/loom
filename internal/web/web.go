// Package web serves Loom's read-only web UI: server-rendered pages that
// htmx upgrades to pane fragments, and the vendored static assets.
//
// Every URL returns a full page, or just a pane when the request carries
// HX-Request: the pane named by HX-Target (#nav, #list-pane or #issue-pane) or,
// for any other target, the page's #main content.
package web

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HellRok/loom/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Ref is a short reference to another Issue.
type Ref struct {
	Number int64
	Title  string
	Closed bool
}

// Relations are an Issue's place in the hierarchy and blocking graph.
type Relations struct {
	Parent   *Ref
	Blockers []Ref
	Blocks   []Ref
	Children []Ref
}

// blocked reports whether any Blocker is still open.
func (r Relations) blocked() bool {
	for _, b := range r.Blockers {
		if !b.Closed {
			return true
		}
	}
	return false
}

// openChildren counts the Issue's children that aren't closed. An Issue other
// than a Map that has any is waiting on them, as if blocked.
func (r Relations) openChildren(i store.Issue) int {
	if i.Kind == store.KindMap {
		return 0
	}
	n := 0
	for _, c := range r.Children {
		if !c.Closed {
			n++
		}
	}
	return n
}

// waitingText says how many open children an Issue is waiting on.
func waitingText(n int) string {
	if n == 1 {
		return "waiting on 1 open child"
	}
	return "waiting on " + strconv.Itoa(n) + " open children"
}

// Graph supplies the hierarchy and blocking the pages show. It is optional:
// without one, every Issue is parentless, unblocked and childless.
type Graph interface {
	// Relations of Issue n in the Project slug.
	Relations(ctx context.Context, slug string, n int64) (Relations, error)
	// Frontier is the open, unblocked, unassigned children of Issue parent,
	// in creation order.
	Frontier(ctx context.Context, slug string, parent int64) ([]store.Issue, error)
}

// Deps are what the UI needs from the daemon.
type Deps struct {
	Store  *store.Store
	Graph  Graph // may be nil
	Logger *slog.Logger
}

type server struct {
	Deps
	pages  map[string]*template.Template
	now    func() time.Time
	broker *broker
}

// Register adds the UI's routes to mux: /, /static/, /events, /sessions,
// /<slug>, /<slug>/events, /<slug>/glossary and /<slug>/issues/<n>. It claims
// every path no more specific pattern takes, so a Project can't have the Slug
// "static" or "events" or "sessions".
//
// It also takes over the Store's one OnChange hook to publish every write to the SSE
// streams, so register once per Store. The returned function ends the open
// streams, for shutdown.
func Register(mux *http.ServeMux, d Deps) (closeStreams func()) {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	s := &server{Deps: d, now: time.Now}
	s.pages = loadPages(s)
	s.broker = newBroker()
	d.Store.OnChange(s.broker.publish)
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // the embedded tree is fixed at build time
	}
	assets := http.StripPrefix("/static/", staticHandler(http.FileServerFS(static)))
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		s.broker.serve(w, r, "")
	})
	mux.HandleFunc("GET /{slug}/events", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if _, err := s.Store.ProjectBySlug(r.Context(), slug); err != nil {
			s.fail(w, r, err)
			return
		}
		s.broker.serve(w, r, slug)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "loom: method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.URL.Path == "/":
			s.home(w, r)
		case parts[0] == "static":
			assets.ServeHTTP(w, r)
		case parts[0] == "sessions":
			s.sessionsRoute(w, r)
		case len(parts) == 1:
			s.projectPage(w, r, parts[0], 0)
		case len(parts) == 2 && parts[1] == "glossary":
			s.glossaryPage(w, r, parts[0])
		case len(parts) == 3 && parts[1] == "issues":
			n, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || n < 1 || strconv.FormatInt(n, 10) != parts[2] {
				s.fail(w, r, store.ErrNotFound)
				return
			}
			s.projectPage(w, r, parts[0], n)
		default:
			s.fail(w, r, store.ErrNotFound)
		}
	})
	return s.broker.close
}

// staticHandler lets browsers cache the pinned, versioned assets.
func staticHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}

func loadPages(s *server) map[string]*template.Template {
	funcs := template.FuncMap{
		"ago":          s.ago,
		"time":         func(t time.Time) string { return t.Local().Format("2006-01-02 15:04") },
		"colData":      func(key, name string, cards []card) column { return column{key, name, cards} },
		"shortSession": shortSession,
		"iso":          func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
	}
	base := template.Must(template.New("base").Funcs(funcs).ParseFS(templateFS, "templates/layout.html"))
	pages := map[string]*template.Template{}
	for _, name := range []string{"home", "project", "notfound", "sessions", "session", "glossary"} {
		pages[name] = template.Must(template.Must(base.Clone()).ParseFS(templateFS, "templates/"+name+".html", "templates/tabs.html"))
	}
	return pages
}

func (s *server) ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := s.now().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("2006-01-02")
}

// render writes page as a full document, or as the fragment the request
// asked for.
func (s *server) render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	tmpl := s.pages[page]
	name := "page"
	// A history restore after a cache miss wants the whole page back.
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-History-Restore-Request") == "" {
		name = "main"
		switch r.Header.Get("HX-Target") {
		case "nav":
			name = "nav"
		case "list-pane":
			name = "list-pane"
		case "issue-pane":
			name = "issue-pane"
		case "home-pane", "glossary-pane", "sessions-pane", "session-pane":
			name = r.Header.Get("HX-Target")
		}
	}
	var buf bytes.Buffer
	if tmpl.Lookup(name) == nil {
		name = "main"
	}
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.Logger.Error("rendering page", "page", page, "err", err)
		http.Error(w, "loom: rendering failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request, HX-Target, HX-History-Restore-Request")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		s.render(w, r, http.StatusNotFound, "notfound", map[string]any{"Title": "Not found"})
		return
	}
	s.Logger.Error("serving page", "path", r.URL.Path, "err", err)
	http.Error(w, "loom: internal error", http.StatusInternalServerError)
}

// relations is the Issue's Relations, empty without a Graph.
func (s *server) relations(ctx context.Context, slug string, n int64) (Relations, error) {
	if s.Graph == nil {
		return Relations{}, nil
	}
	return s.Graph.Relations(ctx, slug, n)
}

// ---- home ----

type homeMap struct {
	store.Issue
	Closed, Total int
	Percent       int
	Next          []store.Issue
}

type homeProject struct {
	store.ProjectSummary
	Maps []homeMap
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sums, err := s.Store.ProjectSummaries(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ps := make([]homeProject, 0, len(sums))
	for _, sum := range sums {
		hp := homeProject{ProjectSummary: sum}
		maps, _, err := s.Store.Issues(ctx, sum.Slug, store.IssueFilter{Kind: store.KindMap, State: store.StateOpen})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, m := range maps {
			hm := homeMap{Issue: m}
			rel, err := s.relations(ctx, sum.Slug, m.Number)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			hm.Total = len(rel.Children)
			for _, c := range rel.Children {
				if c.Closed {
					hm.Closed++
				}
			}
			if hm.Total > 0 {
				hm.Percent = hm.Closed * 100 / hm.Total
			}
			if s.Graph != nil {
				if hm.Next, err = s.Graph.Frontier(ctx, sum.Slug, m.Number); err != nil {
					s.fail(w, r, err)
					return
				}
			}
			hp.Maps = append(hp.Maps, hm)
		}
		ps = append(ps, hp)
	}
	s.render(w, r, http.StatusOK, "home", map[string]any{"Title": "Loom", "Projects": ps})
}

// ---- project ----

var (
	yes = true
	no  = false
)

// frontierFilter is the Frontier of a Project: open, unblocked, unclaimed
// Issues other than its Maps. It matches Store.Frontier's ready sort.
var frontierFilter = store.IssueFilter{State: store.StateOpen, NotKind: store.KindMap, Blocked: &no, Claimed: &no, NoOpenChildren: true}

// A saved view is a named filter; the first is the default.
var views = []struct {
	Key, Name string
	Filter    store.IssueFilter
}{
	{"maps", "Maps", store.IssueFilter{State: store.StateOpen, Kind: store.KindMap}},
	{viewFrontier, "Frontier", frontierFilter},
	{"open", "Open", store.IssueFilter{State: store.StateOpen}},
	{"in-progress", "In progress", store.IssueFilter{State: store.StateOpen, Claimed: &yes}},
	{"blocked", "Blocked", store.IssueFilter{State: store.StateOpen, Blocked: &yes}},
	{"needs-triage", "Needs triage", store.IssueFilter{State: store.StateOpen, Labels: []string{"needs-triage"}}},
	{"closed", "Closed", store.IssueFilter{State: store.StateClosed}},
}

// filters are the list filters carried in the URL.
type filters struct {
	View   string
	Labels []string
	// Untagged keeps only Issues carrying neither readiness label.
	Untagged bool
	Q        string
}

// readyForAgent and readyForHuman are the labels that say who an Issue is
// ready for.
const (
	readyForAgent = "ready-for-agent"
	readyForHuman = "ready-for-human"
)

var readyLabels = []string{readyForAgent, readyForHuman}

// viewFrontier is the key of the Frontier saved view.
const viewFrontier = "frontier"

func parseFilters(q url.Values) filters {
	f := filters{View: views[0].Key, Labels: q["label"], Untagged: q.Get("untagged") == "1", Q: strings.TrimSpace(q.Get("q"))}
	for _, v := range views {
		if q.Get("view") == v.Key {
			f.View = v.Key
		}
	}
	return f
}

// query encodes f; the default view is left out.
func (f filters) query() string {
	v := url.Values{}
	if f.View != views[0].Key {
		v.Set("view", f.View)
	}
	for _, l := range f.Labels {
		v.Add("label", l)
	}
	if f.Untagged {
		v.Set("untagged", "1")
	}
	if f.Q != "" {
		v.Set("q", f.Q)
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

func (f filters) withView(key string) filters { f.View = key; return f }

func (f filters) withLabel(name string) filters {
	var ls []string
	found := false
	for _, l := range f.Labels {
		if l == name {
			found = true
			continue
		}
		ls = append(ls, l)
	}
	if !found {
		ls = append(ls, name)
	}
	f.Labels = ls
	return f
}

func (f filters) hasLabel(name string) bool { return slices.Contains(f.Labels, name) }

func (f filters) issueFilter() store.IssueFilter {
	var out store.IssueFilter
	for _, v := range views {
		if v.Key == f.View {
			out = v.Filter
			out.Labels = append([]string(nil), v.Filter.Labels...)
		}
	}
	for _, l := range f.Labels {
		if !slices.Contains(out.Labels, l) {
			out.Labels = append(out.Labels, l)
		}
	}
	if f.Untagged {
		out.NotLabels = readyLabels
	}
	return out
}

// withLimit is f limited to n Issues; the total Issues reports is unaffected.
func withLimit(f store.IssueFilter, n int) store.IssueFilter { f.Limit = n; return f }

type viewLink struct {
	Name   string
	URL    string
	Count  int
	Active bool
}

type labelLink struct {
	store.Label
	// Count shadows Label.Count, the Project-wide figure.
	Count  int
	URL    string
	Active bool
	Zero   bool
}

// labelLinks is the sidebar's Labels. Each Count is the number of rows the
// list would show after clicking the Label: those of rows, the list as
// filtered, carrying an inactive Label, and the whole list without an active
// one, which a click removes. Every Label is listed, so the sidebar doesn't
// jump between views.
func (s *server) labelLinks(ctx context.Context, slug string, f filters, labels []store.Label, rows []row) ([]labelLink, error) {
	counts := map[string]int{}
	for _, rw := range rows {
		for _, l := range rw.Labels {
			counts[l.Name]++
		}
	}
	var ll []labelLink
	for _, l := range labels {
		n := counts[l.Name]
		if f.hasLabel(l.Name) {
			issues, _, err := s.Store.Issues(ctx, slug, f.withLabel(l.Name).issueFilter())
			if err != nil {
				return nil, err
			}
			n = 0
			for _, i := range issues {
				if f.matchesQ(i) {
					n++
				}
			}
		}
		ll = append(ll, labelLink{Label: l, Count: n, Zero: n == 0 && !f.hasLabel(l.Name), URL: "/" + slug + f.withLabel(l.Name).query(), Active: f.hasLabel(l.Name)})
	}
	return ll, nil
}

// countPart is one piece of the Frontier's count line.
type countPart struct {
	Text string
	URL  string
}

// notReady is an open ready-for-agent Issue that is not on the Frontier, and
// why.
type notReady struct {
	row
	Blockers []int64
}

// frontierView is what the Frontier view shows beyond its rows.
type frontierView struct {
	Counts   []countPart
	NotReady []notReady
}

type switcherItem struct {
	Slug   string
	Open   int
	Active bool
}

type row struct {
	store.Issue
	Parent  *Ref
	Blocked bool
	// OpenChildren counts the children it is waiting on, unless it is a Map.
	OpenChildren int
	Labels       []store.Label
	URL          string
	Selected     bool
	Holder       *HolderView // the claiming Session, if any
}

// Status is the Issue's status: closed, blocked, claimed or open.
func (r row) Status() string { return status(r.Issue, r.Blocked) }

// Waiting says what the row is waiting on, if it has open children.
func (r row) Waiting() string {
	if r.OpenChildren == 0 {
		return ""
	}
	return waitingText(r.OpenChildren)
}

// Glyph is the symbol shown for Status.
func (r row) Glyph() string { return glyphs[r.Status()] }

var glyphs = map[string]string{"closed": "✓", "blocked": "⊘", "claimed": "●", "open": "○"}

func status(i store.Issue, blocked bool) string {
	switch {
	case i.State == store.StateClosed:
		return "closed"
	case blocked:
		return "blocked"
	case i.Assignee != "":
		return "claimed"
	}
	return "open"
}

// Waiting says what the Issue is waiting on, if it has open children.
func (v issueView) Waiting() string {
	if v.OpenChildren == 0 {
		return ""
	}
	return waitingText(v.OpenChildren)
}

// issueView is the Issue pane's data.
type issueView struct {
	store.Issue
	Slug     string
	Labels   []store.Label
	Parent   *refLink
	Blockers []refLink
	Blocks   []refLink
	Children []refLink
	Blocked  bool
	// OpenChildren counts the children it is waiting on, unless it is a Map.
	OpenChildren int
	Body         template.HTML
	Comments     []commentView
	Status       string
	Query        string
	Holder       *HolderView // the claiming Session, if any
	Tab          string      // overview, history, and for a Map board and decided
	Tabs         []tabLink
	Self         string // this pane's own URL, tab included
	Board        board
	Decided      []decision
	History      []eventView
}

// refLink is a Ref with the URL of its Issue page, keeping the list filters.
type refLink struct {
	Ref
	URL string
}

type commentView struct {
	store.Comment
	Body template.HTML
}

// projectPage renders the three-pane page of Project slug, with Issue
// selected shown on the right when it isn't 0.
func (s *server) projectPage(w http.ResponseWriter, r *http.Request, slug string, selected int64) {
	ctx := r.Context()
	proj, err := s.Store.ProjectBySlug(ctx, slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := parseFilters(r.URL.Query())

	data := map[string]any{"Slug": slug, "Standalone": proj.Standalone, "Title": slug, "Q": f.Q, "View": f.View, "DefaultView": views[0].Key, "Labels": f.Labels, "Untagged": f.Untagged}

	sums, err := s.Store.ProjectSummaries(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var sw []switcherItem
	for _, p := range sums {
		sw = append(sw, switcherItem{Slug: p.Slug, Open: p.Open, Active: p.Slug == slug})
	}
	data["Switcher"] = sw

	var vl []viewLink
	for _, v := range views {
		g := filters{View: v.Key, Q: f.Q}
		_, n, err := s.Store.Issues(ctx, slug, withLimit(v.Filter, 1))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		vl = append(vl, viewLink{Name: v.Name, URL: "/" + slug + g.query(), Count: n, Active: v.Key == f.View})
	}
	data["Views"] = vl

	labels, err := s.Store.Labels(ctx, slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	colours := map[string]store.Label{}
	for _, l := range labels {
		colours[l.Name] = l
	}

	issues, _, err := s.Store.Issues(ctx, slug, f.issueFilter())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byIssue, err := s.Store.LabelsByIssue(ctx, slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hs, err := s.holders(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var rows []row
	for _, i := range issues {
		if !f.matchesQ(i) {
			continue
		}
		rw, err := s.listRow(ctx, slug, i, f, selected, colours, byIssue, hs)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		rows = append(rows, rw)
	}
	data["Rows"] = rows
	if data["LabelLinks"], err = s.labelLinks(ctx, slug, f, labels, rows); err != nil {
		s.fail(w, r, err)
		return
	}
	data["Clear"] = "/" + slug

	if f.View == viewFrontier {
		fv, err := s.frontierView(ctx, slug, f, selected, colours, byIssue, hs)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data["Frontier"] = fv
	}

	if selected != 0 {
		iv, err := s.issueView(ctx, slug, selected, f, r.URL.Query().Get("tab"), colours)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data["Issue"] = iv
		data["Title"] = fmt.Sprintf("#%d %s · %s", iv.Number, iv.Title, slug)
	}
	s.render(w, r, http.StatusOK, "project", data)
}

func (f filters) matchesQ(i store.Issue) bool {
	return f.Q == "" || strings.Contains(strings.ToLower(i.Title), strings.ToLower(f.Q))
}

// listRow is Issue i as a row of the list pane.
func (s *server) listRow(ctx context.Context, slug string, i store.Issue, f filters, selected int64, colours map[string]store.Label, byIssue map[int64][]string, hs holders) (row, error) {
	rel, err := s.relations(ctx, slug, i.Number)
	if err != nil {
		return row{}, err
	}
	open := rel.openChildren(i)
	rw := row{Issue: i, Parent: rel.Parent, Blocked: rel.blocked() || open > 0, OpenChildren: open,
		URL: "/" + slug + "/issues/" + strconv.FormatInt(i.Number, 10) + f.query(), Selected: i.Number == selected,
		Holder: hs.view(i.HolderSession)}
	for _, name := range byIssue[i.Number] {
		rw.Labels = append(rw.Labels, colours[name])
	}
	return rw, nil
}

// frontierView counts the Frontier by readiness label and lists the open
// ready-for-agent Issues that are tagged ready but are blocked or claimed.
func (s *server) frontierView(ctx context.Context, slug string, f filters, selected int64, colours map[string]store.Label, byIssue map[int64][]string, hs holders) (frontierView, error) {
	count := func(mod func(*store.IssueFilter)) (int, error) {
		sf := withLimit(frontierFilter, 1)
		mod(&sf)
		_, n, err := s.Store.Issues(ctx, slug, sf)
		return n, err
	}
	base := filters{View: viewFrontier, Q: f.Q}
	var fv frontierView
	ready, err := count(func(*store.IssueFilter) {})
	if err != nil {
		return fv, err
	}
	fv.Counts = append(fv.Counts, countPart{Text: strconv.Itoa(ready) + " ready", URL: "/" + slug + base.query()})
	for _, l := range readyLabels {
		n, err := count(func(sf *store.IssueFilter) { sf.Labels = []string{l} })
		if err != nil {
			return fv, err
		}
		fv.Counts = append(fv.Counts, countPart{Text: strconv.Itoa(n) + " " + l, URL: "/" + slug + base.withLabel(l).query()})
	}
	n, err := count(func(sf *store.IssueFilter) { sf.NotLabels = readyLabels })
	if err != nil {
		return fv, err
	}
	untagged := base
	untagged.Untagged = true
	fv.Counts = append(fv.Counts, countPart{Text: strconv.Itoa(n) + " untagged", URL: "/" + slug + untagged.query()})

	tagged := f.issueFilter()
	tagged.Blocked, tagged.Claimed, tagged.NoOpenChildren = nil, nil, false
	tagged.Labels = append([]string{readyForAgent}, withoutLabel(tagged.Labels, readyForAgent)...)
	cands, _, err := s.Store.Issues(ctx, slug, tagged)
	if err != nil {
		return fv, err
	}
	for _, i := range cands {
		if !f.matchesQ(i) {
			continue
		}
		open, err := s.Store.OpenBlockers(ctx, slug, i.Number)
		if err != nil {
			return fv, err
		}
		rw, err := s.listRow(ctx, slug, i, f, selected, colours, byIssue, hs)
		if err != nil {
			return fv, err
		}
		if len(open) == 0 && rw.OpenChildren == 0 && !i.Claimed() {
			continue
		}
		rw.Blocked = len(open) > 0 || rw.OpenChildren > 0
		nr := notReady{row: rw}
		for _, b := range open {
			nr.Blockers = append(nr.Blockers, b.Number)
		}
		fv.NotReady = append(fv.NotReady, nr)
	}
	return fv, nil
}

func withoutLabel(ls []string, name string) []string {
	var out []string
	for _, l := range ls {
		if l != name {
			out = append(out, l)
		}
	}
	return out
}

func (s *server) issueView(ctx context.Context, slug string, n int64, f filters, tab string, colours map[string]store.Label) (issueView, error) {
	i, err := s.Store.Issue(ctx, slug, n)
	if err != nil {
		return issueView{}, err
	}
	names, err := s.Store.IssueLabels(ctx, slug, n)
	if err != nil {
		return issueView{}, err
	}
	comments, err := s.Store.Comments(ctx, slug, n)
	if err != nil {
		return issueView{}, err
	}
	rel, err := s.relations(ctx, slug, n)
	if err != nil {
		return issueView{}, err
	}
	open := rel.openChildren(i)
	iv := issueView{Issue: i, Slug: slug, Blocked: rel.blocked() || open > 0, OpenChildren: open, Body: renderMarkdown(slug, i.Body), Query: f.query()}
	link := func(r Ref) refLink {
		return refLink{Ref: r, URL: "/" + slug + "/issues/" + strconv.FormatInt(r.Number, 10) + iv.Query}
	}
	links := func(rs []Ref) []refLink {
		var out []refLink
		for _, r := range rs {
			out = append(out, link(r))
		}
		return out
	}
	if rel.Parent != nil {
		p := link(*rel.Parent)
		iv.Parent = &p
	}
	iv.Blockers, iv.Blocks, iv.Children = links(rel.Blockers), links(rel.Blocks), links(rel.Children)
	for _, name := range names {
		iv.Labels = append(iv.Labels, colours[name])
	}
	for _, c := range comments {
		iv.Comments = append(iv.Comments, commentView{Comment: c, Body: renderMarkdown(slug, c.Body)})
	}
	iv.Status = status(i, iv.Blocked)
	hs, err := s.holders(ctx)
	if err != nil {
		return issueView{}, err
	}
	iv.Holder = hs.view(i.HolderSession)
	if err := s.loadTab(ctx, &iv, tab, f); err != nil {
		return issueView{}, err
	}
	return iv, nil
}
