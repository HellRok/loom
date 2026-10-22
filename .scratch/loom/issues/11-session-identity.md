# Agent session identity and the assignee

Type: grilling
Status: resolved
Assignee: Sean Earle
Blocked by: 

## Question

Should an agent session be able to get its own session id, so a claim (and the "assigned to" field) shows *which* session holds an Issue, not just the single configured user? Decide how a session gets its id (a tool that issues one? made up by the agent? derived by the shim?), whether claim takes it and lets the same session re-claim (so a claim retried after "outcome unknown" can be checked), how it relates to the configured assignee name, and what the event log and UI show.

## Answer

Resolved by grilling (2026-10-22).

- **Purpose**: a re-claim after "outcome unknown" can tell its own claim from another session's; as a byproduct, the UI shows which session did what.
- **Assignee unchanged**: still the single configured human name (skills "assign to the dev"; `assignee=` filters keep working). A Claim additionally records the **holder session**.
- **Source**: the shim generates a UUIDv7 once per `loom mcp` process and forwards it as `X-Loom-Session` on every call. Subagents share their parent's shim, so they act as its session. A restarted shim is a new session; its old claims look foreign and get released.
- **Direct HTTP**: may pass the `X-Loom-Session` header (preferred) or a `session` tool argument. If both are present the header wins silently.
- **Required everywhere**: every MCP tool call, reads included, without a session is rejected (`no_session`). Any content, 16–256 characters (runes), else `invalid_session`. Opaque to Loom. Web UI and `/healthz` exempt.
- **Claim**: same session re-claiming → success, `already: true`. Another session → `already_claimed {holder, holder_session, claimed_at}`. Any session may `release` (idempotent); the Event records who.
- **Events and Comments**: every one records the session alongside the actor.
- **Session record**: global (not per-Project), keyed by the string, fields first seen + last seen only. Upserted best-effort on every call (failure logged, call still succeeds). No register/end call. Everything else (Projects, activity) comes from searching by session.
- **Stale claims**: no auto-release. Wherever a claim appears (Map board, Issue pane) the UI shows the holder session's idle time.
- **Search**: `list_issues {held_by: <session> | "me"}` (`"me"` = caller's own session). Web UI gets a global Sessions page (by last seen); a session's page lists the Issues it holds and its Events across Projects, newest first. General Event search stays in the Search fog.
- **Display**: sessions shown as their **last** 8 characters (UUIDv7's leading chars are a timestamp, so nearby sessions share them), full string on hover, linking to the session page.
- Glossary: added **Session**; sharpened **Claim**.

## Comments

- From [MCP tool surface](08-mcp-tool-surface.md): today `claim {ref}` sets the single configured assignee. Sessions A and B both claim as `sean`, so after a lost reply A's retry fails with `already_claimed by sean` and A can't tell whether it won the claim. The user wants agents to be able to identify themselves.
