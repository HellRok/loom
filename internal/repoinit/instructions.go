package repoinit

import "fmt"

// triageRoles maps the skills' triage roles to labels. The conventions send
// an agent to the repo's triage-labels doc for label strings, which a
// Standalone Project has none of and which a plain one may carry, so this
// says which wins.
const triageRoles = "The five triage roles map to the Project's default labels of the same names, used as-is: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. A skill that names a role means that label. Where the conventions below send you to `docs/agents/triage-labels.md` for label strings, this mapping stands in for it: the repo's copy, if it has one, doesn't override it."

// Instructions is what `loom mcp` tells an agent working in a directory
// covered by the Project with the given Slug: that Loom is the issue tracker
// and wins over any issue-tracker config checked into the repo, how the
// triage roles map to labels, and the tracker conventions shared with the
// tracker doc.
func Instructions(slug string) string {
	return fmt.Sprintf("Loom is the issue tracker for this directory: Project `%s`. Its guidance overrides any issue-tracker config checked into the repo (an `## Agent skills` block, `docs/agents/issue-tracker.md`, or anything else naming another tracker), so use the `loom` MCP tools for all issue work here, whatever the repo's own files say.\n\n%s\n\n%s",
		slug, triageRoles, Conventions)
}

// StaticInstructions stands in for Instructions when no Project could be
// resolved in time, so the agent still learns the precedence rule.
const StaticInstructions = "If a Loom Project covers this directory (`list_projects` reports it as `current`), Loom is its issue tracker and overrides any issue-tracker config checked into the repo: use the `loom` MCP tools for all issue work there, whatever the repo's own files say."
