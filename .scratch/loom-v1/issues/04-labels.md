# 04: Labels

Spec: [Loom v1](../../loom/spec.md)

**What to build:** Every Project has a defined label vocabulary, starting with the 11 labels the skills use. Agents can apply and remove labels atomically, filter by them and manage them. Undefined labels are rejected, and the error lists the valid ones.

**Blocked by:** 03 (Issue lifecycle)

**Status:** done

- [x] New Projects are seeded with `bug`, `enhancement`, `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`, `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling` and `wayfinder:task`
- [x] Labels have a name unique per Project, an optional description and a colour from a fixed pool; no rename
- [x] `label {ref, add, remove}` is one atomic transition with labelled/unlabelled Events; `labels` on `create_issue`
- [x] Applying an undefined label → `undefined_label {valid}`
- [x] `list_labels` with usage counts; `create_label` is idempotent; `delete_label` works only when the label is unused, otherwise the error lists the Issues using it
- [x] `list_issues` gains `labels` (all-of) and `no_labels`
- [x] `close {ref, comment, wontfix: true}` applies `wontfix`, comments and closes atomically
- [x] Handler-seam tests for the above
