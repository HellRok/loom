# MCP tool surface

Type: prototype
Status: resolved
Assignee: Sean Earle
Blocked by: 02, 04, 07

## Question

What tools does Loom expose over MCP: names, arguments, and outputs? Granular verbs mirroring `gh`/`glab` (create, view, list, comment, label, close, assign, link) vs coarser domain tools (e.g. a native frontier query, claim). Prototype the tool definitions and a few example calls/outputs to react to.

## Answer

Resolved by prototype (2026-10-22). Prototype: branch `prototype/mcp-tool-surface`, file `prototype/mcp-tool-surface.prototype.html` (single-file logic demo with an in-memory fake Loom and 8 walkthroughs).

- **Granularity**: separate verbs, about one per tracker-doc operation, plus a few tools that do a whole step in one call where the skills need atomicity. 23 tools:
  - Projects: `create_project {slug?}`, `list_projects`, `delete_project {slug, confirm}`.
  - Reading: `view_issue {ref, comments=true, events=false}` (children come back as summaries without bodies, which gives the low-res map view), `list_issues {state=open|closed|all, labels (all-of), no_labels, parent, kind, assignee=none|any|name, blocked, detail=summary|full, limit=50}` (oldest first, returns `total`), `frontier {parent}` (returns frontier in creation order plus `skipped: {claimed, blocked}`).
  - Writing: `create_issue {title, body, kind, parent, labels, blocked_by}` (all set atomically; a Ticket is created with its type label), `edit_issue {ref, version, title?, body?}`, `append_to_section {ref, heading, text}` (no version needed; for concurrent Decisions-so-far appends; removing lines goes through `edit_issue`), `comment`, `label {ref, add, remove}` (one atomic transition), `close {ref, comment?, wontfix?}`, `reopen`, `claim`, `release`, `set_parent {ref, parent|null}`, `add_blockers`, `remove_blockers`, `delete_issue {ref, confirm: "#N"}`.
  - Labels: `list_labels` (with usage counts), `create_label` (idempotent), `delete_label` (unused only).
- **Common**: every tool takes optional `project` (Slug). Refs accept `#N`, `N`, `slug#N`, or the issue URL (`http://127.0.0.1:<port>/<slug>/issues/<n>`); a URL or `slug#N` names its own Project.
- **Outputs**: structured JSON (`structuredContent` plus a JSON text copy). Create returns `{ref, url, title, kind, version}`.
- **Errors**: MCP tool errors carrying `{error: {code, message, ...data}}`, with whatever the agent needs next: `valid` labels / `valid_types`, `holder` + `claimed_at` on `already_claimed`, `current {version, title, body}` on `version_conflict`, `headings` on `no_section`, `projects` on `no_project`. A failed call changes nothing.
- **Verify, don't redo**: `close`, `reopen`, `release` and `create_label` are idempotent (`already: true`). After an "outcome unknown" create, the agent lists the parent's children to check whether it landed.
- **`close` + `wontfix`**: `wontfix: true` applies the label in the same atomic step as the comment and the close. It is rejected on Tickets, since `wontfix` is a triage label; out-of-scope Tickets are just closed.
- **Deferred**: whether claim/assignee should identify the agent session. Two sessions both claim as the configured user, so a retried claim after "outcome unknown" can't tell its own claim from another session's. Graduated to [Agent session identity and the assignee](11-session-identity.md). Claim stays as prototyped until then.

## Comments

- From [Shim–daemon design for `loom mcp`](06-shim-daemon-design.md): the shim mirrors whatever the daemon lists, so tool definitions live only in the daemon. Transport failures arrive as "outcome unknown" tool errors, so non-idempotent tools (e.g. create) should be easy to verify via list/view.
- From [Data model](07-data-model.md): needs tool shapes for the atomic claim (error names holder), version-checked edits (conflict error; consider a section-append op for map Decisions-so-far), `kind: map` on create, reopen, hard delete, and the Ticket-rule rejections (exactly one `wayfinder:<type>` label, no triage labels, no grandchildren). Frontier query takes any parent.
