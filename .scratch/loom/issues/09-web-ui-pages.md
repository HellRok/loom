# Web UI pages

Type: prototype
Status: resolved
Assignee: Sean Earle
Blocked by: 07

## Question

What does the read-only web UI show and how is it laid out: project list, issue list with filters, issue detail with Markdown body, comments and event timeline, and a wayfinder Map view that shows the frontier and blocking? Prototype rough HTML pages to react to.

## Answer

Resolved by prototype (2026-10-22). Prototype: branch `prototype/web-ui-pages`, file `prototype/web-ui-pages.prototype.html` (three variants, `?variant=A|B|C`; verdict: C for home, B for everything else).

- **Home (`/`)**: variant C. A text-first project list: each Project shows Slug, Root, open count and last activity, then each open Map with a progress bar (closed/total Tickets) and its frontier Tickets as "next:" links. No cross-project activity feed.
- **Inside a Project**: variant B three-pane layout.
  - Left: Project switcher with open counts, saved views (Open, Maps, Needs triage, Ready for agent, Unassigned, Closed) and labels with counts.
  - Middle: compact issue list, oldest first. Each row shows `#N`, parent, updated time, a status glyph (closed / claimed / blocked), title and labels, plus a filter box and the active filters.
  - Right: the selected Issue. Filters stay in the URL when you select an Issue, so links can be shared.
- **Issue detail (right pane)**: two tabs, **Overview** and **History**.
  - Overview: a key/value block (state + status, assignee, labels, parent, blocked by with closed blockers struck through, blocks, children, version), then the Markdown body, then the **discussion thread** of comments below it (not a separate tab).
  - History: the Event log, newest first; title/body edits expand to show a diff.
- **Map view**: a Map opens on a **Board** tab with Frontier, Claimed and Blocked columns. Cards show the type label, title, `#N`, assignee and open blockers. Closed Tickets move to their own **Decided** tab (to save space on the board): a list of each closed Ticket with the gist of its resolution comment. Overview (the Map document + discussion) and History are the remaining tabs (Board, Decided, Overview, History). Amended after review; the prototype still shows Decided as a fourth column.
- **URLs**: `/`, `/<slug>` (+ filter query), `/<slug>/issues/<n>` (+ `?tab=`), matching the refs the MCP tool surface uses.
- **Not decided here**: how the pages get rendered (server templates vs client JS) and live updates stay in the fog; search is just a text filter in the prototype, and full-text search is still in the fog.

## Comments
- From [Data model](07-data-model.md): Map is a native `kind`, so the Map view keys off kind, not a label. Labels have colours; Events store previous title/body for diffs.
- From [MCP tool surface](08-mcp-tool-surface.md): issue URLs are `http://127.0.0.1:<port>/<slug>/issues/<n>` (returned by create/view and accepted as refs), and comment anchors are `#comment-<id>`. What the "assigned to" column shows may change with [Agent session identity and the assignee](11-session-identity.md).
