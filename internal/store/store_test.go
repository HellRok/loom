package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/HellRok/loom/internal/store"
)

func migrations(files map[string]string) fstest.MapFS {
	fs := fstest.MapFS{}
	for name, sql := range files {
		fs[name] = &fstest.MapFile{Data: []byte("-- +goose Up\n" + sql)}
	}
	return fs
}

func TestOpenRefusesSchemaNewerThanBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	newer := migrations(map[string]string{
		"00001_a.sql":    "CREATE TABLE a (x INTEGER);",
		"09999_next.sql": "CREATE TABLE b (x INTEGER);",
	})
	st, err := store.OpenWith(context.Background(), path, newer)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	_, err = store.Open(context.Background(), path)
	var tooNew *store.SchemaTooNewError
	if !errors.As(err, &tooNew) || tooNew.DB != 9999 || tooNew.Binary >= 9999 {
		t.Fatalf("want SchemaTooNewError for DB 9999, got %v", err)
	}
}

func TestFailedMigrationLeavesLastGoodVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	good := map[string]string{"00001_a.sql": "CREATE TABLE a (x INTEGER);"}
	bad := map[string]string{
		"00001_a.sql": good["00001_a.sql"],
		"00002_b.sql": "CREATE TABLE b (x INTEGER);\nTHIS IS NOT SQL;",
	}
	if _, err := store.OpenWith(context.Background(), path, migrations(bad)); err == nil {
		t.Fatal("want a migration error")
	}
	// The DB is at version 1: it opens with only the good migration, and
	// version 2 then applies cleanly because its partial work was rolled back.
	st, err := store.OpenWith(context.Background(), path, migrations(good))
	if err != nil {
		t.Fatalf("reopen at last good version: %v", err)
	}
	st.Close()
	fixed := map[string]string{
		"00001_a.sql": good["00001_a.sql"],
		"00002_b.sql": "CREATE TABLE b (x INTEGER);",
	}
	st, err = store.OpenWith(context.Background(), path, migrations(fixed))
	if err != nil {
		t.Fatalf("apply fixed migration: %v", err)
	}
	st.Close()
}
