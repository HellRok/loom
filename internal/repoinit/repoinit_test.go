package repoinit_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/HellRok/loom/internal/repoinit"
)

const skillsBlock = `# Widget

## Agent skills

### Issue tracker

Issues are tracked in GitHub. See ` + "`docs/agents/issue-tracker.md`" + `.

### Triage labels

Defaults. See ` + "`docs/agents/triage-labels.md`" + `.

## Other

Keep this.
`

func write(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInspectNeedsTheAgentSkillsBlockAndTheTrackerDoc(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"nothing":               {},
		"no tracker doc":        {"CLAUDE.md": skillsBlock},
		"no block":              {"CLAUDE.md": "# Widget\n", "docs/agents/issue-tracker.md": "x"},
		"tracker outside block": {"CLAUDE.md": "## Other\n\n### Issue tracker\n", "docs/agents/issue-tracker.md": "x"},
		// Setup writes to the first of CLAUDE.md/AGENTS.md that exists.
		"block only in the second file": {"CLAUDE.md": "# Widget\n", "AGENTS.md": skillsBlock, "docs/agents/issue-tracker.md": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			for f, c := range files {
				write(t, root, f, c)
			}
			_, err := repoinit.Inspect(root)
			if _, ok := errors.AsType[*repoinit.NotSetUpError](err); !ok {
				t.Fatalf("want NotSetUpError, got %v", err)
			}
		})
	}

	root := t.TempDir()
	write(t, root, "AGENTS.md", skillsBlock)
	write(t, root, "docs/agents/issue-tracker.md", "x")
	r, err := repoinit.Inspect(root)
	if err != nil || r.AgentsFile != "AGENTS.md" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestPointAtLoomRewritesOnlyTheSummaryLine(t *testing.T) {
	root := t.TempDir()
	write(t, root, "CLAUDE.md", skillsBlock)
	write(t, root, "docs/agents/issue-tracker.md", "x")
	r, err := repoinit.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}

	changed, err := r.PointAtLoom()
	if err != nil || !changed {
		t.Fatalf("first run: %v %v", changed, err)
	}
	want := strings.Replace(skillsBlock, "Issues are tracked in GitHub. See `docs/agents/issue-tracker.md`.", repoinit.Summary, 1)
	if got := read(t, root, "CLAUDE.md"); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if changed, err := r.PointAtLoom(); err != nil || changed {
		t.Fatalf("second run: %v %v", changed, err)
	}
}

func TestPointAtLoomFillsAnEmptySection(t *testing.T) {
	root := t.TempDir()
	write(t, root, "CLAUDE.md", "## Agent skills\n\n### Issue tracker\n")
	write(t, root, "docs/agents/issue-tracker.md", "x")
	r, err := repoinit.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.PointAtLoom(); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, root, "CLAUDE.md"), "## Agent skills\n\n### Issue tracker\n\n"+repoinit.Summary+"\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWriteTrackerDocOverwritesAndReportsChange(t *testing.T) {
	root := t.TempDir()
	write(t, root, "CLAUDE.md", skillsBlock)
	write(t, root, "docs/agents/issue-tracker.md", "old")
	r, err := repoinit.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := r.WriteTrackerDoc(); err != nil || !changed {
		t.Fatalf("first: %v %v", changed, err)
	}
	if read(t, root, repoinit.TrackerDocPath) != repoinit.TrackerDoc {
		t.Fatal("doc not overwritten")
	}
	if changed, err := r.WriteTrackerDoc(); err != nil || changed {
		t.Fatalf("second: %v %v", changed, err)
	}
}

func TestTrackerDocIsUnchangedByHowItIsAssembled(t *testing.T) {
	golden, err := os.ReadFile("testdata/issue-tracker-loom.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	if repoinit.TrackerDoc != string(golden) {
		t.Error("tracker doc differs from testdata/issue-tracker-loom.golden.md")
	}
}

func TestConventionsOmitTheDocOnlyParts(t *testing.T) {
	for _, want := range []string{
		"## Conventions",
		"do not retry blindly",
		"## Wayfinding operations",
	} {
		if !strings.Contains(repoinit.Conventions, want) {
			t.Errorf("conventions lack %q", want)
		}
	}
	for _, doc := range []string{
		"Written by `loom init`",
		"Register the server once per machine",
		"# Issue tracker: Loom",
	} {
		if strings.Contains(repoinit.Conventions, doc) {
			t.Errorf("conventions carry the doc-only %q", doc)
		}
	}
}

func TestTrackerDocCarriesTheRequiredGuidance(t *testing.T) {
	for _, want := range []string{
		"Written by `loom init`",
		"`loom mcp`",
		"`create_label`",
		"docs/agents/triage-labels.md",
		"do not retry blindly",
		"## Wayfinding operations",
	} {
		if !strings.Contains(repoinit.TrackerDoc, want) {
			t.Errorf("tracker doc lacks %q", want)
		}
	}
	if strings.Contains(repoinit.TrackerDoc, "claude mcp add") {
		t.Error("tracker doc still names claude mcp add")
	}
}

func TestTriageLabelsReadsTheLabelInOurTrackerColumn(t *testing.T) {
	root := t.TempDir()
	write(t, root, "CLAUDE.md", skillsBlock)
	write(t, root, "docs/agents/issue-tracker.md", "x")
	r, err := repoinit.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := r.TriageLabels(); ok || err != nil {
		t.Fatalf("absent file: %v %v", ok, err)
	}
	write(t, root, repoinit.TriageLabelsPath, `# Triage Labels

Some prose | with a pipe.

| Label in mattpocock/skills | Label in our tracker | Meaning                  |
| -------------------------- | -------------------- | ------------------------ |
| `+"`needs-triage`"+`              | `+"`triage/new`"+`           | Maintainer needs to look |
| `+"`ready-for-agent`"+`           | ready-for-agent      | Ready                    |

| Other | Table |
| ----- | ----- |
| a     | b     |
`)
	got, ok, err := r.TriageLabels()
	if err != nil || !ok || !slices.Equal(got, []string{"triage/new", "ready-for-agent"}) {
		t.Fatalf("got %v %v %v", got, ok, err)
	}
}
