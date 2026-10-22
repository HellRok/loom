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
An Issue whose parent is a Map, holding one decision question and typed by exactly one wayfinder type label. The children of an ordinary Issue (e.g. the slices `/to-tickets` creates under a spec) are not Tickets, even though the skills call them "tickets".
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

**Frontier**:
The open, unblocked, unclaimed children of an Issue, in creation order.
_Avoid_: Ready queue, backlog

**Claim**:
Assigning an unassigned Issue to oneself before working it, recording the Session that holds it; a claim fails if another Session already holds it, and succeeds again for the Session that does.
_Avoid_: Lock, checkout

**Session**:
One agent connection acting on Loom, identified by an opaque string it presents on every request; its subagents act as the same Session. Distinct from the assignee, which is always the human.
_Avoid_: Agent, client, user
