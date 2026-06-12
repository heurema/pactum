package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scanString runs the detectors over a single fixture file's content and
// returns the finding count and report output.
func scanString(content string) (int, string) {
	var out strings.Builder
	findings := scanData(".heurema/fixture.md", []byte(content), &out)
	return findings, out.String()
}

func TestScanFlagsLeaks(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		detector string
		secret   string // must never appear in the report
	}{
		{"macos home path", "cwd was /Users/alice/repos/proj", "home-path", "alice"},
		{"linux home path", "log at /home/runner/work/out.txt", "home-path", "runner"},
		{"windows home path", `saved to C:\Users\bob\proj`, "home-path", "bob"},
		{"json-escaped windows path", `{"cwd":"C:\\Users\\bob\\proj"}`, "home-path", "bob"},
		{"github classic token", "token ghp_AbCd1234EfGh5678IjKl9012MnOp3456QrSt", "github-token", "3456QrSt"},
		{"github fine-grained token", "github_pat_11ABCDEFG0_abcdefghijklmnopqrstuvwxyz", "github-token", "qrstuvwxyz"},
		{"openai token", "key sk-Abc123Def456Ghi789Jkl012", "openai-token", "789Jkl012"},
		{"aws access key", "id AKIAIOSFODNN7EXAMPLE", "aws-access-key", "FODNN7EXAMPLE"},
		{"slack token", "xoxb-1234567890-abcdef", "slack-token", "7890-abcdef"},
		{"private key block", "-----BEGIN OPENSSH PRIVATE KEY-----", "private-key", "OPENSSH PRIVATE KEY"},
		{"bearer header", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload", "bearer-token", "eyJhbGciOiJIUzI1NiJ9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings, out := scanString(tc.content)
			if findings == 0 {
				t.Fatalf("expected a finding, report was empty")
			}
			if !strings.Contains(out, ".heurema/fixture.md:1: "+tc.detector+": ") {
				t.Fatalf("report missing file:line and detector %q:\n%s", tc.detector, out)
			}
			if strings.Contains(out, tc.secret) {
				t.Fatalf("report leaked secret material %q:\n%s", tc.secret, out)
			}
		})
	}
}

func TestScanIgnoresBareExamples(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"bare home prefixes", `scan for /Users/, /home/, and C:\Users prefixes`},
		{"bare token prefixes", "prefixes such as ghp_, github_pat_, sk-, AKIA, and xox"},
		{"bearer without token", "set an Authorization: Bearer header"},
		{"bearer placeholder", "Authorization: Bearer <token>"},
		{"private key ellipsis", "-----BEGIN ... PRIVATE KEY----- markers"},
		{"kebab prose containing sk-", "the task-to-interrogated-contract pipeline"},
		{"standalone kebab sk- prose", "see sk-to-interrogated-contract-notes for details"},
		{"akia in prose", "AKIA access keys are scanned"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings, out := scanString(tc.content)
			if findings != 0 {
				t.Fatalf("expected no findings, got %d:\n%s", findings, out)
			}
		})
	}
}

func TestScanReportsEveryFindingWithLineNumbers(t *testing.T) {
	findings, out := scanString(strings.Join([]string{
		"clean line",
		"path /Users/alice/x and key sk-Abc123Def456Ghi789Jkl012",
		"clean line",
		"id AKIAIOSFODNN7EXAMPLE",
	}, "\n"))
	if findings != 3 {
		t.Fatalf("expected 3 findings, got %d:\n%s", findings, out)
	}
	for _, want := range []string{
		".heurema/fixture.md:2: home-path: ",
		".heurema/fixture.md:2: openai-token: ",
		".heurema/fixture.md:4: aws-access-key: ",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
}

func TestScanSkipsBinaryContent(t *testing.T) {
	findings, out := scanString("/Users/alice\x00binary")
	if findings != 0 {
		t.Fatalf("expected no findings, got %d:\n%s", findings, out)
	}
}

// TestScanIndexReadsStagedContent pins the gate to index blobs: staged
// secrets are flagged even when the worktree copy was deleted or scrubbed
// afterwards, and an unstaged worktree edit is not what gets verified.
func TestScanIndexReadsStagedContent(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")

	deleted := filepath.Join(".heurema", "deleted.md")
	writeTestFile(t, filepath.Join(root, deleted), "key sk-Abc123Def456Ghi789Jkl012")
	runGit(t, root, "add", deleted)
	if err := os.Remove(filepath.Join(root, deleted)); err != nil {
		t.Fatal(err)
	}

	scrubbed := filepath.Join(".heurema", "scrubbed.md")
	writeTestFile(t, filepath.Join(root, scrubbed), "id AKIAIOSFODNN7EXAMPLE")
	runGit(t, root, "add", scrubbed)
	writeTestFile(t, filepath.Join(root, scrubbed), "scrubbed in the worktree only")

	unstaged := filepath.Join(".heurema", "unstaged.md")
	writeTestFile(t, filepath.Join(root, unstaged), "clean record")
	runGit(t, root, "add", unstaged)
	writeTestFile(t, filepath.Join(root, unstaged), "worktree-only /Users/alice/secret")

	files, err := heuremaFiles(root)
	if err != nil {
		t.Fatalf("heuremaFiles failed: %v", err)
	}
	var out strings.Builder
	findings, err := scanIndex(root, files, &out)
	if err != nil {
		t.Fatalf("scanIndex failed: %v", err)
	}
	if findings != 2 {
		t.Fatalf("expected 2 findings, got %d:\n%s", findings, out.String())
	}
	for _, want := range []string{
		".heurema/deleted.md:1: openai-token: ",
		".heurema/scrubbed.md:1: aws-access-key: ",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "unstaged.md") {
		t.Fatalf("unstaged worktree edit was scanned:\n%s", out.String())
	}
}

// TestHeuremaFiles verifies the file-selection contract: tracked files and
// staged additions are scanned, untracked scratch files are not.
func TestHeuremaFiles(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	writeTestFile(t, filepath.Join(root, ".heurema", "tracked.md"), "committed record")
	runGit(t, root, "add", ".heurema/tracked.md")
	runGit(t, root, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet", "-m", "init")
	writeTestFile(t, filepath.Join(root, ".heurema", "staged.md"), "key sk-Abc123Def456Ghi789Jkl012")
	runGit(t, root, "add", ".heurema/staged.md")
	writeTestFile(t, filepath.Join(root, ".heurema", "untracked.md"), "scratch /Users/alice/secret")
	writeTestFile(t, filepath.Join(root, "outside.md"), "/Users/alice elsewhere")

	files, err := heuremaFiles(root)
	if err != nil {
		t.Fatalf("heuremaFiles failed: %v", err)
	}
	want := []string{".heurema/staged.md", ".heurema/tracked.md"}
	if len(files) != len(want) || files[0] != want[0] || files[1] != want[1] {
		t.Fatalf("heuremaFiles = %v, want %v", files, want)
	}

	var out strings.Builder
	findings, err := scanIndex(root, files, &out)
	if err != nil {
		t.Fatalf("scanIndex failed: %v", err)
	}
	if findings != 1 || !strings.Contains(out.String(), ".heurema/staged.md:1: openai-token: ") {
		t.Fatalf("expected one staged-file finding, got %d:\n%s", findings, out.String())
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

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

// TestScanIndexCountsFindingsForNonzeroExit pins the command-level failure
// contract: scanIndex returns a positive finding count for offending content
// (main exits 1 on that) and zero for clean content.
func TestScanIndexCountsFindingsForNonzeroExit(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	offending := filepath.Join(".heurema", "leak.jsonl")
	writeTestFile(t, filepath.Join(root, offending), `{"path": "/Users/someone/repo/file.go"}`+"\n")
	runGit(t, root, "add", offending)
	var out strings.Builder
	findings, err := scanIndex(root, []string{offending}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if findings == 0 {
		t.Fatalf("offending file must yield findings, output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "leak.jsonl") {
		t.Fatalf("finding must name the file:\n%s", out.String())
	}

	clean := filepath.Join(".heurema", "clean.jsonl")
	writeTestFile(t, filepath.Join(root, clean), `{"note": "all good"}`+"\n")
	runGit(t, root, "add", clean)
	out.Reset()
	findings, err = scanIndex(root, []string{clean}, &out)
	if err != nil || findings != 0 {
		t.Fatalf("clean file must yield zero findings (err %v), output:\n%s", err, out.String())
	}
}
