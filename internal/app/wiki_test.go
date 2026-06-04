package app

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

// wikiRunOK runs a Pactum command and fails the test if it exits non-zero,
// returning captured stdout.
func wikiRunOK(t *testing.T, app App, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := app.Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, stderr.String())
	}
	return stdout.String()
}

type wikiSearchResult struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Title    string `json:"title"`
	CodeKind string `json:"code_kind"`
}

type wikiSearchResponse struct {
	Results []wikiSearchResult `json:"results"`
}

func wikiSearch(t *testing.T, app App, args ...string) wikiSearchResponse {
	t.Helper()
	out := wikiRunOK(t, app, append([]string{"search"}, append(args, "--json")...)...)
	var response wikiSearchResponse
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("unmarshal search response: %v\n%s", err, out)
	}
	return response
}

func initVueFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Web App\n")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{
  "name": "web",
  "main": "src/main.ts",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "test": "vitest run",
    "lint": "eslint ."
  },
  "dependencies": { "vue": "^3.4.0" },
  "devDependencies": { "vite": "^5.0.0", "vitest": "^1.0.0" }
}
`)
	mustWriteFile(t, filepath.Join(root, "vite.config.ts"), "import { defineConfig } from 'vite'\nexport default defineConfig({})\n")
	mustWriteFile(t, filepath.Join(root, "tsconfig.json"), "{\n  \"compilerOptions\": {}\n}\n")
	mustWriteFile(t, filepath.Join(root, "src", "main.ts"), "import { createApp } from 'vue'\n")
	mustWriteFile(t, filepath.Join(root, "src", "App.vue"), "<template><div>app</div></template>\n")
	mustWriteFile(t, filepath.Join(root, "src", "components", "Foo.vue"), "<template><span>foo</span></template>\n")

	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

// TestWikiVueFixtureProducesUsefulPages covers Part H #7: a Vite/Vue project
// gets useful wiki pages even though .vue files produce no code items.
func TestWikiVueFixtureProducesUsefulPages(t *testing.T) {
	_, paths := initVueFixture(t)

	structure := mustReadFile(t, paths.WikiStructure)
	if !strings.Contains(structure, "Vue") || !strings.Contains(structure, ".vue files are present") {
		t.Fatalf("structure.md should mention .vue files:\n%s", structure)
	}

	areaSrc := mustReadFile(t, filepath.Join(paths.WikiAreasDir, "src.md"))
	for _, want := range []string{"src/App.vue", "src/components/Foo.vue", "src/main.ts"} {
		if !strings.Contains(areaSrc, want) {
			t.Fatalf("areas/src.md missing %q:\n%s", want, areaSrc)
		}
	}

	entrypoints := mustReadFile(t, paths.WikiEntrypoints)
	if !strings.Contains(entrypoints, "src/main.ts") || !strings.Contains(entrypoints, "entrypoint") {
		t.Fatalf("entrypoints.md should list src/main.ts as a candidate entrypoint:\n%s", entrypoints)
	}
	if !strings.Contains(entrypoints, "vite.config.ts") || !strings.Contains(entrypoints, "Vite config / build-related candidate") {
		t.Fatalf("entrypoints.md should reference vite.config.ts as a build-related candidate:\n%s", entrypoints)
	}

	config := mustReadFile(t, paths.WikiConfig)
	for _, want := range []string{"package.json", "vite.config.ts", "tsconfig.json"} {
		if !strings.Contains(config, want) {
			t.Fatalf("config.md missing %q:\n%s", want, config)
		}
	}

	commands := mustReadFile(t, paths.WikiCommands)
	for _, want := range []string{"vite build", "vitest run", "npm run build"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("commands.md missing package.json script %q:\n%s", want, commands)
		}
	}

	// .vue files must not be expected to produce code items.
	codeItems := readCodeItems(t, paths.CodeItemsJSONL)
	for _, item := range codeItems {
		if strings.HasSuffix(item.Path, ".vue") {
			t.Fatalf("did not expect code items for .vue file: %#v", item)
		}
	}
}

// TestWikiCommandsListMakeAndScripts covers Part H #5 and #6: Makefile targets
// and package.json scripts both appear in commands.md.
func TestWikiCommandsListMakeAndScripts(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/demo\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(root, "Makefile"), "VERSION := 0.1.0\nLDFLAGS := -X main.v=$(VERSION)\n\nbuild:\n\tgo build ./...\n\ntest:\n\tgo test ./...\n\n.PHONY: build test\n")
	mustWriteFile(t, filepath.Join(root, "package.json"), `{
  "name": "demo",
  "scripts": { "start": "node index.js", "test": "jest" }
}
`)
	app := testApp(root)
	wikiRunOK(t, app, "init")
	paths := artifacts.New(root)

	commands := mustReadFile(t, paths.WikiCommands)
	for _, want := range []string{"make build", "make test", "npm run start", "node index.js", "go test ./..."} {
		if !strings.Contains(commands, want) {
			t.Fatalf("commands.md missing %q:\n%s", want, commands)
		}
	}
	// Makefile variable assignments are not build targets.
	for _, unwanted := range []string{"make VERSION", "make LDFLAGS"} {
		if strings.Contains(commands, unwanted) {
			t.Fatalf("commands.md should not list Makefile variable %q:\n%s", unwanted, commands)
		}
	}
}

// TestWikiUnsupportedFrameworkFilesStillUseful covers Part H #8: a repo whose
// files yield zero code items still gets useful wiki pages that mention those
// files.
func TestWikiUnsupportedFrameworkFilesStillUseful(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Frontend only\n")
	mustWriteFile(t, filepath.Join(root, "index.html"), "<!doctype html><html></html>\n")
	mustWriteFile(t, filepath.Join(root, "styles", "app.css"), "body { margin: 0; }\n")
	mustWriteFile(t, filepath.Join(root, "src", "App.vue"), "<template><div/></template>\n")

	app := testApp(root)
	wikiRunOK(t, app, "init")
	paths := artifacts.New(root)

	if items := readCodeItems(t, paths.CodeItemsJSONL); len(items) != 0 {
		t.Fatalf("expected zero code items for unsupported framework files, got %#v", items)
	}

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "src/") || !strings.Contains(overview, "styles/") {
		t.Fatalf("overview.md should mention the top-level areas:\n%s", overview)
	}
	areaSrc := mustReadFile(t, filepath.Join(paths.WikiAreasDir, "src.md"))
	if !strings.Contains(areaSrc, "src/App.vue") {
		t.Fatalf("areas/src.md should mention App.vue:\n%s", areaSrc)
	}
	if !strings.Contains(areaSrc, "No code hints for this area") {
		t.Fatalf("areas/src.md should note the absence of code hints:\n%s", areaSrc)
	}
}

// TestWikiMapRefreshRegeneratesWiki covers Part H #2: a map refresh regenerates
// the wiki artifacts even after they are deleted.
func TestWikiMapRefreshRegeneratesWiki(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/demo\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	paths := artifacts.New(root)

	assertNoError(t, os.RemoveAll(paths.WikiDir))
	wikiRunOK(t, app, "map", "refresh")

	for _, page := range []string{paths.WikiOverview, paths.WikiStructure, paths.WikiCommands, paths.WikiEntrypoints, paths.WikiConfig, paths.WikiTests} {
		assertFile(t, page)
	}
}

// TestWikiSearchIndexesWikiPages covers Part H #9: wiki pages are indexed in
// search and reachable both via "any" and via --kind wiki.
func TestWikiSearchIndexesWikiPages(t *testing.T) {
	root, _ := initVueFixture(t)
	app := testApp(root)

	vite := wikiSearch(t, app, "vite")
	foundWiki := false
	for _, result := range vite.Results {
		if result.Kind == "wiki" && (strings.Contains(result.Path, "wiki/config") || strings.Contains(result.Path, "wiki/entrypoints") || strings.Contains(result.Path, "wiki/commands")) {
			foundWiki = true
		}
	}
	if !foundWiki {
		t.Fatalf("search \"vite\" did not return a wiki config/entrypoints/commands hit: %#v", vite.Results)
	}

	entrypoint := wikiSearch(t, app, "entrypoint", "--kind", "wiki")
	if len(entrypoint.Results) == 0 {
		t.Fatalf("search \"entrypoint\" --kind wiki returned no results")
	}
	for _, result := range entrypoint.Results {
		if result.Kind != "wiki" {
			t.Fatalf("search --kind wiki returned non-wiki result: %#v", result)
		}
	}
}

// TestWikiSearchDeEmphasizesImports covers Part H #10: import-like items are not
// returned under --kind code_item but are returned under --kind import.
func TestWikiSearchDeEmphasizesImports(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/demo\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n\nimport \"github.com/acme/main/client\"\n\nfunc main() {}\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")

	codeItems := wikiSearch(t, app, "main", "--kind", "code_item")
	if len(codeItems.Results) == 0 {
		t.Fatalf("search \"main\" --kind code_item returned no results")
	}
	for _, result := range codeItems.Results {
		if result.Kind != "code_item" {
			t.Fatalf("--kind code_item returned non-code_item: %#v", result)
		}
		if result.CodeKind == "go_import" || result.CodeKind == "go_package" {
			t.Fatalf("--kind code_item should not return import-like entries: %#v", result)
		}
	}

	imports := wikiSearch(t, app, "main", "--kind", "import")
	if len(imports.Results) == 0 {
		t.Fatalf("search \"main\" --kind import returned no import entries")
	}
	for _, result := range imports.Results {
		if result.Kind != "import" {
			t.Fatalf("--kind import returned non-import: %#v", result)
		}
	}
}

// TestWikiPagesArePortable covers Part H #11: generated wiki markdown must not
// embed the absolute repository path.
func TestWikiPagesArePortable(t *testing.T) {
	root, paths := initVueFixture(t)

	err := filepath.WalkDir(paths.WikiDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		content := mustReadFile(t, path)
		if strings.Contains(content, root) {
			t.Fatalf("wiki page %s contains the absolute repo path %q", path, root)
		}
		return nil
	})
	assertNoError(t, err)
}

// TestWikiManifestListsWikiArtifacts covers Part H #12: the map manifest lists
// the wiki artifacts and stays portable (no absolute path, schema unchanged).
func TestWikiManifestListsWikiArtifacts(t *testing.T) {
	root, paths := initVueFixture(t)
	manifest, err := readMapManifest(paths.MapManifest)
	assertNoError(t, err)

	for key, want := range map[string]string{
		"wiki_overview":    "map/wiki/overview.md",
		"wiki_structure":   "map/wiki/structure.md",
		"wiki_commands":    "map/wiki/commands.md",
		"wiki_entrypoints": "map/wiki/entrypoints.md",
		"wiki_config":      "map/wiki/config.md",
		"wiki_tests":       "map/wiki/tests.md",
		"wiki_areas":       "map/wiki/areas/",
	} {
		if manifest.Artifacts[key] != want {
			t.Fatalf("manifest %s = %q, want %q", key, manifest.Artifacts[key], want)
		}
	}
	if manifest.RepoRoot != "." {
		t.Fatalf("manifest repo_root = %q, want . (portable)", manifest.RepoRoot)
	}
	raw := mustReadFile(t, paths.MapManifest)
	if strings.Contains(raw, root) {
		t.Fatalf("manifest embeds absolute repo path %q:\n%s", root, raw)
	}
}
