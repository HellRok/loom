# 11: Map Board, Decided and History tabs

Spec: [Loom v1](../../loom/spec.md)

**What to build:** A developer opens a Map and sees where the effort stands on a Board of Frontier, Claimed and Blocked Tickets, and reviews the route walked on the Decided tab. Any Issue's History tab shows its Events, with diffs for edits.

**Blocked by:** 05 (Hierarchy, blocking and the frontier), 06 (Claims and Sessions), 10 (Web UI shell)

**Status:** done

- [x] A Map opens on the Board tab with Frontier, Claimed and Blocked columns; cards show type label, title, `#N`, assignee and open Blockers
- [x] The Decided tab lists closed Tickets with the gist of each resolution comment
- [x] Map tabs are Board, Decided, Overview and History; plain Issues have Overview and History; the tab is selected via `?tab=`
- [x] History lists Events newest first; title/body edits expand (`<details>`) to show a diff
- [x] `httptest` + `goquery` tests for each tab in both modes
