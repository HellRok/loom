# Go MCP SDK choice

Type: research
Status: resolved
Blocked by: 

## Question

Which Go MCP library should Loom use: the official `modelcontextprotocol/go-sdk`, `mark3labs/mcp-go`, or another? Compare: streamable HTTP server support, stdio client/server support, whether it can act as a stdio-to-HTTP proxy (for `loom mcp`), per-session context (headers/metadata passed from client), tool schema ergonomics, maturity, spec version, licence.

## Answer

Findings: branch `research/go-mcp-sdk`, file `research/go-mcp-sdk.md`.

- **Recommend** official `modelcontextprotocol/go-sdk` v1.8.0 (2026-09-14): semver since v1.0, supports MCP spec 2026-07-28 back to 2024-11-05, typed `mcp.AddTool[In, Out]` with auto schema + input validation; Apache-2.0 (new) / MIT (legacy).
- **Daemon**: `NewStreamableHTTPHandler` with `Stateless: true` (required for 2026-07-28), localhost DNS-rebinding protection on by default.
- **Shim**: SDK-level proxy (stdio `mcp.Server` facing the agent, HTTP `mcp.Client` facing the daemon, mirroring tools); cwd forwarded via a custom header (e.g. `X-Loom-Cwd`), read server-side from `req.Extra.Header`. See SDK `examples/server/proxy`.
- **Avoid** raw byte piping between transports (relies on unexported internals; 2026-07-28 needs `Mcp-Method`/`Mcp-Name` headers matching the body) and MCP Roots (deprecated in 2026-07-28).
- **Stateless protocol**: 2026-07-28 drops sessions, initialize and ping; no per-session server state.
- **Fallback**: `mark3labs/mcp-go` v1.3.0 (MIT), schema validation opt-in.
