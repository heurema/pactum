package projectmap

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/codeindex"
)

func renderWikiForTest(t *testing.T, root string) map[string]string {
	t.Helper()
	scan, err := Scan(root, ScanOptions{CodeIndexMode: codeindex.ModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	pages := RenderWiki(root, time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC), scan)
	out := make(map[string]string, len(pages))
	for _, page := range pages {
		out[page.RelPath] = string(page.Content)
	}
	return out
}

func TestRenderWikiEcosystemEvidenceForFrontend(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), `{
  "name": "web",
  "devDependencies": { "vite": "^5.0.0", "vue": "^3.4.0" }
}
`)
	writeTestFile(t, filepath.Join(root, "vite.config.ts"), "export default {}\n")
	writeTestFile(t, filepath.Join(root, "src", "App.vue"), "<template/>\n")

	pages := renderWikiForTest(t, root)
	overview := pages["overview.md"]
	for _, want := range []string{
		"Likely role: frontend",
		"Evidence:",
		"package.json depends on vite",
		"vite.config.ts exists",
		".vue files are present",
	} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview.md missing %q:\n%s", want, overview)
		}
	}
}

func TestRenderWikiUsesConservativeLanguage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/demo\n\ngo 1.22\n")
	writeTestFile(t, filepath.Join(root, "cmd", "demo", "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(root, "internal", "core", "core.go"), "package core\n\nfunc Run() {}\n")

	pages := renderWikiForTest(t, root)

	var combined strings.Builder
	for _, content := range pages {
		combined.WriteString(content)
		combined.WriteString("\n")
	}
	all := strings.ToLower(combined.String())

	// Inferred sections must use conservative, evidence-backed language.
	for _, phrase := range []string{"candidate", "likely", "evidence"} {
		if !strings.Contains(all, phrase) {
			t.Fatalf("generated wiki should use conservative phrase %q", phrase)
		}
	}

	// Overclaiming language is forbidden anywhere in the generated wiki.
	for rel, content := range pages {
		lower := strings.ToLower(content)
		for _, forbidden := range []string{"definitely", "complete semantic truth", "guaranteed"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("wiki page %s contains overclaiming phrase %q", rel, forbidden)
			}
		}
		// "source of truth" may only describe source files, never the wiki/map.
		for _, line := range strings.Split(content, "\n") {
			ll := strings.ToLower(line)
			if strings.Contains(ll, "source of truth") && !strings.Contains(ll, "source files") {
				t.Fatalf("wiki page %s claims 'source of truth' outside a source-files statement: %q", rel, line)
			}
		}
	}

	// Source files must be described as the source of truth somewhere.
	if !strings.Contains(combined.String(), "Source files remain the source of truth.") {
		t.Fatalf("generated wiki should describe source files as the source of truth")
	}
}

func TestRenderWikiNeverEmbedsRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/demo\n\ngo 1.22\n")
	writeTestFile(t, filepath.Join(root, "cmd", "demo", "main.go"), "package main\n\nfunc main() {}\n")

	pages := renderWikiForTest(t, root)
	for rel, content := range pages {
		if strings.Contains(content, root) {
			t.Fatalf("wiki page %s embeds absolute root %q", rel, root)
		}
	}
	if !strings.Contains(pages["entrypoints.md"], "cmd/demo/main.go") {
		t.Fatalf("entrypoints.md should list cmd/demo/main.go:\n%s", pages["entrypoints.md"])
	}
}
