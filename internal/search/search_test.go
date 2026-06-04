package search

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/projectmap"
)

func TestRebuildAndQueryUsesFTS5(t *testing.T) {
	dbPath := buildSearchTestIndex(t)

	response, err := Query(dbPath, QueryOptions{Query: "Runner", Kind: KindCodeItem})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one result", response.Results)
	}
	if response.Results[0].Kind != KindCodeItem || response.Results[0].Title != "Runner" {
		t.Fatalf("result = %#v, want Runner code_item", response.Results[0])
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

func TestRebuildIndexesWikiPagesAndSeparatesImports(t *testing.T) {
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
		CodeItems: []codeindex.Item{
			{Path: "cmd/app/main.go", Kind: "go_import", Language: "go", Name: "example.com/main/client", ImportPath: "example.com/main/client", StartLine: 3},
			{Path: "cmd/app/main.go", Kind: "go_main", Language: "go", Name: "main", StartLine: 5},
		},
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

	codeItem, err := Query(dbPath, QueryOptions{Query: "main", Kind: KindCodeItem})
	if err != nil {
		t.Fatal(err)
	}
	if len(codeItem.Results) == 0 {
		t.Fatal("code_item query for main returned nothing")
	}
	for _, result := range codeItem.Results {
		if result.CodeKind == "go_import" {
			t.Fatalf("code_item query returned import-like entry: %#v", result)
		}
	}

	imports, err := Query(dbPath, QueryOptions{Query: "main", Kind: KindImport})
	if err != nil {
		t.Fatal(err)
	}
	if len(imports.Results) == 0 {
		t.Fatal("import query for main returned nothing")
	}
	for _, result := range imports.Results {
		if result.Kind != KindImport {
			t.Fatalf("import query returned non-import: %#v", result)
		}
	}
}

func TestNormalizeKindAcceptsWikiAndImport(t *testing.T) {
	for _, kind := range []string{"wiki", "import"} {
		got, err := NormalizeKind(kind)
		if err != nil {
			t.Fatalf("NormalizeKind(%q) error: %v", kind, err)
		}
		if got != kind {
			t.Fatalf("NormalizeKind(%q) = %q", kind, got)
		}
	}
}

func buildSearchTestIndex(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 5, 31, 18, 40, 12, 0, time.UTC),
		RepoMapBody: []byte(`# Pactum Project Map

## Code surface

Contract runner index.
`),
		LLMSBody: []byte("Use code-items.jsonl for code surface.\n"),
		Files: []projectmap.FileRecord{{
			Path:     "internal/contract/runner.go",
			Kind:     "source",
			Language: "Go",
		}},
		CodeItems: []codeindex.Item{{
			Path:      "internal/contract/runner.go",
			Kind:      "go_type",
			Language:  "go",
			Name:      "Runner",
			Package:   "contract",
			Exported:  true,
			Signature: "type Runner struct",
			StartLine: 3,
			EndLine:   3,
		}},
	})
	if err != nil {
		t.Fatalf("Rebuild failed; modernc SQLite FTS5 must be available: %v", err)
	}
	return dbPath
}
