package projectmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/codeindex"
)

func TestScanExtractsCodeItemsWhenEnabled(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "cmd", "pactum", "main.go"), `package main

import "fmt"

type App struct{}
type hidden struct{}

func main() {}
func Run() {}
func helper() {}

var _ = fmt.Println
`)

	scan, err := Scan(root, ScanOptions{CodeIndexMode: codeindex.ModeAuto})
	if err != nil {
		t.Fatal(err)
	}

	assertCodeItem(t, scan.CodeItems, "cmd/pactum/main.go", "go_package", "main")
	assertCodeItem(t, scan.CodeItems, "cmd/pactum/main.go", "go_import", "fmt")
	assertCodeItem(t, scan.CodeItems, "cmd/pactum/main.go", "go_main", "main")
	assertCodeItem(t, scan.CodeItems, "cmd/pactum/main.go", "go_func", "Run")
	assertCodeItem(t, scan.CodeItems, "cmd/pactum/main.go", "go_type", "App")
	assertNoCodeItem(t, scan.CodeItems, "cmd/pactum/main.go", "go_func", "helper")
	assertNoCodeItem(t, scan.CodeItems, "cmd/pactum/main.go", "go_type", "hidden")
	assertSortedCodeItems(t, scan.CodeItems)
	assertStringSliceEqual(t, scan.CodeIndexLanguagesSeen, []string{"go"})
	assertStringSliceEqual(t, scan.CodeIndexLanguagesIndexed, []string{"go"})
}

func TestScanCodeIndexOffSkipsExtractionWarnings(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "broken.go"), "package broken\nfunc {\n")

	scan, err := Scan(root, ScanOptions{CodeIndexMode: codeindex.ModeOff})
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.CodeItems) != 0 {
		t.Fatalf("CodeItems = %#v, want empty when code index is off", scan.CodeItems)
	}
	if len(scan.Warnings) != 0 {
		t.Fatalf("Warnings = %#v, want none when code index is off", scan.Warnings)
	}
	assertStringSliceEqual(t, scan.CodeIndexLanguagesSeen, []string{"go"})
	if len(scan.CodeIndexLanguagesIndexed) != 0 {
		t.Fatalf("CodeIndexLanguagesIndexed = %#v, want empty", scan.CodeIndexLanguagesIndexed)
	}
}

func TestScanToleratesCodeIndexParseErrors(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "broken.go"), "package broken\nfunc {\n")

	scan, err := Scan(root, ScanOptions{CodeIndexMode: codeindex.ModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Warnings) == 0 {
		t.Fatal("expected parse warning")
	}
	if !strings.Contains(strings.Join(scan.Warnings, "\n"), "broken.go") {
		t.Fatalf("warnings did not mention parse failure: %#v", scan.Warnings)
	}
	assertNoCodeItem(t, scan.CodeItems, "broken.go", "go_package", "broken")
}

func TestScanSkipsFilesOverMaxFileBytes(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "small.go"), "package small\n")
	writeTestFile(t, filepath.Join(root, "large.go"), "package large\n"+strings.Repeat("x", 128))

	scan, err := Scan(root, ScanOptions{MaxFileBytes: 64, CodeIndexMode: codeindex.ModeAuto})
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

func assertCodeItem(t *testing.T, items []codeindex.Item, path string, kind string, name string) {
	t.Helper()
	for _, item := range items {
		if item.Path == path && item.Kind == kind && item.Name == name {
			return
		}
	}
	t.Fatalf("missing code item path=%s kind=%s name=%s in %#v", path, kind, name, items)
}

func assertNoCodeItem(t *testing.T, items []codeindex.Item, path string, kind string, name string) {
	t.Helper()
	for _, item := range items {
		if item.Path == path && item.Kind == kind && item.Name == name {
			t.Fatalf("unexpected code item path=%s kind=%s name=%s in %#v", path, kind, name, items)
		}
	}
}

func assertSortedCodeItems(t *testing.T, items []codeindex.Item) {
	t.Helper()
	for i := 1; i < len(items); i++ {
		prev := items[i-1]
		next := items[i]
		if prev.Path > next.Path ||
			(prev.Path == next.Path && prev.Kind > next.Kind) ||
			(prev.Path == next.Path && prev.Kind == next.Kind && prev.Parent > next.Parent) ||
			(prev.Path == next.Path && prev.Kind == next.Kind && prev.Parent == next.Parent && prev.Name > next.Name) {
			t.Fatalf("code items are not sorted at %d: %#v before %#v", i, prev, next)
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

func assertStringSliceEqual(t *testing.T, got []string, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
