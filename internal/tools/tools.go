// Package tools defines Loom's MCP tool surface: each tool's schema and its
// handler, request-context resolution, and error coding.
package tools

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/store"
)

// Deps are what the tool handlers need from the daemon.
type Deps struct {
	Store   *store.Store
	Version string
	Logger  *slog.Logger
}

// handlers holds what every tool handler shares.
type handlers struct {
	store  *store.Store
	logger *slog.Logger
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
	h := &handlers{store: d.Store, logger: logger}
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
		if err := h.store.TouchSession(ctx, c.Session); err != nil {
			h.logger.Warn("recording session", "session", c.Session, "err", err)
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
