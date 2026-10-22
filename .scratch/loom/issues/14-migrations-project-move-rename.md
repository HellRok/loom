# Schema upgrades and moving or renaming Projects

Type: grilling
Status: resolved
Assignee: Sean Earle
Blocked by: 

## Question

How does Loom's data survive change? Schema: with `goose` migrations embedded (see [Go SQLite driver choice](03-sqlite-driver.md)), when do migrations run (daemon start only?), is a downgrade (older binary on a newer DB) refused, and is the DB backed up first? Projects: when a repo moves, how is its Root rebound (a tool? a CLI command? detected automatically?), and can a Slug be changed, given that `slug#N` refs and `Loom: N` commit trailers may already point at it?

## Answer

Resolved by grilling (2026-10-22).

- **When**: goose migrations run at daemon start, after taking the flock and before binding the port, so `/healthz` never answers from a half-migrated DB. No `loom migrate` command; the upgrade path (new shim stops old daemon, spawns its own) migrates automatically.
- **Downgrade**: DB schema newer than the binary knows → daemon refuses to start, logging both versions; shim surfaces a tool error pointing at `serve.log`. No down-migrations.
- **Backups**: none; backing up `loom.db` is the user's responsibility.
- **Failure**: each migration in its own transaction; on failure the DB stays at the last good version, the daemon logs the failing migration and exits non-zero; shim surfaces "daemon failed to start, see serve.log". No automatic recovery.
- **Moved repo**: the Slug is the Project's identity; only its Root (the locator) changes. New MCP tool `move_project {project: <slug>}` rebinds the Root to the caller's resolved location (main-worktree git root, or cwd outside git); CLI `loom project mv <slug> [dir]`. Duplicate-Root rejection and nesting rules as at creation. The `no_project` error flags Projects whose Root no longer exists on disk as `root_missing`, steering the agent to `move_project` over `create_project`. No automatic detection. Tool count becomes 24 (amends [MCP tool surface](08-mcp-tool-surface.md)).
- **Slug renames**: not in v1. A Slug is fixed at creation; renaming the repo folder doesn't change it, so `slug#N` refs and `Loom: slug#N` trailers stay valid.
- Glossary: no changes.

## Comments

- From [Config and daemon lifecycle](13-config-daemon-lifecycle.md): an upgraded shim now stops an older daemon and spawns its own, so a new binary meets the old DB at daemon start; a newer daemon is used by older shims as-is. The DB is `$LOOM_HOME/loom.db`.
