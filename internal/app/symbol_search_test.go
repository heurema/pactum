package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/codeindex"
	searchpkg "github.com/heurema/pactum/internal/search"

	_ "modernc.org/sqlite"
)

func TestSearchSymbolFlagResolvesIdentifierWithoutQuery(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}

func BuildRunner() {}
`)

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	// Lower-cased --symbol with no positional query proves the standalone,
	// case-insensitive lookup.
	if code := testApp(root).Run([]string{"search", "--symbol", "runner"}, &stdout, &stderr); code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "code_item internal/contracts/runner.go:") {
		t.Fatalf("symbol lookup did not render a ranged code_item address:\n%s", got)
	}
	if strings.Contains(got, "BuildRunner") {
		t.Fatalf("symbol lookup matched a substring/other symbol:\n%s", got)
	}
}

func TestSearchSymbolFlagRejectsIncompatibleKind(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code := testApp(root).Run([]string{"search", "Runner", "--symbol", "Runner", "--kind", "file"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit for --symbol with --kind file; stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "--symbol only applies to code_item results") {
		t.Fatalf("usage error did not explain the constraint:\n%s", stderr.String())
	}

	// --kind code_item is the one explicit kind that stays compatible.
	stdout.Reset()
	stderr.Reset()
	if code := testApp(root).Run([]string{"search", "--symbol", "Runner", "--kind", "code_item"}, &stdout, &stderr); code != 0 {
		t.Fatalf("search --symbol --kind code_item exited %d, stderr: %s", code, stderr.String())
	}
}

func TestSearchHumanOutputShowsRangedAddressAndSignature(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := testApp(root).Run([]string{"search", "Runner", "--kind", "code_item"}, &stdout, &stderr); code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "code_item internal/contracts/runner.go:3-3") {
		t.Fatalf("human output missing path:start-end address:\n%s", got)
	}
	// The go_type signature is the type_spec text ("Runner struct"); the `type`
	// keyword lives on the parent node and is not part of the signature.
	if !strings.Contains(got, "signature: Runner struct") {
		t.Fatalf("human output missing signature line:\n%s", got)
	}
}

func TestSearchJSONSymbolMetadataOnlyForCodeItems(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := testApp(root).Run([]string{"search", "Runner", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}

	// Decode into a permissive shape so we can assert exactly which fields are
	// present per result kind.
	var decoded struct {
		Results []map[string]json.RawMessage `json:"results"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
	if len(decoded.Results) == 0 {
		t.Fatalf("no results:\n%s", stdout.String())
	}

	sawCodeItem, sawOther := false, false
	for _, result := range decoded.Results {
		kind := mustJSONString(t, result["kind"])
		_, hasStart := result["start_line"]
		_, hasEnd := result["end_line"]
		_, hasSig := result["signature"]
		if kind == "code_item" {
			sawCodeItem = true
			if !hasStart || !hasEnd || !hasSig {
				t.Fatalf("code_item JSON missing symbol metadata: %v", result)
			}
			if got := mustJSONString(t, result["start_line"]); got != "3" {
				t.Fatalf("code_item start_line = %s, want 3", got)
			}
		} else {
			sawOther = true
			if hasStart || hasEnd || hasSig {
				t.Fatalf("%s JSON gained symbol metadata fields: %v", kind, result)
			}
		}
	}
	if !sawCodeItem || !sawOther {
		t.Fatalf("expected both a code_item and a non-code_item result; code_item=%v other=%v", sawCodeItem, sawOther)
	}
}

func TestRenderExecutorContextRangedAddresses(t *testing.T) {
	results := runSearchResults{
		QuerySource: "task",
		Queries:     []string{"Runner"},
		Results: []runSearchResultItem{
			{Result: searchpkg.Result{Rank: 1, ID: "code_item:internal/run/runner.go:go_type:Runner:3", Kind: "code_item", Path: "internal/run/runner.go", Title: "Runner", StartLine: 3, EndLine: 3, Signature: "type Runner struct"}},
			{Result: searchpkg.Result{Rank: 2, ID: "file:internal/run/runner.go", Kind: "file", Path: "internal/run/runner.go", Title: "runner.go"}},
			{Result: searchpkg.Result{Rank: 3, ID: "code_item:internal/run/x.go:go_func:Partial:0", Kind: "code_item", Path: "internal/run/x.go", Title: "Partial", Signature: "func Partial()"}},
		},
	}

	rendered := string(renderExecutorContext(contractRunState{}, "map_run", "hash", results, nil, promptManifestMemorySelected{}))

	if !strings.Contains(rendered, "1. code_item internal/run/runner.go:3-3 (Runner) — type Runner struct") {
		t.Fatalf("executor context missing ranged code_item address:\n%s", rendered)
	}
	if !strings.Contains(rendered, "2. file internal/run/runner.go\n") {
		t.Fatalf("non-code_item result line changed shape:\n%s", rendered)
	}
	// The Partial hit has no valid range: it still renders, with its signature,
	// but never as path:0-0.
	if !strings.Contains(rendered, "3. code_item internal/run/x.go (Partial) — func Partial()") {
		t.Fatalf("invalid-range code_item rendered incorrectly:\n%s", rendered)
	}
	if strings.Contains(rendered, ":0-0") {
		t.Fatalf("executor context rendered a path:0-0 address:\n%s", rendered)
	}
	if !strings.Contains(rendered, "read that line range directly") {
		t.Fatalf("retrieval guidance does not mention reading symbol ranges:\n%s", rendered)
	}
}

func TestRunContextSearchPreservesDistinctSymbols(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := searchpkg.Rebuild(dbPath, searchpkg.IndexInput{
		GeneratedAt: time.Date(2026, 6, 12, 7, 4, 27, 0, time.UTC),
		CodeItems: []codeindex.Item{
			{Path: "internal/cache/cache.go", Kind: "go_method", Language: "go", Name: "Start", Parent: "Cache", Signature: "func (c *Cache) Start()", StartLine: 5, EndLine: 8},
			{Path: "internal/cache/cache.go", Kind: "go_method", Language: "go", Name: "Start", Parent: "Worker", Signature: "func (w *Worker) Start()", StartLine: 15, EndLine: 18},
		},
	})
	assertNoError(t, err)

	combined, err := runContextSearch(dbPath, []string{"Start"}, runContextSearchLimit)
	assertNoError(t, err)

	addresses := map[string]bool{}
	for _, item := range combined {
		if item.Title == "Start" {
			addresses[item.Address()] = true
		}
	}
	if !addresses["internal/cache/cache.go:5-8"] || !addresses["internal/cache/cache.go:15-18"] {
		t.Fatalf("distinct same-name methods collapsed in run-context search: %#v", combined)
	}
}

func TestSearchWithoutQueryOrSymbolFails(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code := testApp(root).Run([]string{"search"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit for search with no query or --symbol; stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("expected a usage error:\n%s", stderr.String())
	}
}

func TestSearchStaleSchemaPrintsGuidance(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	// Simulate a legacy index built before the schema marker existed.
	db, err := sql.Open("sqlite", artifacts.New(root).SearchSQLite)
	assertNoError(t, err)
	_, err = db.Exec(`DROP TABLE meta`)
	assertNoError(t, err)
	assertNoError(t, db.Close())

	stdout.Reset()
	stderr.Reset()
	if code := testApp(root).Run([]string{"search", "Runner"}, &stdout, &stderr); code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Search index is stale. Run: pactum map refresh") {
		t.Fatalf("stale schema did not produce refresh guidance:\n%s", got)
	}
}

func mustJSONString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	return strings.Trim(string(raw), `"`)
}
