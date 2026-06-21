package search

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/projectmap"
)

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

func TestNormalizeKindAcceptsAllSupportedKinds(t *testing.T) {
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

func TestNormalizeKindRejectsRemovedKinds(t *testing.T) {
	for _, kind := range []string{"code_item", "import"} {
		if _, err := NormalizeKind(kind); err == nil {
			t.Fatalf("NormalizeKind(%q) should fail for removed kind", kind)
		}
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
		Files: []projectmap.FileRecord{{Path: "cmd/app/main.go", Kind: "source", Language: "Go"}},
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

	file, err := Query(dbPath, QueryOptions{Query: "main", Kind: KindFile})
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Results) == 0 {
		t.Fatal("file query for main returned nothing")
	}
	for _, result := range file.Results {
		if result.Kind != KindFile {
			t.Fatalf("file query returned non-file: %#v", result)
		}
	}
}

func buildSearchTestIndex(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 5, 31, 18, 40, 12, 0, time.UTC),
		RepoMapBody: []byte(`# Pactum Project Map

## Summary

Contract runner index.
`),
		LLMSBody: []byte("Deterministic project map.\n"),
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
