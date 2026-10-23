package tools_test

import "testing"

func TestStandaloneIsOffByDefaultAndShownInProjectInfo(t *testing.T) {
	h := newHarness(t)
	dir := mkdir(t, t.TempDir(), "app")
	created := h.mustOK(h.call(dir, "create_project", map[string]any{"slug": "app"}))
	if created["standalone"] != nil {
		t.Fatalf("a new Project is standalone: %v", created)
	}

	out := h.mustOK(h.call(dir, "set_standalone", map[string]any{"standalone": true}))
	if out["standalone"] != true || out["already"] != false {
		t.Fatalf("want standalone, not already: %v", out)
	}
	listed := h.mustOK(h.call(dir, "list_projects", nil))["projects"].([]any)
	if got := listed[0].(map[string]any)["standalone"]; got != true {
		t.Fatalf("list_projects: %v", listed)
	}

	again := h.mustOK(h.call(dir, "set_standalone", map[string]any{"standalone": true}))
	if again["already"] != true {
		t.Fatalf("setting it twice should be already: %v", again)
	}

	cleared := h.mustOK(h.call(dir, "set_standalone", map[string]any{"project": "app", "standalone": false}))
	if cleared["standalone"] != nil || cleared["already"] != false {
		t.Fatalf("want cleared: %v", cleared)
	}
}
