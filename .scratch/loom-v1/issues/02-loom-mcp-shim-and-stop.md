# 02: `loom mcp` shim and `loom stop`

Spec: [Loom v1](../../loom/spec.md)

**What to build:** An agent harness can register `loom mcp` as a stdio MCP server and use Loom with no other setup. The shim starts the daemon if needed, reuses it if it's running, and forwards every call with the agent's cwd and a per-process Session. `loom stop` shuts the daemon down cleanly.

**Blocked by:** 01 (Projects over MCP from `loom serve`)

**Status:** done

- [x] The shim is an SDK-level stdio→HTTP proxy that forwards `tools/list` and every call live, with no tool logic and no cache
- [x] It generates one UUIDv7 per process and sends it as `X-Loom-Session` with `X-Loom-Cwd` on every call
- [x] Detect-and-spawn: probe `/healthz` (about 1s); if there's no daemon, spawn `serve` detached (setsid, stdin nil, stdout/stderr to `serve.log`) and poll `/healthz` with exponential backoff from 50ms, capped at about 5–10s; nothing but MCP ever reaches the shim's stdout
- [x] Two racing shims end up with one daemon
- [x] A forwarding failure is never retried; it becomes a tool error saying the outcome may be unknown, telling the agent to verify before redoing, and naming the log path; the next call re-runs detect-and-spawn
- [x] An older daemon is stopped (as `loom stop` does) and replaced; a newer daemon is used as is; dev builds compare the full build string
- [x] A daemon reporting a different Loom home is refused, naming both paths and suggesting `port`; a non-Loom responder on the port is an error naming the port and the config key
- [x] Daemon startup failures (newer DB, failed migration) surface as a tool error pointing at `serve.log`
- [x] `loom stop` reads the pid from `loom.lock`, confirms `/healthz` reports the same pid, sends SIGTERM and waits for the port to free; otherwise it prints `not running` and signals nothing
- [x] Unit tests for the probe and version/home decision logic; a small smoke suite against the built binary (per-test temp `LOOM_HOME` and free `port`): spawn and call a tool, a second shim reuses the daemon, `loom stop`
