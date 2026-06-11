package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// requiredDocFiles are the user-facing docs that must exist, named explicitly
// (repo-root relative). The tests below read exactly these files rather than
// scanning whatever docs/*.md happens to be present, so a missing file is a
// failure rather than a silently smaller set.
var requiredDocFiles = []string{
	"README.md",
	"docs/install.md",
	"docs/flow.md",
	"docs/workspace.md",
	"docs/agents.md",
	"docs/memory.md",
}

// forbiddenDocPhrases are command flags, config keys, and milestone wordings
// that no longer describe Pactum. They must never appear in user-facing docs.
//
// Generic "not implemented" language is intentionally allowed: the docs use it
// to describe genuine MVP limitations (Docker, web UI, custom agents). Only the
// specific stale/removed concepts below are forbidden.
var forbiddenDocPhrases = []string{
	"--allow-execute",
	"--mode yolo",
	"agents.adapters",
	"does not execute agents in this milestone",
	"when execution becomes available",
	// M5.3 replaced the top-level `run` command with `task new`.
	"--contract-only",
	`pactum run "`,
	// M23.0 normalized the command grammar; the old spellings are hard-removed.
	// Artifact paths like execute/dry-run.json are still valid and do not match
	// these command phrases.
	"pactum agents doctor",
	"pactum clarify ask",
	"pactum clarify loop",
	"pactum clarify status",
	"pactum clarify list",
	"contract show-draft",
	"contract accept-draft",
	"pactum execute dry-run",
	"pactum execute status",
	"pactum review dry-run",
	"review add-finding",
	"pactum review resolve",
	"review propose-findings",
	"review accept-proposal",
	"review reject-proposal",
	"review apply-fix-outcomes",
	"pactum task current",
}

// requiredDocMentions are current commands the user-facing docs must describe.
var requiredDocMentions = []string{
	"pactum execute plan",
	"pactum execute run",
	"pactum gate run",
	"pactum review proposal collect",
	"pactum memory refresh",
	"pactum doctor",
	// Packaging / install surface (M5.2).
	"make build",
	"go install ./cmd/pactum",
	"scripts/smoke.sh",
	// CLI v0.2 surface (M5.3).
	"pactum task new",
	"pactum task list",
	"pactum task use",
	"pactum version",
	"--yes",
	// Release-readiness foundation (M5.4).
	"CHANGELOG.md",
}

// TestRequiredDocsExist fails if any required user-facing doc is missing.
func TestRequiredDocsExist(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range requiredDocFiles {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("required doc %s is missing: %v", rel, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("required doc %s is a directory, not a file", rel)
		}
	}
}

// TestDocsHaveNoStaleCommandConcepts fails if any user-facing doc references a
// removed flag, a removed config key, or a stale milestone wording.
func TestDocsHaveNoStaleCommandConcepts(t *testing.T) {
	for _, doc := range userFacingDocs(t) {
		for _, phrase := range forbiddenDocPhrases {
			if strings.Contains(doc.content, phrase) {
				t.Errorf("%s contains stale/removed concept %q", doc.name, phrase)
			}
		}
	}
}

// TestDocsMentionCurrentCommands ensures the user-facing docs collectively
// describe the current command surface.
func TestDocsMentionCurrentCommands(t *testing.T) {
	var combined strings.Builder
	for _, doc := range userFacingDocs(t) {
		combined.WriteString(doc.content)
		combined.WriteByte('\n')
	}
	all := combined.String()
	for _, mention := range requiredDocMentions {
		if !strings.Contains(all, mention) {
			t.Errorf("user-facing docs do not mention %q", mention)
		}
	}
}

type docFile struct {
	name    string
	content string
}

// userFacingDocs reads exactly the requiredDocFiles from the repository root.
// A missing file is fatal, so the stale-concept and required-mention checks
// always run against the full, expected doc set.
func userFacingDocs(t *testing.T) []docFile {
	t.Helper()
	root := repoRoot(t)

	docs := make([]docFile, 0, len(requiredDocFiles))
	for _, rel := range requiredDocFiles {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read required doc %s: %v", rel, err)
		}
		docs = append(docs, docFile{name: rel, content: string(data)})
	}
	return docs
}

// repoRoot walks up from the test's working directory to the module root (the
// directory containing go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}
