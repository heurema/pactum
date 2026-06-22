package search

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/projectmap"

	_ "modernc.org/sqlite"
)

func TestRebuildAndQueryUsesFTS5(t *testing.T) {
	dbPath := buildSearchTestIndex(t)

	response, err := Query(dbPath, QueryOptions{Query: "runner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) == 0 {
		t.Fatalf("results = %#v, want at least one result", response.Results)
	}
}

func TestQueryTreatsOperatorLikeInputAsLiteralTokens(t *testing.T) {
	dbPath := buildSearchTestIndex(t)

	for _, query := range []string{
		"OR",
		"NEAR",
		"contract OR runner",
		"foo - bar",
	} {
		t.Run(query, func(t *testing.T) {
			if _, err := Query(dbPath, QueryOptions{Query: query}); err != nil {
				t.Fatalf("Query(%q) returned error: %v", query, err)
			}
		})
	}
}

func TestQueryNormalMultiTokenSearchStillWorks(t *testing.T) {
	dbPath := buildSearchTestIndex(t)

	response, err := Query(dbPath, QueryOptions{Query: "contract runner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) == 0 {
		t.Fatal("expected multi-token search results")
	}
}

func TestFTSQueryQuotesTokens(t *testing.T) {
	if got, want := ftsQuery("contract runner"), `"contract" "runner"`; got != want {
		t.Fatalf("ftsQuery() = %q, want %q", got, want)
	}
}

func TestRebuildIndexesWikiPages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC),
		RepoMapBody: []byte("# Map\n"),
		LLMSBody:    []byte("router\n"),
		WikiPages: []projectmap.WikiPage{{
			RelPath: "config.md",
			Title:   "Configuration",
			Content: []byte("# Configuration\n\n- `vite.config.ts` — Vite build configuration\n"),
		}},
	})
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	wiki, err := Query(dbPath, QueryOptions{Query: "vite", Kind: KindWiki})
	if err != nil {
		t.Fatal(err)
	}
	if len(wiki.Results) != 1 || wiki.Results[0].Kind != KindWiki || wiki.Results[0].Path != "map/wiki/config.md" {
		t.Fatalf("wiki query = %#v, want one map/wiki/config.md hit", wiki.Results)
	}
}

func TestNormalizeKindRejectsUnknownKinds(t *testing.T) {
	for _, kind := range []string{"unknown", "invalid", "symbol"} {
		_, err := NormalizeKind(kind)
		if err == nil {
			t.Fatalf("NormalizeKind(%q) should have failed", kind)
		}
	}
}

func TestNormalizeKindAcceptsRemainingKinds(t *testing.T) {
	for _, kind := range []string{"any", "repo_map", "llms", "wiki", "file"} {
		got, err := NormalizeKind(kind)
		if err != nil {
			t.Fatalf("NormalizeKind(%q) error: %v", kind, err)
		}
		if got != kind {
			t.Fatalf("NormalizeKind(%q) = %q", kind, got)
		}
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

func buildSearchTestIndex(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 5, 31, 18, 40, 12, 0, time.UTC),
		RepoMapBody: []byte(`# Pactum Project Map

## File tree

Contract runner index.
`),
		LLMSBody: []byte("Use files.jsonl for the file inventory.\n"),
		Files: []projectmap.FileRecord{{
			Path:     "internal/contract/runner.go",
			Kind:     "source",
			Language: "Go",
		}},
	})
	if err != nil {
		t.Fatalf("Rebuild failed; modernc SQLite FTS5 must be available: %v", err)
	}
	return dbPath
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
