package tools

import (
	"context"
	"net/http"

	"github.com/HellRok/loom/internal/store"
)

// ResolveProject exposes request-context Project resolution to tests: it
// returns the Slug a call from cwd with the given `project` argument acts on.
// No tool resolves a Project until the Issue tools arrive; these tests then
// move to the handler seam and this export goes away.
func ResolveProject(st *store.Store, cwd, project string) (string, error) {
	hdr := http.Header{headerSession: {"test-session-0001"}}
	if cwd != "" {
		hdr.Set(headerCwd, cwd)
	}
	c, err := newCall(hdr, Common{Project: project})
	if err != nil {
		return "", err
	}
	p, err := (&handlers{store: st}).resolveProject(context.Background(), c)
	return p.Slug, err
}
