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
