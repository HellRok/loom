// Package repoinit holds what `loom init` does to a repo's files: detecting
// that /setup-matt-pocock-skills ran, installing the tracker doc, repointing
// the Agent skills block and reading the triage labels.
package repoinit

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed conventions.md.tmpl
var conventionsTmpl string

// Conventions is everything the tracker doc tells an agent about using Loom
// as the issue tracker, without the parts that only make sense in a doc
// written into a repo. The tracker doc and the MCP instructions are both
// built from it. Its first paragraph is the introduction; the rest follows
// the doc's registration paragraph.
var Conventions = conventions(true)

// StandaloneConventions is Conventions for a Standalone Project, whose repo
// carries no Loom conventions: it leaves out the commit trailer.
var StandaloneConventions = conventions(false)

func conventions(trailers bool) string {
	var b strings.Builder
	t := template.Must(template.New("conventions").Parse(conventionsTmpl))
	if err := t.Execute(&b, struct{ Trailers bool }{trailers}); err != nil {
		panic(err)
	}
	return b.String()
}

const (
	docTitle = "# Issue tracker: Loom\n\n" +
		"> Written by `loom init`. Re-running `/setup-matt-pocock-skills` will overwrite this; run `loom init` again afterwards."

	docRegistration = "Register the server once per machine in your agent harness: a stdio MCP server named `loom`, command `loom mcp`. The Project is found from the working directory; if a call reports no Project, create one with `create_project`."
)

// TrackerDoc is the Loom tracker doc installed as docs/agents/issue-tracker.md.
var TrackerDoc = trackerDoc()

func trackerDoc() string {
	intro, rest, _ := strings.Cut(Conventions, "\n\n")
	return strings.Join([]string{docTitle, intro, docRegistration, rest}, "\n\n")
}

// TrackerDocPath is the tracker doc's path, relative to the repo root.
const TrackerDocPath = "docs/agents/issue-tracker.md"

// TriageLabelsPath is the triage labels doc's path, relative to the repo root.
const TriageLabelsPath = "docs/agents/triage-labels.md"

// Summary is the line Loom writes under `### Issue tracker`.
const Summary = "Issues are tracked in Loom, a local MCP issue tracker (installed by `loom init`, replacing the tracker chosen in `/setup-matt-pocock-skills`). See `docs/agents/issue-tracker.md`."

// agentFiles are the files setup writes its Agent skills block to, in the
// order it picks one.
var agentFiles = []string{"CLAUDE.md", "AGENTS.md"}

// NotSetUpError reports that /setup-matt-pocock-skills hasn't (fully) run.
type NotSetUpError struct {
	Reason string
}

func (e *NotSetUpError) Error() string {
	return e.Reason + ".\nInstall the skills from https://github.com/mattpocock/skills and run /setup-matt-pocock-skills in this repo first, then run `loom init` again. To use Loom here without adding files to the repo, run `loom init --standalone`. Nothing was changed."
}

// Repo is a repo that setup has run in.
type Repo struct {
	// Root is the directory holding CLAUDE.md/AGENTS.md and docs/.
	Root string
	// AgentsFile is the setup file with the Agent skills block, relative to
	// Root.
	AgentsFile string
}

// Inspect checks that setup ran in root: the first of CLAUDE.md/AGENTS.md
// has an `## Agent skills` block with an `### Issue tracker` section, and the
// tracker doc exists. It returns a *NotSetUpError otherwise.
func Inspect(root string) (Repo, error) {
	var file string
	for _, name := range agentFiles {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			file = name
			break
		}
	}
	if file == "" {
		return Repo{}, &NotSetUpError{"Neither CLAUDE.md nor AGENTS.md exists"}
	}
	b, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		return Repo{}, err
	}
	if _, _, ok := findIssueTracker(strings.Split(string(b), "\n")); !ok {
		return Repo{}, &NotSetUpError{fmt.Sprintf("%s has no `## Agent skills` block with an `### Issue tracker` section", file)}
	}
	if _, err := os.Stat(filepath.Join(root, TrackerDocPath)); err != nil {
		return Repo{}, &NotSetUpError{TrackerDocPath + " doesn't exist"}
	}
	return Repo{Root: root, AgentsFile: file}, nil
}

// WriteTrackerDoc overwrites the tracker doc with the embedded one and
// reports whether its content changed.
func (r Repo) WriteTrackerDoc() (changed bool, err error) {
	return writeIfChanged(filepath.Join(r.Root, TrackerDocPath), TrackerDoc)
}

// PointAtLoom rewrites the `### Issue tracker` summary line of the Agent
// skills block, leaving everything else alone, and reports whether the file
// changed.
func (r Repo) PointAtLoom() (changed bool, err error) {
	path := filepath.Join(r.Root, r.AgentsFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(b), "\n")
	summary := Summary
	if strings.Contains(string(b), "\r\n") {
		summary += "\r" // keep the file's line endings consistent
	}
	heading, end, ok := findIssueTracker(lines)
	if !ok {
		return false, fmt.Errorf("%s has no Issue tracker section", r.AgentsFile)
	}
	// The summary is the first non-blank line after the heading.
	at := -1
	for i := heading + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			at = i
			break
		}
	}
	if at < 0 {
		lines = append(lines[:heading+1], append([]string{strings.TrimSuffix(summary, Summary), summary}, lines[heading+1:]...)...)
	} else {
		lines[at] = summary
	}
	return writeIfChanged(path, strings.Join(lines, "\n"))
}

// findIssueTracker locates the `### Issue tracker` heading inside the
// `## Agent skills` section, returning its line and the line its section
// ends at.
func findIssueTracker(lines []string) (heading, end int, ok bool) {
	inSkills, fenced := false, false
	heading = -1
	for i, l := range lines {
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			fenced = !fenced
		}
		if fenced {
			continue
		}
		switch {
		case heading >= 0 && headingLevel(l) >= 1 && headingLevel(l) <= 3:
			return heading, i, true
		case headingLevel(l) == 2:
			inSkills = strings.TrimSpace(strings.TrimLeft(l, "#")) == "Agent skills"
		case inSkills && headingLevel(l) == 3 && strings.TrimSpace(strings.TrimLeft(l, "#")) == "Issue tracker":
			heading = i
		}
	}
	if heading >= 0 {
		return heading, len(lines), true
	}
	return 0, 0, false
}

// headingLevel is the number of leading #s of an ATX heading, or 0.
func headingLevel(l string) int {
	n := len(l) - len(strings.TrimLeft(l, "#"))
	if n == 0 || n > 6 || len(l) == n || l[n] != ' ' {
		return 0
	}
	return n
}

// TriageLabels returns the names in the "Label in our tracker" column of the
// triage labels doc, in order. ok is false if the doc doesn't exist.
func (r Repo) TriageLabels() (labels []string, ok bool, err error) {
	b, err := os.ReadFile(filepath.Join(r.Root, TriageLabelsPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return parseTriageLabels(string(b)), true, nil
}

const labelColumn = "label in our tracker"

func parseTriageLabels(doc string) []string {
	col := -1
	var out []string
	for l := range strings.SplitSeq(doc, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "|") {
			if col >= 0 { // a non-table line after the header
				break // the table ended
			}
			continue
		}
		cells := splitRow(l)
		if col < 0 {
			// Only a header row naming the column starts the table.
			for i, c := range cells {
				if strings.EqualFold(c, labelColumn) {
					col = i
				}
			}
			continue
		}
		if col >= len(cells) || strings.Trim(cells[col], "-: ") == "" {
			continue // the delimiter row, or a short row
		}
		out = append(out, cells[col])
	}
	return out
}

// splitRow splits a Markdown table row into trimmed cells, unwrapping code
// spans.
func splitRow(l string) []string {
	cells := strings.Split(strings.Trim(l, "|"), "|")
	for i, c := range cells {
		cells[i] = strings.Trim(strings.TrimSpace(c), "`")
	}
	return cells
}

func writeIfChanged(path, content string) (bool, error) {
	old, err := os.ReadFile(path)
	if err == nil && string(old) == content {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(content), 0o644)
}
