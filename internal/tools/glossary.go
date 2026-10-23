package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/HellRok/loom/internal/store"
)

type glossaryOut struct {
	Body    string `json:"body"`
	Version int64  `json:"version"`
}

type viewGlossaryIn struct {
	Common
}

func (h *handlers) viewGlossary(ctx context.Context, c call, in viewGlossaryIn) (glossaryOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return glossaryOut{}, err
	}
	g, err := h.store.Glossary(ctx, p.Slug)
	if errors.Is(err, store.ErrNotFound) {
		// The Project can vanish between the lookup and the read.
		return glossaryOut{}, h.unknownProject(ctx, p.Slug)
	} else if err != nil {
		return glossaryOut{}, err
	}
	return glossaryOut{Body: g.Body, Version: g.Version}, nil
}

type editGlossaryIn struct {
	Common
	Version int64  `json:"version" jsonschema:"The version you last read; the edit is refused if the Glossary has changed since"`
	Body    string `json:"body" jsonschema:"The whole new Markdown Glossary, replacing the old one"`
}

type editGlossaryOut struct {
	Version int64 `json:"version"`
}

func (h *handlers) editGlossary(ctx context.Context, c call, in editGlossaryIn) (editGlossaryOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return editGlossaryOut{}, err
	}
	g, err := h.store.EditGlossary(ctx, p.Slug, in.Version, in.Body)
	if conflict, ok := errors.AsType[*store.GlossaryConflictError](err); ok {
		cur := conflict.Current
		return editGlossaryOut{}, fail(codeVersionConflict,
			fmt.Sprintf("The Glossary changed since you read it; it is now at version %d. Re-apply your change to the current body and retry with version %d.", cur.Version, cur.Version),
			map[string]any{"current": map[string]any{"version": cur.Version, "body": cur.Body}})
	}
	if errors.Is(err, store.ErrNotFound) {
		return editGlossaryOut{}, h.unknownProject(ctx, p.Slug)
	} else if err != nil {
		return editGlossaryOut{}, err
	}
	return editGlossaryOut{Version: g.Version}, nil
}
