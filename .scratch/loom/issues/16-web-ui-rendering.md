# Web UI rendering and live updates

Type: grilling
Status: resolved
Assignee: Sean Earle
Blocked by: 

## Question

How is the read-only web UI from [Web UI pages](09-web-ui-pages.md) rendered and kept current? Decide: server-rendered HTML per URL (Go `html/template`, one full page per pane state) vs some client JS for the three-pane layout; how pages update when agents write (plain refresh, polling, or SSE off the Event log); how Markdown bodies and comments are rendered and sanitised; and how the UI is tested (deferred here from [Testing strategy](15-testing-strategy.md); interim plan was HTTP-level tests asserting status and key content per page).

## Answer

Resolved by grilling (2026-10-22).

- **Rendering**: server-rendered Go `html/template`. Every URL (`/`, `/<slug>?filters`, `/<slug>/issues/<n>?tab=`) returns the full three-pane page, or only the pane fragment when `HX-Request` is set (one handler + template set per URL; fragment is a sub-template). Navigation via htmx with `hx-push-url`; back/forward, reload and pasted links work, and pages work with JS off (minus live updates).
- **Client deps**: pinned htmx + its SSE extension + idiomorph, vendored under `/static/` via `go:embed`, with hand-written CSS. No npm, bundler or CDN.
- **Live updates**: SSE. The daemon (sole writer) publishes in-process after each committed write; streams `/events` (home) and `/<slug>/events` (per Project). Named events: `issue-<n>` for the changed Issue and its parent, plus `changed`. List pane and home refresh on `changed`; the Issue pane refreshes only on its own `issue-<n>` (so a Map's board updates when any Ticket changes, including blockers closing).
- **Swaps**: idiomorph morph swaps so open `<details>` diffs and scroll position survive a refresh.
- **Reconnect**: htmx SSE auto-reconnects (e.g. after `loom stop` or version replacement); on reconnect every pane refetches once. No Event-log replay.
- **Markdown**: goldmark + GFM (tables, task lists, strikethrough, autolinks), raw HTML disabled/escaped, so no separate sanitiser; `#N` / `slug#N` refs auto-linked to Issue pages.
- **Testing**: `httptest` against real handlers and a seeded real SQLite file; per URL assert status + key content via `goquery` selectors in both full-page and fragment modes. One SSE test (write via a tool handler → stream emits `issue-<n>`, `issue-<parent>`, `changed`); one Markdown test (raw HTML escaped, refs linked). No browser/headless tests, no golden HTML.

