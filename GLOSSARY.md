# Loom

A local-first issue tracker that agents drive through MCP, built to serve as the issue tracker for the `mattpocock/skills` engineering skills. Humans observe it; agents act on it.

## Language

**Issue**:
The unit of tracked work or decision in Loom, the thing a skill creates when it says "publish to the issue tracker."
_Avoid_: Ticket (except as below), task, card

**Project**:
One tracked codebase (typically one repo), identified by its **Root**; every Issue and Label belongs to exactly one Project.
_Avoid_: Repo, workspace

**Root**:
The directory a Project is anchored to; work happening anywhere beneath it belongs to that Project, the deepest Root winning when Roots nest.
_Avoid_: Path, home

**Standalone**:
A Project whose skill configuration lives in Loom rather than in files under its Root, for repos that can't carry those files.
_Avoid_: Global, repo-less

**Glossary**:
A Standalone Project's domain glossary, held in Loom in place of a checked-in `GLOSSARY.md`; it takes precedence over any glossary the repo itself carries.
_Avoid_: Dictionary, terms

**Slug**:
A Project's short, unique name, used wherever a Project is named rather than found by its Root.
_Avoid_: Project name, key

**Loom home**:
The one directory holding everything a Loom installation keeps on a machine: its config, its data, and its daemon's log; one daemon serves exactly one Loom home.
_Avoid_: Data dir, config dir

**Label**:
A named tag defined on a Project and applied to Issues; a new Project starts with a default set.
_Avoid_: Tag

**Ticket**:
An Issue whose parent is a Map, holding one decision question and typed by exactly one wayfinder type label. A Ticket may itself have children (e.g. the slices `/to-tickets` creates from it); like the children of any ordinary Issue (such as a spec's slices) they are not Tickets, even though the skills call them "tickets".
_Avoid_: using "ticket" for Issues in general or for a spec's children

**Map**:
An Issue of the map kind that indexes a wayfinder effort (its destination, decisions so far, and fog), with Tickets as its children. Its kind is fixed when it is created.
_Avoid_: Epic, plan

**Event**:
One recorded change to an Issue, kept in that Issue's history.
_Avoid_: Activity, audit entry

**Blocker**:
An Issue that must be closed before another Issue it blocks can be taken.
_Avoid_: Dependency, prerequisite

**Open child**:
A child Issue that isn't closed yet (closing as won't-fix counts as closed). A closed child doesn't hide the open Issues below it: they count as the Issue's open children too. While an Issue other than a Map has one, it can't be taken, as if blocked, but no Blocker is recorded: the block lifts by itself when the last one closes.
_Avoid_: Sub-issue, implicit blocker

**Frontier**:
The open, unblocked, unclaimed Issues among a set, in creation order: either the children of an Issue, or a whole Project's Issues other than its Maps. Whether an Issue is tagged `ready-for-agent` plays no part; a tag is a claim about readiness, the Frontier is the fact.
_Avoid_: Ready queue, backlog

**Claim**:
Assigning an unassigned Issue to oneself before working it, recording the Session that holds it; a claim fails if another Session already holds it, and succeeds again for the Session that does.
_Avoid_: Lock, checkout

**Session**:
One agent connection acting on Loom, identified by an opaque string it presents on every request; its subagents act as the same Session. Distinct from the assignee, which is always the human.
_Avoid_: Agent, client, user
