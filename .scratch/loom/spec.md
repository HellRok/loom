# Spec: Loom v1

Status: ready-for-agent

Synthesised from the resolved [Loom v1 map](map.md). Each decision's full detail lives in its ticket under `issues/`; vocabulary follows `GLOSSARY.md` (Issue, Project, Root, Slug, Loom home, Label, Ticket, Map, Event, Blocker, Frontier, Claim, Session).

## Problem Statement

A developer drives the `mattpocock/skills` engineering skills (`to-spec`, `to-tickets`, `triage`, `implement`, `implement-spec`, `code-review`, `wayfinder`, `ask-matt`) with coding agents. These skills need an issue tracker, and the available ones fit poorly:

- GitHub/GitLab need a remote, network access and a hosted account, and their CLIs (`gh`, `glab`) are a clumsy interface for agents: no native frontier, no atomic claim, and whole-body edits that lose concurrent appends to a map.
- The local-markdown tracker needs no account, but every "query" is a directory scan by convention, nothing stops two sessions claiming the same ticket, nothing checks labels or blocking, and the human gets no useful view of what the agents are doing.

The developer wants a tracker that lives on their own machine, that agents drive natively and safely (including several sessions at once), and that they can watch in a browser, without workarounds in any skill.

## Solution

Loom is a local-first issue tracker that agents drive through MCP and humans watch through a read-only web UI. It is a single Go binary:

- `loom mcp` is the stdio MCP server an agent harness spawns. It is a thin proxy: it makes sure the background daemon is running (starting it if needed) and forwards every call to it, adding the working directory and a per-process Session id.
- `loom serve` is the long-running daemon. It owns the SQLite database, serves MCP over HTTP on `127.0.0.1:7337`, and serves the read-only web UI, which updates live as agents write.
- `loom init` installs Loom as a repo's tracker on top of `/setup-matt-pocock-skills`: it writes the Loom tracker doc into the repo, creates the Project, and creates any custom triage labels.
- `loom stop` and `loom project mv|rm` cover daemon and Project housekeeping.

The Project is found from the agent's working directory. Maps, Tickets, blocking, the frontier, claims and section appends are native, so every tracker-touching skill runs end-to-end with no tracker-specific workarounds. That is the v1 bar.

## User Stories

### Installing and setting up

1. As a developer, I want to install Loom with `go install`, so that I get one static binary with no cgo or C toolchain needed.
2. As a developer, I want to run `loom init` after `/setup-matt-pocock-skills`, so that my repo's skills use Loom without hand-editing any docs.
3. As a developer, I want `loom init` to stop and change nothing if setup hasn't run (or ran halfway), pointing me to install `mattpocock/skills` and run `/setup-matt-pocock-skills`, so that I don't end up with a half-configured repo.
4. As a developer, I want `loom init` to always overwrite `docs/agents/issue-tracker.md` with the doc built into the binary, so that re-running it after an upgrade refreshes the doc.
5. As a developer, I want `loom init` to report `updated` or `unchanged` for each file it touches, so that I know what changed in my repo.
6. As a developer, I want `loom init` to rewrite only the `### Issue tracker` line of the `## Agent skills` block in `CLAUDE.md`/`AGENTS.md`, so that the rest of setup's output is kept.
7. As a developer, I want `loom init` to create the Project eagerly through the daemon, so that the first agent session doesn't have to.
8. As a developer, I want `loom init` to do nothing when a Project already exists at this exact Root, reporting its Slug, so that it is safe to re-run.
9. As a developer outside a git repo, or whose repo folder name collides with an existing Slug, I want to pass `--slug`, so that I can still create a Project.
10. As a developer with custom triage label names in `docs/agents/triage-labels.md`, I want `loom init` to create any of them the Project lacks, so that triage doesn't fail on undefined labels.
11. As a developer using any agent harness, I want `loom init` to print how to register the stdio MCP server (`loom`, command `loom mcp`) plus the common `mcpServers` JSON snippet, without editing my harness config, so that I stay in control of my harness.
12. As a developer who has already connected an agent, I want `loom init` to print only a one-line reminder about MCP registration, so that its output stays short.
13. As a developer, I want the installed tracker doc to note that re-running `/setup-matt-pocock-skills` overwrites it and that `loom init` should be run again afterwards, so that I know how to recover.

### Projects

14. As an agent, I want the Project resolved from my working directory, so that I never have to name it on ordinary calls.
15. As an agent working in a linked git worktree outside the repo, I want to resolve to the main worktree's Project, so that worktrees share one tracker.
16. As an agent in a subdirectory of a nested Root, I want the deepest Root to win, so that a nested Project is reachable.
17. As an agent, I want to pass an optional `project` Slug on any tool, so that I can act on another Project explicitly.
18. As an agent in a directory with no Project, I want a `no_project` error listing the existing Projects with their Roots and pointing at `create_project`, so that I know how to proceed.
19. As an agent, I want `create_project` with no arguments inside a git repo to use the git root as the Root and its folder name as the Slug, so that creating a Project is trivial.
20. As an agent outside git, I want `create_project` to require a `slug` and use my cwd as the Root, so that non-git directories work too.
21. As an agent, I want a Slug collision or an exact duplicate Root rejected with a clear error, so that Projects stay unambiguous.
22. As an agent, I want a new Project seeded with the 11 labels the skills use, so that triage and wayfinder work out of the box.
23. As an agent, I want `list_projects`, so that I can see every Project with its Root.
24. As a developer, I want `delete_project` (with `confirm: "<slug>"`) and `loom project rm` to hard-delete a Project and everything in it, freeing its Slug and Root, so that I can clean up.
25. As a developer who moved a repo, I want the `no_project` error to flag Projects whose Root no longer exists as `root_missing`, so that the agent rebinds instead of creating a duplicate.
26. As an agent, I want `move_project {project}` to rebind that Project's Root to my resolved location, so that a moved repo keeps its Issues.
27. As a developer, I want `loom project mv <slug> [dir]`, so that I can rebind a moved repo from the shell.
28. As a developer, I want Slugs fixed at creation, so that `slug#N` refs and `Loom: slug#N` commit trailers never go stale.

### Issues

29. As an agent, I want `create_issue {title, body, kind, parent, labels, blocked_by}` to set everything in one atomic call, so that a half-created Issue never exists.
30. As an agent, I want create to return `{ref, url, title, kind, version}`, so that I can link the new Issue right away.
31. As an agent, I want per-Project sequential `#N` ids that are never reused, even after a delete, so that refs stay unambiguous.
32. As an agent, I want refs to accept `#N`, `N`, `slug#N` and the issue URL, with `slug#N` and URLs naming their own Project, so that any ref I find works.
33. As an agent, I want `view_issue` to return the body and comments (events optional), including for closed Issues, so that I can fetch the relevant ticket.
34. As an agent, I want `view_issue` to return children as summaries without bodies, so that loading a Map gives me the low-res view.
35. As an agent, I want `list_issues` filtered by state, labels (all-of), `no_labels`, parent, kind, assignee (`none`/`any`/name), blocked and `held_by`, oldest first with a `total`, so that triage and implement can find their work.
36. As an agent, I want `detail: "full"` on `list_issues` to include bodies and comments, so that I can batch-read.
37. As an agent, I want `edit_issue` to require the `version` I last read and to reject a mismatch with `version_conflict` carrying the current version, title and body, so that I never overwrite another session's edit by accident.
38. As an agent, I want `append_to_section {ref, heading, text}` to append under a Markdown heading without a version, so that concurrent sessions can add to a Map's Decisions-so-far safely.
39. As an agent, I want `append_to_section` on a missing heading to fail with `no_section` listing the headings that exist, so that I can correct it.
40. As an agent, I want to `comment` on an Issue, so that conversation is recorded; comments are immutable.
41. As an agent, I want `close {ref, comment?}` to close with an optional comment in one step, so that a resolution and its close land together.
42. As a triaging agent, I want `close {ref, comment, wontfix: true}` to apply the `wontfix` label, comment and close atomically, so that a won't-fix close is one step.
43. As an agent, I want `wontfix: true` rejected on Tickets, so that triage labels never land on wayfinder Tickets.
44. As an agent, I want `reopen`, so that a wrongly closed Issue can be revived.
45. As an agent, I want `close`, `reopen`, `release` and `create_label` to be idempotent and report `already: true`, so that I can safely verify after an uncertain call.
46. As an agent, I want `delete_issue {ref, confirm: "#N"}` to hard-delete an Issue and its comments, events, label links and blocking edges, leaving its children parentless, so that mistakes can be removed.
47. As an agent, I want `set_parent {ref, parent | null}` with cycles and cross-Project parents rejected, so that hierarchy stays a tree.
48. As an agent, I want children ordered by ascending `#N`, so that "first in order" is well defined.

### Labels

49. As an agent, I want applying an undefined label rejected with the list of valid labels, so that typos never create junk labels.
50. As an agent, I want `label {ref, add, remove}` applied as one atomic transition, so that triage state moves cleanly.
51. As an agent, I want `list_labels` with usage counts, so that I can see what's defined and used.
52. As an agent, I want `create_label` (idempotent) and `delete_label` (only when unused, else an error listing the Issues using it), so that I can manage the vocabulary safely.
53. As a developer, I want each label to get a colour from a fixed pool, so that the UI tells labels apart.

### Wayfinder Maps and Tickets

54. As a wayfinding agent, I want to create a Map with `create_issue {kind: "map"}`, so that the Map is native rather than a label convention.
55. As a wayfinding agent, I want `list_issues {kind: "map"}`, so that I can find Maps.
56. As a wayfinding agent, I want a Map's direct children to be Tickets that must carry exactly one `wayfinder:<type>` label and no triage labels, with violations rejected and the valid types listed, so that the Map stays well formed.
57. As a wayfinding agent, I want Maps to have no parent and Tickets no children, so that the structure stays two levels deep.
58. As an agent, I want blocked-by edges between any Issues in the same Project, with self-blocks and cycles rejected and duplicates ignored, so that blocking is native and safe.
59. As an agent, I want `add_blockers` / `remove_blockers`, or `blocked_by` at create, so that I can wire blocking in a second pass.
60. As an agent, I want `frontier {parent}` to return the open, unblocked, unclaimed children in creation order, plus `skipped: {claimed, blocked}`, so that I can pick the next Ticket (or the next spec slice for `/implement-spec`) in one call.
61. As an agent, I want an Issue to count as unblocked once every Blocker is closed or deleted, so that the frontier advances as work closes.

### Sessions and claims

62. As an agent, I want my shim to give me a Session id (a UUIDv7 per `loom mcp` process) and send it on every call, so that I never manage identity myself.
63. As an agent, I want `claim` to atomically set the assignee to the configured human and record my Session, so that two sessions can't both take a Ticket.
64. As an agent re-claiming after an "outcome unknown" error, I want `already: true` when my own Session holds it, so that I know my claim landed.
65. As an agent, I want a claim held by another Session refused with `already_claimed {holder, holder_session, claimed_at}`, so that I move on.
66. As an agent, I want claiming a closed or blocked Issue refused, so that I only take workable Issues.
67. As an agent, I want any Session to `release` a claim, with the Event recording who, so that stale claims can be freed.
68. As an agent, I want `list_issues {held_by: "me"}`, so that I can find my own claims.
69. As a direct-HTTP client, I want to send my Session in the `X-Loom-Session` header or a `session` argument (header wins), so that I can use Loom without the shim.
70. As an agent, I want any call without a Session rejected with `no_session`, and one outside 16–256 characters rejected with `invalid_session`, so that every write is attributable.
71. As an agent spawning subagents, I want them to act as my Session, so that claims made on my behalf count as mine.

### Daemon and shim

72. As an agent, I want `loom mcp` to start the daemon if it isn't running and reuse it if it is, so that I never manage a background process.
73. As an agent, I want two `loom mcp` processes racing at startup to end up with exactly one daemon, so that the database has one writer.
74. As an agent, I want the shim to never write anything but MCP to its stdout, with daemon output going to `serve.log`, so that my MCP channel is never corrupted.
75. As an agent, I want the shim to forward `tools/list` live from the daemon, so that tool definitions have one source.
76. As an agent, I want any forwarding failure to become a tool error saying the outcome may be unknown, telling me to verify before redoing and naming the daemon log, so that I never double-create from a silent retry.
77. As an agent, I want the next call after a daemon crash to respawn it, so that I don't need to restart my session.
78. As a developer who upgraded Loom, I want a newer shim to replace an older daemon automatically, so that I never run stale code.
79. As a developer, I want an older shim to keep using a newer daemon, so that open sessions keep working during an upgrade.
80. As a developer with two Loom homes, I want the shim to refuse a daemon from a different Loom home, naming both paths and suggesting a `port` override, so that I never write to the wrong database.
81. As a developer, I want a non-Loom process on the port reported with the port and the config key, so that I can fix the clash.
82. As a developer, I want the daemon to run until I stop it, so that the UI is always available.
83. As a developer, I want `loom stop` to stop the daemon gracefully (drain in-flight calls, checkpoint, close) and report `not running` when there is none, so that I can restart or upgrade cleanly.
84. As a developer, I want the daemon to reject requests whose `Host` isn't `127.0.0.1:<port>` or `localhost:<port>`, so that malicious web pages can't drive it through DNS rebinding.

### Configuration and data

85. As a developer, I want everything Loom keeps in one Loom home (`$LOOM_HOME`, default `~/.loom`), so that I know where my data is and can back it up.
86. As a developer, I want an optional `config.toml` with every key defaulted (port 7337, assignee from git `user.name`, then `$USER`), so that Loom works with no setup.
87. As a developer, I want the daemon to log its effective config at startup, so that I can see what it's using.
88. As a developer, I want schema migrations to run automatically at daemon start, before it answers any request, so that upgrades need no extra command.
89. As a developer, I want a database newer than the binary refused, with both versions in the log and a tool error pointing at `serve.log`, so that a downgrade never corrupts data.
90. As a developer, I want a failed migration to leave the database at the last good version and the daemon to exit with the reason logged, so that I can recover by hand.

### Commits and closing work

91. As an implementing agent, I want the tracker doc to tell me to add a `Loom: N` (or `Loom: slug#N`) trailer to commits, so that commits link to Issues without colliding with GitHub `#N` autolinks.
92. As an implementing agent, I want the tracker doc to tell me to close the Issue myself with a comment citing the commit SHA, so that work closes without a PR merge.
93. As a skill, I want the tracker doc's Pull requests section to say PRs are not applicable, so that PR-gated steps are skipped cleanly.

### Watching in the web UI

94. As a developer, I want a home page listing each Project with its Root, open count, last activity, and each open Map's progress bar and next frontier Tickets, so that I see the state of every effort at a glance.
95. As a developer, I want a three-pane Project page (nav, issue list, Issue), so that I can browse without losing my place.
96. As a developer, I want saved views (Open, Maps, Needs triage, Ready for agent, Unassigned, Closed) and labels with counts in the nav, so that I can jump to common filters.
97. As a developer, I want each list row to show `#N`, parent, updated time, a status glyph (closed / claimed / blocked), title and labels, plus a filter box, so that I can scan quickly.
98. As a developer, I want filters kept in the URL while I select an Issue, so that I can share links.
99. As a developer, I want an Issue's Overview tab to show state, assignee, labels, parent, Blockers (closed ones struck through), what it blocks, children and version, then the rendered body and the comment thread, so that I can read it in one place.
100. As a developer, I want a History tab listing Events newest first, with title/body edits expandable to a diff, so that I can see how an Issue changed.
101. As a developer, I want a Map to open on a Board with Frontier, Claimed and Blocked columns, cards showing type, title, `#N`, assignee and open Blockers, so that I can see where the effort stands.
102. As a developer, I want a Map's Decided tab listing closed Tickets with the gist of each resolution, so that I can review the route walked.
103. As a developer, I want claims to show the holder Session's last 8 characters and idle time, linking to its page, so that I can spot stale claims.
104. As a developer, I want a Sessions page ordered by last seen, and a page per Session listing the Issues it holds and its Events across Projects, so that I can see what each agent did.
105. As a developer, I want the pages to update live as agents write, keeping open diffs and scroll position, so that I can watch without reloading.
106. As a developer, I want pages to reconnect and refresh after the daemon restarts, so that a `loom stop` or upgrade doesn't leave a stale tab.
107. As a developer, I want Markdown rendered with GFM (tables, task lists, strikethrough, autolinks), raw HTML escaped, and `#N` / `slug#N` linked to Issue pages, so that bodies read well and safely.
108. As a developer, I want every page to work with JavaScript off (except live updates), with back/forward, reload and pasted links working, so that the UI is robust.
109. As a developer, I want the UI to be read-only, so that all writes go through agents and land in the Event log with a Session.

## Implementation Decisions

### Process architecture

- One Go binary with subcommands `mcp`, `serve`, `init`, `stop`, `project mv`, `project rm`.
- **Daemon (`loom serve`)**: the only process that opens the database and the only writer. Startup order: take an exclusive non-blocking `flock` on `loom.lock` (held for life; the file handle must stay reachable), write its pid into the lockfile, open the DB, run migrations, then bind `127.0.0.1:<port>`. Losing the lock or the bind exits quietly. Serves `/mcp`, `/healthz`, the web UI and `/static/`. On SIGTERM/SIGINT: drain in-flight requests (about 5s cap), checkpoint WAL, close, exit. No idle timeout, no HTTP shutdown endpoint.
- **`/healthz`**: identifies Loom and returns version (full build string for dev builds), pid and Loom home. Exempt from the Session requirement.
- **Host check**: every request whose `Host` is not `127.0.0.1:<port>` or `localhost:<port>` is rejected. No auth.
- **Shim (`loom mcp`)**: an SDK-level proxy (stdio `mcp.Server` facing the agent, HTTP `mcp.Client` facing the daemon), with no tool logic and no cached tool list. Per call: detect-and-spawn if needed, then forward with `X-Loom-Cwd` (raw cwd) and `X-Loom-Session` (a UUIDv7 generated once per process). Never retries; a forwarding failure becomes an "outcome may be unknown, verify before redoing, see `<log path>`" tool error.
- **Detect-and-spawn**: probe `/healthz` (about 1s timeout). If absent, spawn `os.Executable() serve` with `Setsid`, stdin nil and stdout/stderr to `serve.log`, then `Release()`; poll `/healthz` with exponential backoff from 50ms, capped at about 5–10s. If the daemon reports an older version, stop it as `loom stop` does and spawn a new one. If it reports a newer one, use it. If it reports a different Loom home, refuse. A non-Loom responder on the port is an error naming the port and the `port` key. Startup failures, including a newer DB or a failed migration, surface as a tool error pointing at `serve.log`.
- **`loom stop`**: read the pid from `loom.lock`, confirm `/healthz` reports the same pid, send SIGTERM and wait for the port to free; otherwise report `not running` and signal nothing.

### Config and Loom home

- Loom home is `$LOOM_HOME`, default `~/.loom`, holding `config.toml`, `loom.db`, `loom.lock` and `serve.log`.
- `config.toml` is optional and every key has a default; Loom never writes it. Keys: `port` (default 7337, never auto-picked) and `assignee` (default `git config --global user.name`, then `$USER`, resolved at daemon start). Shim and daemon read the same file. Changes apply after a restart.

### Storage

- `modernc.org/sqlite` (pure Go; keep `mattn/go-sqlite3` out of the dependency graph; pin `modernc.org/libc` to the version modernc requires). DSN pragmas: `busy_timeout`, `journal_mode(WAL)`, `foreign_keys(1)`; `_txlock=immediate` so a claim's check-and-set is atomic.
- `pressly/goose/v3` migrations embedded with `embed.FS`, each in its own transaction, run only at daemon start. A DB newer than the binary is refused. No down-migrations, no automatic backups.

### Data model

- **Project**: Slug (unique, `[a-z0-9-]`, fixed at creation), Root (unique absolute path). Nesting is allowed. Hard delete cascades to everything.
- **Issue**: per-Project sequential number (never reused), `kind` (`issue` | `map`, immutable), title, Markdown body, state open/closed, author (configured name), created/updated, `version` (incremented on every edit), nullable assignee, nullable holder Session plus claimed-at, nullable parent.
- **Parent/child**: at most one parent, same Project, no cycles, children ordered by number.
- **Map rules**: a Map has no parent. Its direct children are Tickets: exactly one `wayfinder:<type>` label (`research`/`prototype`/`grilling`/`task`), no triage labels, no children. A spec stays a plain Issue whose children are plain Issues.
- **Blocked-by**: many-to-many edges within one Project, independent of parentage. Self-blocks and cycles are rejected, duplicates ignored. Unblocked means every Blocker is closed or deleted.
- **Label**: name unique per Project, optional description, colour from a fixed pool. No rename; delete only when unused. The 11 seeded labels are `bug`, `enhancement`, `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`, `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling` and `wayfinder:task`. There is no `wayfinder:map` label.
- **Comment**: immutable body, author, Session, timestamp.
- **Event**: one per mutation (created, title/body edited with the previous value, labelled/unlabelled, assigned/unassigned, claimed/released, parent set/cleared, blocker added/removed, closed/reopened, commented), with timestamp, actor and Session.
- **Session**: global record keyed by the opaque string, with first-seen and last-seen, upserted best-effort on every call (a failure is logged and the call still succeeds).

### Request context

- Each MCP call resolves (Project, Session) before the handler runs.
- **Session**: the `X-Loom-Session` header, else a `session` argument (the header wins silently). Missing → `no_session`. Not 16–256 runes → `invalid_session`. Required on every tool, reads included.
- **Project**: an explicit `project` Slug, or a `slug#N`/URL ref, wins. Otherwise the cwd from `X-Loom-Cwd` is mapped to the main worktree root (parent of `git rev-parse --git-common-dir`) when inside a git worktree, then the Project with the longest-prefix Root is picked. No cwd and no `project` → error. No match → `no_project {projects: [{slug, root, root_missing?}]}`.

### MCP tool surface

Built on the official `modelcontextprotocol/go-sdk` v1.8.x with typed `mcp.AddTool[In, Out]`, and a `NewStreamableHTTPHandler` in stateless mode (no Roots, no per-session server state). 23 tools: the 22 from the prototype (branch `prototype/mcp-tool-surface`) plus `move_project`:

- **Projects**: `create_project {slug?}`, `list_projects`, `delete_project {slug, confirm: "<slug>"}`, `move_project {project}`.
- **Reading**: `view_issue {ref, comments=true, events=false}` (children as summaries); `list_issues {state=open|closed|all, labels, no_labels, parent, kind, assignee=none|any|<name>, blocked, held_by=<session>|"me", detail=summary|full, limit=50}` (oldest first, with `total`); `frontier {parent}` (returns `{frontier, skipped: {claimed, blocked}}`).
- **Writing**: `create_issue {title, body, kind, parent, labels, blocked_by}`, `edit_issue {ref, version, title?, body?}`, `append_to_section {ref, heading, text}`, `comment {ref, body}`, `label {ref, add, remove}`, `close {ref, comment?, wontfix?}`, `reopen`, `claim`, `release`, `set_parent {ref, parent|null}`, `add_blockers`, `remove_blockers`, `delete_issue {ref, confirm: "#N"}`.
- **Labels**: `list_labels` (with usage counts), `create_label` (idempotent), `delete_label`.
- **Common**: every tool takes an optional `project` (and `session` for direct HTTP). Refs accept `#N`, `N`, `slug#N` or `http://127.0.0.1:<port>/<slug>/issues/<n>`. Outputs are structured JSON (`structuredContent` plus a JSON text copy). Create returns `{ref, url, title, kind, version}`.
- **Errors**: MCP tool errors shaped `{error: {code, message, ...data}}`, carrying what the agent needs next. Codes include `no_project` (`projects`, with `root_missing`), `no_session`, `invalid_session`, `already_claimed` (`holder`, `holder_session`, `claimed_at`), `version_conflict` (`current {version, title, body}`), `undefined_label` (`valid`), `invalid_ticket` (`valid_types`), `no_section` (`headings`) and cycle/self-block rejections. A failed call changes nothing.
- **Idempotent**: `close`, `reopen`, `release` and `create_label` return `already: true` on a no-op, as does `claim` when the same Session re-claims.
- `close {wontfix: true}` applies the label, comment and close atomically. It is rejected on Tickets.

### Tracker doc and `loom init`

- The tracker doc (draft on branch `prototype/issue-tracker-loom`) is embedded in the binary. It uses the same section names as `issue-tracker-github.md` (Conventions, Pull requests, Commits, "publish to the issue tracker", "fetch the relevant ticket", Wayfinding operations). Each bullet names the tool plus only the arguments a skill would get wrong, with no schemas and no failure section. Changes to apply to the draft:
  - Add a provenance note under the title: written by `loom init`; re-running `/setup-matt-pocock-skills` overwrites it, so run `loom init` again afterwards.
  - Make the setup line harness-neutral (stdio server `loom`, command `loom mcp`), dropping `claude mcp add`.
  - Add to the labels bullet: label strings come from `docs/agents/triage-labels.md`; if one is rejected as undefined, create it with `create_label` and retry.
  - Keep the guidance to verify rather than retry after "outcome unknown".
- **`loom init`**:
  1. Check that setup ran: the first of `CLAUDE.md`/`AGENTS.md` (setup's order) has `## Agent skills` with `### Issue tracker`, and `docs/agents/issue-tracker.md` exists. If not, change nothing and point at the `mattpocock/skills` install and `/setup-matt-pocock-skills`.
  2. Overwrite the tracker doc.
  3. Rewrite the `### Issue tracker` summary line to: "Issues are tracked in Loom, a local MCP issue tracker (installed by `loom init`, replacing the tracker chosen in `/setup-matt-pocock-skills`). See `docs/agents/issue-tracker.md`."
  4. Create the Project through the daemon (spawned as the shim does), with `--slug` when needed.
  5. Create the labels listed in the "Label in our tracker" column of `docs/agents/triage-labels.md` that the Project lacks.
  6. Print MCP registration guidance: in full if no Session has ever been recorded, otherwise a one-line reminder.

### Web UI

- Server-rendered Go `html/template`. URLs are `/`, `/<slug>?<filters>` and `/<slug>/issues/<n>?tab=overview|history|board|decided`, plus the Sessions page and per-Session pages. Each URL has one handler and one template set, and returns the full three-pane page, or just the pane fragment when `HX-Request` is set.
- Navigation uses htmx with `hx-push-url`. htmx, its SSE extension and idiomorph are pinned and vendored under `/static/` via `go:embed`, with hand-written CSS. No npm, bundler or CDN.
- **Live updates**: after each committed write the daemon publishes in-process to the SSE streams `/events` (home) and `/<slug>/events`. Each write emits the named events `issue-<n>` for the changed Issue and for its parent, then `changed`. The list pane and home refresh on `changed`; the Issue pane refreshes only on its own `issue-<n>`. Swaps use idiomorph morphing, so open `<details>` and scroll position survive. On reconnect every pane refetches once; there is no Event replay.
- **Markdown**: goldmark with GFM and raw HTML escaped, so no separate sanitiser is needed. `#N`/`slug#N` are auto-linked. Comment anchors are `#comment-<id>`.
- Session display: last 8 characters, full string on hover, linked to the Session page, with idle time wherever a claim is shown.
- Layout follows the prototype on branch `prototype/web-ui-pages`: variant C for home, variant B elsewhere. The one change is that Decided is a tab, not a fourth Board column.

### Modules (deep, behind few seams)

- **Store**: opens the DB with pragmas, runs migrations, and owns all transactional domain operations (create, edit, claim, frontier and so on) with their invariants.
- **Request context**: headers/args → (Project, Session), including git worktree resolution.
- **Tools**: the MCP tool definitions and handlers mapping onto the store, error coding and ref parsing.
- **Shim**: health probe, version/home decisions, spawn, proxy.
- **Daemon**: lifecycle (lock, migrate, bind, drain), Host check, mux, SSE broker.
- **Web**: handlers, templates, Markdown rendering.
- **Init**: repo-file detection and rewriting, triage-label parsing.
- **Config**: Loom home and TOML defaults.

## Testing Decisions

- **What makes a good test**: test external behaviour through the three agreed seams, never internals. Use real SQLite files (one per test in `t.TempDir()`, migrated through the daemon's own migration path), not storage fakes. Assert structured outputs and error codes, not SQL or struct internals.
- **Seam 1: MCP tool handlers**, called directly with request context (cwd, Session) and no live HTTP server. This seam covers the domain, storage, Project binding, claims and the Map rules:
  - Each tool's outputs and coded errors (`no_project`, `root_missing`, `no_session`, `invalid_session`, `version_conflict`, undefined label, invalid Ticket, `no_section`, cycles).
  - Request context: `X-Loom-Cwd`/`X-Loom-Session`/`session` argument → Project + Session, including worktrees, nesting and missing-session errors.
  - Claims, sequentially: another Session's claim is refused, the same Session's re-claim returns `already: true`, closed and blocked Issues are refused. No goroutine race tests.
  - A golden `tools/list` (names and schemas checked in), so surface changes show up as a diff.
  - Migrations: committed snapshot DBs per released schema version under `testdata/`, starting at v1, each opened with the current binary's migration path. A newer DB is refused and a failed migration exits.
  - **Skills bar**: one table-driven test per tracker-touching skill (`to-spec`, `to-tickets`, `triage`, `implement`, `implement-spec`, `code-review`, `wayfinder`, `ask-matt`), replaying its operations from the operations inventory (branch `research/skill-operations-inventory`) in order and asserting each output carries what the skill needs next. This is how the v1 bar is proven. No manual checklist and no headless agent runs.
- **Seam 2: web UI HTTP handlers**, through `httptest` against real handlers and a seeded real SQLite file. For each URL, assert status and key content with `goquery` selectors in both full-page and fragment (`HX-Request`) modes. One SSE test: a write through a tool handler makes the stream emit `issue-<n>`, `issue-<parent>` and `changed`. One Markdown test: raw HTML is escaped and refs are linked. No browser tests and no golden HTML.
- **Seam 3: the `loom` binary as a process**: build once, then give each test its own temporary `LOOM_HOME` and a free `port`. A small smoke suite: `loom mcp` spawns the daemon and calls a tool, a second `loom mcp` reuses it, and `loom stop` stops it. `loom init` runs against a temporary repo with setup output present and absent. Edge cases (older/newer version, foreign Loom home, non-Loom port, "outcome unknown") are unit tests of the shim's probe and decision logic, not process tests.
- **Prior art**: none in the repo, which is greenfield. The go-sdk's in-repo tests and its `examples/server/proxy` show how to call handlers and how to shape the proxy.
- **Platforms**: `go test ./...` must pass on Linux and macOS before release. CI is out of scope until a remote exists.

## Out of Scope

- Pull/merge requests: Loom tracks Issues only and is not a code host.
- Multi-user use, authentication, hosting and remote access.
- Writing from the web UI.
- An HTTP-fallback skill (driving Loom over direct HTTP without MCP). Direct HTTP `/mcp` works and is mentioned as secondary, but the v1 tracker doc covers MCP only.
- Search and full-text search over Issues, comments and Events (the UI has only a list filter box).
- Export/import, e.g. migrating an effort from `.scratch/` or GitHub.
- Slug renames and aliases.
- Automatic backups, down-migrations and a `loom migrate` command.
- Automatic release of stale claims (the UI shows idle time instead).
- Automatic detection of moved repos (`root_missing` plus `move_project` instead).
- Editing harness MCP config, and changes upstream in `mattpocock/skills`.
- CI.

## Further Notes

- Decision detail and rationale: `.scratch/loom/issues/01`–`16`. Research findings live on the local `research/*` branches, and prototypes on `prototype/mcp-tool-surface`, `prototype/web-ui-pages` and `prototype/issue-tracker-loom`. None of these branches is merged.
- Skill text that says "labelled `wayfinder:map`" means `create_issue {kind: "map"}` in Loom; the tracker doc explains this.
- `/triage` requires exactly one of `bug`/`enhancement` per Issue, which is why both are seeded.
- The upgrade path is: new binary → new shim sees an older daemon → stops it → spawns its own → migrations run at start. In-flight calls on the old daemon surface as "outcome unknown".
- Children ordered by `#N` (no position field) is a deliberate simplification. Frontier order is creation order.
