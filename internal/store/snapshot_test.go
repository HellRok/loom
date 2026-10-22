package store_test

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/HellRok/loom/internal/store"
	_ "modernc.org/sqlite"
)

// Snapshot DBs are real databases written by a released Loom, one per
// released schema, committed under testdata/snapshots. Every later binary
// must open each of them and migrate it forward. Add a snapshot when a
// release ships a new schema; never regenerate an old one. To write one:
//
//	go test ./internal/store -run TestWriteSnapshot -update-snapshot v1
var updateSnapshot = flag.String("update-snapshot", "", "write testdata/snapshots/<name>.db from the current schema")

const snapshotDir = "testdata/snapshots"

var (
	ctx  = context.Background()
	ann  = store.Actor{Name: "ann", Session: "session-ann-0000001"}
	bob  = store.Actor{Name: "bob", Session: "session-bob-0000001"}
	root = "/snapshots/demo"
)

// seed fills every table: it is the one place that says what a snapshot
// holds, and TestOpenMigratesEverySnapshot says what a migrated copy must
// still hold.
func seed(t *testing.T, st *store.Store) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := st.CreateProject(ctx, "demo", root)
	must(err)
	must(st.TouchSession(ctx, ann.Session))
	must(st.TouchSession(ctx, bob.Session))
	_, _, err = st.CreateLabel(ctx, "demo", "extra", "a custom label")
	must(err)

	_, err = st.CreateIssue(ctx, "demo", ann, store.NewIssue{Kind: "map", Title: "The map", Body: "## Goal\nShip it"})
	must(err)
	_, err = st.CreateIssue(ctx, "demo", ann, store.NewIssue{Kind: "issue", Title: "First ticket", Body: "body", Parent: 1, Labels: []string{"wayfinder:task", "extra"}})
	must(err)
	_, err = st.CreateIssue(ctx, "demo", bob, store.NewIssue{Kind: "issue", Title: "Blocked ticket", Body: "body", Parent: 1, Labels: []string{"wayfinder:task"}, BlockedBy: []int64{2}})
	must(err)
	_, err = st.CreateIssue(ctx, "demo", bob, store.NewIssue{Kind: "issue", Title: "Done ticket", Body: "body"})
	must(err)
	_, err = st.AddComment(ctx, "demo", 2, "a comment", bob)
	must(err)
	edited := "First ticket, edited"
	_, err = st.EditIssue(ctx, "demo", 2, 1, &edited, nil, ann)
	must(err)
	_, _, err = st.ClaimIssue(ctx, "demo", 2, ann)
	must(err)
	_, _, err = st.CloseIssue(ctx, "demo", 4, "finished", false, bob)
	must(err)
}

// requireEveryTableHasRows reads the table list from the database itself, so
// a table added by a later migration can't be left out of the seed.
func requireEveryTableHasRows(t *testing.T, path string) {
	t.Helper()
	rows, err := rawDB(t, path).Query(`SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if len(tables) == 0 {
		t.Fatal("no tables found")
	}
	for _, table := range tables {
		if rawCount(t, path, "SELECT COUNT(*) FROM "+table) == 0 {
			t.Errorf("%s: table %s is empty", filepath.Base(path), table)
		}
	}
}

func snapshots(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(snapshotDir, "*.db"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no snapshots under %s (err %v)", snapshotDir, err)
	}
	return paths
}

// copyOf copies a snapshot into a temp dir, so opening it never changes the
// committed file.
func copyOf(t *testing.T, src string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "loom.db")
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func rawCount(t *testing.T, path, query string) int {
	t.Helper()
	var n int
	if err := rawDB(t, path).QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func rawExec(t *testing.T, path, stmt string) {
	t.Helper()
	if _, err := rawDB(t, path).Exec(stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

// TestWriteSnapshot regenerates a snapshot on request; otherwise it only
// checks the seed still covers every table, so the generator can't rot.
func TestWriteSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	seed(t, st)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	requireEveryTableHasRows(t, path)
	if *updateSnapshot == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(snapshotDir, *updateSnapshot+".db")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}

// A snapshot only guards a table if it holds data in it.
func TestEverySnapshotHoldsDataInEveryTable(t *testing.T) {
	for _, snap := range snapshots(t) {
		requireEveryTableHasRows(t, copyOf(t, snap))
	}
}

func TestOpenMigratesEverySnapshotWithItsDataIntact(t *testing.T) {
	for _, snap := range snapshots(t) {
		t.Run(filepath.Base(snap), func(t *testing.T) {
			st, err := store.Open(ctx, copyOf(t, snap))
			if err != nil {
				t.Fatalf("opening snapshot: %v", err)
			}
			defer st.Close()

			if ps, err := st.Projects(ctx); err != nil || len(ps) != 1 || ps[0].Slug != "demo" || ps[0].Root != root {
				t.Fatalf("projects = %v, %v", ps, err)
			}
			if i, err := st.Issue(ctx, "demo", 2); err != nil ||
				i.Title != "First ticket, edited" || i.Version != 2 || i.HolderSession != ann.Session || i.State != "open" {
				t.Errorf("issue 2 = %+v, %v", i, err)
			}
			if i, err := st.Issue(ctx, "demo", 1); err != nil || i.Kind != "map" {
				t.Errorf("issue 1 = %+v, %v", i, err)
			}
			if i, err := st.Issue(ctx, "demo", 4); err != nil || i.State != "closed" {
				t.Errorf("issue 4 = %+v, %v", i, err)
			}
			if ls, err := st.IssueLabels(ctx, "demo", 2); err != nil || !slices.Equal(slices.Sorted(slices.Values(ls)), []string{"extra", "wayfinder:task"}) {
				t.Errorf("labels = %v, %v", ls, err)
			}
			if cs, err := st.Comments(ctx, "demo", 2); err != nil || len(cs) != 1 || cs[0].Body != "a comment" {
				t.Errorf("comments = %v, %v", cs, err)
			}
			if es, err := st.Events(ctx, "demo", 2); err != nil || len(es) == 0 {
				t.Errorf("events = %v, %v", es, err)
			}
			if bs, err := st.Blockers(ctx, "demo", 3); err != nil || len(bs) != 1 || bs[0].Number != 2 {
				t.Errorf("blockers = %v, %v", bs, err)
			}
			if kids, err := st.Children(ctx, "demo", 1); err != nil || len(kids) != 2 {
				t.Errorf("children = %v, %v", kids, err)
			}
			if _, err := st.SessionByID(ctx, bob.Session); err != nil {
				t.Errorf("session: %v", err)
			}
			// It is a working database, not just a readable one.
			if _, err := st.CreateIssue(ctx, "demo", ann, store.NewIssue{Kind: "issue", Title: "after migration", Body: "b"}); err != nil {
				t.Errorf("writing to migrated snapshot: %v", err)
			}
		})
	}
}

func TestOpenRefusesASnapshotMadeNewerThanTheBinary(t *testing.T) {
	for _, snap := range snapshots(t) {
		t.Run(filepath.Base(snap), func(t *testing.T) {
			path := copyOf(t, snap)
			rawExec(t, path, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (9999, 1)")
			_, err := store.Open(ctx, path)
			var tooNew *store.SchemaTooNewError
			if !errors.As(err, &tooNew) || tooNew.DB != 9999 {
				t.Fatalf("want SchemaTooNewError for 9999, got %v", err)
			}
		})
	}
}

// Re-running the newest migration of this binary fails against a database
// that already has what it adds; the DB must stay at its last good version.
func TestFailedMigrationOfASnapshotLeavesItAtTheLastGoodVersion(t *testing.T) {
	for _, snap := range snapshots(t) {
		t.Run(filepath.Base(snap), func(t *testing.T) {
			path := copyOf(t, snap)
			const maxVersion = "SELECT MAX(version_id) FROM goose_db_version"
			latest := rawCount(t, path, maxVersion)
			rawExec(t, path, "DELETE FROM goose_db_version WHERE version_id = "+strconv.Itoa(latest))
			if _, err := store.Open(ctx, path); err == nil {
				t.Fatal("want the re-run of the last migration to fail")
			}
			if got := rawCount(t, path, maxVersion); got != latest-1 {
				t.Errorf("version after failed migration = %d, want %d", got, latest-1)
			}
		})
	}
}
