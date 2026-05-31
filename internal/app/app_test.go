package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/projectmap"
	searchpkg "github.com/heurema/pactum/internal/search"
)

func TestInitCreatesExpectedLayoutAndProjectMap(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	mustWriteFile(t, filepath.Join(root, "src", "main.go"), `package main

type Server struct{}

func main() {}
func Start() {}
func helper() {}
`)
	mustWriteFile(t, filepath.Join(root, "node_modules", "ignored.js"), "console.log('ignored')\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	paths := artifacts.New(root)
	for _, dir := range paths.Dirs() {
		assertDir(t, dir)
	}
	for _, file := range []string{
		paths.Manifest,
		paths.Config,
		paths.Gitignore,
		paths.MapManifest,
		paths.LLMS,
		paths.RepoMap,
		paths.AreaIndex,
		paths.FilesJSONL,
		paths.CodeItemsJSONL,
		paths.HashesJSONL,
		paths.SearchSQLite,
		paths.ProjectMemory,
		paths.StaleReport,
		paths.EventsJSONL,
		paths.UsageJSONL,
		paths.CostJSON,
	} {
		assertFile(t, file)
	}
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	if config.Schema != "pactum.config.v1" {
		t.Fatalf("config schema = %q, want pactum.config.v1", config.Schema)
	}
	if config.ProjectMap.MaxFileBytes != 500000 {
		t.Fatalf("config project_map.max_file_bytes = %d, want 500000", config.ProjectMap.MaxFileBytes)
	}
	if config.ProjectMap.CodeIndex != codeindex.ModeAuto {
		t.Fatalf("config project_map.code_index = %q, want auto", config.ProjectMap.CodeIndex)
	}
	configYAML := mustReadFile(t, paths.Config)
	for _, forbidden := range []string{"include_go_ast", "tree_sitter", "tree_sitter_languages", "entrypoints"} {
		if strings.Contains(configYAML, forbidden) {
			t.Fatalf("config.yaml should not contain %q:\n%s", forbidden, configYAML)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.MapDir, "entries.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("entries.jsonl should not be generated as a primary artifact")
	}

	files := readFileRecords(t, paths.FilesJSONL)
	seen := map[string]bool{}
	for _, file := range files {
		seen[file.Path] = true
	}
	if !seen["README.md"] {
		t.Fatalf("README.md was not indexed: %#v", files)
	}
	if !seen["src/main.go"] {
		t.Fatalf("src/main.go was not indexed: %#v", files)
	}
	if seen["node_modules/ignored.js"] {
		t.Fatalf("node_modules file should have been ignored: %#v", files)
	}

	hashes := readLines(t, paths.HashesJSONL)
	if len(hashes) != len(files) {
		t.Fatalf("hash record count = %d, want %d", len(hashes), len(files))
	}
	codeItems := readCodeItems(t, paths.CodeItemsJSONL)
	if !hasCodeItem(codeItems, "src/main.go", "go_main", "main") {
		t.Fatalf("code items missing go_main main: %#v", codeItems)
	}
	if !hasCodeItem(codeItems, "src/main.go", "go_func", "Start") {
		t.Fatalf("code items missing exported Start func: %#v", codeItems)
	}
	if !hasCodeItem(codeItems, "src/main.go", "go_type", "Server") {
		t.Fatalf("code items missing exported Server type: %#v", codeItems)
	}
	if hasCodeItem(codeItems, "src/main.go", "go_func", "helper") {
		t.Fatalf("code items should not include unexported helper: %#v", codeItems)
	}

	repoMap := mustReadFile(t, paths.RepoMap)
	for _, want := range []string{
		"# Pactum Project Map",
		"## Summary",
		"Code items:",
		"## Code surface",
		"`src/main.go`: `go_main` `main`",
		"`src/main.go`: `go_func` `main.Start`",
		"`src/main.go`: `go_type` `main.Server`",
		"## Language support",
		"Go, Python, JavaScript, TypeScript/TSX/JSX, and C#",
		"not complete semantic truth",
		"## Agent guidance",
		"Before adding new code, search/read relevant files and code items.",
		"README.md",
		"src/main.go",
		"Go: 1 file(s)",
	} {
		if !strings.Contains(repoMap, want) {
			t.Fatalf("repo-map.md missing %q:\n%s", want, repoMap)
		}
	}
	if strings.Contains(repoMap, "Important entrypoints") {
		t.Fatalf("repo-map.md should not use old entrypoints terminology:\n%s", repoMap)
	}
	llms := mustReadFile(t, paths.LLMS)
	for _, want := range []string{
		"generated Pactum project map",
		"repo-map.md",
		"files.jsonl",
		"code-items.jsonl",
		"Go, Python, JavaScript, TypeScript/TSX/JSX, and C#",
		"not complete semantic truth",
		"Not every possible symbol is indexed.",
		"inspect relevant existing files",
		"If ownership is unclear, ask for clarification.",
	} {
		if !strings.Contains(llms, want) {
			t.Fatalf("llms.txt missing %q:\n%s", want, llms)
		}
	}
	if strings.Contains(llms, "Tree-sitter") {
		t.Fatalf("llms.txt should not mention Tree-sitter:\n%s", llms)
	}
	manifest, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)
	if manifest.Artifacts["code_items"] != "map/code-items.jsonl" {
		t.Fatalf("manifest code_items artifact = %q", manifest.Artifacts["code_items"])
	}
	if manifest.Artifacts["search"] != "map/search.sqlite" {
		t.Fatalf("manifest search artifact = %q", manifest.Artifacts["search"])
	}
	if manifest.Artifacts["entries"] != "" {
		t.Fatalf("manifest should not point to entries artifact: %#v", manifest.Artifacts)
	}
	if manifest.CodeIndex.Mode != codeindex.ModeAuto {
		t.Fatalf("manifest code_index.mode = %q, want auto", manifest.CodeIndex.Mode)
	}
	if manifest.CodeIndex.Items != len(codeItems) {
		t.Fatalf("manifest code_index.items = %d, want %d", manifest.CodeIndex.Items, len(codeItems))
	}

	events := readLines(t, paths.EventsJSONL)
	if len(events) != 6 {
		t.Fatalf("events line count = %d, want 6: %v", len(events), events)
	}
	for i, want := range []string{"init_started", "map_refresh_started", "search_index_started", "search_index_finished", "map_refresh_finished", "init_finished"} {
		if !strings.Contains(events[i], want) {
			t.Fatalf("event %d = %s, want %s", i, events[i], want)
		}
	}
}

func TestInitUsesConfigMaxFileBytesAndManifestWarnings(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	config := defaultConfigFile()
	config.ProjectMap.MaxFileBytes = 64
	assertNoError(t, writeYAML(paths.Config, config))
	mustWriteFile(t, filepath.Join(root, "small.go"), "package small\n")
	mustWriteFile(t, filepath.Join(root, "large.go"), "package large\n"+strings.Repeat("x", 128))

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	files := readFileRecords(t, paths.FilesJSONL)
	seen := map[string]bool{}
	for _, file := range files {
		seen[file.Path] = true
	}
	if !seen["small.go"] {
		t.Fatalf("small.go was not indexed: %#v", files)
	}
	if seen["large.go"] {
		t.Fatalf("large.go should have been skipped: %#v", files)
	}

	manifest, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)
	if manifest.FilesSkipped != 1 {
		t.Fatalf("manifest files_skipped = %d, want 1", manifest.FilesSkipped)
	}
	if manifest.CodeIndex.Items == 0 {
		t.Fatalf("manifest code_index.items = 0, want at least package item")
	}
	if !strings.Contains(strings.Join(manifest.Warnings, "\n"), "skipped large file: large.go") {
		t.Fatalf("manifest warnings did not mention skipped large.go: %#v", manifest.Warnings)
	}
}

func TestInitCodeIndexOffDisablesCodeItemExtraction(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	config := defaultConfigFile()
	config.ProjectMap.CodeIndex = codeindex.ModeOff
	assertNoError(t, writeYAML(paths.Config, config))
	mustWriteFile(t, filepath.Join(root, "main.go"), `package main

type Server struct{}

func main() {}
func Start() {}
`)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	codeItems := readCodeItems(t, paths.CodeItemsJSONL)
	if len(codeItems) != 0 {
		t.Fatalf("code items should be empty when code_index is off: %#v", codeItems)
	}
	manifest, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)
	if manifest.CodeIndex.Mode != codeindex.ModeOff {
		t.Fatalf("manifest code_index.mode = %q, want off", manifest.CodeIndex.Mode)
	}
	if manifest.CodeIndex.Items != 0 {
		t.Fatalf("manifest code_index.items = %d, want 0", manifest.CodeIndex.Items)
	}
}

func TestInitRecordsGoParseWarningsWithoutFailing(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "broken.go"), "package broken\nfunc {\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	manifest, err := readMapManifest(artifacts.New(root).MapManifest)
	assertNoError(t, err)
	if !strings.Contains(strings.Join(manifest.Warnings, "\n"), "broken.go") {
		t.Fatalf("manifest warnings did not mention parse failure: %#v", manifest.Warnings)
	}
}

func TestStatusBeforeAndAfterInit(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("status before init output mismatch:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status after init exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Pactum status",
		"initialized: yes",
		"status: fresh",
		"run: map_20260531_184012",
		"files indexed:",
		"active: 0",
		"items: 0",
		"estimated cost: $0.00",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status after init missing %q:\n%s", want, got)
		}
	}
}

func TestInitPrefersGitRoot(t *testing.T) {
	root := t.TempDir()
	assertNoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
	subdir := filepath.Join(root, "nested", "pkg")
	assertNoError(t, os.MkdirAll(subdir, 0o755))
	mustWriteFile(t, filepath.Join(subdir, "main.go"), "package pkg\n")

	var stdout, stderr bytes.Buffer
	code := testApp(subdir).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	assertDir(t, artifacts.New(root).Workspace)
	if _, err := os.Stat(artifacts.New(subdir).Workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace should not be created under subdir")
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(subdir).Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "root: "+root) {
		t.Fatalf("status did not report git root:\n%s", stdout.String())
	}
}

func TestSearchBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"search", "anything"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("search before init output mismatch:\n%s", got)
	}
}

func TestSearchMissingIndexPrintsGuidance(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	paths := artifacts.New(root)
	assertNoError(t, os.Remove(paths.SearchSQLite))

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"search", "Example"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Search index is missing. Run: pactum init") {
		t.Fatalf("missing search index output mismatch:\n%s", got)
	}
}

func TestSearchFindsFilesAndCodeItems(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}

func BuildRunner() {}
func helper() {}
`)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"search", "runner"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Search results for: runner",
		"internal/contracts/runner.go",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("search output missing %q:\n%s", want, got)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"search", "BuildRunner"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	got = stdout.String()
	for _, want := range []string{
		"code_item internal/contracts/runner.go",
		"kind: go_func",
		"name: BuildRunner",
		"language: go",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("code item search output missing %q:\n%s", want, got)
		}
	}
}

func TestSearchKindFilter(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"search", "Runner", "--kind", "code_item"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "code_item internal/contracts/runner.go") {
		t.Fatalf("code_item filter did not return code item:\n%s", got)
	}
	if strings.Contains(got, "file internal/contracts/runner.go") {
		t.Fatalf("code_item filter returned file result:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"search", "Runner", "--kind", "file"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	got = stdout.String()
	if !strings.Contains(got, "file internal/contracts/runner.go") {
		t.Fatalf("file filter did not return file result:\n%s", got)
	}
	if strings.Contains(got, "code_item internal/contracts/runner.go") {
		t.Fatalf("file filter returned code item result:\n%s", got)
	}
}

func TestSearchJSONOutput(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"search", "Runner", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	var response searchpkg.Response
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Query != "Runner" {
		t.Fatalf("query = %q, want Runner", response.Query)
	}
	if len(response.Results) == 0 {
		t.Fatalf("json search results empty:\n%s", stdout.String())
	}
	if response.Results[0].Rank == 0 || response.Results[0].ID == "" {
		t.Fatalf("json search result incomplete: %#v", response.Results[0])
	}
}

func TestSearchNoResults(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"search", "unlikelytermxyz"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No results found.") {
		t.Fatalf("no-results output mismatch:\n%s", got)
	}
}

func testApp(root string) App {
	return App{
		WorkingDir: root,
		Now: func() time.Time {
			return time.Date(2026, 5, 31, 18, 40, 12, 0, time.UTC)
		},
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	assertNoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assertNoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	assertNoError(t, err)
	return string(data)
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	content := strings.TrimSpace(mustReadFile(t, path))
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func readFileRecords(t *testing.T, path string) []projectmap.FileRecord {
	t.Helper()
	lines := readLines(t, path)
	records := make([]projectmap.FileRecord, 0, len(lines))
	for _, line := range lines {
		var record projectmap.FileRecord
		assertNoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func readCodeItems(t *testing.T, path string) []codeindex.Item {
	t.Helper()
	lines := readLines(t, path)
	records := make([]codeindex.Item, 0, len(lines))
	for _, line := range lines {
		var record codeindex.Item
		assertNoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func hasCodeItem(items []codeindex.Item, path string, kind string, name string) bool {
	for _, item := range items {
		if item.Path == path && item.Kind == kind && item.Name == name {
			return true
		}
	}
	return false
}

func assertDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	assertNoError(t, err)
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	assertNoError(t, err)
	if !info.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file", path)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
