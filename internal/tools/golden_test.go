package tools_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestToolsListGolden pins the tool surface: names, descriptions and
// schemas. Run with -update after an intended change and review the diff.
func TestToolsListGolden(t *testing.T) {
	h := newHarness(t)
	res, err := h.client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(res.Tools, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	path := filepath.Join("testdata", "tools_list.golden.json")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("tools/list differs from %s (run with -update to accept):\n%s", path, got)
	}
}
