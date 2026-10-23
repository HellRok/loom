package repoinit

import "fmt"

// triageRoles maps the skills' triage roles to labels. The conventions send
// an agent to the repo's triage-labels doc for label strings, which a
// Standalone Project has none of and which a plain one may carry, so this
// says which wins.
const triageRoles = "The five triage roles map to the Project's default labels of the same names, used as-is: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. A skill that names a role means that label. Where the conventions below send you to `docs/agents/triage-labels.md` for label strings, this mapping stands in for it: the repo's copy, if it has one, doesn't override it."

// standaloneDomainDocs is how a Standalone Project's agent does the domain
// skills' docs without repo files: the Glossary lives in Loom, ADR-worthy
// decisions become Issues.
const standaloneDomainDocs = "## Domain docs\n\n" +
	"This Project is Standalone: its domain docs live in Loom, not in repo files.\n\n" +
	"- **Glossary**: use `view_glossary` and `edit_glossary` wherever a skill says to read or write `GLOSSARY.md`. Read the repo's own `GLOSSARY.md` (if it has one) first and Loom's Glossary second; Loom's wins on conflict. Write only to Loom's, never to the repo's.\n" +
	"- **ADRs**: don't write `docs/adr/`. Propose an ADR-worthy decision as a `needs-triage` Issue instead (`create_issue`), with the context, decision and consequences in its body."

// Instructions is what `loom mcp` tells an agent working in a directory
// covered by the Project with the given Slug: that Loom is the issue tracker
// and wins over any issue-tracker config checked into the repo, how the
// triage roles map to labels, the tracker conventions shared with the
// tracker doc and, for a Standalone Project, how to do domain docs in Loom.
// A Standalone Project's repo doesn't carry Loom conventions, so its
// instructions leave out the commit trailer.
func Instructions(slug string, standalone bool) string {
	conventions := Conventions
	if standalone {
		conventions = StandaloneConventions
	}
	out := fmt.Sprintf("Loom is the issue tracker for this directory: Project `%s`. Its guidance overrides any issue-tracker config checked into the repo (an `## Agent skills` block, `docs/agents/issue-tracker.md`, or anything else naming another tracker), so use the `loom` MCP tools for all issue work here, whatever the repo's own files say.\n\n%s\n\n%s",
		slug, triageRoles, conventions)
	if standalone {
		out += "\n\n" + standaloneDomainDocs
	}
	return out
}

// StaticInstructions stands in for Instructions when no Project could be
// resolved in time, so the agent still learns the precedence rule.
const StaticInstructions = "If a Loom Project covers this directory (`list_projects` reports it as `current`), Loom is its issue tracker and overrides any issue-tracker config checked into the repo: use the `loom` MCP tools for all issue work there, whatever the repo's own files say."
