package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HellRok/loom/internal/store"
)

var slugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

type projectOut struct {
	Slug string `json:"slug"`
	Root string `json:"root"`
	// RootMissing flags a Root that no longer exists on disk, as after the
	// repo moved: the cue to call move_project instead of create_project.
	RootMissing bool `json:"root_missing,omitempty"`
	// Standalone marks a Project whose skill config lives in Loom, not in
	// the repo.
	Standalone bool `json:"standalone,omitempty"`
}

func toProjectOut(p store.Project) projectOut {
	_, err := os.Stat(p.Root)
	return projectOut{Slug: p.Slug, Root: p.Root, RootMissing: errors.Is(err, os.ErrNotExist), Standalone: p.Standalone}
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
	// An existing Project here wins over every Slug complaint, so callers
	// can tell "already set up" from "needs a Slug".
	if err := h.rootTaken(ctx, root); err != nil {
		return projectOut{}, err
	}
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
		// Lost a race with another create_project at this Root.
		if err := h.rootTaken(ctx, root); err != nil {
			return projectOut{}, err
		}
		return projectOut{}, h.projectError(ctx, codeRootTaken, fmt.Sprintf("%s is already a Project's Root.", root))
	case err == nil:
		return toProjectOut(p), nil
	default:
		return projectOut{}, err
	}
}

// rootTaken returns a root_taken error, naming the Project's `slug` and
// listing every Project, if one already has root as its Root; else nil.
func (h *handlers) rootTaken(ctx context.Context, root string) error {
	ps, err := h.store.Projects(ctx)
	if err != nil {
		return err
	}
	for _, p := range ps {
		if p.Root == root {
			return fail(codeRootTaken, fmt.Sprintf("%s is already a Project's Root.", root),
				map[string]any{"slug": p.Slug, "projects": toProjectOuts(ps)})
		}
	}
	return nil
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

type moveProjectIn struct {
	Common
}

// moveProject rebinds a Project's Root to where the caller is working. The
// Project must be named or contain the caller, which a moved repo's old Root
// no longer does, so agents pass `project`.
func (h *handlers) moveProject(ctx context.Context, c call, in moveProjectIn) (projectOut, error) {
	if c.Cwd == "" {
		return projectOut{}, fail(codeNoCwd, "move_project rebinds a Project to the caller's working directory, but none was sent (X-Loom-Cwd). Use `loom mcp`, or send the header.", nil)
	}
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return projectOut{}, err
	}
	root, _ := c.location(ctx)
	moved, err := h.store.MoveProject(ctx, p.Slug, root)
	switch {
	case errors.Is(err, store.ErrRootTaken):
		return projectOut{}, h.projectError(ctx, codeRootTaken, fmt.Sprintf("%s is already another Project's Root.", root))
	case errors.Is(err, store.ErrNotFound):
		// The Project can vanish between the lookup and the move.
		return projectOut{}, h.unknownProject(ctx, p.Slug)
	case err != nil:
		return projectOut{}, err
	}
	return toProjectOut(moved), nil
}

type setStandaloneIn struct {
	Common
	Standalone bool `json:"standalone" jsonschema:"true to mark the Project Standalone, false to clear it"`
}

type setStandaloneOut struct {
	projectOut
	Already bool `json:"already"`
}

// setStandalone marks the Project Standalone or clears it; `loom init` calls
// it so the flag is set through the same tools as everything else.
func (h *handlers) setStandalone(ctx context.Context, c call, in setStandaloneIn) (setStandaloneOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return setStandaloneOut{}, err
	}
	already, err := h.store.SetStandalone(ctx, p.Slug, in.Standalone)
	if errors.Is(err, store.ErrNotFound) {
		// The Project can vanish between the lookup and the update.
		return setStandaloneOut{}, h.unknownProject(ctx, p.Slug)
	} else if err != nil {
		return setStandaloneOut{}, err
	}
	p.Standalone = in.Standalone
	return setStandaloneOut{projectOut: toProjectOut(p), Already: already}, nil
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
