# Data model

Type: grilling
Status: resolved
Blocked by: 04

## Question

What are Loom's entities and relationships: Project, Issue, Label, Comment, Event, parent/child, blocked-by, assignee? Which issue states exist beyond open/closed, and are close reasons (completed / wontfix / out of scope) native? How are Maps and Tickets represented: purely via labels + parent, or something more first-class?

## Answer

Resolved by grilling (2026-10-22).

- **Issue**: `kind` (`issue` | `map`), set at creation, immutable. State open/closed; reopen allowed; no close-reason enum (`wontfix` label + comment carry it). Fields: title, Markdown body, author (configured name), created/updated timestamps, `version` (incremented on every edit; edits must pass the version they read, mismatch → conflict error, never silent overwrite).
- **Delete**: hard delete, cascading to comments, events, label links and blocking edges; children become parentless; `#N` never reused.
- **Parent/child**: at most one parent, same Project, cycles rejected. Children ordered by ascending `#N` (no position field). Frontier works under any parent (so `/implement-spec` uses it on spec children).
- **Map rules** (the Map's specialness): a Map has no parent; its direct children are Tickets; a Ticket must carry exactly one `wayfinder:<type>` label (research/prototype/grilling/task), no triage labels, and cannot have children. Violations rejected with errors listing valid types. Map sections (Destination, Notes, Decisions so far, Not yet specified, Out of scope) stay in the Markdown body.
- **Spec**: stays a plain Issue; its children are plain child Issues, not Tickets.
- **Blocked-by**: many-to-many edges within one Project, independent of parentage; self-blocks and cycles rejected, duplicates ignored. Unblocked = every blocker closed (or deleted).
- **Assignee**: single nullable name. Claim = atomic "set if unassigned"; loser gets an error naming the holder. Unassign allowed.
- **Comments**: immutable; author + timestamp.
- **Events**: one per mutation (created, title/body edited, labelled/unlabelled, assigned/unassigned, parent set/cleared, blocker added/removed, closed/reopened, commented), with timestamp + actor; title/body edits store the previous value for UI diffs.
- **Labels**: name (unique per Project), optional description, colour picked at random from a fixed pool. Creatable; deletable only when unused (error lists using Issues); no rename. Seed drops to **11**: `wayfinder:map` is replaced by `kind: map`.
- Glossary: sharpened **Map**, **Ticket** (flagging that spec children are not Tickets); added **Event**, **Blocker**, **Frontier**, **Claim**.
