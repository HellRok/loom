package tools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/HellRok/loom/internal/store"
)

// checkLabels returns an undefined_label error for the first of names the
// Project doesn't define.
func (h *handlers) checkLabels(ctx context.Context, slug string, names []string) error {
	ls, err := h.store.Labels(ctx, slug)
	if err != nil {
		return err
	}
	valid := make([]string, len(ls))
	for k, l := range ls {
		valid[k] = l.Name
	}
	for _, name := range names {
		if !slices.Contains(valid, name) {
			return issueError(slug, 0, &store.UndefinedLabelError{Name: name, Valid: valid})
		}
	}
	return nil
}

type labelIn struct {
	Common
	Ref    string   `json:"ref" jsonschema:"The Issue: #N, N, slug#N or its URL"`
	Add    []string `json:"add,omitempty" jsonschema:"Labels to apply"`
	Remove []string `json:"remove,omitempty" jsonschema:"Labels to remove"`
}

type labelOut struct {
	Ref    string   `json:"ref"`
	Labels []string `json:"labels" jsonschema:"The Issue's labels after the change"`
}

func (h *handlers) label(ctx context.Context, c call, in labelIn) (labelOut, error) {
	p, n, err := h.resolveRef(ctx, c, in.Ref)
	if err != nil {
		return labelOut{}, err
	}
	if len(in.Add) == 0 && len(in.Remove) == 0 {
		return labelOut{}, fail(codeInvalidArgument, "Pass labels to `add`, `remove` or both.", nil)
	}
	for _, name := range in.Add {
		if slices.Contains(in.Remove, name) {
			return labelOut{}, fail(codeInvalidArgument, fmt.Sprintf("%q is in both `add` and `remove`.", name), nil)
		}
	}
	if _, err := h.store.SetLabels(ctx, p.Slug, n, in.Add, in.Remove, h.actor(c)); err != nil {
		return labelOut{}, issueError(p.Slug, n, err)
	}
	labels, err := h.store.IssueLabels(ctx, p.Slug, n)
	if err != nil {
		return labelOut{}, err
	}
	return labelOut{Ref: refString(n), Labels: labels}, nil
}

type labelInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Colour      string `json:"colour"`
	// Count is the Issues carrying the label, open and closed.
	Count int `json:"count"`
}

func toLabelInfo(l store.Label) labelInfo {
	return labelInfo{Name: l.Name, Description: l.Description, Colour: l.Colour, Count: l.Count}
}

type listLabelsIn struct{ Common }

type listLabelsOut struct {
	Labels []labelInfo `json:"labels"`
}

func (h *handlers) listLabels(ctx context.Context, c call, in listLabelsIn) (listLabelsOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return listLabelsOut{}, err
	}
	ls, err := h.store.Labels(ctx, p.Slug)
	if err != nil {
		return listLabelsOut{}, err
	}
	out := listLabelsOut{Labels: make([]labelInfo, len(ls))}
	for k, l := range ls {
		out.Labels[k] = toLabelInfo(l)
	}
	return out, nil
}

type createLabelIn struct {
	Common
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type createLabelOut struct {
	labelInfo
	Already bool `json:"already"`
}

func (h *handlers) createLabel(ctx context.Context, c call, in createLabelIn) (createLabelOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return createLabelOut{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return createLabelOut{}, fail(codeInvalidArgument, "A label needs a `name`.", nil)
	}
	l, already, err := h.store.CreateLabel(ctx, p.Slug, name, in.Description)
	if errors.Is(err, store.ErrNotFound) {
		return createLabelOut{}, h.unknownProject(ctx, p.Slug)
	}
	if err != nil {
		return createLabelOut{}, err
	}
	return createLabelOut{labelInfo: toLabelInfo(l), Already: already}, nil
}

type deleteLabelIn struct {
	Common
	Name string `json:"name"`
}

type deleteLabelOut struct {
	Deleted string `json:"deleted"`
}

func (h *handlers) deleteLabel(ctx context.Context, c call, in deleteLabelIn) (deleteLabelOut, error) {
	p, err := h.resolveProject(ctx, c)
	if err != nil {
		return deleteLabelOut{}, err
	}
	err = h.store.DeleteLabel(ctx, p.Slug, in.Name)
	if errors.Is(err, store.ErrNotFound) {
		if err := h.checkLabels(ctx, p.Slug, []string{in.Name}); err != nil {
			return deleteLabelOut{}, err
		}
		return deleteLabelOut{}, fail(codeNotFound, fmt.Sprintf("%q is already gone.", in.Name), nil)
	}
	if inUse, ok := errors.AsType[*store.LabelInUseError](err); ok {
		refs := make([]string, len(inUse.Issues))
		for k, n := range inUse.Issues {
			refs[k] = refString(n)
		}
		return deleteLabelOut{}, fail(codeLabelInUse,
			fmt.Sprintf("%q is used by %s. Remove it from them first.", in.Name, strings.Join(refs, ", ")),
			map[string]any{"issues": refs})
	}
	if err != nil {
		return deleteLabelOut{}, err
	}
	return deleteLabelOut{Deleted: in.Name}, nil
}
