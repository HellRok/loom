# Tracker operations the skills invoke

Type: research
Status: resolved
Blocked by: 

## Question

Sweep every skill in `~/.claude/skills/` that touches the issue tracker (at least `to-spec`, `to-tickets`, `triage`, `implement`, `implement-spec`, `code-review`, `wayfinder`, `ask-matt`, `setup-matt-pocock-skills` and its `issue-tracker-*.md` / `triage-labels.md`) and inventory every tracker operation they invoke: create, view (with comments), list (which filters), comment, label/unlabel, close (with reason?), assign, parent/child, blocking, frontier query, PR-related operations. For each: which skills use it, inputs, and what output the skill needs back.

## Answer

Findings: branch `research/skill-operations-inventory`, file `research/skill-operations-inventory.md` (local skills match upstream `mattpocock/skills@5beb35b`).

Operations required: create (returns id + URL), view with comments (closed issues too), list (filters: state, label, no-label, parent, unassigned; oldest-first; counts), comment, label/unlabel, close-with-comment (no close reason; `wontfix` label carries it; no reopen), assign/claim (concurrency-safe), parent/child (ordered), blocked-by (open blockers), frontier query, edit title/body, delete, create label, resolve `#N`/URL references.

Surprises feeding later tickets:
- `/triage` requires exactly one of `bug` / `enhancement` per issue: absent from the planned default label set.
- Wayfinder "delete" tickets has no tracker-doc mapping.
- Concurrent appends to the map body risk lost updates with whole-body replace.
- `/implement` never closes its ticket (GitHub relies on PR merge); the Loom doc must say how work closes.
- PR features are flag-gated or tracker-conditional: Loom doc should mark them not applicable / closes directly; `#N` in commits may collide with GitHub autolinks.
