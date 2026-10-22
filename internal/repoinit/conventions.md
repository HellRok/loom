Issues and specs for this repo live in Loom, a local issue tracker driven through its MCP server (`loom`). Use the `loom` MCP tools for all operations; there is no CLI or web write path.

If a tool reports that the outcome may be unknown (the daemon dropped the call), do not retry blindly: check with `view_issue` or `list_issues` whether the change landed, then redo it only if it did not. The error names the daemon log.

## Conventions

- **Refer to an issue**: `#N` inside Loom (bodies, comments, tool `ref` arguments). Tools also accept `slug#N` and the issue URL. Outside Loom, never write a bare `#N` (it collides with GitHub autolinks); see **Commits** below.
- **Create an issue**: `create_issue {title, body}`. Pass `parent`, `labels` and `blocked_by` in the same call rather than setting them afterwards. Returns `{ref, url}`.
- **Read an issue**: `view_issue {ref}` (comments included; closed issues too). Children come back as summaries.
- **List issues**: `list_issues {state, labels, no_labels, assignee, parent, detail: "full"}`. Oldest first; `detail: "full"` includes bodies and comments; `assignee: "none"` for unassigned.
- **Make an issue a child of a parent**: `parent` on `create_issue`, or `set_parent {ref, parent}` afterwards. Children are ordered by creation.
- **Edit title or body**: `edit_issue {ref, version, title?, body?}`, with the `version` from your last read.
- **Comment on an issue**: `comment {ref, body}`.
- **Apply / remove labels**: `label {ref, add, remove}`. Only labels defined on the Project can be applied; `list_labels` shows them, `create_label` adds one. Label strings come from `docs/agents/triage-labels.md`; if one is rejected as undefined, create it with `create_label` and retry.
- **Close**: `close {ref, comment}`. To close as won't-fix, `close {ref, comment, wontfix: true}`.
- **Delete**: `delete_issue {ref, confirm: "#N"}`.

## Pull requests

Not applicable: Loom tracks Issues only. **PRs as a request surface: no.**

## Commits

Reference the Issue a commit works on with a `Loom:` git trailer at the end of the message, one line per Issue:

```
Loom: 12
Loom: other-project#4
```

When the work for an Issue has landed, close it yourself: `close {ref, comment}` citing the commit SHA. No merge will close it for you.

## When a skill says "publish to the issue tracker"

Create a Loom issue (`create_issue`).

## When a skill says "fetch the relevant ticket"

Read it as in **Read an issue** above.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single Issue of kind `map` with **child** Issues as tickets.

- **Map**: `create_issue {kind: "map", title, body}`, holding the Destination / Notes / Decisions-so-far / Fog body. Where the skill says "labelled `wayfinder:map`", this is what it means: Loom has no such label. Find maps with `list_issues {kind: "map"}`.
- **Child ticket**: `create_issue {parent: <map>, labels: ["wayfinder:<type>"]}` (`research`/`prototype`/`grilling`/`task`), exactly one type label.
- **Blocking**: native. `blocked_by` on `create_issue`, or `add_blockers {ref, blocked_by}` afterwards (`remove_blockers` to undo). A ticket is unblocked when every blocker is closed.
- **Frontier query**: `frontier {parent: <map>}`: open, unblocked, unclaimed children in map order; first wins.
- **Claim**: `claim {ref}`, the session's first write. It assigns the ticket to the configured user and records your session; `already: true` means your session already holds it. Your own claims: `list_issues {held_by: "me"}`.
- **Resolve**: `close {ref, comment: "<answer>"}`, then `append_to_section {ref: <map>, heading: "Decisions so far", text: "- <gist + link>"}`. Use `append_to_section` rather than `edit_issue` for map appends, since other sessions may be appending too.
