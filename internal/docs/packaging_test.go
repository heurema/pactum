package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readRepoFile reads a repo-root-relative file or fails the test.
func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// TestMakefileHasLocalTargets ensures the root Makefile exposes the boring local
// build/test/install surface documented in the install docs.
func TestMakefileHasLocalTargets(t *testing.T) {
	content := readRepoFile(t, "Makefile")
	for _, target := range []string{"build:", "test:", "vet:", "check:", "install:", "clean:"} {
		if !strings.Contains(content, target) {
			t.Errorf("Makefile is missing a %q target", target)
		}
	}
	if !strings.Contains(content, "go build") || !strings.Contains(content, "-o bin/pactum ./cmd/pactum") {
		t.Errorf("Makefile build target does not compile ./cmd/pactum into bin/pactum")
	}
}

// TestSmokeScriptExists ensures the local smoke script is present, is a strict
// shell script, and documents/exercises the safe command surface. The script is
// not executed here (it builds a binary and is too slow for the unit suite); it
// is enough to check it exists and references the expected commands.
func TestSmokeScriptExists(t *testing.T) {
	content := readRepoFile(t, "scripts/smoke.sh")

	if !strings.Contains(content, "set -euo pipefail") {
		t.Errorf("scripts/smoke.sh is not a strict shell script (missing set -euo pipefail)")
	}

	for _, mention := range []string{
		"pactum init",
		"pactum status",
		"pactum task new",
		"pactum agents doctor",
	} {
		if !strings.Contains(content, mention) {
			t.Errorf("scripts/smoke.sh does not reference %q", mention)
		}
	}

	// The smoke script must never run a real agent.
	for _, forbidden := range []string{"execute run", "codex exec", "claude -p"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("scripts/smoke.sh must not run real agents, found %q", forbidden)
		}
	}
}

// TestChangelogExists ensures the release-readiness CHANGELOG is present and is
// still in the unreleased state (no fabricated released version yet).
func TestChangelogExists(t *testing.T) {
	content := readRepoFile(t, "CHANGELOG.md")
	if !strings.Contains(content, "# Changelog") {
		t.Errorf("CHANGELOG.md is missing the changelog heading")
	}
	if !strings.Contains(content, "## Unreleased") {
		t.Errorf("CHANGELOG.md is missing an Unreleased section")
	}
}

// TestCIWorkflowRunsLocalChecks ensures the CI workflow runs the same local
// checks Pactum ships, so green CI matches a green local run.
func TestCIWorkflowRunsLocalChecks(t *testing.T) {
	content := readRepoFile(t, ".github/workflows/ci.yml")
	for _, step := range []string{"make check", "make build", "scripts/smoke.sh"} {
		if !strings.Contains(content, step) {
			t.Errorf(".github/workflows/ci.yml does not run %q", step)
		}
	}
}
