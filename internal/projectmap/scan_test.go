package projectmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanExtractsThinGoEntries(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "cmd", "pactum", "main.go"), `package main

import "fmt"

type App struct{}
type hidden struct{}

func main() {}
func Run() {}
func helper() {}
func (App) Serve() {}

var _ = fmt.Println
`)
	writeTestFile(t, filepath.Join(root, "internal", "worker", "worker.go"), `package worker

type Task struct{}
type local struct{}

func Build() {}
func build() {}
`)

	scan, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	assertEntry(t, scan.Entries, "cmd/pactum/main.go", "go_package", "main")
	assertEntry(t, scan.Entries, "cmd/pactum/main.go", "go_import", "fmt")
	assertEntry(t, scan.Entries, "cmd/pactum/main.go", "go_main", "main")
	assertEntry(t, scan.Entries, "cmd/pactum/main.go", "go_func", "Run")
	assertEntry(t, scan.Entries, "cmd/pactum/main.go", "go_type", "App")
	assertEntry(t, scan.Entries, "internal/worker/worker.go", "go_func", "Build")
	assertEntry(t, scan.Entries, "internal/worker/worker.go", "go_type", "Task")

	assertNoEntry(t, scan.Entries, "cmd/pactum/main.go", "go_func", "helper")
	assertNoEntry(t, scan.Entries, "cmd/pactum/main.go", "go_func", "Serve")
	assertNoEntry(t, scan.Entries, "cmd/pactum/main.go", "go_type", "hidden")
	assertNoEntry(t, scan.Entries, "internal/worker/worker.go", "go_func", "build")
	assertSortedEntries(t, scan.Entries)
}

func TestScanToleratesGoParseErrors(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "broken.go"), "package broken\nfunc {\n")

	scan, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Warnings) == 0 {
		t.Fatal("expected parse warning")
	}
	if !strings.Contains(strings.Join(scan.Warnings, "\n"), "go parse failed: broken.go") {
		t.Fatalf("warnings did not mention parse failure: %#v", scan.Warnings)
	}
	assertNoEntry(t, scan.Entries, "broken.go", "go_package", "broken")
}

func TestScanSkipsFilesOverMaxFileBytes(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "small.go"), "package small\n")
	writeTestFile(t, filepath.Join(root, "large.go"), "package large\n"+strings.Repeat("x", 128))

	scan, err := Scan(root, ScanOptions{MaxFileBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	if scan.FilesSkipped != 1 {
		t.Fatalf("FilesSkipped = %d, want 1", scan.FilesSkipped)
	}
	if scan.FilesIgnored != 1 {
		t.Fatalf("FilesIgnored = %d, want 1", scan.FilesIgnored)
	}
	if containsFile(scan.Files, "large.go") {
		t.Fatalf("large.go should have been skipped: %#v", scan.Files)
	}
	if !containsFile(scan.Files, "small.go") {
		t.Fatalf("small.go should have been indexed: %#v", scan.Files)
	}
	if !strings.Contains(strings.Join(scan.Warnings, "\n"), "skipped large file: large.go") {
		t.Fatalf("warnings did not mention skipped file: %#v", scan.Warnings)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertEntry(t *testing.T, entries []EntryRecord, path string, kind string, name string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Path == path && entry.Kind == kind && entry.Name == name {
			return
		}
	}
	t.Fatalf("missing entry path=%s kind=%s name=%s in %#v", path, kind, name, entries)
}

func assertNoEntry(t *testing.T, entries []EntryRecord, path string, kind string, name string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Path == path && entry.Kind == kind && entry.Name == name {
			t.Fatalf("unexpected entry path=%s kind=%s name=%s in %#v", path, kind, name, entries)
		}
	}
}

func assertSortedEntries(t *testing.T, entries []EntryRecord) {
	t.Helper()
	for i := 1; i < len(entries); i++ {
		prev := entries[i-1]
		next := entries[i]
		if prev.Path > next.Path ||
			(prev.Path == next.Path && prev.Kind > next.Kind) ||
			(prev.Path == next.Path && prev.Kind == next.Kind && prev.Name > next.Name) {
			t.Fatalf("entries are not sorted at %d: %#v before %#v", i, prev, next)
		}
	}
}

func containsFile(files []FileRecord, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}
