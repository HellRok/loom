// Package web serves Loom's read-only web UI: server-rendered pages that
// htmx upgrades to pane fragments, and the vendored static assets.
//
// Every URL returns a full page, or just a pane when the request carries
// HX-Request: the pane named by HX-Target (#list-pane or #issue-pane) or,
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
	pages map[string]*template.Template
	now   func() time.Time
}

// Register adds the UI's routes to mux: /, /static/, /<slug> and
// /<slug>/issues/<n>. It claims every path no more specific pattern takes,
// so a Project can't have the Slug "static".
func Register(mux *http.ServeMux, d Deps) {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	s := &server{Deps: d, now: time.Now}
	s.pages = loadPages(s)
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // the embedded tree is fixed at build time
	}
	assets := http.StripPrefix("/static/", staticHandler(http.FileServerFS(static)))
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
		case len(parts) == 1:
			s.projectPage(w, r, parts[0], 0)
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
		"ago":  s.ago,
		"time": func(t time.Time) string { return t.Local().Format("2006-01-02 15:04") },
		"iso":  func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
	}
	base := template.Must(template.New("base").Funcs(funcs).ParseFS(templateFS, "templates/layout.html"))
	pages := map[string]*template.Template{}
	for _, name := range []string{"home", "project", "notfound"} {
		pages[name] = template.Must(template.Must(base.Clone()).ParseFS(templateFS, "templates/"+name+".html"))
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
		case "list-pane":
			name = "list-pane"
		case "issue-pane":
			name = "issue-pane"
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

// A saved view is a named filter; the first is the default.
var views = []struct {
	Key, Name string
	Filter    store.IssueFilter
}{
	{"open", "Open", store.IssueFilter{State: store.StateOpen}},
	{"maps", "Maps", store.IssueFilter{State: store.StateOpen, Kind: store.KindMap}},
	{"needs-triage", "Needs triage", store.IssueFilter{State: store.StateOpen, Labels: []string{"needs-triage"}}},
	{"ready-for-agent", "Ready for agent", store.IssueFilter{State: store.StateOpen, Labels: []string{"ready-for-agent"}}},
	{"unassigned", "Unassigned", store.IssueFilter{State: store.StateOpen, Assignee: store.AssigneeNone}},
	{"closed", "Closed", store.IssueFilter{State: store.StateClosed}},
}

// filters are the list filters carried in the URL.
type filters struct {
	View   string
	Labels []string
	Q      string
}

func parseFilters(q url.Values) filters {
	f := filters{View: views[0].Key, Labels: q["label"], Q: strings.TrimSpace(q.Get("q"))}
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
	return out
}

type viewLink struct {
	Name   string
	URL    string
	Active bool
}

type labelLink struct {
	store.Label
	URL    string
	Active bool
}

type switcherItem struct {
	Slug   string
	Open   int
	Active bool
}

type row struct {
	store.Issue
	Parent   *Ref
	Blocked  bool
	Labels   []store.Label
	URL      string
	Selected bool
}

// Status is the Issue's status: closed, blocked, claimed or open.
func (r row) Status() string { return status(r.Issue, r.Blocked) }

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
	Body     template.HTML
	Comments []commentView
	Status   string
	Query    string
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
	if _, err := s.Store.ProjectBySlug(ctx, slug); err != nil {
		s.fail(w, r, err)
		return
	}
	f := parseFilters(r.URL.Query())

	data := map[string]any{"Slug": slug, "Title": slug, "Q": f.Q, "View": f.View, "Labels": f.Labels}

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
		vl = append(vl, viewLink{Name: v.Name, URL: "/" + slug + g.query(), Active: v.Key == f.View})
	}
	data["Views"] = vl

	labels, err := s.Store.Labels(ctx, slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	colours := map[string]store.Label{}
	var ll []labelLink
	for _, l := range labels {
		colours[l.Name] = l
		ll = append(ll, labelLink{Label: l, URL: "/" + slug + f.withLabel(l.Name).query(), Active: f.hasLabel(l.Name)})
	}
	data["LabelLinks"] = ll

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
	var rows []row
	for _, i := range issues {
		if f.Q != "" && !strings.Contains(strings.ToLower(i.Title), strings.ToLower(f.Q)) {
			continue
		}
		rel, err := s.relations(ctx, slug, i.Number)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		rw := row{Issue: i, Parent: rel.Parent, Blocked: rel.blocked(),
			URL: "/" + slug + "/issues/" + strconv.FormatInt(i.Number, 10) + f.query(), Selected: i.Number == selected}
		for _, name := range byIssue[i.Number] {
			rw.Labels = append(rw.Labels, colours[name])
		}
		rows = append(rows, rw)
	}
	data["Rows"] = rows
	data["Clear"] = "/" + slug

	if selected != 0 {
		iv, err := s.issueView(ctx, slug, selected, f, colours)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data["Issue"] = iv
		data["Title"] = fmt.Sprintf("#%d %s · %s", iv.Number, iv.Title, slug)
	}
	s.render(w, r, http.StatusOK, "project", data)
}

func (s *server) issueView(ctx context.Context, slug string, n int64, f filters, colours map[string]store.Label) (issueView, error) {
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
	iv := issueView{Issue: i, Slug: slug, Blocked: rel.blocked(), Body: renderMarkdown(slug, i.Body), Query: f.query()}
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
	return iv, nil
}
