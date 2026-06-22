package projectmap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestScanGitWorktreeHonorsGitIgnore(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	runGit(t, root, "init")
	writeTestFile(t, filepath.Join(root, ".gitignore"), strings.Join([]string{
		"__pycache__/",
		"*.pyc",
		"dist/",
		"ignored/",
		"",
	}, "\n"))
	writeTestFile(t, filepath.Join(root, "tracked.py"), "def tracked():\n    pass\n")
	writeTestFile(t, filepath.Join(root, "src", "app.py"), "def run():\n    pass\n")
	writeTestFile(t, filepath.Join(root, "__pycache__", "module.pyc"), "compiled")
	writeTestFile(t, filepath.Join(root, "dist", "generated.py"), "def generated():\n    pass\n")
	writeTestFile(t, filepath.Join(root, "ignored", "hidden.py"), "def hidden():\n    pass\n")
	writeTestFile(t, filepath.Join(root, ".heurema", "state.py"), "def workspace():\n    pass\n")
	writeTestFile(t, filepath.Join(root, "asset.png"), "not really an image")
	writeTestFile(t, filepath.Join(root, "large.py"), "def large():\n    pass\n"+strings.Repeat("x", 128))
	runGit(t, root, "add", ".gitignore", "tracked.py", "asset.png")
	runGit(t, root, "add", "-f", ".heurema/state.py")

	scan, err := Scan(root, ScanOptions{MaxFileBytes: 64})
	if err != nil {
		t.Fatal(err)
	}

	assertScanPath(t, scan, ".gitignore")
	assertScanPath(t, scan, "src/app.py")
	assertScanPath(t, scan, "tracked.py")
	assertNoScanPath(t, scan, "__pycache__/module.pyc")
	assertNoScanPath(t, scan, "dist/generated.py")
	assertNoScanPath(t, scan, "ignored/hidden.py")
	assertNoScanPath(t, scan, ".heurema/state.py")
	assertNoScanPath(t, scan, "asset.png")
	assertNoScanPath(t, scan, "large.py")
	if scan.FilesSkipped != 1 {
		t.Fatalf("FilesSkipped = %d, want 1", scan.FilesSkipped)
	}
	if !strings.Contains(strings.Join(scan.Warnings, "\n"), "skipped large file: large.py") {
		t.Fatalf("warnings did not mention skipped git-enumerated file: %#v", scan.Warnings)
	}
	assertSortedFiles(t, scan.Files)
	assertSortedHashes(t, scan.Hashes)
}

func TestScanNonGitFallbackUsesIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "keep.go"), "package keep\n")
	writeTestFile(t, filepath.Join(root, "dist", "generated.go"), "package generated\n")
	writeTestFile(t, filepath.Join(root, ".heurema", "state.go"), "package state\n")

	scan, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	assertScanPath(t, scan, "keep.go")
	assertNoScanPath(t, scan, "dist/generated.go")
	assertNoScanPath(t, scan, ".heurema/state.go")
	if scan.FilesIgnored != 2 {
		t.Fatalf("FilesIgnored = %d, want 2", scan.FilesIgnored)
	}
}

func TestScanFallsBackWhenGitUnavailable(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "keep.go"), "package keep\n")
	writeTestFile(t, filepath.Join(root, "target", "generated.go"), "package generated\n")
	emptyPath := filepath.Join(root, "empty-path")
	if err := os.Mkdir(emptyPath, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyPath)

	scan, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	assertScanPath(t, scan, "keep.go")
	assertNoScanPath(t, scan, "target/generated.go")
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

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func assertScanPath(t *testing.T, scan ScanResult, path string) {
	t.Helper()
	if !containsFile(scan.Files, path) {
		t.Fatalf("missing file record for %s in %#v", path, scan.Files)
	}
	if !containsHash(scan.Hashes, path) {
		t.Fatalf("missing hash record for %s in %#v", path, scan.Hashes)
	}
}

func assertNoScanPath(t *testing.T, scan ScanResult, path string) {
	t.Helper()
	if containsFile(scan.Files, path) {
		t.Fatalf("unexpected file record for %s in %#v", path, scan.Files)
	}
	if containsHash(scan.Hashes, path) {
		t.Fatalf("unexpected hash record for %s in %#v", path, scan.Hashes)
	}
}

func assertSortedFiles(t *testing.T, files []FileRecord) {
	t.Helper()
	for i := 1; i < len(files); i++ {
		if files[i-1].Path > files[i].Path {
			t.Fatalf("files are not sorted at %d: %#v before %#v", i, files[i-1], files[i])
		}
	}
}

func assertSortedHashes(t *testing.T, hashes []HashRecord) {
	t.Helper()
	for i := 1; i < len(hashes); i++ {
		if hashes[i-1].Path > hashes[i].Path {
			t.Fatalf("hashes are not sorted at %d: %#v before %#v", i, hashes[i-1], hashes[i])
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

func containsHash(hashes []HashRecord, path string) bool {
	for _, hash := range hashes {
		if hash.Path == path {
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
