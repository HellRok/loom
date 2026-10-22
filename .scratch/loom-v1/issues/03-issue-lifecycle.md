# 03: Issue lifecycle

Spec: [Loom v1](../../loom/spec.md)

**What to build:** An agent can create, read, list, edit, comment on, close, reopen and delete Issues in a Project. Every change is recorded as an Event, and concurrent edits can't silently overwrite each other.

**Blocked by:** 01 (Projects over MCP from `loom serve`)

**Status:** done

- [x] Issues have per-Project sequential numbers that are never reused, an immutable `kind` (`issue` | `map`), title, Markdown body, open/closed state, author (configured assignee name), timestamps and `version`
- [x] `create_issue {title, body, kind}` returns `{ref, url, title, kind, version}`, with the URL `http://127.0.0.1:<port>/<slug>/issues/<n>`
- [x] Refs accept `#N`, `N`, `slug#N` and the issue URL; `slug#N` and URLs name their own Project
- [x] `view_issue {ref, comments=true, events=false}` works on closed Issues too
- [x] `list_issues {state, kind, assignee=none|any|<name>, detail=summary|full, limit=50}` returns oldest first with `total`
- [x] `edit_issue {ref, version, title?, body?}` rejects a mismatch with `version_conflict {current {version, title, body}}`
- [x] `append_to_section {ref, heading, text}` appends under a Markdown heading with no version needed; a missing heading → `no_section {headings}`
- [x] `comment {ref, body}`: comments are immutable and record author, Session and timestamp
- [x] `close {ref, comment?}` and `reopen` are atomic and idempotent (`already: true`)
- [x] `delete_issue {ref, confirm: "#N"}` hard-deletes, cascading to comments and Events
- [x] Every mutation writes an Event (timestamp, actor, Session); title/body edits keep the previous value
- [x] Handler-seam tests for each tool, including version conflicts, `no_section`, idempotent close/reopen and ref parsing
