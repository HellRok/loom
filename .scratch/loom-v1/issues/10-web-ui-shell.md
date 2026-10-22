# 10: Web UI shell: home, issue list, Issue Overview

Spec: [Loom v1](../../loom/spec.md)

**What to build:** A developer opens `http://127.0.0.1:7337/` and can browse every Project and Issue read-only. That means a home page of Projects, a three-pane Project page with saved views and filters, and an Issue's Overview with its rendered body and comment thread.

**Blocked by:** 04 (Labels)

**Status:** done

- [x] Server-rendered `html/template`; each URL returns the full page, or only the pane fragment when `HX-Request` is set; htmx navigation with `hx-push-url`; works with JS off
- [x] htmx, the SSE extension and idiomorph are pinned and vendored under `/static/` with hand-written CSS, all embedded in the binary
- [x] `/`: each Project with Slug, Root, open count and last activity, and each open Map with a progress bar and its next frontier Tickets (variant C of `prototype/web-ui-pages`)
- [x] `/<slug>?<filters>`: nav with a Project switcher, saved views (Open, Maps, Needs triage, Ready for agent, Unassigned, Closed) and labels with counts; a list of rows showing `#N`, parent, updated time, status glyph, title and coloured labels, plus a filter box; filters stay in the URL when an Issue is selected (variant B)
- [x] `/<slug>/issues/<n>`: the Overview tab with a key/value block (state, assignee, labels, parent, Blockers with closed ones struck through, what it blocks, children, version), the rendered body and the comment thread with `#comment-<id>` anchors
- [x] Markdown via goldmark with GFM and raw HTML escaped; `#N`/`slug#N` linked to Issue pages
- [x] `httptest` + `goquery` tests per URL in full-page and fragment modes; one Markdown test (HTML escaped, refs linked)
