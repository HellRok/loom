package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/HellRok/loom/internal/store"
)

var (
	numberRefRE = regexp.MustCompile(`^#?([0-9]+)$`)
	slugRefRE   = regexp.MustCompile(`^([a-z0-9-]+)#([0-9]+)$`)
	urlPathRE   = regexp.MustCompile(`^/([a-z0-9-]+)/issues/([0-9]+)/?$`)
)

// ref names an Issue: its number, and its Project's Slug when the ref
// carries one.
type ref struct {
	Slug string
	N    int64
}

// parseRef reads `#N`, `N`, `slug#N` or this daemon's issue URL.
func (h *handlers) parseRef(s string) (ref, error) {
	var slug, num string
	if m := numberRefRE.FindStringSubmatch(s); m != nil {
		num = m[1]
	} else if m := slugRefRE.FindStringSubmatch(s); m != nil {
		slug, num = m[1], m[2]
	} else if u, err := url.Parse(s); err == nil && u.Scheme == "http" && h.ourHost(u.Host) {
		if m := urlPathRE.FindStringSubmatch(u.Path); m != nil {
			slug, num = m[1], m[2]
		}
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 1 {
		return ref{}, fail(codeBadRef, fmt.Sprintf("Can't read %q as an Issue ref. Use #N, N, slug#N or %s/<slug>/issues/<n>.", s, h.baseURL()), nil)
	}
	return ref{Slug: slug, N: n}, nil
}

func (h *handlers) ourHost(host string) bool {
	p := strconv.Itoa(h.port)
	return host == "127.0.0.1:"+p || host == "localhost:"+p
}

func (h *handlers) baseURL() string { return fmt.Sprintf("http://127.0.0.1:%d", h.port) }

func (h *handlers) issueURL(slug string, n int64) string {
	return fmt.Sprintf("%s/%s/issues/%d", h.baseURL(), slug, n)
}

func refString(n int64) string { return "#" + strconv.FormatInt(n, 10) }

// resolveRef parses s and picks its Project: the one the ref names, else the
// call's.
func (h *handlers) resolveRef(ctx context.Context, c call, s string) (store.Project, int64, error) {
	r, err := h.parseRef(s)
	if err != nil {
		return store.Project{}, 0, err
	}
	if r.Slug != "" {
		c.ProjectSlug = r.Slug
	}
	p, err := h.resolveProject(ctx, c)
	return p, r.N, err
}

// resolveIssue resolves s to an existing Issue.
func (h *handlers) resolveIssue(ctx context.Context, c call, s string) (store.Project, store.Issue, error) {
	p, n, err := h.resolveRef(ctx, c, s)
	if err != nil {
		return p, store.Issue{}, err
	}
	i, err := h.store.Issue(ctx, p.Slug, n)
	if errors.Is(err, store.ErrNotFound) {
		return p, i, notFound(p.Slug, n)
	}
	return p, i, err
}

func notFound(slug string, n int64) error {
	return fail(codeNotFound, fmt.Sprintf("%s has no Issue #%d.", slug, n), nil)
}
