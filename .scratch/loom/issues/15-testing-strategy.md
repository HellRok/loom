# Testing strategy

Type: grilling
Status: resolved
Assignee: Sean Earle
Blocked by: 

## Question

How is Loom tested, and how is the v1 bar ("every tracker-touching skill runs end-to-end against Loom with no tracker-specific workarounds") proven? Decide the layers (domain logic against real SQLite, MCP tools through the go-sdk in-process client, shim↔daemon spawning, web UI pages), what's faked versus real, and whether the skills bar is checked by scripted agent runs, a manual checklist per skill, or replaying the operations inventory from [Tracker operations the skills invoke](04-skill-operations-inventory.md) as tool-call fixtures.

## Answer

Resolved by grilling (2026-10-22).

- **Storage**: real SQLite file per test in `t.TempDir()`, migrated via the daemon's own migration path; no storage fakes.
- **Claims**: sequential tests only: second session's claim refused as already held, same-session re-claim → `already: true`, claiming a closed or blocked Ticket refused. No goroutine race test.
- **Tools**: unit tests calling handlers directly (no live HTTP server); assert structured outputs and coded errors (`no_project`, `root_missing`, version conflict, undefined label).
- **Request context**: unit test of `X-Loom-Cwd` / `X-Loom-Session` (or `session` arg) → Project + session, incl. missing-session error.
- **Golden `tools/list`**: names and schemas checked in; surface changes show as an explicit diff.
- **Shim↔daemon**: small integration smoke suite: build the binary once, per-test temp `LOOM_HOME` + free `port` (existing config key, see [Config and daemon lifecycle](13-config-daemon-lifecycle.md)); `loom mcp` spawns daemon and calls a tool, a second `loom mcp` reuses it, `loom stop`. Edge cases (version replacement, foreign Loom home, "outcome unknown") are unit tests of probe/version-check logic.
- **Migrations**: committed snapshot DBs per released schema version under `testdata/`, starting at v1; tests open each with the current binary, refuse a newer DB, exit on a failed migration. `move_project` / `root_missing` covered by tool unit tests.
- **Skills bar**: one table-driven Go test per tracker-touching skill, replaying its operations from [Tracker operations the skills invoke](04-skill-operations-inventory.md) in order against the handlers, asserting each output carries what the skill needs next. No manual checklist, no headless agent runs; real use covers the rest.
- **Web UI**: deferred to the rendering fog patch.
- **Platforms**: `go test ./...` passes on Linux and macOS before release; CI out of scope until a remote exists.

## Comments
- From [Schema upgrades and moving or renaming Projects](14-migrations-project-move-rename.md): cover migrating at daemon start (from each prior schema version), refusing a newer DB, exit on a failed migration, and `move_project` / `root_missing`.
- From [Web UI rendering and live updates](16-web-ui-rendering.md): web UI testing decided there (`httptest` + `goquery`, full-page and fragment modes, one SSE test, one Markdown test).
