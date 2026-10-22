# 09: `loom init` and the tracker doc

Spec: [Loom v1](../../loom/spec.md)

**What to build:** After `/setup-matt-pocock-skills`, a developer runs `loom init` and their repo's skills now use Loom. The tracker doc is installed, the Agent skills block points at Loom, the Project and any custom triage labels exist, and the developer is told how to register the MCP server.

**Blocked by:** 02 (`loom mcp` shim and `loom stop`), 04 (Labels)

**Status:** done

- [x] The final tracker doc is embedded in the binary: the draft on `prototype/issue-tracker-loom` plus the provenance note, a harness-neutral setup line (stdio server `loom`, command `loom mcp`), the triage-labels/`create_label` hint, and the verify-don't-retry guidance
- [x] Precondition: the first of `CLAUDE.md`/`AGENTS.md` has `## Agent skills` with `### Issue tracker`, and `docs/agents/issue-tracker.md` exists; if not, nothing changes and the output points at the `mattpocock/skills` install and `/setup-matt-pocock-skills`
- [x] Always overwrites `docs/agents/issue-tracker.md`; rewrites only the `### Issue tracker` summary line; reports `updated`/`unchanged` per file
- [x] Creates the Project through the daemon (spawning it as the shim does), with `--slug` outside git or on a collision; an existing Project at the exact Root is a no-op that reports its Slug
- [x] Creates any labels from the "Label in our tracker" column of `docs/agents/triage-labels.md` that the Project lacks, and reports them; skips this if the file is absent
- [x] Prints full MCP registration guidance (plus the `mcpServers` JSON snippet) if no Session has ever been recorded, otherwise a one-line reminder; never edits harness config
- [x] Process-seam tests against temp repos with setup output present, partial and absent
