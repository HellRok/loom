package web

import (
	"net/http"
	"strings"
)

// glossaryPage serves /<slug>/glossary: the Project's Glossary rendered as
// Markdown, or an empty state if none has been written. An unknown Project
// is a 404. It is read-only: agents write the Glossary through the MCP
// tools.
func (s *server) glossaryPage(w http.ResponseWriter, r *http.Request, slug string) {
	g, err := s.Store.Glossary(r.Context(), slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "glossary", map[string]any{
		"Title": "Glossary · " + slug,
		"Slug":  slug,
		"Empty": strings.TrimSpace(g.Body) == "",
		"Body":  renderMarkdown(slug, g.Body),
	})
}
