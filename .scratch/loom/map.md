# Map: Loom v1

## Destination

An implementation-ready spec for Loom v1 plus a drop-in `issue-tracker-loom.md` for `setup-matt-pocock-skills`. v1 is done when every tracker-touching skill (`to-spec`, `to-tickets`, `triage`, `implement`, `implement-spec`, `code-review`, `wayfinder`, `ask-matt`) runs end-to-end against Loom with no tracker-specific workarounds.

## Notes

- **Domain**: Loom is a local-first issue tracker that agents drive through MCP. Humans observe; agents act. Vocabulary lives in `GLOSSARY.md` (Issue, Project, Label, Map, Ticket).
- **Stack**: Go (MCP server), SQLite (all storage), plain HTML (web UI).
- **Processes**: `loom serve` runs one long-running daemon serving MCP over HTTP and the read-only web UI, bound to `127.0.0.1`, no auth. `loom mcp` is the stdio command agents spawn; it checks whether the daemon is running, starts it in the background if not, and connects to it.
- **Users**: one developer on one machine. Single configured assignee name; a claim sets it.
- **Data**: one global DB with Projects; per-Project sequential `#N` issue ids. Labels, parent, blocked-by, assignee, and open/closed are native fields. Labels are per-Project; each new Project is seeded with the 11 labels the skills reference: `bug`, `enhancement` (triage categories), `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix` (triage states), and `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling`, `wayfinder:task`; applying an undefined label is rejected (error lists valid labels). A Map is an Issue with native `kind: map` (no `wayfinder:map` label). Bodies and comments are Markdown. Every Issue keeps an event log, which the UI renders as a timeline.
- **Web UI**: read-only. All writes go through an agent.
- **Skills every session should consult**: `grilling` + `domain-modeling` (decisions and glossary), `codebase-design` (module seams), and read `GLOSSARY.md`.
- **Tracker for this map**: local markdown (Loom can't host its own planning yet). Research findings live on local, unpushed `research/<name>` branches (no remote).

## Decisions so far

<!-- one line per resolved ticket: [title](issues/NN-slug.md): gist -->

- [Tracker operations the skills invoke](issues/04-skill-operations-inventory.md): 14 operations incl. ordered parent/child, blocked-by, frontier, concurrency-safe claim, edit, delete, `#N` resolution; triage also needs `bug`/`enhancement` labels.
- [Background daemon spawning and single-instance in Go](issues/01-daemon-spawning.md): shim probes `/healthz` (with version), spawns `loom serve` via Setsid + log-file stdio; daemon holds a flock for life plus the port bind; no pidfile.
- [Go MCP SDK choice](issues/02-go-mcp-sdk.md): official `go-sdk` v1.8.0; stateless streamable HTTP in the daemon; shim as an SDK-level stdio→HTTP proxy forwarding cwd in a header; no Roots, no session state.
- [Go SQLite driver choice](issues/03-sqlite-driver.md): `modernc.org/sqlite` (pure Go; WAL/JSON/FTS5 built in) + `goose` migrations embedded; DSN pragmas and `_txlock=immediate` for safe claims.
- [How a session binds to a Project](issues/05-session-project-binding.md): one registered Root per Project; cwd → main-worktree root → longest-prefix match; optional `project` Slug override; explicit `create_project` (Slug = git root basename, required outside git); hard `delete_project` with confirm.
- [Shim–daemon design for `loom mcp`](issues/06-shim-daemon-design.md): thin SDK-level proxy, live `tools/list`, forwards only `X-Loom-Cwd`; direct HTTP `/mcp` supported as secondary; no retries, failures surface as "outcome unknown" tool errors; next call respawns the daemon.
- [Data model](issues/07-data-model.md): Issues with immutable `kind` (issue|map); open/closed + reopen; version-checked edits; hard delete; single ordered parent; Map children are Tickets with exactly one `wayfinder:<type>` label; same-Project blocked-by edges; atomic claim; immutable comments; Events keep prior values; coloured labels, 11 seeded.
- [MCP tool surface](issues/08-mcp-tool-surface.md): 23 separate tools (prototype on `prototype/mcp-tool-surface`), with atomic `create_issue` (parent/labels/blockers), `frontier`, `claim`, `append_to_section`, `close {comment, wontfix}`; version-checked `edit_issue`; refs `#N`/`slug#N`/URL; structured JSON; coded errors carrying what the agent needs next.
- [Web UI pages](issues/09-web-ui-pages.md): home is a project list with per-Map progress and next frontier Tickets; inside a Project a three-pane layout (nav / list / Issue); Issue pane has Overview (body + discussion thread) and History (Events with diffs); Maps open on a Frontier/Claimed/Blocked board, with closed Tickets and their gists on a separate Decided tab.
- [Agent session identity and the assignee](issues/11-session-identity.md): shim sends a per-process UUIDv7 as `X-Loom-Session`, required on every call (direct HTTP: header or `session` arg); assignee stays the human, Claim also records holder session; same-session re-claim is `already: true`; Events/Comments record session; background first/last-seen record; `held_by` filter + Sessions page.
- [Contents of issue-tracker-loom.md](issues/10-issue-tracker-loom-doc.md): GitHub-doc section layout, tool + key args per operation, no failure section; agents close Issues themselves citing the commit; `Loom: N` commit trailer; MCP-only setup with lazy Project creation; shipped by Loom via `loom init`, not upstream.
- [`loom init` command](issues/12-loom-init.md): run after `/setup-matt-pocock-skills` (stops if setup hasn't run); always overwrites `issue-tracker.md` + the `### Issue tracker` line, saying it amends setup; eager Project via daemon; creates labels from `triage-labels.md`; prints harness-agnostic MCP registration, never edits harness config.
- [Config and daemon lifecycle](issues/13-config-daemon-lifecycle.md): optional TOML in `$LOOM_HOME` (default `~/.loom`); fixed port 7337; assignee from git `user.name`; daemon runs until `loom stop` (SIGTERM via lockfile pid, verified by `/healthz`); Host-header check; shim replaces older daemons, refuses a foreign Loom home.
- [Schema upgrades and moving or renaming Projects](issues/14-migrations-project-move-rename.md): migrations at daemon start before port bind; newer DB refused; no backups (user's job); failure exits daemon; `move_project` tool + `loom project mv` rebind a moved Root (`root_missing` hint); Slugs fixed.
- [Testing strategy](issues/15-testing-strategy.md): real SQLite file per test; handler unit tests + golden `tools/list`; small real-process shim↔daemon smoke suite; snapshot DBs per schema version; skills bar proven by one replay test per skill; Linux + macOS, no CI.
- [Web UI rendering and live updates](issues/16-web-ui-rendering.md): server-rendered `html/template` with htmx fragment swaps (full page without `HX-Request`), vendored htmx + SSE ext + idiomorph; in-process SSE publishes `issue-<n>`/parent/`changed`, morph swaps, refetch on reconnect; goldmark GFM with raw HTML off; `httptest` + `goquery` tests.

## Not yet specified

<!-- empty: the way to the destination is clear -->

## Out of scope

- Pull/merge requests: Loom tracks Issues only; it is not a code host.
- Multi-user, auth, hosting, remote access.
- Writing from the web UI.
- HTTP fallback skill (driving Loom over direct HTTP without MCP): post-v1; the v1 tracker doc covers MCP only. Decided in [Contents of issue-tracker-loom.md](issues/10-issue-tracker-loom-doc.md).
- Search / full-text over Issues and comments: no tracker-touching skill invokes search (see [Tracker operations the skills invoke](issues/04-skill-operations-inventory.md)).
- Export/import (e.g. migrating an effort from `.scratch/` or GitHub): not needed for skills to run against Loom.
- Slug renames (and aliases for old Slugs): no skill needs them; Slugs are fixed at creation. Decided in [Schema upgrades and moving or renaming Projects](issues/14-migrations-project-move-rename.md).
