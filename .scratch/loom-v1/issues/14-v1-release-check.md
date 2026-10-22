# 14: v1 release check

Spec: [Loom v1](../../loom/spec.md)

**What to build:** Loom v1 is ready to release. The schema is frozen as v1 with snapshot tests guarding future migrations, and the whole suite passes on both supported platforms.

**Blocked by:** 01–13

**Status:** ready-for-agent

- [ ] A v1 snapshot DB is committed under `testdata/`
- [ ] Tests open each snapshot with the current binary's migration path, refuse a DB newer than the binary, and exit on a failed migration
- [ ] `go test ./...` passes on Linux and macOS
- [ ] The skill replay tests (08) pass against the final tool surface
