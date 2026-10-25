package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// shippedMigrations pins every migration a release has shipped (v1.1.0 and
// v1.2.0 carry 00001-00007). Editing one changes what an upgraded database
// looks like compared with a fresh one, so a schema change is a new
// migration, never an edit. When a release ships a new migration, add its
// hash here.
var shippedMigrations = map[string]string{
	"00001_projects_sessions.sql": "3816ee7748e52c4132e87a0593aa601fe8d333187527f554b6f6920243f6e9ca",
	"00002_issues.sql":            "2b42de53a0d60b8d3fccd6c0bee096d7865e0daf0c449074ae47fbae67f55118",
	"00003_labels.sql":            "fbdb165d0030289d75b93f3ab95c11de6c1d2488c62eb7a67a80e16c0b79edd2",
	"00004_hierarchy.sql":         "1086f8a64adcc2aaada9c712158111b91b99a6566abd37a04e5a257d26e72d03",
	"00005_claims.sql":            "ee20172b6682f6ca6cbe9900af2830fcde0d0929f02f6a2b17905f3b30eb4229",
	"00006_standalone.sql":        "9d7998276e3805c4a2eb8c9c21b312c62c956f7a410bb04cc0d9a30eb7e61fd3",
	"00007_glossary.sql":          "cead9199271f3b14cf52f971247479e13ae1d0743ba6d9cfd72dff00e8a2d667",
}

func TestShippedMigrationsAreNotEdited(t *testing.T) {
	for name, want := range shippedMigrations {
		b, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Errorf("%s: shipped migration is gone: %v", name, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s changed since it shipped (sha256 %s, pinned %s); add a new migration instead", name, got, want)
		}
	}
}
