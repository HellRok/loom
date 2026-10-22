# `loom init` command

Type: grilling
Status: resolved
Assignee: Sean Earle
Blocked by: 10

## Question

What does `loom init` do to install Loom as a repo's issue tracker for the engineering skills? Decide: does it write `docs/agents/issue-tracker.md` (the draft on `prototype/issue-tracker-loom`), and what happens if one already exists; does it create the Project eagerly (despite lazy creation via `no_project`) and does it need the daemon running; does it register the MCP server (`claude mcp add`) or only print the command; how it sequences with `setup-matt-pocock-skills` (run first, so setup finds and keeps the existing tracker doc?) and the `## Agent skills` block; and how triage-label overrides from `docs/agents/triage-labels.md` get created as Loom labels.

## Answer

Resolved by grilling (2026-10-22).

- **Order**: Matt's skills first, Loom second. Install `mattpocock/skills`, run `/setup-matt-pocock-skills` (any tracker choice; local markdown is simplest), then `loom init`. Loom openly *amends* setup's output rather than replacing setup.
- **Precondition**: setup has run iff `CLAUDE.md`/`AGENTS.md` (setup's file-picking order) has an `## Agent skills` block with `### Issue tracker`, **and** `docs/agents/issue-tracker.md` exists. Otherwise (incl. half-done setup) init stops, changes nothing, and points at the `mattpocock/skills` install + `/setup-matt-pocock-skills`.
- **Tracker doc**: always overwrites `docs/agents/issue-tracker.md` with the doc embedded in the binary. No marker, no `--force`; git is the safety net. Re-running after a Loom upgrade refreshes it. Output reports `updated`/`unchanged` per file.
- **Agent skills block**: rewrites only the `### Issue tracker` summary line: "Issues are tracked in Loom, a local MCP issue tracker (installed by `loom init`, replacing the tracker chosen in `/setup-matt-pocock-skills`). See `docs/agents/issue-tracker.md`." Rest of the block untouched.
- **Project**: created eagerly via the daemon (spawned as the shim does; single writer), same Root/Slug rules as `create_project`; `--slug` outside git or on Slug collision. A Project already at this exact Root is a no-op (reports its Slug). Lazy `no_project` → `create_project` remains the fallback.
- **Triage labels**: if `docs/agents/triage-labels.md` exists, parse its "Label in our tracker" column and create any labels the Project lacks (default colours), reporting them. Absent file → skip.
- **MCP registration**: harness-agnostic; init never edits any harness config. If the daemon has never recorded a Session, print the generic instruction (stdio MCP server `loom`, command `loom mcp`) plus the common `mcpServers` JSON snippet `{"loom": {"command": "loom", "args": ["mcp"]}}`; otherwise a one-line reminder.
- **Changes to the tracker-doc draft** (`prototype/issue-tracker-loom`), from [Contents of issue-tracker-loom.md](10-issue-tracker-loom-doc.md):
  - Provenance note under the title: "Written by `loom init`. Re-running `/setup-matt-pocock-skills` will overwrite this; run `loom init` again afterwards."
  - Setup line becomes harness-neutral (stdio server `loom`, command `loom mcp`), dropping `claude mcp add`.
  - Labels bullet gains: "Label strings come from `docs/agents/triage-labels.md`; if one is rejected as undefined, create it with `create_label` and retry."

## Comments
