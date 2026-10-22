package tools

import (
	"encoding/json"
	"maps"
)

// Code names a ToolError so an agent can branch on it.
type Code string

// The codes Loom's tools return.
const (
	codeNoSession       Code = "no_session"
	codeInvalidSession  Code = "invalid_session"
	codeNoProject       Code = "no_project"
	codeUnknownProject  Code = "unknown_project"
	codeNoCwd           Code = "no_cwd"
	codeSlugRequired    Code = "slug_required"
	codeInvalidSlug     Code = "invalid_slug"
	codeSlugTaken       Code = "slug_taken"
	codeRootTaken       Code = "root_taken"
	codeConfirmMismatch Code = "confirm_mismatch"
	codeBadRef          Code = "bad_ref"
	codeNotFound        Code = "not_found"
	codeInvalidArgument Code = "invalid_argument"
	codeVersionConflict Code = "version_conflict"
	codeNoSection       Code = "no_section"
	codeUndefinedLabel  Code = "undefined_label"
	codeLabelInUse      Code = "label_in_use"
	codeInternal        Code = "internal"
)

// ToolError is a coded error an agent can act on. It reaches the agent as an
// MCP tool error whose text is {"error": {"code", "message", ...data}}.
type ToolError struct {
	Code    Code
	Message string
	Data    map[string]any
}

func fail(code Code, message string, data map[string]any) *ToolError {
	return &ToolError{Code: code, Message: message, Data: data}
}

func (e *ToolError) Error() string {
	body := map[string]any{}
	maps.Copy(body, e.Data)
	body["code"] = e.Code
	body["message"] = e.Message
	b, _ := json.Marshal(map[string]any{"error": body})
	return string(b)
}
