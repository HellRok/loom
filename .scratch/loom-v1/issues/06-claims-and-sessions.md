# 06: Claims and Sessions

Spec: [Loom v1](../../loom/spec.md)

**What to build:** An agent can claim an Issue atomically, and a retried claim can tell whether its own Session already holds it. Agents can find their own claims and release any claim, and every Event and Comment shows which Session made it.

**Blocked by:** 05 (Hierarchy, blocking and the frontier)

**Status:** done

- [x] `claim {ref}` atomically sets the assignee to the configured human and records the holder Session and claimed-at
- [x] The same Session re-claiming → `already: true`; another Session → `already_claimed {holder, holder_session, claimed_at}`
- [x] Claiming a closed or blocked Issue is refused
- [x] `release` is idempotent, works from any Session, and its Event records who released
- [x] `list_issues {held_by: <session> | "me"}`
- [x] Every Event and Comment records the Session alongside the actor
- [x] Sequential handler-seam tests (no goroutine race tests)
