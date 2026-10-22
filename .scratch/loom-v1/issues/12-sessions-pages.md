# 12: Sessions pages

Spec: [Loom v1](../../loom/spec.md)

**What to build:** A developer can see which agent Session holds what, spot stale claims by idle time, and look at everything a Session did across Projects.

**Blocked by:** 06 (Claims and Sessions), 10 (Web UI shell)

**Status:** ready-for-agent

- [ ] Sessions are shown as their last 8 characters, with the full string on hover, linking to the Session page
- [ ] Wherever a claim appears (Map Board cards, Issue Overview, list status), the holder Session's idle time is shown
- [ ] A global Sessions page ordered by last seen
- [ ] A per-Session page listing the Issues it holds and its Events across Projects, newest first
- [ ] `httptest` + `goquery` tests
