# 13: Live updates over SSE

Spec: [Loom v1](../../loom/spec.md)

**What to build:** Pages update on their own as agents write, without losing open diffs or scroll position, and recover after the daemon restarts.

**Blocked by:** 10 (Web UI shell)

**Status:** done

- [x] After each committed write the daemon publishes in-process to the SSE streams `/events` (home) and `/<slug>/events`
- [x] Each write emits `issue-<n>` for the changed Issue and for its parent, then `changed`
- [x] The list pane and home refresh on `changed`; the Issue pane refreshes only on its own `issue-<n>` (so a Map's Board updates when any Ticket changes)
- [x] Swaps use idiomorph morphing, so open `<details>` and scroll position survive
- [x] On reconnect every pane refetches once; no Event replay
- [x] One SSE test: a write through a tool handler makes the stream emit `issue-<n>`, `issue-<parent>` and `changed`
