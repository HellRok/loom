# 05: Hierarchy, blocking and the frontier

Spec: [Loom v1](../../loom/spec.md)

**What to build:** Agents can build parent/child trees and blocked-by graphs, create Maps whose children are well-formed Tickets, and ask for the frontier under any parent in one call.

**Blocked by:** 04 (Labels)

**Status:** ready-for-agent

- [ ] `parent` on `create_issue` and `set_parent {ref, parent|null}`: at most one parent, same Project, cycles rejected; children ordered by `#N`; deleting a parent leaves its children parentless
- [ ] `blocked_by` on `create_issue`, plus `add_blockers` / `remove_blockers`: same Project, self-blocks and cycles rejected, duplicates ignored; deleting an Issue removes its edges; Events for each change
- [ ] Unblocked means every Blocker is closed or deleted
- [ ] `frontier {parent}` returns the open, unblocked, unassigned children in creation order, plus `skipped: {claimed, blocked}`
- [ ] Map rules: a Map has no parent; its direct children are Tickets with exactly one `wayfinder:<type>` label and no triage labels; Tickets cannot have children; violations → `invalid_ticket {valid_types}`; `close {wontfix}` is rejected on Tickets
- [ ] `view_issue` returns children as summaries without bodies
- [ ] `list_issues` gains `parent` and `blocked`
- [ ] All of `create_issue`'s parent, labels and blockers are applied atomically: a rejected create changes nothing
- [ ] Handler-seam tests, including cycle rejection, Ticket rule violations and frontier ordering and skips
