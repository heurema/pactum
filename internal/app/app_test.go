package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/agents"
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
		filepath.Join(paths.MapRunsDir, "map_20260531_184012.json"),
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
	for _, forbidden := range []string{"agents:", "adapters:", "default_executor:", "default_reviewer:", "include_go_ast", "tree_sitter", "tree_sitter_languages", "entrypoints"} {
		if strings.Contains(configYAML, forbidden) {
			t.Fatalf("config.yaml should not contain %q:\n%s", forbidden, configYAML)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.MapDir, "entries.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("entries.jsonl should not be generated as a primary artifact")
	}
	gitignore := mustReadFile(t, paths.Gitignore)
	for _, want := range []string{
		"map/",
		"ledger/",
		"cache/",
		"tmp/",
		"runs/*/ledger/",
		"runs/*/execute/",
		"runs/*/review/",
	} {
		if !strings.Contains(gitignore, want) {
			t.Fatalf(".gitignore missing %q:\n%s", want, gitignore)
		}
	}
	for _, forbidden := range []string{"runs/*/task.md", "runs/*/context/", "runs/*/contract/", "runs/*/memory/", "contract/*.md", "contract/*.json"} {
		if strings.Contains(gitignore, forbidden) {
			t.Fatalf(".gitignore should not ignore %q:\n%s", forbidden, gitignore)
		}
	}
	workspaceManifest, err := readWorkspaceManifest(paths.Manifest)
	assertNoError(t, err)
	if workspaceManifest.RepoRoot != "." {
		t.Fatalf("workspace manifest repo_root = %q, want .", workspaceManifest.RepoRoot)
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
		"Repository root: `.`",
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
	if manifest.RepoRoot != "." {
		t.Fatalf("map manifest repo_root = %q, want .", manifest.RepoRoot)
	}
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
	if manifest.ConfigHash == "" {
		t.Fatalf("manifest config_hash should be populated")
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

func TestStatusReportsStaleReasonsAndRefreshClearsThem(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	mustWriteFile(t, filepath.Join(root, "old.go"), "package old\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")
	assertNoError(t, os.Remove(filepath.Join(root, "old.go")))
	mustWriteFile(t, filepath.Join(root, "new.go"), "package newfile\n")

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"status: stale",
		"changed file: README.md",
		"missing file: old.go",
		"new file: new.go",
		"Suggested:",
		"pactum map refresh",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stale status missing %q:\n%s", want, got)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status after refresh exited %d, stderr: %s", code, stderr.String())
	}
	got = stdout.String()
	if !strings.Contains(got, "status: fresh") {
		t.Fatalf("status after refresh should be fresh:\n%s", got)
	}
	if strings.Contains(got, "Stale reasons:") {
		t.Fatalf("status after refresh should not print stale reasons:\n%s", got)
	}
}

func TestStatusReportsMissingArtifactAndConfigChange(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	paths := artifacts.New(root)
	assertNoError(t, os.Remove(paths.SearchSQLite))
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.ProjectMap.MaxFileBytes = 123
	assertNoError(t, writeYAML(paths.Config, config))

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"status: stale",
		"search index: missing",
		"missing artifact: map/search.sqlite",
		"config changed: .heurema/pactum/config.yaml",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing %q:\n%s", want, got)
		}
	}
}

func TestStatusJSONOutput(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"status", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status --json before init exited %d, stderr: %s", code, stderr.String())
	}
	var before struct {
		Initialized bool   `json:"initialized"`
		Message     string `json:"message"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &before))
	if before.Initialized || before.Message != "Pactum is not initialized. Run: pactum init" {
		t.Fatalf("unexpected status --json before init: %#v", before)
	}

	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"status", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status --json after init exited %d, stderr: %s", code, stderr.String())
	}
	var after statusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &after))
	if !after.Initialized {
		t.Fatalf("status --json should report initialized: %#v", after)
	}
	if after.RepoRoot != root {
		t.Fatalf("repo_root = %q, want %q", after.RepoRoot, root)
	}
	if after.Workspace != artifacts.New(root).Workspace {
		t.Fatalf("workspace = %q, want %q", after.Workspace, artifacts.New(root).Workspace)
	}
	if after.ProjectMap.Status != "fresh" {
		t.Fatalf("project_map.status = %q, want fresh: %#v", after.ProjectMap.Status, after.ProjectMap)
	}
	if after.ProjectMap.SearchIndex != "ready" {
		t.Fatalf("project_map.search_index = %q, want ready", after.ProjectMap.SearchIndex)
	}
	if len(after.ProjectMap.StaleReasons) != 0 {
		t.Fatalf("project_map.stale_reasons = %#v, want empty", after.ProjectMap.StaleReasons)
	}
	if after.Runs.Active != 0 || after.Memory.Items != 0 || after.Usage.TotalTokens != 0 || after.Usage.EstimatedCostUSD != 0 {
		t.Fatalf("unexpected zero-status sections: %#v", after)
	}
}

func TestRunBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"run", "add sqlite cache", "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("run before init output mismatch:\n%s", got)
	}
}

func TestRunWithoutContractOnlyPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"run", "add sqlite cache"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run without --contract-only exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Execution is not implemented yet. Use --contract-only.") {
		t.Fatalf("run without --contract-only output mismatch:\n%s", got)
	}
}

func TestRunContractOnlyCreatesLayoutAndArtifacts(t *testing.T) {
	root := t.TempDir()
	task := "add sqlite cache"
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", task, "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Run created",
		"id: run_20260531_184012",
		"status: contract_draft",
		"task: add sqlite cache",
		"task: .heurema/pactum/runs/run_20260531_184012/task.md",
		"context: .heurema/pactum/runs/run_20260531_184012/context/repo-context.md",
		"contract: .heurema/pactum/runs/run_20260531_184012/contract/contract.md",
		"Review the generated contract draft.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run output missing %q:\n%s", want, got)
		}
	}

	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, "run_20260531_184012")
	for _, dir := range []string{
		runDir,
		filepath.Join(runDir, "context"),
		filepath.Join(runDir, "clarify"),
		filepath.Join(runDir, "contract"),
		filepath.Join(runDir, "execute"),
		filepath.Join(runDir, "review"),
		filepath.Join(runDir, "memory"),
		filepath.Join(runDir, "ledger"),
	} {
		assertDir(t, dir)
	}
	for _, file := range []string{
		filepath.Join(runDir, "run.json"),
		filepath.Join(runDir, "task.md"),
		filepath.Join(runDir, "context", "repo-context.md"),
		filepath.Join(runDir, "context", "search-results.json"),
		filepath.Join(runDir, "clarify", "questions.jsonl"),
		filepath.Join(runDir, "clarify", "answers.jsonl"),
		filepath.Join(runDir, "clarify", "decisions.jsonl"),
		filepath.Join(runDir, "contract", "contract.json"),
		filepath.Join(runDir, "contract", "contract.md"),
		filepath.Join(runDir, "contract", "prompt.md"),
		filepath.Join(runDir, "contract", "approval.json"),
	} {
		assertFile(t, file)
	}

	var state contractRunState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(runDir, "run.json"))), &state))
	if state.Schema != "pactum.run.v1" || state.RunID != "run_20260531_184012" || state.Status != "contract_draft" {
		t.Fatalf("unexpected run state: %#v", state)
	}
	if state.Task != task || state.RepoRoot != "." || state.Workspace != artifacts.WorkspaceRel || state.MapRunID != "map_20260531_184012" {
		t.Fatalf("run state has unexpected task/root/workspace/map: %#v", state)
	}
	if state.Artifacts.ContractJSON != "contract/contract.json" || state.Artifacts.SearchResults != "context/search-results.json" {
		t.Fatalf("run state artifacts mismatch: %#v", state.Artifacts)
	}

	taskMD := mustReadFile(t, filepath.Join(runDir, "task.md"))
	if !strings.Contains(taskMD, "# Task") || !strings.Contains(taskMD, task) || !strings.Contains(taskMD, "Generated: 2026-05-31T18:40:12Z") {
		t.Fatalf("task.md content mismatch:\n%s", taskMD)
	}
	repoContext := mustReadFile(t, filepath.Join(runDir, "context", "repo-context.md"))
	for _, want := range []string{
		"Map run: map_20260531_184012",
		"Repo map path: .heurema/pactum/map/repo-map.md",
		"LLMS path: .heurema/pactum/map/llms.txt",
		"Search index path: .heurema/pactum/map/search.sqlite",
		"Pactum has not yet done agentic clarification.",
		"deterministic context",
		"# Pactum Project Map",
		"Repository root: `.`",
	} {
		if !strings.Contains(repoContext, want) {
			t.Fatalf("repo-context.md missing %q:\n%s", want, repoContext)
		}
	}

	var searchResults runSearchResults
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(runDir, "context", "search-results.json"))), &searchResults))
	if searchResults.Query != task || searchResults.Results == nil {
		t.Fatalf("unexpected search-results.json: %#v", searchResults)
	}

	var contract draftContract
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(runDir, "contract", "contract.json"))), &contract))
	if contract.Schema != "pactum.contract.v1" || contract.RunID != "run_20260531_184012" || contract.Goal != task {
		t.Fatalf("unexpected contract.json: %#v", contract)
	}
	if len(contract.Scope.In) != 0 || len(contract.Scope.Out) != 0 || len(contract.AcceptanceCriteria) != 0 || len(contract.Validation.Commands) != 0 {
		t.Fatalf("contract draft should keep empty deterministic sections: %#v", contract)
	}
	if len(contract.OpenQuestions) != 0 {
		t.Fatalf("initial contract open_questions = %#v, want empty", contract.OpenQuestions)
	}
	if len(contract.Clarifications.Questions) != 0 {
		t.Fatalf("initial contract clarifications = %#v, want empty", contract.Clarifications.Questions)
	}
	contractMD := mustReadFile(t, filepath.Join(runDir, "contract", "contract.md"))
	if !strings.Contains(contractMD, "## Goal\n"+task) ||
		!strings.Contains(contractMD, "Manual clarification and approval are available; agent execution is not implemented yet.") ||
		!strings.Contains(contractMD, "Repo map: .heurema/pactum/map/repo-map.md") ||
		!strings.Contains(contractMD, "Search results: context/search-results.json") ||
		!strings.Contains(contractMD, "## Clarifications\n- None") ||
		!strings.Contains(contractMD, "## Validation commands\nTBD") ||
		!strings.Contains(contractMD, "## Assumptions\nTBD") ||
		!strings.Contains(contractMD, "## Open questions\n- None") {
		t.Fatalf("contract.md content mismatch:\n%s", contractMD)
	}
	for _, stale := range []string{"Clarification loop is not implemented yet.", "Clarification and agent execution are not implemented yet."} {
		if strings.Contains(contractMD, stale) {
			t.Fatalf("contract.md contains stale wording %q:\n%s", stale, contractMD)
		}
	}
	promptMD := mustReadFile(t, filepath.Join(runDir, "contract", "prompt.md"))
	if !strings.Contains(promptMD, "This prompt is not executable yet. Manual clarification and approval are available, but agent execution is not implemented.") ||
		!strings.Contains(promptMD, "## Goal\n"+task) ||
		!strings.Contains(promptMD, "## Validation commands\nTBD") {
		t.Fatalf("prompt.md content mismatch:\n%s", promptMD)
	}
	if strings.Contains(promptMD, "Contract clarification and approval are not implemented.") {
		t.Fatalf("prompt.md contains stale wording:\n%s", promptMD)
	}
	var approval approvalState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(runDir, "contract", "approval.json"))), &approval))
	if approval.Schema != approvalSchema || approval.Status != "pending" || approval.ApprovedAt != nil || approval.ApprovedBy != nil || approval.ContractSHA256 != nil {
		t.Fatalf("unexpected approval.json: %#v", approval)
	}

	events := readLines(t, paths.EventsJSONL)
	if len(events) != 8 {
		t.Fatalf("events line count = %d, want 8: %v", len(events), events)
	}
	for i, want := range []string{"run_created", "contract_draft_created"} {
		event := events[len(events)-2+i]
		if !strings.Contains(event, want) || !strings.Contains(event, "run_20260531_184012") || !strings.Contains(event, root) {
			t.Fatalf("event %d = %s, want %s with run/root", len(events)-2+i, event, want)
		}
	}
}

func TestRunContractOnlyArtifactsUseRepoRelativePaths(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "task.md"), "# Task\n\ncontract task notes\n")

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", "task", "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}

	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, "run_20260531_184012")
	workspaceManifest := mustReadFile(t, paths.Manifest)
	mapManifest := mustReadFile(t, paths.MapManifest)
	repoMap := mustReadFile(t, paths.RepoMap)
	mapRun := mustReadFile(t, filepath.Join(paths.MapRunsDir, "map_20260531_184012.json"))
	runJSON := mustReadFile(t, filepath.Join(runDir, "run.json"))
	repoContext := mustReadFile(t, filepath.Join(runDir, "context", "repo-context.md"))
	contractMD := mustReadFile(t, filepath.Join(runDir, "contract", "contract.md"))
	searchResults := mustReadFile(t, filepath.Join(runDir, "context", "search-results.json"))

	for name, content := range map[string]string{
		"manifest.json":       workspaceManifest,
		"map/manifest.json":   mapManifest,
		"map/repo-map.md":     repoMap,
		"map/runs/run.json":   mapRun,
		"run.json":            runJSON,
		"repo-context.md":     repoContext,
		"contract.md":         contractMD,
		"search-results.json": searchResults,
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}

	for name, content := range map[string]string{
		"manifest.json":     workspaceManifest,
		"map/manifest.json": mapManifest,
		"map/runs/run.json": mapRun,
	} {
		if !strings.Contains(content, `"repo_root": "."`) {
			t.Fatalf("%s missing portable repo_root:\n%s", name, content)
		}
	}
	if !strings.Contains(repoMap, "Repository root: `.`") {
		t.Fatalf("repo-map.md missing portable repository root:\n%s", repoMap)
	}
	for _, want := range []string{
		`"repo_root": "."`,
		`"workspace": ".heurema/pactum"`,
		`"task": "task.md"`,
		`"repo_context": "context/repo-context.md"`,
		`"contract_json": "contract/contract.json"`,
	} {
		if !strings.Contains(runJSON, want) {
			t.Fatalf("run.json missing portable path %q:\n%s", want, runJSON)
		}
	}
	for _, want := range []string{
		"Repo map path: .heurema/pactum/map/repo-map.md",
		"LLMS path: .heurema/pactum/map/llms.txt",
		"Search index path: .heurema/pactum/map/search.sqlite",
		"Repository root: `.`",
	} {
		if !strings.Contains(repoContext, want) {
			t.Fatalf("repo-context.md missing repo-relative path %q:\n%s", want, repoContext)
		}
	}
	for _, want := range []string{
		"Repo map: .heurema/pactum/map/repo-map.md",
		"Search results: context/search-results.json",
	} {
		if !strings.Contains(contractMD, want) {
			t.Fatalf("contract.md missing repo-relative path %q:\n%s", want, contractMD)
		}
	}
	if !strings.Contains(searchResults, `"path": "task.md"`) {
		t.Fatalf("search-results.json should contain repo-relative result path:\n%s", searchResults)
	}
}

func TestRunContractOnlySearchResultsWarnWhenIndexMissing(t *testing.T) {
	root := t.TempDir()
	task := "add sqlite cache"
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	paths := artifacts.New(root)
	assertNoError(t, os.Remove(paths.SearchSQLite))

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", task, "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}

	var searchResults runSearchResults
	runDir := filepath.Join(paths.RunsDir, "run_20260531_184012")
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(runDir, "context", "search-results.json"))), &searchResults))
	if searchResults.Query != task || len(searchResults.Results) != 0 {
		t.Fatalf("unexpected search results for missing index: %#v", searchResults)
	}
	if len(searchResults.Warnings) != 1 || !strings.Contains(searchResults.Warnings[0], "Search index is stale") {
		t.Fatalf("missing stale search warning: %#v", searchResults)
	}
}

func TestRunContractOnlyJSONOutput(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", "add sqlite cache", "--contract-only", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only --json exited %d, stderr: %s", code, stderr.String())
	}
	var state contractRunState
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &state))
	if state.RunID != "run_20260531_184012" || state.Status != "contract_draft" || state.Task != "add sqlite cache" {
		t.Fatalf("unexpected run json output: %#v", state)
	}
	if state.RepoRoot != "." || state.Workspace != artifacts.WorkspaceRel {
		t.Fatalf("run json should use portable paths: %#v", state)
	}
	if strings.Contains(stdout.String(), "Run created") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestRunIDsAreCollisionSafeWithFixedTimestamp(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", "first task", "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first run exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", "second task", "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("second run exited %d, stderr: %s", code, stderr.String())
	}

	paths := artifacts.New(root)
	for _, runID := range []string{"run_20260531_184012", "run_20260531_184012_02"} {
		assertDir(t, filepath.Join(paths.RunsDir, runID))
		assertFile(t, filepath.Join(paths.RunsDir, runID, "run.json"))
	}
	var second contractRunState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(paths.RunsDir, "run_20260531_184012_02", "run.json"))), &second))
	if second.RunID != "run_20260531_184012_02" || second.Task != "second task" {
		t.Fatalf("unexpected second run state: %#v", second)
	}
}

func TestRunContractOnlyConcurrentRunsUseDistinctDirectories(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	const runCount = 2
	var wg sync.WaitGroup
	errs := make(chan string, runCount)
	for i := range runCount {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var stdout, stderr bytes.Buffer
			code := app.Run([]string{"run", fmt.Sprintf("task %d", i), "--contract-only"}, &stdout, &stderr)
			if code != 0 {
				errs <- fmt.Sprintf("run %d exited %d, stderr: %s", i, code, stderr.String())
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	paths := artifacts.New(root)
	for _, runID := range []string{"run_20260531_184012", "run_20260531_184012_02"} {
		runDir := filepath.Join(paths.RunsDir, runID)
		assertDir(t, runDir)
		assertFile(t, filepath.Join(runDir, "run.json"))

		var state contractRunState
		assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(runDir, "run.json"))), &state))
		if state.RunID != runID {
			t.Fatalf("run.json in %s has run_id %q", runDir, state.RunID)
		}
	}
}

func TestStatusActiveRunsCountIncreasesAfterContractOnlyRun(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status before run exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "active: 0") {
		t.Fatalf("status before run should report active 0:\n%s", got)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", "add sqlite cache", "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"status"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status after run exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "active: 1") {
		t.Fatalf("status after run should report active 1:\n%s", got)
	}
}

func TestStatusJSONIncludesActiveRunCount(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", "add sqlite cache", "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"status", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status --json exited %d, stderr: %s", code, stderr.String())
	}
	var status statusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.Runs.Active != 1 {
		t.Fatalf("status --json runs.active = %d, want 1: %#v", status.Runs.Active, status)
	}
}

func TestContractBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	for _, args := range [][]string{
		{"contract", "show", "run_x"},
		{"contract", "revise", "run_x", "--goal", "new goal"},
		{"contract", "approve", "run_x"},
	} {
		var stdout, stderr bytes.Buffer
		code := testApp(root).Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
		if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
			t.Fatalf("%v output mismatch:\n%s", args, got)
		}
	}
}

func TestContractShowExistingRun(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Contract",
		"id: " + runID,
		"status: contract_draft",
		"Approval:",
		"status: pending",
		"# Contract Draft",
		"## Goal\nadd sqlite cache",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("contract show missing %q:\n%s", want, got)
		}
	}
}

func TestContractShowJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show --json exited %d, stderr: %s", code, stderr.String())
	}
	var response contractShowResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.RunID != runID || response.RunStatus != "contract_draft" || response.Contract.Goal != "add sqlite cache" || response.Approval.Status != "pending" {
		t.Fatalf("unexpected contract show json: %#v", response)
	}
	if strings.Contains(stdout.String(), "Contract\n") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestContractRunNotFoundReturnsError(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	app := testApp(root)
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "show", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("contract show missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestContractReviseGoal(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--goal", "add sqlite-backed cache"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Contract revised") || !strings.Contains(got, "status: contract_draft") {
		t.Fatalf("contract revise output mismatch:\n%s", got)
	}

	state := readRunState(t, runPaths.RunJSON)
	if state.Task != "add sqlite cache" || state.Status != "contract_draft" {
		t.Fatalf("run state should keep original task and draft status: %#v", state)
	}
	contract := readContractDraft(t, runPaths.ContractJSON)
	if contract.Goal != "add sqlite-backed cache" {
		t.Fatalf("contract goal = %q", contract.Goal)
	}
	if got := mustReadFile(t, runPaths.ContractMD); !strings.Contains(got, "## Goal\nadd sqlite-backed cache") {
		t.Fatalf("contract.md missing revised goal:\n%s", got)
	}
	if got := mustReadFile(t, runPaths.PromptMD); !strings.Contains(got, "## Goal\nadd sqlite-backed cache") {
		t.Fatalf("prompt.md missing revised goal:\n%s", got)
	}
	approval := readApproval(t, runPaths.ApprovalJSON)
	if approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("approval should remain pending: %#v", approval)
	}
	if events := strings.Join(readLines(t, paths.EventsJSONL), "\n"); !strings.Contains(events, "contract_revised") {
		t.Fatalf("events missing contract_revised:\n%s", events)
	}
}

func TestContractReviseAppendsFields(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{
		"contract", "revise", runID,
		"--add-in-scope", "Add show, revise, and approve commands",
		"--add-in-scope", "Persist cache metadata",
		"--add-in-scope", "Expose status summary",
		"--add-out-of-scope", "Implement cache eviction",
		"--add-acceptance", "Cache survives process restart",
		"--add-validation", "go test ./...",
		"--add-assumption", "SQLite is available through the standard driver",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}

	contract := readContractDraft(t, runPaths.ContractJSON)
	if len(contract.Scope.In) != 3 || contract.Scope.In[0] != "Add show, revise, and approve commands" {
		t.Fatalf("contract in-scope should preserve comma-containing item: %#v", contract.Scope.In)
	}
	if got := strings.Join(contract.Scope.In, "\n"); !strings.Contains(got, "Persist cache metadata") || !strings.Contains(got, "Expose status summary") {
		t.Fatalf("contract in-scope mismatch: %#v", contract.Scope.In)
	}
	if len(contract.Scope.Out) != 1 || contract.Scope.Out[0] != "Implement cache eviction" {
		t.Fatalf("contract out-of-scope mismatch: %#v", contract.Scope.Out)
	}
	if len(contract.AcceptanceCriteria) != 1 || contract.AcceptanceCriteria[0] != "Cache survives process restart" {
		t.Fatalf("contract acceptance mismatch: %#v", contract.AcceptanceCriteria)
	}
	if len(contract.Validation.Commands) != 1 || contract.Validation.Commands[0] != "go test ./..." {
		t.Fatalf("contract validation mismatch: %#v", contract.Validation.Commands)
	}
	if len(contract.Assumptions) != 1 || contract.Assumptions[0] != "SQLite is available through the standard driver" {
		t.Fatalf("contract assumptions mismatch: %#v", contract.Assumptions)
	}

	contractMD := mustReadFile(t, runPaths.ContractMD)
	for _, want := range []string{
		"## In scope\n- Add show, revise, and approve commands\n- Persist cache metadata\n- Expose status summary",
		"## Out of scope\n- Implement cache eviction",
		"## Acceptance criteria\n- Cache survives process restart",
		"## Validation commands\n- go test ./...",
		"## Assumptions\n- SQLite is available through the standard driver",
	} {
		if !strings.Contains(contractMD, want) {
			t.Fatalf("contract.md missing %q:\n%s", want, contractMD)
		}
	}
	promptMD := mustReadFile(t, runPaths.PromptMD)
	for _, want := range []string{
		"## In scope\n- Add show, revise, and approve commands\n- Persist cache metadata\n- Expose status summary",
		"## Out of scope\n- Implement cache eviction",
		"## Acceptance criteria\n- Cache survives process restart",
		"## Validation commands\n- go test ./...",
	} {
		if !strings.Contains(promptMD, want) {
			t.Fatalf("prompt.md missing %q:\n%s", want, promptMD)
		}
	}
}

func TestContractReviseWithoutFlagsErrors(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("contract revise without flags should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "no contract revisions provided") {
		t.Fatalf("contract revise stderr mismatch:\n%s", got)
	}
}

func TestContractApproveSucceedsWithoutBlockingQuestions(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Contract approved") || !strings.Contains(got, "status: contract_approved") || !strings.Contains(got, "approved by: manual") {
		t.Fatalf("contract approve output mismatch:\n%s", got)
	}

	state := readRunState(t, runPaths.RunJSON)
	if state.Status != "contract_approved" {
		t.Fatalf("run status = %q, want contract_approved", state.Status)
	}
	contract := readContractDraft(t, runPaths.ContractJSON)
	if contract.Status != "approved" {
		t.Fatalf("contract status = %q, want approved", contract.Status)
	}
	if got := mustReadFile(t, runPaths.ContractMD); !strings.Contains(got, "Contract status: approved") || strings.Contains(got, "Contract status: draft") {
		t.Fatalf("contract.md should show approved status:\n%s", got)
	}
	approval := readApproval(t, runPaths.ApprovalJSON)
	if approval.Schema != approvalSchema || approval.Status != "approved" || approval.ApprovedAt == nil || *approval.ApprovedAt != "2026-05-31T18:40:12Z" || approval.ApprovedBy == nil || *approval.ApprovedBy != "manual" || approval.ContractSHA256 == nil || *approval.ContractSHA256 == "" {
		t.Fatalf("unexpected approval: %#v", approval)
	}
	hash, err := fileSHA256(runPaths.ContractJSON)
	assertNoError(t, err)
	if *approval.ContractSHA256 != hash {
		t.Fatalf("approval hash = %q, want %q", *approval.ContractSHA256, hash)
	}
	if events := strings.Join(readLines(t, paths.EventsJSONL), "\n"); !strings.Contains(events, "contract_approved") {
		t.Fatalf("events missing contract_approved:\n%s", events)
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show after approve exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Contract status: approved") || strings.Contains(got, "Contract status: draft") {
		t.Fatalf("contract show should render approved status:\n%s", got)
	}
}

func TestContractApproveByRecordsApprover(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID, "--by", "alice"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve --by exited %d, stderr: %s", code, stderr.String())
	}
	approval := readApproval(t, runPaths.ApprovalJSON)
	if approval.ApprovedBy == nil || *approval.ApprovedBy != "alice" {
		t.Fatalf("approved_by = %#v, want alice", approval.ApprovedBy)
	}
}

func TestContractApproveJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve --json exited %d, stderr: %s", code, stderr.String())
	}
	var response contractApproveResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.RunID != runID || response.RunStatus != "contract_approved" || response.Approval.Status != "approved" || response.Approval.ContractSHA256 == nil {
		t.Fatalf("unexpected approve json: %#v", response)
	}
	if strings.Contains(stdout.String(), "Contract approved") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
}

func TestContractApproveBlockedByOpenBlockingClarification(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Should approval wait?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("contract approve should fail while blocking questions remain")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot approve contract: blocking clarification questions remain") {
		t.Fatalf("approve stderr mismatch:\n%s", got)
	}
	if got := stdout.String(); !strings.Contains(got, "Blocking clarification questions remain") || !strings.Contains(got, "q_001 Should approval wait?") {
		t.Fatalf("approve stdout should list blocking questions:\n%s", got)
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "clarifying" {
		t.Fatalf("run status = %q, want clarifying", state.Status)
	}
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("approval should remain pending: %#v", approval)
	}
}

func TestContractApproveAllowsNonBlockingOpenClarification(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Optional context?"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve with non-blocking open question exited %d, stderr: %s", code, stderr.String())
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "contract_approved" {
		t.Fatalf("run status = %q, want contract_approved", state.Status)
	}
	if contract := readContractDraft(t, runPaths.ContractJSON); contract.Status != "approved" || len(contract.OpenQuestions) != 1 || contract.OpenQuestions[0] != "Optional context?" {
		t.Fatalf("contract should retain non-blocking open question: %#v", contract.OpenQuestions)
	}
}

func TestContractReviseApprovedContractResetsApproval(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "revise", runID, "--add-in-scope", "Use sqlite"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "reset: true") {
		t.Fatalf("contract revise should report approval reset:\n%s", got)
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "contract_draft" {
		t.Fatalf("run status = %q, want contract_draft", state.Status)
	}
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil || approval.ApprovedBy != nil || approval.ApprovedAt != nil {
		t.Fatalf("approval should reset to pending: %#v", approval)
	}
	if contract := readContractDraft(t, runPaths.ContractJSON); contract.Status != "draft" {
		t.Fatalf("contract status = %q, want draft after revision", contract.Status)
	}
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	for _, want := range []string{"contract_approved", "contract_approval_reset", "contract_revised"} {
		if !strings.Contains(events, want) {
			t.Fatalf("events missing %q:\n%s", want, events)
		}
	}
}

func TestClarifyAfterApprovedContractResetsApproval(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "ask", runID, "Need approval reset?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "clarifying" {
		t.Fatalf("run status = %q, want clarifying", state.Status)
	}
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("approval should reset after clarification: %#v", approval)
	}
	if contract := readContractDraft(t, runPaths.ContractJSON); contract.Status != "draft" {
		t.Fatalf("contract status = %q, want draft after clarification", contract.Status)
	}
	if events := strings.Join(readLines(t, paths.EventsJSONL), "\n"); !strings.Contains(events, "contract_approval_reset") {
		t.Fatalf("events missing contract_approval_reset:\n%s", events)
	}
}

func TestContractArtifactsUseRepoRelativePaths(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"contract", "revise", runID, "--goal", "portable contract", "--add-in-scope", "Use repo-relative paths", "--add-validation", "go test ./..."},
		{"contract", "approve", runID},
	} {
		stdout.Reset()
		stderr.Reset()
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	for name, content := range map[string]string{
		"run.json":                    mustReadFile(t, runPaths.RunJSON),
		"context/repo-context.md":     mustReadFile(t, runPaths.RepoContext),
		"context/search-results.json": mustReadFile(t, runPaths.SearchResults),
		"contract/contract.json":      mustReadFile(t, runPaths.ContractJSON),
		"contract/contract.md":        mustReadFile(t, runPaths.ContractMD),
		"contract/prompt.md":          mustReadFile(t, runPaths.PromptMD),
		"contract/approval.json":      mustReadFile(t, runPaths.ApprovalJSON),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
	contractMD := mustReadFile(t, runPaths.ContractMD)
	for _, want := range []string{
		"Repo map: .heurema/pactum/map/repo-map.md",
		"Search results: context/search-results.json",
	} {
		if !strings.Contains(contractMD, want) {
			t.Fatalf("contract.md missing repo-relative path %q:\n%s", want, contractMD)
		}
	}
}

func TestStatusCountsApprovedRunAsActive(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"status", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status --json exited %d, stderr: %s", code, stderr.String())
	}
	var status statusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.Runs.Active != 1 {
		t.Fatalf("status runs.active = %d, want 1: %#v", status.Runs.Active, status)
	}
}

func TestClarifyBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"clarify", "status", "run_x"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("clarify before init output mismatch:\n%s", got)
	}
}

func TestClarifyBeforeInitJSONOutput(t *testing.T) {
	root := t.TempDir()

	for _, args := range [][]string{
		{"clarify", "ask", "run_x", "Question?", "--json"},
		{"clarify", "answer", "run_x", "q_001", "Answer.", "--json"},
		{"clarify", "status", "run_x", "--json"},
	} {
		var stdout, stderr bytes.Buffer
		code := testApp(root).Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
		var response struct {
			Initialized bool   `json:"initialized"`
			Message     string `json:"message"`
		}
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
		if response.Initialized || response.Message != "Pactum is not initialized. Run: pactum init" {
			t.Fatalf("%v unexpected json response: %#v\n%s", args, response, stdout.String())
		}
		if strings.Contains(stdout.String(), "Pactum is not initialized. Run: pactum init\n") && !strings.Contains(stdout.String(), `"message"`) {
			t.Fatalf("%v should not emit plain text guidance for --json:\n%s", args, stdout.String())
		}
	}
}

func TestClarifyAskBlockingQuestionUpdatesArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Should this feature update existing contract artifacts?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Clarification question added",
		"status: clarifying",
		"id: q_001",
		"blocking: true",
		"status: open",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify ask output missing %q:\n%s", want, got)
		}
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	questions := readLines(t, runPaths.QuestionsJSONL)
	if len(questions) != 1 {
		t.Fatalf("questions line count = %d, want 1: %v", len(questions), questions)
	}
	var question clarificationQuestionRecord
	assertNoError(t, json.Unmarshal([]byte(questions[0]), &question))
	if question.ID != "q_001" || question.RunID != runID || !question.Blocking || question.Status != "open" || question.Source != "manual" {
		t.Fatalf("unexpected question record: %#v", question)
	}

	var state contractRunState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.RunJSON)), &state))
	if state.Status != "clarifying" {
		t.Fatalf("run status = %q, want clarifying", state.Status)
	}

	var contract draftContract
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.ContractJSON)), &contract))
	if len(contract.Clarifications.Questions) != 1 || contract.Clarifications.Questions[0].Status != "open" {
		t.Fatalf("contract clarification mismatch: %#v", contract.Clarifications)
	}
	if len(contract.OpenQuestions) != 1 || contract.OpenQuestions[0] != question.Question {
		t.Fatalf("contract open_questions mismatch: %#v", contract.OpenQuestions)
	}
	contractMD := mustReadFile(t, runPaths.ContractMD)
	for _, want := range []string{
		"Manual clarification and approval are available; agent execution is not implemented yet.",
		"## Clarifications",
		"q_001 [blocking] — Should this feature update existing contract artifacts?",
		"Answer: pending",
		"## Open questions\n- Should this feature update existing contract artifacts?",
	} {
		if !strings.Contains(contractMD, want) {
			t.Fatalf("contract.md missing %q:\n%s", want, contractMD)
		}
	}
}

func TestClarifyAnswerQuestionUpdatesArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Should this write to contract.md?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "answer", runID, "q_001", "Yes, update contract artifacts."}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify answer exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Clarification answer recorded",
		"status: contract_draft",
		"id: a_001",
		"question: q_001",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify answer output missing %q:\n%s", want, got)
		}
	}

	var answer clarificationAnswerRecord
	assertNoError(t, json.Unmarshal([]byte(readLines(t, runPaths.AnswersJSONL)[0]), &answer))
	if answer.ID != "a_001" || answer.QuestionID != "q_001" || answer.Answer != "Yes, update contract artifacts." || answer.Source != "manual" {
		t.Fatalf("unexpected answer record: %#v", answer)
	}
	var decision clarificationDecisionRecord
	assertNoError(t, json.Unmarshal([]byte(readLines(t, runPaths.DecisionsJSONL)[0]), &decision))
	if decision.ID != "d_001" || decision.QuestionID != "q_001" || decision.Decision != answer.Answer || decision.Source != "manual_answer" {
		t.Fatalf("unexpected decision record: %#v", decision)
	}
	var state contractRunState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.RunJSON)), &state))
	if state.Status != "contract_draft" {
		t.Fatalf("run status = %q, want contract_draft", state.Status)
	}
	var contract draftContract
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.ContractJSON)), &contract))
	if len(contract.OpenQuestions) != 0 {
		t.Fatalf("open_questions = %#v, want empty", contract.OpenQuestions)
	}
	if got := contract.Clarifications.Questions[0]; got.Status != "answered" || got.Answer != answer.Answer {
		t.Fatalf("contract answer mismatch: %#v", got)
	}
	if got := mustReadFile(t, runPaths.ContractMD); !strings.Contains(got, "Answer: Yes, update contract artifacts.") || !strings.Contains(got, "## Open questions\n- None") {
		t.Fatalf("contract.md missing answer:\n%s", got)
	}
	events := strings.Join(readLines(t, paths.EventsJSONL), "\n")
	for _, want := range []string{"clarification_question_added", "clarification_answer_recorded", "clarification_decision_recorded"} {
		if !strings.Contains(events, want) {
			t.Fatalf("events missing %q:\n%s", want, events)
		}
	}
}

func TestClarifyMultipleQuestionsStatusCounts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"clarify", "ask", runID, "First blocking question?", "--blocking"},
		{"clarify", "ask", runID, "Second blocking question?", "--blocking"},
		{"clarify", "answer", runID, "q_001", "First answer."},
	} {
		stdout.Reset()
		stderr.Reset()
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	lines := readLines(t, runPaths.QuestionsJSONL)
	if len(lines) != 2 {
		t.Fatalf("questions line count = %d, want 2", len(lines))
	}
	var q1, q2 clarificationQuestionRecord
	assertNoError(t, json.Unmarshal([]byte(lines[0]), &q1))
	assertNoError(t, json.Unmarshal([]byte(lines[1]), &q2))
	if q1.ID != "q_001" || q2.ID != "q_002" {
		t.Fatalf("question ids = %q, %q; want q_001, q_002", q1.ID, q2.ID)
	}

	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"clarify", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify status exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"total: 2", "answered: 1", "open: 1", "blocking open: 1", "q_002 [blocking] Second blocking question?"} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify status missing %q:\n%s", want, got)
		}
	}
}

func TestClarifyNonBlockingQuestionKeepsContractDraft(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Optional context?"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	var state contractRunState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(paths.RunsDir, runID, "run.json"))), &state))
	if state.Status != "contract_draft" {
		t.Fatalf("non-blocking clarify status = %q, want contract_draft", state.Status)
	}
}

func TestClarifyStatusJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "ask", runID, "Blocking question?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify ask exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "status", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify status --json exited %d, stderr: %s", code, stderr.String())
	}
	var status clarifyStatusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.RunID != runID || status.RunStatus != "clarifying" || status.Total != 1 || status.Open != 1 || status.BlockingOpen != 1 || len(status.Questions) != 1 {
		t.Fatalf("unexpected clarify status json: %#v", status)
	}
}

func TestClarifyStatusReportsApprovedRun(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "status", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify status exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "status: contract_approved") {
		t.Fatalf("clarify status should preserve approved status:\n%s", got)
	}
}

func TestClarifyStatusJSONReportsApprovedRun(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "status", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify status --json exited %d, stderr: %s", code, stderr.String())
	}
	var status clarifyStatusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.RunID != runID || status.RunStatus != "contract_approved" || status.BlockingOpen != 0 {
		t.Fatalf("unexpected approved clarify status json: %#v", status)
	}
}

func TestClarifyLatestAnswerWinsForDisplay(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"clarify", "ask", runID, "Which answer wins?", "--blocking"},
		{"clarify", "answer", runID, "q_001", "First answer."},
		{"clarify", "answer", runID, "q_001", "Second answer."},
	} {
		stdout.Reset()
		stderr.Reset()
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}
	if lines := readLines(t, runPaths.AnswersJSONL); len(lines) != 2 {
		t.Fatalf("answers should be append-only, got %d lines", len(lines))
	}

	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"clarify", "status", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify status --json exited %d, stderr: %s", code, stderr.String())
	}
	var status clarifyStatusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.Questions[0].Answer != "Second answer." {
		t.Fatalf("latest answer = %q, want Second answer.", status.Questions[0].Answer)
	}
	var contract draftContract
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.ContractJSON)), &contract))
	if contract.Clarifications.Questions[0].Answer != "Second answer." {
		t.Fatalf("contract latest answer = %q", contract.Clarifications.Questions[0].Answer)
	}
	if got := mustReadFile(t, runPaths.ContractMD); !strings.Contains(got, "Answer: Second answer.") || strings.Contains(got, "Answer: First answer.") {
		t.Fatalf("contract.md latest answer mismatch:\n%s", got)
	}
}

func TestClarifyRunNotFoundReturnsError(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"clarify", "status", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("clarify status missing run should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "run not found: run_missing") {
		t.Fatalf("missing run stderr mismatch:\n%s", got)
	}
}

func TestClarifyQuestionNotFoundReturnsError(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "answer", runID, "q_999", "No answer."}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("clarify answer missing question should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "question not found: q_999") {
		t.Fatalf("missing question stderr mismatch:\n%s", got)
	}
}

func TestClarifyArtifactsUseRepoRelativePaths(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"clarify", "ask", runID, "Should paths stay portable?", "--blocking"},
		{"clarify", "answer", runID, "q_001", "Yes, keep them repo-relative."},
	} {
		stdout.Reset()
		stderr.Reset()
		code := app.Run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", args, code, stderr.String())
		}
	}

	for name, content := range map[string]string{
		"questions.jsonl":      mustReadFile(t, runPaths.QuestionsJSONL),
		"answers.jsonl":        mustReadFile(t, runPaths.AnswersJSONL),
		"decisions.jsonl":      mustReadFile(t, runPaths.DecisionsJSONL),
		"run.json":             mustReadFile(t, runPaths.RunJSON),
		"repo-context.md":      mustReadFile(t, runPaths.RepoContext),
		"search-results.json":  mustReadFile(t, runPaths.SearchResults),
		"contract/contract.md": mustReadFile(t, runPaths.ContractMD),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestMapRefreshJSONOutput(t *testing.T) {
	root := t.TempDir()
	app := testAppSequence(root)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"map", "refresh", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh --json exited %d, stderr: %s", code, stderr.String())
	}
	var result MapRefreshResult
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &result))
	if result.RunID == "" || result.RepoRoot != "." || result.SearchIndex != "ready" || result.FilesIndexed == 0 {
		t.Fatalf("unexpected map refresh --json result: %#v", result)
	}
}

func TestMapRefreshRunIDsAreCollisionSafe(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	config := defaultConfigFile()
	config.ProjectMap.MaxFileBytes = 64
	assertNoError(t, writeYAML(paths.Config, config))
	mustWriteFile(t, filepath.Join(root, "small.go"), "package small\n")
	mustWriteFile(t, filepath.Join(root, "large.go"), "package large\n"+strings.Repeat("x", 128))
	mustWriteFile(t, filepath.Join(root, "node_modules", "ignored.js"), "console.log('ignored')\n")

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	firstManifest, err := readWorkspaceManifest(paths.Manifest)
	assertNoError(t, err)
	firstRunID := firstManifest.Map.CurrentRunID
	if firstRunID != "map_20260531_184012" {
		t.Fatalf("first run id = %q, want map_20260531_184012", firstRunID)
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	secondManifest, err := readWorkspaceManifest(paths.Manifest)
	assertNoError(t, err)
	secondRunID := secondManifest.Map.CurrentRunID
	if secondRunID != "map_20260531_184012_02" {
		t.Fatalf("second run id = %q, want map_20260531_184012_02", secondRunID)
	}
	if firstRunID == secondRunID {
		t.Fatalf("run ids should differ: %q", firstRunID)
	}

	firstRunPath := filepath.Join(paths.MapRunsDir, firstRunID+".json")
	secondRunPath := filepath.Join(paths.MapRunsDir, secondRunID+".json")
	assertFile(t, firstRunPath)
	assertFile(t, secondRunPath)

	var secondRun MapRefreshResult
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, secondRunPath)), &secondRun))
	if secondRun.RunID != secondRunID {
		t.Fatalf("run artifact run_id = %q, want %q", secondRun.RunID, secondRunID)
	}
	if secondRun.RepoRoot != "." {
		t.Fatalf("run artifact repo_root = %q, want .", secondRun.RepoRoot)
	}
	if secondRun.FilesIgnored == 0 {
		t.Fatalf("run artifact files_ignored = 0, want non-zero")
	}
	if secondRun.FilesSkipped != 1 {
		t.Fatalf("run artifact files_skipped = %d, want 1", secondRun.FilesSkipped)
	}

	var raw map[string]json.RawMessage
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, secondRunPath)), &raw))
	for _, key := range []string{"repo_root", "files_ignored", "files_skipped"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("run artifact missing key %q: %s", key, mustReadFile(t, secondRunPath))
		}
	}
}

func TestMapRefreshCommandRebuildsMapOnly(t *testing.T) {
	root := t.TempDir()
	app := testAppSequence(root)
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	paths := artifacts.New(root)
	configBefore := mustReadFile(t, paths.Config)
	mustWriteFile(t, paths.ProjectMemory, "# Project Memory\n\nKeep this.\n")
	mustWriteFile(t, filepath.Join(paths.RunsDir, "keep.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\nchanged\n")

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"map", "refresh", "--full"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Project map refreshed",
		"Run:",
		"files indexed:",
		"files ignored:",
		"files skipped:",
		"code items:",
		"warnings:",
		"search index: ready",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("map refresh output missing %q:\n%s", want, got)
		}
	}
	if mustReadFile(t, paths.Config) != configBefore {
		t.Fatalf("map refresh should not rewrite config.yaml")
	}
	if got := mustReadFile(t, paths.ProjectMemory); !strings.Contains(got, "Keep this.") {
		t.Fatalf("map refresh should not rewrite memory artifacts:\n%s", got)
	}
	assertFile(t, filepath.Join(paths.RunsDir, "keep.json"))

	workspaceManifest, err := readWorkspaceManifest(paths.Manifest)
	assertNoError(t, err)
	mapManifest, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)
	if workspaceManifest.Map.CurrentRunID != mapManifest.RunID {
		t.Fatalf("workspace current map run = %q, want %q", workspaceManifest.Map.CurrentRunID, mapManifest.RunID)
	}
	runPath := filepath.Join(paths.MapRunsDir, mapManifest.RunID+".json")
	assertFile(t, runPath)
	var run MapRefreshResult
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPath)), &run))
	if run.RunID != mapManifest.RunID || run.StartedAt.IsZero() || run.FinishedAt.IsZero() {
		t.Fatalf("map run artifact incomplete: %#v", run)
	}

	events := readLines(t, paths.EventsJSONL)
	if len(events) != 10 {
		t.Fatalf("events line count = %d, want 10: %v", len(events), events)
	}
	for i, want := range []string{"map_refresh_started", "search_index_started", "search_index_finished", "map_refresh_finished"} {
		if !strings.Contains(events[len(events)-4+i], want) {
			t.Fatalf("event %d = %s, want %s", len(events)-4+i, events[len(events)-4+i], want)
		}
	}
}

func TestMapRefreshRequiresInitializedWorkspace(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("map refresh before init should fail")
	}
	if got := stderr.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("map refresh before init stderr mismatch:\n%s", got)
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
	if got := stdout.String(); !strings.Contains(got, "Search index is missing. Run: pactum map refresh") {
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

type fixedAgentRegistry struct {
	defaultExecutor string
	defaultReviewer string
	order           []string
	descriptors     map[string]agents.AgentDescriptor
}

func testAgentRegistry(extra ...agents.AgentDescriptor) agents.Registry {
	descriptors := map[string]agents.AgentDescriptor{}
	order := []string{}
	for _, descriptor := range agents.ListBuiltins() {
		descriptors[descriptor.Name] = descriptor
		order = append(order, descriptor.Name)
	}
	for _, descriptor := range extra {
		if _, ok := descriptors[descriptor.Name]; !ok {
			order = append(order, descriptor.Name)
		}
		descriptors[descriptor.Name] = descriptor
	}
	return fixedAgentRegistry{
		defaultExecutor: agents.DefaultExecutor(),
		defaultReviewer: agents.DefaultReviewer(),
		order:           order,
		descriptors:     descriptors,
	}
}

func helperAgentDescriptor(name string) agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    name,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecutionHelperProcess"},
		Input:   agents.InputPromptFile,
	}
}

func (r fixedAgentRegistry) DefaultExecutor() string {
	return r.defaultExecutor
}

func (r fixedAgentRegistry) DefaultReviewer() string {
	return r.defaultReviewer
}

func (r fixedAgentRegistry) ResolveExecutor(name string) (agents.AgentDescriptor, error) {
	return r.resolve(name, r.defaultExecutor)
}

func (r fixedAgentRegistry) ResolveReviewer(name string) (agents.AgentDescriptor, error) {
	return r.resolve(name, r.defaultReviewer)
}

func (r fixedAgentRegistry) ListBuiltins() []agents.AgentDescriptor {
	descriptors := make([]agents.AgentDescriptor, 0, len(r.order))
	for _, name := range r.order {
		descriptors = append(descriptors, cloneTestAgentDescriptor(r.descriptors[name]))
	}
	return descriptors
}

func (r fixedAgentRegistry) resolve(name string, defaultName string) (agents.AgentDescriptor, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultName
	}
	descriptor, ok := r.descriptors[name]
	if !ok {
		return agents.AgentDescriptor{}, fmt.Errorf("unsupported agent: %s", name)
	}
	return cloneTestAgentDescriptor(descriptor), nil
}

func cloneTestAgentDescriptor(descriptor agents.AgentDescriptor) agents.AgentDescriptor {
	descriptor.Args = append([]string{}, descriptor.Args...)
	return descriptor
}

func testAppSequence(root string) App {
	now := time.Date(2026, 5, 31, 18, 40, 11, 0, time.UTC)
	return App{
		WorkingDir: root,
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
	}
}

func setupContractRun(t *testing.T, root string) (App, artifacts.Paths, string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"run", "add sqlite cache", "--contract-only"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}
	return app, artifacts.New(root), "run_20260531_184012"
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

func readRunState(t *testing.T, path string) contractRunState {
	t.Helper()
	var state contractRunState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &state))
	return state
}

func readContractDraft(t *testing.T, path string) draftContract {
	t.Helper()
	var contract draftContract
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &contract))
	return contract
}

func readApproval(t *testing.T, path string) approvalState {
	t.Helper()
	var approval approvalState
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &approval))
	return approval
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

func assertDoesNotContainRoot(t *testing.T, name string, content string, root string) {
	t.Helper()
	for _, forbidden := range []string{
		root,
		filepath.ToSlash(root),
		strings.ReplaceAll(root, `\`, `\\`),
	} {
		if forbidden != "" && strings.Contains(content, forbidden) {
			t.Fatalf("%s contains absolute repo root %q:\n%s", name, forbidden, content)
		}
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
