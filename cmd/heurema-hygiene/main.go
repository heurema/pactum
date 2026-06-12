// Command heurema-hygiene is the deterministic leak gate for the committed
// .heurema run record. It scans the .heurema files in the git index — tracked
// files plus staged additions, never unrelated untracked scratch files — for
// absolute home-directory paths and credential-shaped strings, prints every
// finding as file:line with the detector name and a redacted preview, and
// exits nonzero when anything is found. Content is read from the index blobs,
// not the worktree, so the gate verifies exactly the bytes a commit would
// record even when a staged file was edited or deleted afterwards.
//
// Detectors require real token material so bare documentation examples such
// as "sk-", "ghp_", "/Users/", or "Authorization: Bearer" never match. Only
// files under .heurema are scanned, so the patterns defined here are never
// matched against this file, the Makefile, or fixtures elsewhere in the repo.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// detector is one family of forbidden content. keep is how many leading
// characters of a match the report shows; every pattern's minimum match is
// longer than its keep, so a full secret or home path never reaches the
// output. plausible, when set, rejects regex matches that lack real token
// material.
type detector struct {
	name      string
	keep      int
	re        *regexp.Regexp
	plausible func(match string) bool
}

var detectors = []detector{
	// Requires a username segment so a bare "/Users/" doc example passes.
	// "C:\\+Users" tolerates JSON-escaped backslashes in run-record files.
	{name: "home-path", keep: 6,
		re: regexp.MustCompile(`/Users/[A-Za-z0-9._-]+|/home/[A-Za-z0-9._-]+|C:\\+Users\\+[A-Za-z0-9._-]+`)},
	{name: "github-token", keep: 8,
		re: regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`)},
	// Real keys mix case and digits; the plausible check drops kebab-case
	// prose like "task-to-interrogated-contract" (which contains "sk-to-…").
	{name: "openai-token", keep: 6,
		re:        regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`),
		plausible: hasTokenMaterial},
	{name: "aws-access-key", keep: 8,
		re: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{name: "slack-token", keep: 8,
		re: regexp.MustCompile(`\bxox[a-z]-[A-Za-z0-9-]{10,}`)},
	{name: "private-key", keep: 16,
		re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{name: "bearer-token", keep: 22,
		re: regexp.MustCompile(`(?i)\bauthorization: *bearer +[A-Za-z0-9._~+/=-]{16,}`)},
}

func hasTokenMaterial(match string) bool {
	return strings.IndexFunc(match, func(r rune) bool {
		return unicode.IsDigit(r) || unicode.IsUpper(r)
	}) >= 0
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "heurema-hygiene:", err)
		os.Exit(2)
	}
	files, err := heuremaFiles(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "heurema-hygiene:", err)
		os.Exit(2)
	}
	findings, err := scanIndex(root, files, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "heurema-hygiene:", err)
		os.Exit(2)
	}
	if findings > 0 {
		fmt.Printf("heurema-hygiene: %d finding(s) in the committed .heurema tree\n", findings)
		os.Exit(1)
	}
	fmt.Printf("heurema-hygiene: clean (%d files scanned)\n", len(files))
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// heuremaFiles lists the .heurema files the gate guards: the git index, which
// covers tracked files and staged additions but never untracked scratch files.
func heuremaFiles(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", ".heurema").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files .heurema: %w", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// scanIndex reports every detector match in the staged content of the given
// files (index paths relative to root) to w and returns the finding count.
// One cat-file --batch call serves all files; reading index blobs instead of
// worktree bytes means the gate checks what a commit would actually record,
// including staged files whose worktree copy was edited or deleted.
func scanIndex(root string, files []string, w io.Writer) (int, error) {
	if len(files) == 0 {
		return 0, nil
	}
	var req bytes.Buffer
	for _, f := range files {
		fmt.Fprintf(&req, ":%s\n", f)
	}
	cmd := exec.Command("git", "-C", root, "cat-file", "--batch")
	cmd.Stdin = &req
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git cat-file --batch: %w", err)
	}
	findings := 0
	r := bufio.NewReader(bytes.NewReader(out))
	for _, f := range files {
		data, err := readBatchBlob(r)
		if err != nil {
			return findings, fmt.Errorf("staged content of %s: %w", f, err)
		}
		findings += scanData(f, data, w)
	}
	return findings, nil
}

// readBatchBlob consumes one cat-file --batch response: an
// "<oid> <type> <size>" header, the blob bytes, and a trailing newline. Any
// other response (e.g. "missing") is an error — every requested path was just
// listed by git ls-files, and the gate must never skip a file silently.
func readBatchBlob(r *bufio.Reader) ([]byte, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(header)
	if len(fields) != 3 {
		return nil, fmt.Errorf("unexpected cat-file response %q", strings.TrimSpace(header))
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("unexpected cat-file response %q", strings.TrimSpace(header))
	}
	data := make([]byte, size+1)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data[:size], nil
}

// scanData reports every detector match in one file's content to w and
// returns the finding count. Binary content is skipped.
func scanData(path string, data []byte, w io.Writer) int {
	if bytes.IndexByte(data, 0) >= 0 {
		return 0
	}
	findings := 0
	for i, line := range strings.Split(string(data), "\n") {
		for _, d := range detectors {
			for _, m := range d.re.FindAllString(line, -1) {
				if d.plausible != nil && !d.plausible(m) {
					continue
				}
				fmt.Fprintf(w, "%s:%d: %s: %s\n", path, i+1, d.name, redact(m, d.keep))
				findings++
			}
		}
	}
	return findings
}

// redact truncates a match to its detector prefix. Matches are ASCII by
// construction, so byte slicing is safe.
func redact(m string, keep int) string {
	return m[:keep] + fmt.Sprintf("…(+%d chars)", len(m)-keep)
}
