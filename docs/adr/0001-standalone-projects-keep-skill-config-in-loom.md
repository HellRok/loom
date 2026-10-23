# Standalone Projects keep their skill config in Loom

Some repos can't carry the `mattpocock/skills` config files (e.g. a team repo where others use different flows), yet we still want to track work there in Loom. A Standalone Project keeps that config in Loom instead: `loom init --standalone` writes nothing under the Root, and `loom mcp` resolves the Project for its cwd at startup and serves MCP instructions carrying the tracker conventions, a precedence rule over any checked-in tracker config, and (for Standalone Projects only) the `view_glossary`/`edit_glossary` tools that hold the Project's Glossary in place of `GLOSSARY.md`.

## Considered Options

- **A global-memory snippet alone** (the user pastes "repos with a Loom Project use Loom" into `~/.claude/CLAUDE.md`): needs manual setup per harness and makes every repo's agent check for a Project. Kept only as an optional fallback printed by init, for harnesses that ignore MCP instructions.
- **Leaving domain docs out of Loom**: the domain skills would then write `GLOSSARY.md` into a repo that can't take it. We store the Glossary but not ADRs; agents propose ADR-worthy decisions as `needs-triage` Issues until that proves painful.

## Consequences

- Standalone is a stored property of the Project, since the instructions differ; switching a Project back to plain keeps its Glossary but stops showing it to agents.
- Instructions are per-cwd, so the shim contacts the daemon before answering `initialize`, falling back to static conditional text after a short timeout.
- Standalone instructions omit the commit-trailer convention: the repo's commits aren't Loom's to shape, so agents are told to close Issues citing the commit SHA but never to add a `Loom:` trailer.
