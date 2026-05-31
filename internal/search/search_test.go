package search

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/projectmap"
)

func TestRebuildAndQueryUsesFTS5(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 5, 31, 18, 40, 12, 0, time.UTC),
		RepoMapBody: []byte(`# Pactum Project Map

## Code surface
`),
		LLMSBody: []byte("Use code-items.jsonl for code surface.\n"),
		Files: []projectmap.FileRecord{{
			Path:     "internal/contracts/runner.go",
			Kind:     "source",
			Language: "Go",
		}},
		CodeItems: []codeindex.Item{{
			Path:      "internal/contracts/runner.go",
			Kind:      "go_type",
			Language:  "go",
			Name:      "Runner",
			Package:   "contracts",
			Exported:  true,
			Signature: "type Runner struct",
			StartLine: 3,
			EndLine:   3,
		}},
	})
	if err != nil {
		t.Fatalf("Rebuild failed; modernc SQLite FTS5 must be available: %v", err)
	}

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
