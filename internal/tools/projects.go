package tools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HellRok/loom/internal/store"
)

var slugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

type projectOut struct {
	Slug string `json:"slug"`
	Root string `json:"root"`
}

func toProjectOut(p store.Project) projectOut {
	return projectOut{Slug: p.Slug, Root: p.Root}
}

func toProjectOuts(ps []store.Project) []projectOut {
	out := make([]projectOut, len(ps))
	for i, p := range ps {
		out[i] = toProjectOut(p)
	}
	return out
}

type createProjectIn struct {
	Common
	Slug string `json:"slug,omitempty" jsonschema:"Defaults to the git root's folder name; required outside git. Lowercase letters, digits and hyphens."`
}

func (h *handlers) createProject(ctx context.Context, c call, in createProjectIn) (projectOut, error) {
	if c.Cwd == "" {
		return projectOut{}, fail(codeNoCwd, "create_project registers the caller's working directory, but none was sent (X-Loom-Cwd). Use `loom mcp`, or send the header.", nil)
	}
	root, inGit := c.location(ctx)
	slug := in.Slug
	if slug == "" {
		if !inGit {
			return projectOut{}, fail(codeSlugRequired, fmt.Sprintf("%s is not inside a git repository, so pass a `slug` for the new Project.", root), nil)
		}
		slug = filepath.Base(root)
	}
	if !slugRE.MatchString(slug) {
		return projectOut{}, fail(codeInvalidSlug, fmt.Sprintf("Slug %q must use only lowercase letters, digits and hyphens. Pass a `slug`.", slug), map[string]any{"suggestion": suggestSlug(slug)})
	}
	p, err := h.store.CreateProject(ctx, slug, root)
	switch {
	case errors.Is(err, store.ErrSlugTaken):
		taken, err := h.store.ProjectBySlug(ctx, slug)
		if err != nil {
			return projectOut{}, err
		}
		return projectOut{}, fail(codeSlugTaken, fmt.Sprintf("Slug %q is already used by the Project at %s. Pass a different `slug`.", slug, taken.Root), nil)
	case errors.Is(err, store.ErrRootTaken):
		return projectOut{}, h.projectError(ctx, codeRootTaken, fmt.Sprintf("%s is already a Project's Root.", root))
	case err == nil:
		return toProjectOut(p), nil
	default:
		return projectOut{}, err
	}
}

// suggestSlug turns a folder name into a valid Slug.
func suggestSlug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

type listProjectsIn struct {
	Common
}

type listProjectsOut struct {
	Projects []projectOut `json:"projects"`
}

func (h *handlers) listProjects(ctx context.Context, c call, in listProjectsIn) (listProjectsOut, error) {
	ps, err := h.store.Projects(ctx)
	if err != nil {
		return listProjectsOut{}, err
	}
	return listProjectsOut{Projects: toProjectOuts(ps)}, nil
}

type deleteProjectIn struct {
	Common
	Slug    string `json:"slug" jsonschema:"The Project to delete"`
	Confirm string `json:"confirm" jsonschema:"Must equal slug"`
}

type deleteProjectOut struct {
	Deleted string `json:"deleted"`
}

func (h *handlers) deleteProject(ctx context.Context, c call, in deleteProjectIn) (deleteProjectOut, error) {
	if _, err := h.store.ProjectBySlug(ctx, in.Slug); errors.Is(err, store.ErrNotFound) {
		return deleteProjectOut{}, h.unknownProject(ctx, in.Slug)
	} else if err != nil {
		return deleteProjectOut{}, err
	}
	if in.Confirm != in.Slug {
		return deleteProjectOut{}, fail(codeConfirmMismatch, fmt.Sprintf("Pass confirm: %q to delete that Project and every Issue in it.", in.Slug), nil)
	}
	// The Project can vanish between the lookup and the delete.
	if err := h.store.DeleteProject(ctx, in.Slug); errors.Is(err, store.ErrNotFound) {
		return deleteProjectOut{}, h.unknownProject(ctx, in.Slug)
	} else if err != nil {
		return deleteProjectOut{}, err
	}
	return deleteProjectOut{Deleted: in.Slug}, nil
}
