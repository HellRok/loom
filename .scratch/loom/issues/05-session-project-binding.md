# How a session binds to a Project

Type: grilling
Status: resolved
Blocked by: 

## Question

`loom mcp` runs in the agent's working directory. How does that directory map to a Project? Options to weigh: git toplevel path, git remote URL, an explicit `loom` registration / config file in the repo, auto-create on first use vs require `loom init`. What happens for worktrees, moved repos, or a directory that isn't a git repo?

## Answer

Resolved by grilling (2026-10-22).

- **Identity**: a Project has exactly one registered Root directory (not git remote, not a marker file). Nothing is written into the repo.
- **Resolution** (daemon-side; the shim forwards only raw cwd):
  1. If cwd is inside a git worktree, replace it with the main worktree root (parent of `git rev-parse --git-common-dir`), so linked worktrees outside the repo resolve to the same Project.
  2. Pick the Project whose Root is the longest prefix of that path.
- **Override**: every tool takes an optional `project` (Slug) argument that bypasses cwd resolution. A request with no cwd header (direct HTTP client) must pass `project`.
- **No match**: error listing existing Projects with their Roots and pointing to `create_project`. No auto-create.
- **Creation**: explicit only: `create_project` MCP tool + `loom init` CLI; seeds the 12 default labels. `setup-matt-pocock-skills` (via `issue-tracker-loom.md`) is the natural place to call it.
  - In a git repo: Root = git root (folder holding `.git`); Slug = that folder's basename, no argument needed. Slug collision → error, agent must pass `slug`.
  - Outside git: `slug` required; Root = cwd.
  - Slugs unique, `[a-z0-9-]`. No separate display name.
- **Nesting**: a Root inside another Root is allowed (longest prefix wins); exact duplicate Root rejected.
- **Deletion**: hard delete cascading to Issues, comments, events, labels; frees Slug and Root. `delete_project` MCP tool (requires `confirm: "<slug>"`) + `loom project rm` CLI.
- **Moving / renaming**: deferred (map fog). A moved repo currently just stops resolving.
- Glossary: sharpened **Project**; added **Root**, **Slug**.
