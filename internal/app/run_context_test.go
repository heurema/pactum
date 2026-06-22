package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func TestExtractRunContextQueries(t *testing.T) {
	queries := extractRunContextQueries("add a formatPercent helper to apps/admin/src/lib/format.ts following formatCurrency")

	for _, want := range []string{"apps/admin/src/lib/format.ts", "formatPercent", "formatCurrency", "format"} {
		if !contains(queries, want) {
			t.Fatalf("queries missing %q: %#v", want, queries)
		}
	}
	if !contains(queries, "percent") && !contains(queries, "currency") {
		t.Fatalf("queries should include split domain terms: %#v", queries)
	}
	// Path-like query comes first.
	if len(queries) == 0 || queries[0] != "apps/admin/src/lib/format.ts" {
		t.Fatalf("path-like query should rank first: %#v", queries)
	}
	// Generic verbs / filler must not become queries.
	for _, unwanted := range []string{"add", "helper", "following", "with", "a", "to"} {
		if contains(queries, unwanted) {
			t.Fatalf("queries should not include stopword %q: %#v", unwanted, queries)
		}
	}
	if len(queries) > 8 {
		t.Fatalf("queries should be capped at 8: %#v", queries)
	}
}

// initFormatFixture creates a small TS monorepo-ish layout where the target file
// is several directories deep, so a full task sentence (ANDed) would not match
// any single document but targeted queries will.
func initFormatFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	skipIfNoGit(t)
	mustGitG(t, root, "init")
	mustGitG(t, root, "config", "user.email", "test@test.com")
	mustGitG(t, root, "config", "user.name", "Test")
	mustGitG(t, root, "config", "commit.gpgsign", "false")
	mustWriteFile(t, filepath.Join(root, "package.json"), "{\n  \"name\": \"web\"\n}\n")
	mustWriteFile(t, filepath.Join(root, "apps", "admin", "src", "lib", "format.ts"), `export function formatCurrency(value: number): string {
  return value.toFixed(2)
}
`)
	mustWriteFile(t, filepath.Join(root, "apps", "admin", "src", "lib", "format.test.ts"), `import { formatCurrency } from "./format"
`)
	app := testApp(root)
	wikiRunOK(t, app, "init")
	mustGitG(t, root, "add", "package.json")
	mustGitG(t, root, "commit", "-m", "init")
	return root, artifacts.New(root)
}

func readRunSearchResultsFile(t *testing.T, path string) runSearchResults {
	t.Helper()
	var results runSearchResults
	if err := json.Unmarshal([]byte(mustReadFile(t, path)), &results); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return results
}

func runContextResultPaths(results runSearchResults) []string {
	paths := make([]string, 0, len(results.Results))
	for _, r := range results.Results {
		paths = append(paths, r.Path)
	}
	return paths
}

func TestRunContextSearchFromTask(t *testing.T) {
	root, paths := initFormatFixture(t)
	runDir := filepath.Join(paths.RunsDir, "run_20260531_184012")
	srPath := filepath.Join(runDir, "context", "search-results.json")

	app := testApp(root)
	wikiRunOK(t, app, "task", "new", "add a formatPercent helper to apps/admin/src/lib/format.ts following formatCurrency")

	results := readRunSearchResultsFile(t, srPath)
	if results.QuerySource != "task" {
		t.Fatalf("query_source = %q, want task", results.QuerySource)
	}
	if len(results.Queries) < 2 {
		t.Fatalf("expected multiple targeted queries, got %#v", results.Queries)
	}
	if len(results.Results) == 0 {
		t.Fatalf("run-context search should not be empty for a natural-language task; queries=%#v", results.Queries)
	}
	if !contains(runContextResultPaths(results), "apps/admin/src/lib/format.ts") {
		t.Fatalf("results should include the target file; got %#v", runContextResultPaths(results))
	}
	// Dedupe: each (kind,path,title,code_kind) appears once.
	seen := map[string]bool{}
	for _, r := range results.Results {
		key := r.Kind + "|" + r.Path + "|" + r.Title + "|" + r.CodeKind
		if seen[key] {
			t.Fatalf("duplicate result %q in %#v", key, results.Results)
		}
		seen[key] = true
	}
	// Portability: the run artifact must not embed the absolute repo path.
	if strings.Contains(mustReadFile(t, srPath), root) {
		t.Fatalf("search-results.json embeds absolute repo path %q", root)
	}
}

func TestRunContextSearchSurfacesReferencedFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "package.json"), "{\n  \"name\": \"web\"\n}\n")
	mustWriteFile(t, filepath.Join(root, "apps", "admin", "src", "lib", "format.ts"), `export function formatCurrency(value: number): string {
  return value.toFixed(2)
}
`)
	mustWriteFile(t, filepath.Join(root, "apps", "admin", "src", "lib", "format-helpers.ts"), `export const reexport = 1
`)
	app := testApp(root)
	wikiRunOK(t, app, "init")
	srPath := filepath.Join(artifacts.New(root).RunsDir, "run_20260531_184012", "context", "search-results.json")
	wikiRunOK(t, app, "task", "new", "update apps/admin/src/lib/format.ts and formatCurrency")

	results := readRunSearchResultsFile(t, srPath)
	if len(results.Results) == 0 {
		t.Fatal("expected run-context results")
	}

	// The explicitly-mentioned file is surfaced.
	foundFile := false
	for _, r := range results.Results {
		if strings.Contains(r.Path, "format.ts") {
			foundFile = true
		}
	}
	if !foundFile {
		t.Fatalf("format.ts should be represented in run-context results: %#v", results.Results)
	}
}

func TestRunContextSearchRefreshedFromContract(t *testing.T) {
	root, paths := initFormatFixture(t)
	runDir := filepath.Join(paths.RunsDir, "run_20260531_184012")
	srPath := filepath.Join(runDir, "context", "search-results.json")
	ecPath := filepath.Join(runDir, "context", "executor-context.md")

	app := testApp(root)
	// Vague initial task that does not match the target file.
	wikiRunOK(t, app, "task", "new", "improve developer experience")
	taskResults := readRunSearchResultsFile(t, srPath)
	if contains(runContextResultPaths(taskResults), "apps/admin/src/lib/format.ts") {
		t.Fatalf("vague task should not already surface the target file: %#v", runContextResultPaths(taskResults))
	}

	// Clarify the contract to point at the real file/symbol, then build the prompt.
	reviseDoc := writeReviseDocForTest(t, contractRunPaths(runDir), map[string]any{
		"goal":  "add formatPercent to apps/admin/src/lib/format.ts",
		"scope": map[string]any{"in": []string{"follow formatCurrency in apps/admin/src/lib/format.ts"}},
	})
	wikiRunOK(t, app, "contract", "revise", "--from", reviseDoc)
	wikiRunOK(t, app, "contract", "approve", "--by", "manual")
	wikiRunOK(t, app, "prompt", "build")

	results := readRunSearchResultsFile(t, srPath)
	if results.QuerySource != "contract" {
		t.Fatalf("query_source = %q, want contract after prompt build", results.QuerySource)
	}
	if !contains(runContextResultPaths(results), "apps/admin/src/lib/format.ts") {
		t.Fatalf("contract-refreshed results should include the target file; got %#v", runContextResultPaths(results))
	}

	ec := mustReadFile(t, ecPath)
	for _, absent := range []string{"repo-map.md", "llms.txt", "search.sqlite", "context/search-results.json", "Relevant search results"} {
		if strings.Contains(ec, absent) {
			t.Fatalf("executor-context.md must not contain map/search injection %q:\n%s", absent, ec)
		}
	}
	if strings.Contains(ec, root) {
		t.Fatalf("executor-context.md embeds absolute repo path %q", root)
	}
}
