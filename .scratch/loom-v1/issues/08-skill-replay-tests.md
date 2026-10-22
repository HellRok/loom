# 08: Skill replay tests

Spec: [Loom v1](../../loom/spec.md)

**What to build:** Proof of the v1 bar. Every tracker-touching skill's sequence of tracker operations replays against Loom with no workarounds, and each output carries what the skill needs for its next step.

**Blocked by:** 04 (Labels), 05 (Hierarchy, blocking and the frontier), 06 (Claims and Sessions)

**Status:** ready-for-agent

- [ ] One table-driven test per skill: `to-spec`, `to-tickets`, `triage`, `implement`, `implement-spec`, `code-review`, `wayfinder` and `ask-matt`
- [ ] Each replays the skill's operations in order from the operations inventory (branch `research/skill-operations-inventory`), mapped to tools as in the tracker doc
- [ ] Each step asserts that the output carries what the skill needs next (refs, URLs, versions, frontier entries, `already` flags, valid-label lists)
- [ ] Any gap found is fixed in the tool surface (with golden `tools/list` updated) or recorded as a new ticket
