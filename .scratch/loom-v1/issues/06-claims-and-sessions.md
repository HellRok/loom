# 06: Claims and Sessions

Spec: [Loom v1](../../loom/spec.md)

**What to build:** An agent can claim an Issue atomically, and a retried claim can tell whether its own Session already holds it. Agents can find their own claims and release any claim, and every Event and Comment shows which Session made it.

**Blocked by:** 05 (Hierarchy, blocking and the frontier)

**Status:** ready-for-agent

- [ ] `claim {ref}` atomically sets the assignee to the configured human and records the holder Session and claimed-at
- [ ] The same Session re-claiming → `already: true`; another Session → `already_claimed {holder, holder_session, claimed_at}`
- [ ] Claiming a closed or blocked Issue is refused
- [ ] `release` is idempotent, works from any Session, and its Event records who released
- [ ] `list_issues {held_by: <session> | "me"}`
- [ ] Every Event and Comment records the Session alongside the actor
- [ ] Sequential handler-seam tests (no goroutine race tests)
