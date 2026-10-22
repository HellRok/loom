package shim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startTimeout caps how long a spawned daemon has to answer /healthz.
const startTimeout = 8 * time.Second

// The codes of the tool errors the shim itself returns, in the daemon's
// {error: {code, message, ...}} shape.
const (
	codeDaemonUnavailable = "daemon_unavailable"
	codeOutcomeUnknown    = "outcome_unknown"
)

// Shim proxies one agent's stdio MCP session to the daemon, as one Session.
type Shim struct {
	Target
	session string
	// spawn starts `loom serve` and returns a channel that yields its exit.
	spawn func(context.Context) (<-chan error, error)
}

// New returns a shim for t with a fresh UUIDv7 Session.
func New(t Target) (*Shim, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generating a Session id: %w", err)
	}
	s := &Shim{Target: t, session: id.String()}
	s.spawn = s.spawnServe
	return s, nil
}

// Run serves the agent over stdin/stdout until it disconnects.
func (s *Shim) Run(ctx context.Context) error {
	return s.Server().Run(ctx, &mcp.StdioTransport{})
}

// Server is the agent-facing MCP server. It holds no tools of its own:
// tools/list and tools/call go to the daemon live, on every request.
func (s *Shim) Server() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "loom", Version: s.Version}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})
	srv.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch r := req.(type) {
			case *mcp.ListToolsRequest:
				return s.listTools(ctx, r.Params)
			case *mcp.CallToolRequest:
				return s.callTool(ctx, r.Params), nil
			}
			return next(ctx, method, req)
		}
	})
	return srv
}

func (s *Shim) listTools(ctx context.Context, p *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, fmt.Errorf("loom daemon unavailable: %w; see %s", err, s.LogPath())
	}
	cs, err := s.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing Loom's tools: %w; see %s", err, s.LogPath())
	}
	defer cs.Close()
	return cs.ListTools(ctx, p)
}

// callTool forwards one call. It never retries: a call that fails in
// flight may have landed, so the agent is told to check before redoing it.
func (s *Shim) callTool(ctx context.Context, p *mcp.CallToolParamsRaw) *mcp.CallToolResult {
	if err := s.ensure(ctx); err != nil {
		return s.toolError(codeDaemonUnavailable, fmt.Sprintf(
			"Loom's daemon is unavailable, so the call was not made: %v. See %s.", err, s.LogPath()))
	}
	res, err := s.forward(ctx, p)
	if err != nil {
		return s.toolError(codeOutcomeUnknown, fmt.Sprintf(
			"Lost contact with Loom's daemon during the call (%v), so the outcome is unknown: verify whether it took effect before redoing it. See %s.", err, s.LogPath()))
	}
	return res
}

func (s *Shim) forward(ctx context.Context, p *mcp.CallToolParamsRaw) (*mcp.CallToolResult, error) {
	cs, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer cs.Close()
	return cs.CallTool(ctx, &mcp.CallToolParams{Meta: p.Meta, Name: p.Name, Arguments: p.Arguments})
}

// toolError is a coded tool error in the daemon's {error: {...}} shape.
func (s *Shim) toolError(code, message string) *mcp.CallToolResult {
	b, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message, "log": s.LogPath()}})
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}

// connect opens a fresh client session to the daemon, so a restarted
// daemon is picked up without any state to reset.
func (s *Shim) connect(ctx context.Context) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "loom-mcp", Version: s.Version}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             s.baseURL() + "/mcp",
		HTTPClient:           &http.Client{Transport: loomHeaders{session: s.session}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
}

// loomHeaders sends the Session and the agent's raw cwd on every request.
type loomHeaders struct{ session string }

func (h loomHeaders) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Loom-Session", h.session)
	if cwd, err := os.Getwd(); err == nil {
		r.Header.Set("X-Loom-Cwd", cwd)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// ensure leaves a usable daemon on the port: the running one if it will
// do, else a freshly spawned one, stopping an older daemon first.
func (s *Shim) ensure(ctx context.Context) error {
	act, h, err := s.Detect(ctx)
	if err != nil {
		return err
	}
	switch act {
	case Use:
		return nil
	case Replace:
		if _, err := s.Stop(ctx, h.PID); err != nil {
			return fmt.Errorf("stopping the older daemon (version %s, pid %d): %w", h.Version, h.PID, err)
		}
	}
	return s.start(ctx)
}

// start spawns a daemon and waits for one to answer. A spawned daemon that
// exits cleanly lost a race to another, which is then waited for.
func (s *Shim) start(ctx context.Context) error {
	exited, err := s.spawn(ctx)
	if err != nil {
		return fmt.Errorf("starting the daemon: %w", err)
	}
	deadline := time.Now().Add(startTimeout)
	delay := 50 * time.Millisecond
	for {
		act, _, err := s.Detect(ctx)
		if err != nil {
			return err
		}
		if act == Use {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the daemon did not come up within %s", startTimeout)
		}
		select {
		case err := <-exited:
			if err != nil {
				return fmt.Errorf("the daemon failed to start (%w)", err)
			}
			exited = nil
		case <-time.After(delay):
			delay = min(2*delay, time.Second)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
