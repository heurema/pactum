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
	"github.com/heurema/pactum/internal/projectmap"
)

func TestInitCreatesExpectedLayoutAndProjectMap(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	mustWriteFile(t, filepath.Join(root, "src", "main.go"), "package main\n")
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
		paths.EntriesJSONL,
		paths.HashesJSONL,
		paths.ProjectMemory,
		paths.StaleReport,
		paths.EventsJSONL,
		paths.UsageJSONL,
		paths.CostJSON,
	} {
		assertFile(t, file)
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

	repoMap := mustReadFile(t, paths.RepoMap)
	for _, want := range []string{"# Pactum Project Map", "README.md", "src/main.go", "Go: 1 file(s)"} {
		if !strings.Contains(repoMap, want) {
			t.Fatalf("repo-map.md missing %q:\n%s", want, repoMap)
		}
	}
	llms := mustReadFile(t, paths.LLMS)
	for _, want := range []string{"repo-map.md", "files.jsonl", "before adding new code"} {
		if !strings.Contains(llms, want) {
			t.Fatalf("llms.txt missing %q:\n%s", want, llms)
		}
	}

	events := readLines(t, paths.EventsJSONL)
	if len(events) != 4 {
		t.Fatalf("events line count = %d, want 4: %v", len(events), events)
	}
	for i, want := range []string{"init_started", "map_refresh_started", "map_refresh_finished", "init_finished"} {
		if !strings.Contains(events[i], want) {
			t.Fatalf("event %d = %s, want %s", i, events[i], want)
		}
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
