// Package tools defines Loom's MCP tool surface: each tool's schema and its
// handler, request-context resolution, and error coding.
package tools

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/shim"
	"github.com/HellRok/loom/internal/store"
)

// Deps are what the tool handlers need from the daemon.
type Deps struct {
	Store   *store.Store
	Version string
	Logger  *slog.Logger
	// Port is the daemon's port, for Issue URLs.
	Port int
	// Assignee is the configured human, the author of every change.
	Assignee string
}

// handlers holds what every tool handler shares.
type handlers struct {
	store    *store.Store
	logger   *slog.Logger
	port     int
	assignee string
}

// NewServer returns an MCP server exposing every Loom tool.
func NewServer(d Deps) *mcp.Server {
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	version := d.Version
	if version == "" {
		version = "dev"
	}
	h := &handlers{store: d.Store, logger: logger, port: d.Port, assignee: d.Assignee}
	// The SDK logs every stateless request's connect and disconnect at Info.
	sdkLogger := slog.New(minLevel{logger.Handler(), slog.LevelWarn})
	s := mcp.NewServer(&mcp.Implementation{Name: "loom", Version: version}, &mcp.ServerOptions{Logger: sdkLogger})

	add(s, h, &mcp.Tool{
		Name:        "create_project",
		Description: "Register a Project: inside git, the main worktree's root with its folder name as the Slug; outside git, the cwd with the `slug` you pass.",
	}, h.createProject)
	add(s, h, &mcp.Tool{
		Name:        "list_projects",
		Description: "All Projects with their Roots.",
	}, h.listProjects)
	add(s, h, &mcp.Tool{
		Name:        "delete_project",
		Description: "Hard-delete a Project and everything in it, freeing its Slug and Root.",
	}, h.deleteProject)
	add(s, h, &mcp.Tool{
		Name:        "move_project",
		Description: "Rebind a Project's Root to your working directory (the git root, or the cwd outside git) after its repo moved, keeping its Slug and Issues. Pass `project`. Prefer this over create_project when no_project flags a Project as root_missing.",
	}, h.moveProject)

	add(s, h, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create an Issue, or a Map with kind: map. Returns its ref and URL.",
	}, h.createIssue)
	add(s, h, &mcp.Tool{
		Name:        "view_issue",
		Description: "One Issue, open or closed, with its body and comments (and, on request, its Event history).",
	}, h.viewIssue)
	add(s, h, &mcp.Tool{
		Name:        "list_issues",
		Description: "List a Project's Issues, oldest first, with the total matching. Filters combine with AND.",
	}, h.listIssues)
	add(s, h, &mcp.Tool{
		Name:        "edit_issue",
		Description: "Replace an Issue's title and/or body. Pass the version you read: if the Issue changed since, the edit is refused with the current text instead of overwriting it.",
	}, h.editIssue)
	add(s, h, &mcp.Tool{
		Name:        "append_to_section",
		Description: "Append text to the end of the section under a Markdown heading in an Issue's body. Needs no version, so concurrent sessions can all append safely.",
	}, h.appendToSection)
	add(s, h, &mcp.Tool{
		Name:        "comment",
		Description: "Add an immutable Markdown comment to an Issue.",
	}, h.comment)
	add(s, h, &mcp.Tool{
		Name:        "close",
		Description: "Close an Issue, optionally posting a comment in the same step. Closing a closed Issue changes nothing and returns already: true.",
	}, h.closeIssue)
	add(s, h, &mcp.Tool{
		Name:        "reopen",
		Description: "Reopen a closed Issue. Reopening an open Issue changes nothing and returns already: true.",
	}, h.reopenIssue)
	add(s, h, &mcp.Tool{
		Name:        "delete_issue",
		Description: "Hard-delete an Issue with its comments and history. Its #N is never reused.",
	}, h.deleteIssue)

	add(s, h, &mcp.Tool{
		Name:        "label",
		Description: "Add and/or remove labels on an Issue as one atomic step. Every label must be defined in the Project, else nothing changes and the error lists the valid ones.",
	}, h.label)
	add(s, h, &mcp.Tool{
		Name:        "list_labels",
		Description: "The Project's labels with their colour and how many Issues carry each.",
	}, h.listLabels)
	add(s, h, &mcp.Tool{
		Name:        "create_label",
		Description: "Define a label in the Project. Creating one that exists changes nothing and returns already: true. Labels can't be renamed.",
	}, h.createLabel)
	add(s, h, &mcp.Tool{
		Name:        "delete_label",
		Description: "Delete a label no Issue carries. If Issues carry it, the error lists them.",
	}, h.deleteLabel)

	add(s, h, &mcp.Tool{
		Name:        "list_sessions",
		Description: "Every agent Session Loom has recorded, with when it was first and last seen. Calls from `loom` subcommands aren't recorded.",
	}, h.listSessions)
	return s
}

// withCommon is satisfied by every tool's input, through its embedded Common.
type withCommon interface{ common() Common }

// add registers a tool whose handler runs after the call's Session and cwd
// are resolved and the Session is recorded.
func add[In withCommon, Out any](s *mcp.Server, h *handlers, tool *mcp.Tool, handle func(context.Context, call, In) (Out, error)) {
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		var hdr http.Header
		if req.Extra != nil {
			hdr = req.Extra.Header
		}
		c, err := newCall(hdr, in.common())
		if err != nil {
			return nil, zero, err
		}
		if !strings.HasPrefix(c.Session, shim.CLISessionPrefix) {
			if err := h.store.TouchSession(ctx, c.Session); err != nil {
				h.logger.Warn("recording session", "session", c.Session, "err", err)
			}
		}
		out, err := handle(ctx, c, in)
		if err != nil {
			te, ok := errors.AsType[*ToolError](err)
			if !ok {
				h.logger.Error("tool failed", "tool", tool.Name, "err", err)
				te = fail(codeInternal, err.Error(), nil)
			}
			return nil, zero, te
		}
		return nil, out, nil
	})
}

// minLevel drops records below min.
type minLevel struct {
	slog.Handler
	min slog.Level
}

func (h minLevel) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.min && h.Handler.Enabled(ctx, l)
}

func (h minLevel) WithAttrs(as []slog.Attr) slog.Handler {
	return minLevel{h.Handler.WithAttrs(as), h.min}
}

func (h minLevel) WithGroup(name string) slog.Handler {
	return minLevel{h.Handler.WithGroup(name), h.min}
}
