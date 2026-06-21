package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/projectmap"
	searchpkg "github.com/heurema/pactum/internal/search"
	"gopkg.in/yaml.v3"
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
		paths.HashesJSONL,
		paths.SearchSQLite,
		paths.WikiOverview,
		paths.WikiStructure,
		paths.WikiCommands,
		paths.WikiEntrypoints,
		paths.WikiConfig,
		paths.WikiTests,
		filepath.Join(paths.MapRunsDir, "map_20260531_184012.json"),
		paths.ProjectMemory,
		paths.MemoryItems,
		paths.StaleReport,
		paths.EventsJSONL,
		paths.UsageJSONL,
		paths.CostJSON,
	} {
		assertFile(t, file)
	}
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	if config.Version != "v1alpha1" {
		t.Fatalf("config version = %q, want v1alpha1", config.Version)
	}
	if config.Map.MaxFileBytes != 500000 {
		t.Fatalf("config map.max_file_bytes = %d, want 500000", config.Map.MaxFileBytes)
	}
	if config.OutOfScope != gateScopeEnforcementBlock {
		t.Fatalf("config out_of_scope = %q, want block", config.OutOfScope)
	}
	if len(config.Agents) != 1 || config.Agents[0].Name != "claude" || config.Agents[0].Model != "claude-opus-4-8" {
		t.Fatalf("config agents should register the single pinned claude entry: %#v", config.Agents)
	}
	configYAML := mustReadFile(t, paths.Config)
	for _, want := range []string{"agents:", "- name: claude", "model: claude-opus-4-8", "map:", "out_of_scope: block", "pipeline:"} {
		if !strings.Contains(configYAML, want) {
			t.Fatalf("config.yaml missing %q:\n%s", want, configYAML)
		}
	}
	for _, forbidden := range []string{"execute:", "models:", "adapters:", "default_executor:", "default_reviewer:", "include_go_ast", "tree_sitter", "tree_sitter_languages", "entrypoints", "default_profile:", "project_map:", "limits:", "memory:", "max_usd", "budget:", "max_tokens:"} {
		if strings.Contains(configYAML, forbidden) {
			t.Fatalf("config.yaml should not contain %q:\n%s", forbidden, configYAML)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.MapDir, "entries.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("entries.jsonl should not be generated as a primary artifact")
	}
	gitignore := mustReadFile(t, paths.Gitignore)
	// Ignore regenerable artifacts and raw command transcripts.
	for _, want := range []string{
		"map/",
		"cache/",
		"tmp/",
		"locks/",
		"runs/*/context/",
		"*.log",
	} {
		if !strings.Contains(gitignore, want) {
			t.Fatalf(".gitignore missing %q:\n%s", want, gitignore)
		}
	}
	// The durable run record is versioned: the ledger (audit timeline), the
	// decision/verdict records, and run inputs are NOT ignored. Only *.log
	// transcripts under execute/review/gate are ignored, not the whole dirs.
	for _, forbidden := range []string{
		"ledger/",
		"runs/*/execute/",
		"runs/*/review/",
		"runs/*/contract/",
		"runs/*/clarify/",
		"runs/*/task.md",
		"contract/*.json",
	} {
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
	if _, err := os.Stat(paths.CodeItemsJSONL); !os.IsNotExist(err) {
		t.Fatalf("code-items.jsonl should not be created by init (tree-sitter removed)")
	}

	repoMap := mustReadFile(t, paths.RepoMap)
	for _, want := range []string{
		"# Pactum Project Map",
		"Repository root: `.`",
		"## Summary",
		"## How to navigate this map",
		"## Wiki pages",
		"`wiki/overview.md`",
		"`wiki/structure.md`",
		"## Project map artifacts",
		"Source files remain the source of truth.",
		"## Agent guidance",
		"Before adding new code, search/read relevant files.",
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
	if strings.Contains(repoMap, "code_item") || strings.Contains(repoMap, "code-items") {
		t.Fatalf("repo-map.md must not mention code items (tree-sitter removed):\n%s", repoMap)
	}
	llms := mustReadFile(t, paths.LLMS)
	for _, want := range []string{
		"deterministic Pactum project map",
		"map/wiki/overview.md",
		"map/wiki/structure.md",
		"map/wiki/commands.md",
		"map/wiki/entrypoints.md",
		"map/wiki/config.md",
		"map/wiki/tests.md",
		"Source files remain the source of truth.",
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
	if strings.Contains(llms, "code-items.jsonl") || strings.Contains(llms, "code_item") {
		t.Fatalf("llms.txt must not mention code items (tree-sitter removed):\n%s", llms)
	}
	manifest, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)
	if manifest.RepoRoot != "." {
		t.Fatalf("map manifest repo_root = %q, want .", manifest.RepoRoot)
	}
	if manifest.Artifacts["code_items"] != "" {
		t.Fatalf("manifest must not list a code_items artifact (tree-sitter removed): %q", manifest.Artifacts["code_items"])
	}
	if manifest.Artifacts["search"] != "map/search.sqlite" {
		t.Fatalf("manifest search artifact = %q", manifest.Artifacts["search"])
	}
	if manifest.Artifacts["entries"] != "" {
		t.Fatalf("manifest should not point to entries artifact: %#v", manifest.Artifacts)
	}
	for key, want := range map[string]string{
		"wiki_overview":    "map/wiki/overview.md",
		"wiki_structure":   "map/wiki/structure.md",
		"wiki_commands":    "map/wiki/commands.md",
		"wiki_entrypoints": "map/wiki/entrypoints.md",
		"wiki_config":      "map/wiki/config.md",
		"wiki_tests":       "map/wiki/tests.md",
		"wiki_areas":       "map/wiki/areas/",
	} {
		if manifest.Artifacts[key] != want {
			t.Fatalf("manifest %s artifact = %q, want %q", key, manifest.Artifacts[key], want)
		}
	}
	if manifest.ConfigHash == "" {
		t.Fatalf("manifest config_hash should be populated")
	}
	if manifest.ConfigHashScope != "map" {
		t.Fatalf("manifest config_hash_scope = %q, want map", manifest.ConfigHashScope)
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

func TestReadConfigNormalizesOutOfScope(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing defaults to block",
			content: "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\n",
			want:    gateScopeEnforcementBlock,
		},
		{
			name:    "block",
			content: "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\nout_of_scope: block\n",
			want:    gateScopeEnforcementBlock,
		},
		{
			name:    "warn",
			content: "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\nout_of_scope: warn\n",
			want:    gateScopeEnforcementWarn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mustWriteFile(t, paths.Config, tt.content)
			config, err := readConfig(paths.Config)
			assertNoError(t, err)
			if config.OutOfScope != tt.want {
				t.Fatalf("out_of_scope = %q, want %q", config.OutOfScope, tt.want)
			}
		})
	}
}

func TestReadConfigRejectsInvalidOutOfScope(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	mustWriteFile(t, paths.Config, "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\nout_of_scope: advisory\n")

	_, err := readConfig(paths.Config)
	if err == nil {
		t.Fatalf("readConfig should reject invalid out_of_scope")
	}
	if !strings.Contains(err.Error(), "out_of_scope") {
		t.Fatalf("invalid out_of_scope error mismatch: %v", err)
	}
}

// TestReadConfigRejectsLegacyReviewBudget pins that the old review.budget block
// is rejected as an unknown key (review: is no longer a known top-level key).
func TestReadConfigRejectsLegacyReviewBudget(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	mustWriteFile(t, paths.Config, "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\nreview:\n  budget:\n    mode: block\n")

	_, err := readConfig(paths.Config)
	if err == nil {
		t.Fatal("readConfig should reject the legacy review key")
	}
	// The strict decoder reports the unknown top-level key "review".
	if !strings.Contains(err.Error(), "review") {
		t.Fatalf("error should mention the unknown key: %v", err)
	}
}

func TestInitUsesConfigMaxFileBytesAndManifestWarnings(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	config := defaultConfigFile()
	config.Map.MaxFileBytes = 64
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
	if !strings.Contains(strings.Join(manifest.Warnings, "\n"), "skipped large file: large.go") {
		t.Fatalf("manifest warnings did not mention skipped large.go: %#v", manifest.Warnings)
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
	config.Map.MaxFileBytes = 123
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
		"map config changed: .heurema/pactum/config.yaml",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status missing %q:\n%s", want, got)
		}
	}
}

// TestMapStalenessPinsOnlyMapSection pins that the map staleness check narrows
// to the map: config section: editing a non-map section (here the agents
// registry) keeps the map fresh, while editing map.max_file_bytes makes it
// stale.
func TestMapStalenessPinsOnlyMapSection(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	paths := artifacts.New(root)

	// Editing a non-map section must not invalidate the map.
	config, err := readConfig(paths.Config)
	assertNoError(t, err)
	config.Agents = append(config.Agents, agentRegistryEntry{Name: "codex", Model: "gpt-5"})
	config.Pipeline.CodeReview.By = stageBy{"codex"}
	assertNoError(t, writeYAML(paths.Config, config))

	statusReasons := func() []string {
		var out, errOut bytes.Buffer
		if code := testApp(root).Run([]string{"status", "--json"}, &out, &errOut); code != 0 {
			t.Fatalf("status --json exited %d, stderr: %s", code, errOut.String())
		}
		var response statusResponse
		assertNoError(t, json.Unmarshal(out.Bytes(), &response))
		return response.ProjectMap.StaleReasons
	}
	for _, reason := range statusReasons() {
		if strings.Contains(reason, "config") {
			t.Fatalf("editing a non-map section must not make the map stale: %v", reason)
		}
	}

	// Editing a map parameter must invalidate the map.
	config, err = readConfig(paths.Config)
	assertNoError(t, err)
	config.Map.MaxFileBytes = 4096
	assertNoError(t, writeYAML(paths.Config, config))
	found := false
	for _, reason := range statusReasons() {
		if strings.Contains(reason, "map config changed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("editing map.max_file_bytes should make the map stale: %v", statusReasons())
	}
}

// TestLegacyMapManifestStaleOnceThenMigrates pins that a manifest without the
// config_hash_scope marker (the legacy whole-file pin) is reported stale once,
// and the next map refresh writes the map-section hash plus the scope marker so
// status is fresh again.
func TestLegacyMapManifestStaleOnceThenMigrates(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	paths := artifacts.New(root)

	// Simulate a manifest written before the scope marker: a whole-file pin with
	// no config_hash_scope.
	manifest, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)
	manifest.ConfigHashScope = ""
	manifest.ConfigHash = "legacy-whole-file-hash"
	assertNoError(t, writeJSON(paths.MapManifest, manifest))

	var out, errOut bytes.Buffer
	if code := testApp(root).Run([]string{"status", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("status --json exited %d, stderr: %s", code, errOut.String())
	}
	var response statusResponse
	assertNoError(t, json.Unmarshal(out.Bytes(), &response))
	if response.ProjectMap.Status != "stale" {
		t.Fatalf("legacy manifest should be stale: %#v", response.ProjectMap)
	}
	legacyReported := false
	for _, reason := range response.ProjectMap.StaleReasons {
		if strings.Contains(reason, "map config pin format changed") {
			legacyReported = true
		}
	}
	if !legacyReported {
		t.Fatalf("legacy manifest should report the pin-format change once: %#v", response.ProjectMap.StaleReasons)
	}

	// A refresh migrates the manifest: map-section hash + scope marker.
	out.Reset()
	errOut.Reset()
	if code := testApp(root).Run([]string{"map", "refresh"}, &out, &errOut); code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, errOut.String())
	}
	migrated, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)
	if migrated.ConfigHashScope != "map" {
		t.Fatalf("refresh should write config_hash_scope=map, got %q", migrated.ConfigHashScope)
	}

	out.Reset()
	errOut.Reset()
	if code := testApp(root).Run([]string{"status", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("status --json exited %d, stderr: %s", code, errOut.String())
	}
	var afterRefresh statusResponse
	assertNoError(t, json.Unmarshal(out.Bytes(), &afterRefresh))
	for _, reason := range afterRefresh.ProjectMap.StaleReasons {
		if strings.Contains(reason, "config") {
			t.Fatalf("migrated manifest should not be config-stale: %v", reason)
		}
	}
	if afterRefresh.ProjectMap.Status != "fresh" {
		t.Fatalf("migrated manifest must be fresh, got %q (reasons: %v)", afterRefresh.ProjectMap.Status, afterRefresh.ProjectMap.StaleReasons)
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
	code := testApp(root).Run([]string{"task", "new", "add sqlite cache"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("task new before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("task new before init stderr mismatch:\n%s", got)
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
	code = app.Run([]string{"task", "new", task}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Run created",
		"id: run_20260531_184012",
		"status: contract_draft",
		"task: add sqlite cache",
		"current: yes",
		"pactum contract approve",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("task new output missing %q:\n%s", want, got)
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
	if state.Schema != "pactum.run.v1alpha1" || state.RunID != "run_20260531_184012" || state.Status != "contract_draft" {
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
	if contract.Schema != "pactum.contract.v1alpha1" || contract.RunID != "run_20260531_184012" || contract.Goal != task {
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
		!strings.Contains(contractMD, "Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.") ||
		!strings.Contains(contractMD, "Repo map: .heurema/pactum/map/repo-map.md") ||
		!strings.Contains(contractMD, "Search results: context/search-results.json") ||
		!strings.Contains(contractMD, "## Clarifications\n- None") ||
		!strings.Contains(contractMD, "## Validation commands\nTBD") ||
		!strings.Contains(contractMD, "## Assumptions\nTBD") ||
		!strings.Contains(contractMD, "## Open questions\n- None") {
		t.Fatalf("contract.md content mismatch:\n%s", contractMD)
	}
	for _, stale := range []string{"Clarification loop is not implemented yet.", "Clarification and agent execution are not implemented yet.", "not implemented yet"} {
		if strings.Contains(contractMD, stale) {
			t.Fatalf("contract.md contains stale wording %q:\n%s", stale, contractMD)
		}
	}
	promptMD := mustReadFile(t, filepath.Join(runDir, "contract", "prompt.md"))
	if !strings.Contains(promptMD, "This is a contract-draft placeholder. Run `pactum prompt build` after the contract is approved to build the executor prompt for `pactum execute`.") ||
		!strings.Contains(promptMD, "## Goal\n"+task) ||
		!strings.Contains(promptMD, "## Validation commands\nTBD") {
		t.Fatalf("prompt.md content mismatch:\n%s", promptMD)
	}
	for _, stale := range []string{"Contract clarification and approval are not implemented.", "not implemented yet", "not executable yet"} {
		if strings.Contains(promptMD, stale) {
			t.Fatalf("prompt.md contains stale wording %q:\n%s", stale, promptMD)
		}
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
		if !strings.Contains(event, want) || !strings.Contains(event, "run_20260531_184012") {
			t.Fatalf("event %d = %s, want %s with run", len(events)-2+i, event, want)
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
	code = app.Run([]string{"task", "new", "task"}, &stdout, &stderr)
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
	code = app.Run([]string{"task", "new", task}, &stdout, &stderr)
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
	code = app.Run([]string{"task", "new", "add sqlite cache", "--json"}, &stdout, &stderr)
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
	code = app.Run([]string{"task", "new", "first task"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("first run exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"task", "new", "second task"}, &stdout, &stderr)
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
			code := app.Run([]string{"task", "new", fmt.Sprintf("task %d", i)}, &stdout, &stderr)
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
	code = app.Run([]string{"task", "new", "add sqlite cache"}, &stdout, &stderr)
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
	code = app.Run([]string{"task", "new", "add sqlite cache"}, &stdout, &stderr)
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

	// Read-only commands stay exit 0 with a stdout notice.
	for _, args := range [][]string{
		{"contract", "show", "run_x"},
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

	// Mutating commands exit 1 with a stderr error.
	for _, args := range [][]string{
		{"contract", "revise", "run_x", "--from", "-"},
		{"contract", "approve", "run_x"},
	} {
		var stdout, stderr bytes.Buffer
		code := testApp(root).Run(args, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("%v exited %d, want 1, stderr: %s", args, code, stderr.String())
		}
		if got := stderr.String(); !strings.Contains(got, "not initialized") {
			t.Fatalf("%v stderr mismatch:\n%s", args, got)
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

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{"goal": "add sqlite-backed cache"})
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile}, &stdout, &stderr)
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

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"scope": map[string]any{
			"in":  []string{"Add show, revise, and approve commands", "Persist cache metadata", "Expose status summary"},
			"out": []string{"Implement cache eviction"},
		},
		"paths_in_scope":      []string{"internal/app/**", "cmd/pactum/*.go"},
		"paths_out_of_scope":  []string{"docs/**"},
		"acceptance_criteria": []string{"Cache survives process restart"},
		"validation":          map[string]any{"commands": []string{"go test ./..."}},
		"assumptions":         []string{"SQLite is available through the standard driver"},
	})
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile}, &stdout, &stderr)
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
	if len(contract.PathsInScope) != 2 || contract.PathsInScope[0] != "internal/app/**" || contract.PathsInScope[1] != "cmd/pactum/*.go" {
		t.Fatalf("contract path in-scope mismatch: %#v", contract.PathsInScope)
	}
	if len(contract.PathsOutOfScope) != 1 || contract.PathsOutOfScope[0] != "docs/**" {
		t.Fatalf("contract path out-of-scope mismatch: %#v", contract.PathsOutOfScope)
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
		"## Paths in scope\n- internal/app/**\n- cmd/pactum/*.go",
		"## Paths out of scope\n- docs/**",
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
		"## Paths in scope\n- internal/app/**\n- cmd/pactum/*.go",
		"## Paths out of scope\n- docs/**",
		"## Acceptance criteria\n- Cache survives process restart",
		"## Validation commands\n- go test ./...",
	} {
		if !strings.Contains(promptMD, want) {
			t.Fatalf("prompt.md missing %q:\n%s", want, promptMD)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show exited %d, stderr: %s", code, stderr.String())
	}
	for _, want := range []string{
		"## Paths in scope\n- internal/app/**\n- cmd/pactum/*.go",
		"## Paths out of scope\n- docs/**",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("contract show missing %q:\n%s", want, stdout.String())
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
	if got := stderr.String(); !strings.Contains(got, "--from") {
		t.Fatalf("contract revise stderr mismatch:\n%s", got)
	}
}

func TestContractReviseInputErrors(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	tests := []struct {
		name     string
		input    string
		wantCode string
	}{
		{"invalid JSON", `not-json`, "INVALID_JSON"},
		{"missing base_version", `{"contract":{}}`, "MISSING_BASE_VERSION"},
		{"unknown outer field", `{"base_version":"x","badfield":"x","contract":{}}`, "UNKNOWN_FIELD"},
		{"null goal", `{"base_version":"x","contract":{"goal":null}}`, "NULL_NOT_ALLOWED"},
		{"unknown contract field", `{"base_version":"x","contract":{"goall":"x"}}`, "UNKNOWN_FIELD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.json")
			if err := os.WriteFile(path, []byte(tt.input), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := app.Run([]string{"contract", "revise", runID, "--from", path, "--json"}, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("expected non-zero exit, stdout: %s", stdout.String())
			}
			var failure contractReviseFailure
			if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
				t.Fatalf("expected structured JSON failure, got: %s", stdout.String())
			}
			if failure.OK || !failure.ContractUnchanged || len(failure.Issues) == 0 {
				t.Fatalf("unexpected failure shape: %#v", failure)
			}
			found := false
			for _, issue := range failure.Issues {
				if issue.Code == tt.wantCode {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("want issue code %q in %#v", tt.wantCode, failure.Issues)
			}
		})
	}
}

func TestContractReviseStaleVersion(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	path := filepath.Join(t.TempDir(), "revise.json")
	if err := os.WriteFile(path, []byte(`{"base_version":"not-the-current-hash","contract":{"goal":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", path, "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("stale version should fail")
	}
	var failure contractReviseFailure
	if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
		t.Fatalf("expected structured JSON failure: %s", stdout.String())
	}
	if failure.OK || !failure.ContractUnchanged {
		t.Fatalf("stale version: ok=%v unchanged=%v", failure.OK, failure.ContractUnchanged)
	}
	if len(failure.Issues) != 1 || failure.Issues[0].Code != "STALE_VERSION" {
		t.Fatalf("want STALE_VERSION, got %#v", failure.Issues)
	}
}

func TestContractReviseApprovalResetRejected(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{"goal": "changed goal"})
	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile, "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("revise approved contract without flag should fail")
	}
	var failure contractReviseFailure
	if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
		t.Fatalf("expected structured JSON failure: %s", stdout.String())
	}
	if failure.OK || !failure.ContractUnchanged {
		t.Fatalf("unexpected: ok=%v unchanged=%v", failure.OK, failure.ContractUnchanged)
	}
	if len(failure.Issues) != 1 || failure.Issues[0].Code != "APPROVAL_RESET_REQUIRED" {
		t.Fatalf("want APPROVAL_RESET_REQUIRED, got %#v", failure.Issues)
	}
}

func TestContractReviseNoOp(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	// Submitting the same goal that is already set produces a no-op.
	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{"goal": "add sqlite cache"})
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("no-op revise exited %d, stderr: %s stdout: %s", code, stderr.String(), stdout.String())
	}
	var resp contractReviseResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("expected JSON response: %s", stdout.String())
	}
	if resp.Changed {
		t.Fatalf("no-op revise: changed = true, want false")
	}
	if resp.NewVersion != resp.BaseVersion {
		t.Fatalf("no-op: new_version %q != base_version %q", resp.NewVersion, resp.BaseVersion)
	}
}

func TestContractReviseApprovedNoOp(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	approval := readApproval(t, runPaths.ApprovalJSON)

	// No-op revise on an approved contract must not change approval state.
	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{"goal": "add sqlite cache"})
	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("no-op revise on approved contract exited %d, stderr: %s stdout: %s", code, stderr.String(), stdout.String())
	}
	var resp contractReviseResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("expected JSON response: %s", stdout.String())
	}
	if resp.Changed {
		t.Fatalf("no-op on approved contract: changed = true")
	}
	after := readApproval(t, runPaths.ApprovalJSON)
	if after.Status != "approved" || after.ContractSHA256 == nil || *after.ContractSHA256 != *approval.ContractSHA256 {
		t.Fatalf("approval should remain unchanged after no-op: before=%#v after=%#v", approval, after)
	}
}

func TestContractReviseDedup(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"scope": map[string]any{"in": []string{"A", "B", "A"}},
	})
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("revise with dups exited %d, stderr: %s stdout: %s", code, stderr.String(), stdout.String())
	}
	var resp contractReviseResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("expected JSON response: %s", stdout.String())
	}
	if len(resp.Deduped) != 1 || !strings.Contains(resp.Deduped[0], "A") {
		t.Fatalf("deduped = %v, want one entry for A", resp.Deduped)
	}
	if len(resp.Contract.Scope.In) != 2 || resp.Contract.Scope.In[0] != "A" || resp.Contract.Scope.In[1] != "B" {
		t.Fatalf("scope.in = %v, want [A B]", resp.Contract.Scope.In)
	}
}

func TestContractReviseNoOpWithDups(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	// Establish scope.in = ["A"].
	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"scope": map[string]any{"in": []string{"A"}},
	})
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"contract", "revise", runID, "--from", fromFile}, &stdout, &stderr); code != 0 {
		t.Fatalf("first revise exited %d, stderr: %s", code, stderr.String())
	}

	// Submitting ["A","A"] deduplicates to ["A"], which equals the current state.
	fromFile2 := writeReviseDocForTest(t, runPaths, map[string]any{
		"scope": map[string]any{"in": []string{"A", "A"}},
	})
	stdout.Reset()
	stderr.Reset()
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile2, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("dedup no-op revise exited %d, stderr: %s stdout: %s", code, stderr.String(), stdout.String())
	}
	var resp contractReviseResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("expected JSON response: %s", stdout.String())
	}
	if resp.Changed {
		t.Fatalf("dedup normalizes to no-op: changed = true")
	}
	if len(resp.Deduped) == 0 {
		t.Fatalf("dedup no-op: Deduped empty, want dropped duplicate reported")
	}
}

func TestContractReviseStdin(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	version, err := storeFileSHA256(runPaths.ContractJSON)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{
		"base_version": version,
		"contract":     map[string]any{"goal": "stdin goal"},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		r.Close()
	})
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	w.Close()

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", "-"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise from stdin exited %d, stderr: %s stdout: %s", code, stderr.String(), stdout.String())
	}
	contract := readContractDraft(t, runPaths.ContractJSON)
	if contract.Goal != "stdin goal" {
		t.Fatalf("contract goal = %q, want stdin goal", contract.Goal)
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
	code := app.Run([]string{"clarify", "add", runID, "Should approval wait?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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
	code := app.Run([]string{"clarify", "add", runID, "Optional context?"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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
	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"scope": map[string]any{"in": []string{"Use sqlite"}},
	})
	code = app.Run([]string{"contract", "revise", runID, "--from", fromFile, "--allow-approval-reset"}, &stdout, &stderr)
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
	code = app.Run([]string{"clarify", "add", runID, "Need approval reset?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{
		"goal":       "portable contract",
		"scope":      map[string]any{"in": []string{"Use repo-relative paths"}},
		"validation": map[string]any{"commands": []string{"go test ./..."}},
	})
	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"contract", "revise", runID, "--from", fromFile},
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
	code := testApp(root).Run([]string{"clarify", "show", "run_x"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("clarify before init output mismatch:\n%s", got)
	}
}

func TestClarifyBeforeInitJSONOutput(t *testing.T) {
	root := t.TempDir()

	// Read-only --json before init: exit 0 with the structured not-initialized
	// status document (no plain text).
	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"clarify", "show", "run_x", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify show --json before init exited %d, stderr: %s", code, stderr.String())
	}
	var response struct {
		Initialized bool   `json:"initialized"`
		Message     string `json:"message"`
	}
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Initialized || response.Message != "Pactum is not initialized. Run: pactum init" {
		t.Fatalf("unexpected json response: %#v\n%s", response, stdout.String())
	}

	// Mutating --json before init: exit 1 with a JSON error envelope on stdout
	// (stderr empty), never plain text.
	for _, args := range [][]string{
		{"clarify", "add", "run_x", "Question?", "--json"},
		{"clarify", "answer", "run_x", "q_001", "Answer.", "--json"},
	} {
		var stdout, stderr bytes.Buffer
		code := testApp(root).Run(args, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("%v exited %d, want 1, stderr: %s", args, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v wrote to stderr in --json mode:\n%s", args, stderr.String())
		}
		var envelope errorEnvelope
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
		if envelope.Schema != errorSchema || envelope.Error.Code != "not_initialized" {
			t.Fatalf("%v unexpected error envelope: %#v\n%s", args, envelope, stdout.String())
		}
	}
}

func TestClarifyAskBlockingQuestionUpdatesArtifacts(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "add", runID, "Should this feature update existing contract artifacts?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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
			t.Fatalf("clarify add output missing %q:\n%s", want, got)
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
		"Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.",
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
	code := app.Run([]string{"clarify", "add", runID, "Should this write to contract.md?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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
		{"clarify", "add", runID, "First blocking question?", "--blocking"},
		{"clarify", "add", runID, "Second blocking question?", "--blocking"},
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
	code := app.Run([]string{"clarify", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"total: 2", "answered: 1", "open: 1", "blocking open: 1", "q_002 [blocking] Second blocking question?"} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify show missing %q:\n%s", want, got)
		}
	}
}

func TestClarifyNonBlockingQuestionKeepsContractDraft(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "add", runID, "Optional context?"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
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
	code := app.Run([]string{"clarify", "add", runID, "Blocking question?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify show --json exited %d, stderr: %s", code, stderr.String())
	}
	var status clarifyStatusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.RunID != runID || status.RunStatus != "clarifying" || status.Total != 1 || status.Open != 1 || status.BlockingOpen != 1 || len(status.Questions) != 1 {
		t.Fatalf("unexpected clarify show json: %#v", status)
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
	code = app.Run([]string{"clarify", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify show exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "status: contract_approved") {
		t.Fatalf("clarify show should preserve approved status:\n%s", got)
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
	code = app.Run([]string{"clarify", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify show --json exited %d, stderr: %s", code, stderr.String())
	}
	var status clarifyStatusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.RunID != runID || status.RunStatus != "contract_approved" || status.BlockingOpen != 0 {
		t.Fatalf("unexpected approved clarify show json: %#v", status)
	}
}

func TestClarifyLatestAnswerWinsForDisplay(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	for _, args := range [][]string{
		{"clarify", "add", runID, "Which answer wins?", "--blocking"},
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
	code := app.Run([]string{"clarify", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify show --json exited %d, stderr: %s", code, stderr.String())
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
	code = testApp(root).Run([]string{"clarify", "show", "run_missing"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("clarify show missing run should fail")
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
		{"clarify", "add", runID, "Should paths stay portable?", "--blocking"},
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
	var result mapRefreshResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &result))
	if result.RunID == "" || result.RepoRoot != "." || result.SearchIndex != "ready" || result.FilesIndexed == 0 {
		t.Fatalf("unexpected map refresh --json result: %#v", result)
	}
	if result.Next == nil || len(result.Next) != 0 {
		t.Fatalf("map refresh next = %v, want the explicit empty array", result.Next)
	}
}

func TestMapRefreshRunIDsAreCollisionSafe(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))
	config := defaultConfigFile()
	config.Map.MaxFileBytes = 64
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
	code = app.Run([]string{"map", "refresh"}, &stdout, &stderr)
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
	if code != 1 {
		t.Fatalf("map refresh before init exited %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
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

func TestSearchFindsFiles(t *testing.T) {
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
	if strings.Contains(got, "code_item") {
		t.Fatalf("search must not return code_item results (tree-sitter removed):\n%s", got)
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
	code = testApp(root).Run([]string{"search", "Runner", "--kind", "file"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "file internal/contracts/runner.go") {
		t.Fatalf("file filter did not return file result:\n%s", got)
	}
	if strings.Contains(got, "code_item") {
		t.Fatalf("file filter must not return code_item (tree-sitter removed):\n%s", got)
	}
}

func TestSearchKindCodeItemRejected(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Hi\n")

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code := testApp(root).Run([]string{"search", "hello", "--kind", "code_item"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("search --kind code_item should fail (kind removed); stdout:\n%s", stdout.String())
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

// subprocessTransport drives helper-process agent descriptors (Command:
// os.Args[0]) as one-shot subprocesses, giving app tests a real execution
// path without a live ACP adapter. Transport selection is covered separately
// in transport_selection_test.go.
type subprocessTransport struct{}

func (subprocessTransport) Run(request agents.RunRequest) (agents.RunResult, error) {
	promptPath := filepath.Join(request.RepoRoot, filepath.FromSlash(request.PromptRepoPath))
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return agents.RunResult{}, err
	}

	artifactDir := strings.Trim(strings.TrimSpace(request.ArtifactDir), "/")
	if artifactDir == "" {
		artifactDir = "execute/attempts"
	}
	attemptDir := filepath.Join(request.RepoRoot, artifacts.WorkspaceRel, "runs", request.RunID,
		filepath.FromSlash(artifactDir), request.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return agents.RunResult{}, err
	}
	stdoutFile, err := os.Create(filepath.Join(attemptDir, "stdout.log"))
	if err != nil {
		return agents.RunResult{}, err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(filepath.Join(attemptDir, "stderr.log"))
	if err != nil {
		return agents.RunResult{}, err
	}
	defer stderrFile.Close()

	var stdoutWriter io.Writer = stdoutFile
	var stderrWriter io.Writer = stderrFile
	if request.LiveOutput != nil {
		live := &subprocessLiveWriter{w: request.LiveOutput}
		stdoutWriter = io.MultiWriter(stdoutFile, live)
		stderrWriter = io.MultiWriter(stderrFile, live)
	}

	cmd := exec.Command(request.Agent.Command, request.Agent.Args...)
	cmd.Dir = request.RepoRoot
	cmd.Stdin = bytes.NewReader(prompt)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	started := time.Now().UTC()
	runErr := cmd.Run()
	finished := time.Now().UTC()

	exitCode := 0
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return agents.RunResult{
		ExitCode:       exitCode,
		StartedAt:      started.Format(time.RFC3339Nano),
		FinishedAt:     finished.Format(time.RFC3339Nano),
		DurationMillis: finished.Sub(started).Milliseconds(),
		StdoutPath:     filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stdout.log")),
		StderrPath:     filepath.ToSlash(filepath.Join(artifactDir, request.AttemptID, "stderr.log")),
	}, runErr
}

// subprocessLiveWriter serializes concurrent writes from stdout and stderr
// goroutines to the shared live output writer.
type subprocessLiveWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *subprocessLiveWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func testApp(root string) App {
	return App{
		WorkingDir:     root,
		AgentTransport: subprocessTransport{},
		Now: func() time.Time {
			return time.Date(2026, 5, 31, 18, 40, 12, 0, time.UTC)
		},
	}
}

// fixedAgentRegistry mirrors the production registry shape after engine
// inference: resolution is keyed by engine name ("codex"/"claude"), with
// separate descriptor tables per role so a test can run a reviewer helper and
// a fixer helper on the same engine.
type fixedAgentRegistry struct {
	defaultExecutor string
	defaultReviewer string
	order           []string
	executors       map[string]agents.AgentDescriptor
	reviewers       map[string]agents.AgentDescriptor
}

// testAgentRegistry overrides an engine's descriptor for both roles; extra
// descriptors must carry an engine name ("codex"/"claude") because resolution
// only ever asks for inferred engines.
func testAgentRegistry(extra ...agents.AgentDescriptor) agents.Registry {
	return testAgentRegistryRoles(extra, extra)
}

// testAgentRegistryRoles overrides executor-role and reviewer-role descriptors
// independently, keyed by engine name.
func testAgentRegistryRoles(executors []agents.AgentDescriptor, reviewers []agents.AgentDescriptor) agents.Registry {
	executorTable := map[string]agents.AgentDescriptor{}
	reviewerTable := map[string]agents.AgentDescriptor{}
	order := []string{}
	for _, descriptor := range agents.ListBuiltins() {
		executorTable[descriptor.Name] = descriptor
		reviewerTable[descriptor.Name] = descriptor
		order = append(order, descriptor.Name)
	}
	for _, descriptor := range executors {
		if _, ok := executorTable[descriptor.Name]; !ok {
			order = append(order, descriptor.Name)
		}
		executorTable[descriptor.Name] = descriptor
	}
	for _, descriptor := range reviewers {
		if _, ok := executorTable[descriptor.Name]; !ok {
			if _, seen := reviewerTable[descriptor.Name]; !seen {
				order = append(order, descriptor.Name)
			}
		}
		reviewerTable[descriptor.Name] = descriptor
	}
	return fixedAgentRegistry{
		defaultExecutor: agents.DefaultExecutor(),
		defaultReviewer: agents.DefaultReviewer(),
		order:           order,
		executors:       executorTable,
		reviewers:       reviewerTable,
	}
}

// helperAgentDescriptor builds an execution helper descriptor registered under
// an engine name. The trailing "--" terminates test-binary flag parsing so the
// registry entry's model/effort pins (always appended now that the model is
// required) are ignored as positional arguments.
func helperAgentDescriptor(engine string) agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    engine,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecutionHelperProcess", "--"},
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
	return resolveTestAgent(r.executors, name, r.defaultExecutor)
}

func (r fixedAgentRegistry) ResolveReviewer(name string) (agents.AgentDescriptor, error) {
	return resolveTestAgent(r.reviewers, name, r.defaultReviewer)
}

func (r fixedAgentRegistry) ListBuiltins() []agents.AgentDescriptor {
	descriptors := make([]agents.AgentDescriptor, 0, len(r.order))
	for _, name := range r.order {
		descriptor, ok := r.executors[name]
		if !ok {
			descriptor = r.reviewers[name]
		}
		descriptors = append(descriptors, cloneTestAgentDescriptor(descriptor))
	}
	return descriptors
}

func resolveTestAgent(descriptors map[string]agents.AgentDescriptor, name string, defaultName string) (agents.AgentDescriptor, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultName
	}
	descriptor, ok := descriptors[name]
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
		WorkingDir:     root,
		AgentTransport: subprocessTransport{},
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
	}
}

// readConfigForTest decodes a workspace config without registry validation so
// config-mutating helpers stay order-independent: a config may temporarily
// hold partial entries while a test composes its registry.
func readConfigForTest(t *testing.T, path string) configFile {
	t.Helper()
	data, err := os.ReadFile(path)
	assertNoError(t, err)
	var config configFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	assertNoError(t, decoder.Decode(&config))
	return config
}

// setAgentRegistryConfig replaces the config agents registry.
func setAgentRegistryConfig(t *testing.T, paths artifacts.Paths, entries ...agentRegistryEntry) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	config.Agents = append([]agentRegistryEntry{}, entries...)
	assertNoError(t, writeYAML(paths.Config, config))
}

// registerTestAgents appends names to the config agents registry (skipping
// already-registered names) so registry-name resolution finds the helper
// agents tests inject through App.AgentRegistry. Each entry gets a model whose
// inferred engine is testAgentEngine(name), the engine the helper descriptor
// must be registered under.
func registerTestAgents(t *testing.T, paths artifacts.Paths, names ...string) {
	t.Helper()
	config := readConfigForTest(t, paths.Config)
	registered := map[string]bool{}
	for _, entry := range config.Agents {
		registered[entry.Name] = true
	}
	for _, name := range names {
		if !registered[name] {
			config.Agents = append(config.Agents, agentRegistryEntry{Name: name, Model: testAgentModel(name)})
			registered[name] = true
		}
	}
	assertNoError(t, writeYAML(paths.Config, config))
}

// testAgentEngine maps a helper-agent name to the engine its registry entry
// infers to: codex-flavored names run on codex, everything else on claude.
func testAgentEngine(name string) string {
	if strings.Contains(name, "codex") {
		return agents.BuiltinCodex
	}
	return agents.BuiltinClaude
}

// testHelperDescriptors builds one helper-process descriptor per distinct
// engine the given helper names infer to. Engine-keyed resolution cannot tell
// two same-engine helper names apart, so they share one descriptor — which
// matches how the configure helpers always built them (identical commands).
// The trailing "--" keeps the appended model/effort pins out of the test
// binary's flag parsing.
func testHelperDescriptors(names []string, helperTest string) []agents.AgentDescriptor {
	seen := map[string]bool{}
	descriptors := []agents.AgentDescriptor{}
	for _, name := range names {
		engine := testAgentEngine(name)
		if seen[engine] {
			continue
		}
		seen[engine] = true
		descriptors = append(descriptors, agents.AgentDescriptor{
			Name:    engine,
			Command: os.Args[0],
			Args:    []string{"-test.run=" + helperTest, "--"},
			Input:   agents.InputPromptFile,
		})
	}
	return descriptors
}

// testAgentModel returns a model that infers to testAgentEngine(name).
func testAgentModel(name string) string {
	if testAgentEngine(name) == agents.BuiltinCodex {
		return "gpt-5"
	}
	return "claude-opus-4-8"
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
	// Register both engines so tests can invoke either by name; codex first
	// keeps the pre-registry default-executor behavior (first entry wins). The
	// config edit invalidates the just-built project map, so refresh it.
	setAgentRegistryConfig(t, artifacts.New(root),
		agentRegistryEntry{Name: "codex", Model: "gpt-5"},
		agentRegistryEntry{Name: "claude", Model: "claude-opus-4-8"},
	)
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"task", "new", "add sqlite cache"}, &stdout, &stderr)
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

// writeReviseDocForTest creates a temp file containing a contract revise
// partial-update document for --from. base_version is read from the current
// contract.json (supports both real and swapped fake stores).
func writeReviseDocForTest(t *testing.T, runPaths contractRunPathSet, contractUpdate any) string {
	t.Helper()
	version, err := storeFileSHA256(runPaths.ContractJSON)
	if err != nil {
		t.Fatalf("writeReviseDocForTest: hash contract: %v", err)
	}
	doc := map[string]any{
		"base_version": version,
		"contract":     contractUpdate,
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("writeReviseDocForTest: marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "revise.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writeReviseDocForTest: write: %v", err)
	}
	return path
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
