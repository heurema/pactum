package projectmap

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func renderWikiForTest(t *testing.T, root string) map[string]string {
	t.Helper()
	scan, err := Scan(root, ScanOptions{})
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

func TestRenderWikiUsesFriendlyRolesForNonCodeDirs(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n")
	writeTestFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "name: CI\non: [push]\n")
	writeTestFile(t, filepath.Join(root, "config", "app.json"), "{}\n")
	// Non-conventionally-named dirs that previously rendered as "likely JSON
	// code" / "likely Markdown code".
	writeTestFile(t, filepath.Join(root, "schemas", "user.json"), "{\"type\":\"object\"}\n")
	writeTestFile(t, filepath.Join(root, "site", "index.md"), "# Site\n")

	pages := renderWikiForTest(t, root)
	var combined strings.Builder
	for _, content := range pages {
		combined.WriteString(content)
		combined.WriteString("\n")
	}
	all := combined.String()

	for _, forbidden := range []string{"likely JSON code", "likely Markdown code", "likely YAML code", "likely Text code", "Likely role: likely"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("generated wiki should not use awkward %q wording:\n%s", forbidden, all)
		}
	}
	for _, want := range []string{"Likely role: configuration", "Likely role: documentation"} {
		if !strings.Contains(all, want) {
			t.Fatalf("generated wiki should use friendly role %q:\n%s", want, all)
		}
	}
}

func TestRenderWikiTighterFrontendDetection(t *testing.T) {
	root := t.TempDir()
	// Vite only as a devDependency; .tsx only under tests/fixtures; no
	// src/main.*, no .vue/.svelte, no app-like structure.
	writeTestFile(t, filepath.Join(root, "package.json"), `{
  "name": "lib",
  "devDependencies": { "vite": "^5.0.0", "vitest": "^1.0.0" }
}
`)
	writeTestFile(t, filepath.Join(root, "src", "index.ts"), "export const x = 1\n")
	writeTestFile(t, filepath.Join(root, "tests", "fixtures", "Sample.tsx"), "export const C = () => null\n")

	pages := renderWikiForTest(t, root)
	overview := pages["overview.md"]
	if strings.Contains(overview, "Likely role: frontend") {
		t.Fatalf("a library using Vite as tooling should not be labeled frontend:\n%s", overview)
	}
	if !strings.Contains(overview, "Node.js / JavaScript") {
		t.Fatalf("overview should still detect the Node.js ecosystem:\n%s", overview)
	}
}

func TestRenderWikiFrontendStillDetectedForRealApp(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), `{
  "name": "app",
  "devDependencies": { "vite": "^5.0.0" },
  "dependencies": { "vue": "^3.4.0" }
}
`)
	writeTestFile(t, filepath.Join(root, "vite.config.ts"), "export default {}\n")
	writeTestFile(t, filepath.Join(root, "src", "main.ts"), "import { createApp } from 'vue'\n")
	writeTestFile(t, filepath.Join(root, "src", "App.vue"), "<template><div/></template>\n")

	pages := renderWikiForTest(t, root)
	if !strings.Contains(pages["overview.md"], "Likely role: frontend") {
		t.Fatalf("a real Vue/Vite app should still be labeled frontend:\n%s", pages["overview.md"])
	}
}

func TestRenderWikiMonorepoTSEntrypoints(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), `{
  "name": "monorepo",
  "workspaces": ["apps/*", "packages/*"]
}
`)
	writeTestFile(t, filepath.Join(root, "apps", "admin", "src", "main.ts"), "export const admin = 1\n")
	writeTestFile(t, filepath.Join(root, "apps", "web", "src", "index.tsx"), "export const web = 1\n")
	writeTestFile(t, filepath.Join(root, "packages", "ui", "src", "index.ts"), "export const ui = 1\n")
	writeTestFile(t, filepath.Join(root, "packages", "utils", "index.ts"), "export const utils = 1\n")

	pages := renderWikiForTest(t, root)
	entry := pages["entrypoints.md"]
	for _, want := range []string{"apps/admin/src/main.ts", "apps/web/src/index.tsx", "packages/ui/src/index.ts", "packages/utils/index.ts"} {
		if !strings.Contains(entry, want) {
			t.Fatalf("entrypoints.md missing %q:\n%s", want, entry)
		}
	}
	if !strings.Contains(entry, "candidate package/library root") {
		t.Fatalf("entrypoints.md should label package/library roots:\n%s", entry)
	}
	if !strings.Contains(entry, "workspace evidence:") {
		t.Fatalf("entrypoints.md should cite workspace evidence:\n%s", entry)
	}
	if !strings.Contains(pages["structure.md"], "package.json workspaces") {
		t.Fatalf("structure.md should surface workspace boundaries:\n%s", pages["structure.md"])
	}
	if !strings.Contains(pages["areas/apps.md"], "apps/admin/src/main.ts") {
		t.Fatalf("areas/apps.md should list the app entrypoint:\n%s", pages["areas/apps.md"])
	}
	if !strings.Contains(pages["areas/packages.md"], "packages/ui/src/index.ts") {
		t.Fatalf("areas/packages.md should list the package root:\n%s", pages["areas/packages.md"])
	}
	for rel, content := range pages {
		if strings.Contains(content, root) {
			t.Fatalf("page %s embeds absolute root %q", rel, root)
		}
	}
}

func TestRenderWikiMonorepoPnpmWorkspace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), "{\n  \"name\": \"mono\"\n}\n")
	writeTestFile(t, filepath.Join(root, "pnpm-workspace.yaml"), "packages:\n  - 'apps/*'\n  - 'packages/*'\n")
	writeTestFile(t, filepath.Join(root, "apps", "api", "src", "main.ts"), "export const api = 1\n")
	writeTestFile(t, filepath.Join(root, "packages", "client", "src", "index.ts"), "export const client = 1\n")

	pages := renderWikiForTest(t, root)
	if !strings.Contains(pages["structure.md"], "pnpm-workspace.yaml") {
		t.Fatalf("structure.md should surface pnpm workspace evidence:\n%s", pages["structure.md"])
	}
	for _, want := range []string{"apps/api/src/main.ts", "packages/client/src/index.ts"} {
		if !strings.Contains(pages["entrypoints.md"], want) {
			t.Fatalf("entrypoints.md missing %q:\n%s", want, pages["entrypoints.md"])
		}
	}
}

func TestRenderWikiMonorepoRustWorkspace(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Cargo.toml"), "[workspace]\nmembers = [\"crates/cli\", \"crates/core\"]\n")
	writeTestFile(t, filepath.Join(root, "crates", "cli", "src", "main.rs"), "fn main() {}\n")
	writeTestFile(t, filepath.Join(root, "crates", "core", "src", "lib.rs"), "pub fn run() {}\n")

	pages := renderWikiForTest(t, root)
	entry := pages["entrypoints.md"]
	if !strings.Contains(entry, "crates/cli/src/main.rs") {
		t.Fatalf("entrypoints.md should list crates/cli/src/main.rs:\n%s", entry)
	}
	if !strings.Contains(entry, "crates/core/src/lib.rs") || !strings.Contains(entry, "candidate package/library root") {
		t.Fatalf("entrypoints.md should list crates/core/src/lib.rs as a library root:\n%s", entry)
	}
	if !strings.Contains(pages["overview.md"], "Cargo.toml [workspace]") {
		t.Fatalf("overview.md should cite the Cargo workspace:\n%s", pages["overview.md"])
	}
}

func TestRenderWikiMonorepoIgnoresNestedDocsAndTestdata(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), "{\n  \"name\": \"x\",\n  \"workspaces\": [\"apps/*\"]\n}\n")
	writeTestFile(t, filepath.Join(root, "docs", "apps", "example", "src", "main.ts"), "export const x = 1\n")
	writeTestFile(t, filepath.Join(root, "testdata", "apps", "foo", "src", "main.ts"), "export const y = 1\n")

	entry := renderWikiForTest(t, root)["entrypoints.md"]
	for _, unwanted := range []string{"docs/apps/example/src/main.ts", "testdata/apps/foo/src/main.ts"} {
		if strings.Contains(entry, unwanted) {
			t.Fatalf("entrypoints.md should not detect nested non-workspace path %q:\n%s", unwanted, entry)
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
