package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HellRok/loom/internal/repoinit"
)

const setupClaude = `# Widget

## Agent skills

### Issue tracker

Issues are tracked in GitHub. See ` + "`docs/agents/issue-tracker.md`" + `.

### Triage labels

See ` + "`docs/agents/triage-labels.md`" + `.
`

const triageLabels = `# Triage Labels

| Label in mattpocock/skills | Label in our tracker | Meaning |
| -------------------------- | -------------------- | ------- |
| ` + "`needs-triage`" + `             | ` + "`needs-triage`" + `        | x       |
| ` + "`needs-info`" + `               | ` + "`triage/waiting`" + `      | x       |
`

// newRepo makes a git repo named name in a temp dir, with files written
// under it.
func newRepo(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	for f, c := range files {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func (in *install) initIn(dir string, args ...string) (string, error) {
	cmd := in.command(append([]string{"init"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func slurp(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInitInstallsLoomOverSetupOutput(t *testing.T) {
	in := newInstall(t)
	repo := newRepo(t, "widget", map[string]string{
		"CLAUDE.md":                    setupClaude,
		"docs/agents/issue-tracker.md": "old doc",
		"docs/agents/triage-labels.md": triageLabels,
	})

	out, err := in.initIn(repo)
	if err != nil {
		t.Fatalf("loom init: %v\n%s", err, out)
	}
	for _, want := range []string{
		"project widget: created",
		"label triage/waiting: created",
		"docs/agents/issue-tracker.md: updated",
		"CLAUDE.md: updated",
		`"mcpServers"`,
		"loom mcp",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "label needs-triage") {
		t.Errorf("created a label the Project already had:\n%s", out)
	}
	if got := slurp(t, filepath.Join(repo, repoinit.TrackerDocPath)); got != repoinit.TrackerDoc {
		t.Errorf("tracker doc not installed:\n%s", got)
	}
	if got, want := slurp(t, filepath.Join(repo, "CLAUDE.md")),
		strings.Replace(setupClaude, "Issues are tracked in GitHub. See `docs/agents/issue-tracker.md`.", repoinit.Summary, 1); got != want {
		t.Errorf("CLAUDE.md:\n%s", got)
	}

	// The Project and label exist in the daemon.
	cs := in.shim()
	list := call(t, cs, "list_projects", nil)
	if got := projectSlugs(list); len(got) != 1 || got[0] != "widget" {
		t.Fatalf("projects: %v", got)
	}
	probe := in.command("mcp")
	probe.Dir = repo
	labels := call(t, shimIn(t, probe), "list_labels", nil)["labels"].([]any)
	found := false
	for _, l := range labels {
		found = found || l.(map[string]any)["name"] == "triage/waiting"
	}
	if !found {
		t.Fatalf("custom label missing: %v", labels)
	}

	// Re-running is safe and changes nothing; no agent has connected from
	// the repo yet... but one has now, so the guidance shrinks to a reminder.
	out, err = in.initIn(repo)
	if err != nil {
		t.Fatalf("second loom init: %v\n%s", err, out)
	}
	for _, want := range []string{
		"project widget: already exists",
		"labels: none to create",
		"docs/agents/issue-tracker.md: unchanged",
		"CLAUDE.md: unchanged",
		"Reminder:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("second output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "mcpServers") {
		t.Errorf("full guidance repeated:\n%s", out)
	}
}

func TestInitWithoutSetupChangesNothing(t *testing.T) {
	cases := map[string]map[string]string{
		"absent":  {},
		"partial": {"AGENTS.md": setupClaude},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			in := newInstall(t)
			repo := newRepo(t, "widget", files)
			out, err := in.initIn(repo)
			if err == nil {
				t.Fatalf("loom init succeeded:\n%s", out)
			}
			for _, want := range []string{"mattpocock/skills", "/setup-matt-pocock-skills", "loom init --standalone"} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if _, err := os.Stat(filepath.Join(repo, "docs")); err == nil {
				t.Error("created docs/")
			}
			if len(files) == 0 {
				if _, err := os.Stat(filepath.Join(repo, "CLAUDE.md")); err == nil {
					t.Error("created CLAUDE.md")
				}
			}
			// Nothing reached the daemon, so no Project exists.
			if got := projectSlugs(call(t, in.shim(), "list_projects", nil)); len(got) != 0 {
				t.Errorf("projects: %v", got)
			}
		})
	}
}

func TestInitNeedsASlugOutsideGitOrOnCollision(t *testing.T) {
	in := newInstall(t)
	files := map[string]string{"CLAUDE.md": setupClaude, "docs/agents/issue-tracker.md": "old"}

	// Outside git: no Slug to derive.
	plain, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for f, c := range files {
		p := filepath.Join(plain, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(c), 0o644)
	}
	out, err := in.initIn(plain)
	if err == nil || !strings.Contains(out, "--slug") {
		t.Fatalf("want a --slug hint, got %v:\n%s", err, out)
	}
	if got := slurp(t, filepath.Join(plain, "CLAUDE.md")); got != setupClaude {
		t.Fatal("CLAUDE.md changed despite the failure")
	}
	if out, err := in.initIn(plain, "--slug", "plain"); err != nil || !strings.Contains(out, "project plain: created") {
		t.Fatalf("with --slug: %v\n%s", err, out)
	}

	// Collision: another repo with the same folder name.
	a := newRepo(t, "dup", files)
	b := newRepo(t, "dup", files)
	if out, err := in.initIn(a); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, err := in.initIn(b); err == nil || !strings.Contains(out, "--slug") {
		t.Fatalf("want a --slug hint on collision, got %v:\n%s", err, out)
	}
	if out, err := in.initIn(b, "--slug", "dup-two"); err != nil || !strings.Contains(out, "project dup-two: created") {
		t.Fatalf("collision with --slug: %v\n%s", err, out)
	}
}

func TestInitSkipsLabelsWithoutTheTriageDoc(t *testing.T) {
	in := newInstall(t)
	repo := newRepo(t, "widget", map[string]string{
		"AGENTS.md":                    setupClaude,
		"docs/agents/issue-tracker.md": "old",
	})
	out, err := in.initIn(repo)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "label") {
		t.Errorf("mentioned labels:\n%s", out)
	}
	if !strings.Contains(out, "AGENTS.md: updated") {
		t.Errorf("AGENTS.md not updated:\n%s", out)
	}
}

// tree maps every file under dir (outside .git) to its contents.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			files[p] = slurp(t, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestStandaloneInitWritesNothingUnderTheRoot(t *testing.T) {
	cases := map[string]map[string]string{
		"no setup":              {"README.md": "hello"},
		"points at another one": {"CLAUDE.md": setupClaude, "docs/agents/issue-tracker.md": "GitHub", "docs/agents/triage-labels.md": triageLabels},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			in := newInstall(t)
			repo := newRepo(t, "widget", files)
			before := tree(t, repo)

			out, err := in.initIn(repo, "--standalone")
			if err != nil {
				t.Fatalf("loom init --standalone: %v\n%s", err, out)
			}
			for _, want := range []string{
				"project widget: created",
				"standalone: yes",
				`"mcpServers"`,
				"overrides checked-in skill config",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "label") {
				t.Errorf("mentioned labels:\n%s", out)
			}
			if after := tree(t, repo); !reflect.DeepEqual(before, after) {
				t.Errorf("files under the Root changed:\nbefore %v\nafter  %v", before, after)
			}
			list := call(t, in.shim(), "list_projects", nil)["projects"].([]any)
			if len(list) != 1 || list[0].(map[string]any)["standalone"] != true {
				t.Errorf("Project not marked standalone: %v", list)
			}
		})
	}
}

func TestUsageDescribesStandaloneAndNoLongerRequiresSetup(t *testing.T) {
	in := newInstall(t)
	for name, args := range map[string][]string{"loom": nil, "loom init -h": {"init", "-h"}} {
		out, _ := in.command(args...).CombinedOutput()
		text := string(out)
		if strings.Contains(text, "after /setup-matt-pocock-skills") {
			t.Errorf("%s still says init requires setup:\n%s", name, text)
		}
		if !strings.Contains(text, "standalone") {
			t.Errorf("%s doesn't describe --standalone:\n%s", name, text)
		}
	}
}

func TestStandaloneInitIsIdempotentAndPlainInitClearsIt(t *testing.T) {
	in := newInstall(t)
	files := map[string]string{
		"CLAUDE.md":                    setupClaude,
		"docs/agents/issue-tracker.md": "old doc",
		"docs/agents/triage-labels.md": triageLabels,
	}
	repo := newRepo(t, "widget", files)
	standalone := func() any {
		return call(t, in.shim(), "list_projects", nil)["projects"].([]any)[0].(map[string]any)["standalone"]
	}

	if out, err := in.initIn(repo, "--standalone"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	before := tree(t, repo)
	if standalone() != true { // also connects an agent Session
		t.Fatal("first --standalone didn't mark the Project")
	}
	out, err := in.initIn(repo, "--standalone")
	if err != nil {
		t.Fatalf("second --standalone: %v\n%s", err, out)
	}
	for _, want := range []string{"project widget: already exists", "standalone: yes", "Reminder:"} {
		if !strings.Contains(out, want) {
			t.Errorf("second output lacks %q:\n%s", want, out)
		}
	}
	if !reflect.DeepEqual(before, tree(t, repo)) || standalone() != true {
		t.Errorf("re-running changed something: standalone=%v", standalone())
	}

	out, err = in.initIn(repo)
	if err != nil {
		t.Fatalf("plain init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "standalone: cleared") || standalone() != nil {
		t.Errorf("plain init didn't clear Standalone (%v):\n%s", standalone(), out)
	}
	if out, err = in.initIn(repo); err != nil || strings.Contains(out, "standalone") {
		t.Errorf("plain init on a plain Project mentioned standalone (%v):\n%s", err, out)
	}

	// Switching back keeps every file the plain init wrote.
	before = tree(t, repo)
	out, err = in.initIn(repo, "--standalone")
	if err != nil || !strings.Contains(out, "standalone: yes") || standalone() != true {
		t.Fatalf("--standalone on a plain Project: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(before, tree(t, repo)) {
		t.Error("switching to Standalone changed files in the repo")
	}
}

func TestStandaloneInitHonoursSlugHints(t *testing.T) {
	in := newInstall(t)
	plain, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out, err := in.initIn(plain, "--standalone")
	if err == nil || !strings.Contains(out, "--slug") {
		t.Fatalf("want a --slug hint outside git, got %v:\n%s", err, out)
	}
	if out, err := in.initIn(plain, "--standalone", "--slug", "plain"); err != nil || !strings.Contains(out, "project plain: created") {
		t.Fatalf("with --slug: %v\n%s", err, out)
	}

	a := newRepo(t, "dup", nil)
	b := newRepo(t, "dup", nil)
	if out, err := in.initIn(a, "--standalone"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, err := in.initIn(b, "--standalone"); err == nil || !strings.Contains(out, "--slug") {
		t.Fatalf("want a --slug hint on collision, got %v:\n%s", err, out)
	}
	if out, err := in.initIn(b, "--standalone", "--slug", "dup-two"); err != nil || !strings.Contains(out, "project dup-two: created") {
		t.Fatalf("collision with --slug: %v\n%s", err, out)
	}
	if got := tree(t, plain); len(got) != 0 {
		t.Errorf("wrote under the Root: %v", got)
	}
}
