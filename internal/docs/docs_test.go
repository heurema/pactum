package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
}

// requiredDocMentions are current commands the user-facing docs must describe.
var requiredDocMentions = []string{
	"pactum execute dry-run",
	"pactum execute run",
	"pactum gate run",
	"pactum review propose-findings",
	"pactum memory refresh",
	"pactum agents doctor",
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

// userFacingDocs returns README.md and every docs/*.md file, read from the
// repository root.
func userFacingDocs(t *testing.T) []docFile {
	t.Helper()
	root := repoRoot(t)

	paths := []string{filepath.Join(root, "README.md")}
	docsDir := filepath.Join(root, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatalf("read docs dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		paths = append(paths, filepath.Join(docsDir, entry.Name()))
	}

	docs := make([]docFile, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		docs = append(docs, docFile{name: filepath.ToSlash(rel), content: string(data)})
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
