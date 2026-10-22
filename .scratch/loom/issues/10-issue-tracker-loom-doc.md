# Contents of issue-tracker-loom.md

Type: grilling
Status: resolved
Assignee: Sean Earle
Blocked by: 05, 08, 11

## Question

What goes in `issue-tracker-loom.md`, the drop-in doc for `setup-matt-pocock-skills`: conventions mapping each skill operation to Loom MCP tools, the "publish to the issue tracker" / "fetch the relevant ticket" sections, and the Wayfinding operations section (map, child ticket, blocking, frontier, claim, resolve)?

## Answer

Resolved by grilling (2026-10-22). Draft: branch `prototype/issue-tracker-loom`, file `prototype/issue-tracker-loom.md`.

- **Shape**: same sections as `issue-tracker-github.md` so skills find them by name: Conventions, Pull requests ("Not applicable: Loom tracks Issues only"; PRs as a request surface: no), Commits, "publish to the issue tracker", "fetch the relevant ticket", Wayfinding operations. No failure-handling section: Loom's errors carry what the agent needs.
- **Detail**: each bullet names the tool plus only the arguments a skill would get wrong (e.g. `detail: "full"`, `assignee: "none"`, `wontfix: true`); no schemas, since `tools/list` is live.
- **Mappings**: as in the comments below. Map = `create_issue {kind: "map"}` (doc explains the skill's "labelled `wayfinder:map`"), listed via `list_issues {kind: "map"}`; delete = `delete_issue {confirm: "#N"}`.
- **Closing work**: once the implementing commit lands, the agent closes the Issue itself with `close {comment}` citing the SHA.
- **Commit references**: `Loom: 12` git trailer, one line per Issue, `Loom: slug#12` for another Project; never a bare `#N` outside Loom.
- **Setup**: only MCP registration (`claude mcp add loom -- loom mcp`). Project creation is lazy: an unregistered repo's `no_project` error leads the agent to `create_project`.
- **No direct HTTP mention**: the doc covers MCP only; an HTTP fallback skill is post-v1 (out of scope).
- **Delivery**: Loom ships the doc itself, installed by `loom init`; nothing goes upstream to `mattpocock/skills` (local edits there would be overwritten on update). Graduated to [`loom init` command](12-loom-init.md).

## Comments

- From [Shim–daemon design for `loom mcp`](06-shim-daemon-design.md): document `loom mcp` as the recommended setup and direct HTTP `127.0.0.1:<port>/mcp` as secondary (must pass `project`). Tell agents to verify, not blindly retry, after an "outcome unknown" error.
- From [Data model](07-data-model.md): skill text "labelled `wayfinder:map`" translates to creating the Issue with `kind: map` (no such label exists in Loom). Spec children are plain child Issues; frontier works on them too.
- From [MCP tool surface](08-mcp-tool-surface.md): map each operation to the tools in its Answer (23 tools; prototype on branch `prototype/mcp-tool-surface`). Notable mappings: Map = `create_issue {kind: "map"}`; child ticket = `create_issue {parent, labels: [<one wayfinder type>]}`; blocking = `add_blockers`, or `blocked_by` at create; frontier = `frontier {parent}`; claim = `claim`; resolve = `close {comment}` + `append_to_section {heading: "Decisions so far"}`; triage wontfix = `close {comment, wontfix: true}`; "how this tracker closes work" = `close`. Claim wording waits on [Agent session identity and the assignee](11-session-identity.md).
- From [Agent session identity and the assignee](11-session-identity.md): claim wording: `claim` sets the assignee to the configured user and records the caller's session; a retried claim returning `already: true` means this session holds it. Direct HTTP callers must send a session (`X-Loom-Session` header preferred, or `session` arg; 16–256 chars, e.g. a UUIDv7) on every call. Agents can find their own claims with `list_issues {held_by: "me"}`.
