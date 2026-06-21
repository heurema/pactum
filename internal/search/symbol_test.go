package search

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSearchResultKinds(t *testing.T) {
	dbPath := buildSearchTestIndex(t)

	response, err := Query(dbPath, QueryOptions{Query: "contract"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) == 0 {
		t.Fatal("expected results for 'contract'")
	}
	sawFile := false
	for _, r := range response.Results {
		if r.Kind == "code_item" || r.Kind == "import" {
			t.Fatalf("result has removed kind %q: %#v", r.Kind, r)
		}
		if r.Kind == KindFile {
			sawFile = true
		}
	}
	if !sawFile {
		t.Fatal("expected at least one file result")
	}
}

func TestStaleSchemaDetected(t *testing.T) {
	t.Run("legacy index without schema marker", func(t *testing.T) {
		dbPath := buildSearchTestIndex(t)
		execOnIndex(t, dbPath, `DROP TABLE meta`)

		_, err := Query(dbPath, QueryOptions{Query: "runner"})
		if !IsStaleIndex(err) {
			t.Fatalf("Query error = %v, want stale index", err)
		}
	})

	t.Run("incompatible schema version", func(t *testing.T) {
		dbPath := buildSearchTestIndex(t)
		execOnIndex(t, dbPath, `UPDATE meta SET value = 'pactum.search.index.v1' WHERE key = 'schema_version'`)

		_, err := Query(dbPath, QueryOptions{Query: "runner"})
		if !IsStaleIndex(err) {
			t.Fatalf("Query error = %v, want stale index", err)
		}
	})
}

func TestEnsureSchemaCurrentSeparatesStaleFromRealErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	// No meta table at all: an index from an older binary — stale.
	if err := ensureSchemaCurrent(db, dbPath); !IsStaleIndex(err) {
		t.Fatalf("missing meta table must read as stale, got %v", err)
	}
	// Meta exists but holds no version row — stale.
	if _, err := db.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := ensureSchemaCurrent(db, dbPath); !IsStaleIndex(err) {
		t.Fatalf("missing version row must read as stale, got %v", err)
	}
	// A closed handle is a real failure, not staleness.
	db.Close()
	err = ensureSchemaCurrent(db, dbPath)
	if err == nil || IsStaleIndex(err) {
		t.Fatalf("a real database failure must not read as stale, got %v", err)
	}
}

func execOnIndex(t *testing.T, dbPath, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

// buildStaleSchemaIndex builds a minimal index for stale-schema tests.
func buildStaleSchemaIndex(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 6, 12, 7, 4, 27, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}
	return dbPath
}
