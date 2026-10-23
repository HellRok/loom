package web

import (
	"net/http"
	"strings"

	"github.com/HellRok/loom/internal/store"
)

// glossaryPage serves /<slug>/glossary: the Project's Glossary rendered as
// Markdown, or an empty state if none has been written. An unknown Project
// is a 404, and so is a plain one: only a Standalone Project's Glossary is
// shown, though a stored one is kept. The Glossary fills the space of the list
// and Issue panes, beside the Project's navigation, which highlights it
// instead of a View. It is read-only: agents write the Glossary through the MCP
// tools.
func (s *server) glossaryPage(w http.ResponseWriter, r *http.Request, slug string) {
	p, err := s.Store.ProjectBySlug(r.Context(), slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !p.Standalone {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	g, err := s.Store.Glossary(r.Context(), slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data, _, err := s.projectData(r.Context(), slug, p, parseFilters(nil), 0)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	views := data["Views"].([]viewLink)
	for i := range views {
		views[i].Active = false
	}
	data["Title"] = "Glossary · " + slug
	data["GlossaryActive"] = true
	data["Empty"] = strings.TrimSpace(g.Body) == ""
	data["Body"] = renderMarkdown(slug, g.Body)
	s.render(w, r, http.StatusOK, "glossary", data)
}
