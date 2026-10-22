# 01: Projects over MCP from `loom serve`

Spec: [Loom v1](../../loom/spec.md)

**What to build:** The tracer bullet. Running `loom serve` starts the daemon. It sets up its Loom home, takes the lock, runs migrations, then binds `127.0.0.1:<port>`. A direct-HTTP MCP client can then create, list and delete Projects. Every call is resolved to a Project and a Session from its request context, and resolution failures come back as coded errors the agent can act on.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `loom serve` uses `$LOOM_HOME` (default `~/.loom`) and an optional `config.toml` (`port` default 7337, `assignee` from git `user.name` then `$USER`), and logs its effective config at startup
- [ ] Startup order: exclusive non-blocking flock on `loom.lock` (pid written into it), open SQLite (WAL, foreign keys, busy timeout, immediate txlock), run embedded goose migrations, then bind; losing the lock or the bind exits quietly
- [ ] A DB schema newer than the binary is refused, with both versions logged; a failed migration leaves the DB at the last good version and exits non-zero
- [ ] `/healthz` identifies Loom and returns version, pid and Loom home
- [ ] Requests whose `Host` isn't `127.0.0.1:<port>` or `localhost:<port>` are rejected
- [ ] SIGTERM/SIGINT drains in-flight requests (about 5s cap), checkpoints WAL and exits
- [ ] `/mcp` is a stateless streamable-HTTP go-sdk server exposing `create_project`, `list_projects` and `delete_project {slug, confirm}`
- [ ] Session comes from `X-Loom-Session`, else a `session` argument (header wins); missing → `no_session`, not 16–256 runes → `invalid_session`; a Session record (first/last seen) is upserted best-effort
- [ ] Project resolution: an explicit `project` Slug, else `X-Loom-Cwd` mapped to the main worktree root when inside git, then the longest-prefix Root; no cwd and no `project` → error; no match → `no_project` listing Projects with their Roots
- [ ] `create_project`: inside git, Root = git root and Slug = its basename; outside git, `slug` is required and Root = cwd; Slug collisions and exact duplicate Roots are rejected; nested Roots are allowed
- [ ] `delete_project` hard-deletes, cascading to everything in the Project
- [ ] Errors use the `{error: {code, message, ...data}}` shape; outputs are structured JSON plus a JSON text copy
- [ ] Tests at the tool-handler seam against a real temp SQLite file: request-context resolution (worktree, nesting, override, missing session) and each tool's outputs and errors; golden `tools/list` checked in
