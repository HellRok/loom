# Shim–daemon design for `loom mcp`

Type: grilling
Status: resolved
Blocked by: 02

## Question

Does `loom mcp` transparently proxy MCP between stdio and the daemon's HTTP endpoint (forwarding context like the working directory as headers/metadata), or does it host the MCP server itself and talk to the daemon over a private API? Should the daemon's HTTP MCP endpoint also be usable directly by clients that support HTTP MCP?

## Answer

Resolved by grilling (2026-10-22).

- **Shim**: `loom mcp` is an SDK-level proxy: stdio `mcp.Server` facing the agent, HTTP `mcp.Client` facing the daemon. No tool logic in the shim; no private API. The daemon's `/mcp` endpoint is the only API.
- **Tool list**: shim forwards `tools/list` and every tool call live, no cache. Tool definitions live only in the daemon.
- **Context**: shim adds only `X-Loom-Cwd` (raw cwd); the daemon does worktree/Project resolution. Shim reads daemon version from `/healthz`; mismatch handling is deferred to the upgrade fog.
- **Direct HTTP**: `127.0.0.1:<port>/mcp` is supported and documented as secondary; calls must pass `project`. `loom mcp` is the recommended setup.
- **Failure**: the shim never retries a call. Any forwarded-call failure (refused, reset, timeout) becomes an MCP tool error saying the outcome may be unknown, telling the agent to verify before redoing, and naming the daemon log path. Rationale: agents should check whether a non-idempotent call landed rather than rely on silent retries.
- **Recovery**: the next call re-runs detect-and-spawn before forwarding, so a dead daemon comes back without restarting the shim.

## Comments

- From [How a session binds to a Project](05-session-project-binding.md): the daemon resolves the Project from forwarded cwd; a request without a cwd header must pass the `project` argument. Relevant if the HTTP endpoint is usable directly.
