# 07: Moving Projects and the Project CLI

Spec: [Loom v1](../../loom/spec.md)

**What to build:** When a repo moves, its Project can be rebound to the new location without losing anything, and agents are steered toward that instead of creating a duplicate. A developer can also move or delete Projects from the shell.

**Blocked by:** 01 (Projects over MCP from `loom serve`)

**Status:** ready-for-agent

- [ ] `no_project` flags Projects whose Root no longer exists on disk as `root_missing`
- [ ] `move_project {project}` rebinds that Project's Root to the caller's resolved location (main-worktree git root, or cwd outside git), with the same duplicate-Root and nesting rules as creation; the Slug is unchanged
- [ ] `loom project mv <slug> [dir]` and `loom project rm <slug>` go through the daemon
- [ ] Handler-seam tests for `move_project` and `root_missing`; golden `tools/list` updated
