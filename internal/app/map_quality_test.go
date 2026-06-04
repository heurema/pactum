package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/codeindex"
)

// --- fixture builders ----------------------------------------------------

func initGoCLIFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/cli\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(root, "Makefile"), "build:\n\tgo build ./...\n\ntest:\n\tgo test ./...\n\n.PHONY: build test\n")
	mustWriteFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n\nimport \"example.com/cli/internal/core\"\n\nfunc main() { core.Run() }\n")
	mustWriteFile(t, filepath.Join(root, "internal", "core", "core.go"), "package core\n\nfunc Run() {}\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

func initPythonFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "pyproject.toml"), "[project]\nname = \"demo\"\nversion = \"0.1.0\"\n")
	mustWriteFile(t, filepath.Join(root, "src", "pkg", "__init__.py"), "\n")
	mustWriteFile(t, filepath.Join(root, "src", "pkg", "module.py"), "def handler():\n    return 1\n")
	mustWriteFile(t, filepath.Join(root, "tests", "test_module.py"), "def test_handler():\n    assert True\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

func initDotNetFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "global.json"), "{\n  \"sdk\": { \"version\": \"8.0.100\" }\n}\n")
	mustWriteFile(t, filepath.Join(root, "MyApp.sln"), "Microsoft Visual Studio Solution File, Format Version 12.00\n")
	mustWriteFile(t, filepath.Join(root, "src", "MyApp", "MyApp.csproj"), "<Project Sdk=\"Microsoft.NET.Sdk\">\n  <PropertyGroup>\n    <TargetFramework>net8.0</TargetFramework>\n  </PropertyGroup>\n</Project>\n")
	mustWriteFile(t, filepath.Join(root, "src", "MyApp", "Program.cs"), "namespace MyApp;\n\npublic class Program\n{\n    public static void Main()\n    {\n    }\n}\n\npublic class Greeter\n{\n    public string Greet()\n    {\n        return \"hi\";\n    }\n}\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

func initMavenFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "pom.xml"), "<project>\n  <modelVersion>4.0.0</modelVersion>\n  <groupId>com.example</groupId>\n  <artifactId>demo</artifactId>\n  <version>1.0.0</version>\n</project>\n")
	mustWriteFile(t, filepath.Join(root, "src", "main", "java", "App.java"), "package com.example;\n\npublic class App {\n    public static void main(String[] args) {}\n}\n")
	mustWriteFile(t, filepath.Join(root, "src", "test", "java", "AppTest.java"), "package com.example;\n\npublic class AppTest {}\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

func initGradleFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "build.gradle"), "plugins { id 'java' }\n")
	mustWriteFile(t, filepath.Join(root, "settings.gradle"), "rootProject.name = 'demo'\n")
	mustWriteFile(t, filepath.Join(root, "gradlew"), "#!/bin/sh\nexec gradle \"$@\"\n")
	mustWriteFile(t, filepath.Join(root, "src", "main", "java", "App.java"), "package com.example;\n\npublic class App {}\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

func initRustFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "Cargo.toml"), "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")
	mustWriteFile(t, filepath.Join(root, "src", "main.rs"), "fn main() {\n    println!(\"hi\");\n}\n")
	mustWriteFile(t, filepath.Join(root, "src", "lib.rs"), "pub fn run() {}\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

func initConfigHeavyFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Infra\n")
	mustWriteFile(t, filepath.Join(root, "Dockerfile"), "FROM alpine\nRUN echo hi\n")
	mustWriteFile(t, filepath.Join(root, "docker-compose.yml"), "services:\n  web:\n    image: nginx\n")
	mustWriteFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "name: CI\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make build\n")
	mustWriteFile(t, filepath.Join(root, "Makefile"), "build:\n\tdocker build .\n\ndeploy:\n\techo deploy\n\n.PHONY: build deploy\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

// --- Part A: map quality fixture matrix ----------------------------------

func TestMapQualityGoCLIRepo(t *testing.T) {
	root, paths := initGoCLIFixture(t)
	app := testApp(root)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Likely role: Go") || !strings.Contains(overview, "go.mod present") {
		t.Fatalf("overview.md should detect Go with evidence:\n%s", overview)
	}

	commands := mustReadFile(t, paths.WikiCommands)
	for _, want := range []string{"make build", "make test", "go build ./...", "go test ./..."} {
		if !strings.Contains(commands, want) {
			t.Fatalf("commands.md missing %q:\n%s", want, commands)
		}
	}

	entrypoints := mustReadFile(t, paths.WikiEntrypoints)
	if !strings.Contains(entrypoints, "cmd/app/main.go") {
		t.Fatalf("entrypoints.md should list cmd/app/main.go:\n%s", entrypoints)
	}

	found := wikiSearch(t, app, "main", "--kind", "code_item")
	if !hasCodeKind(found.Results, "go_main") {
		t.Fatalf("search \"main\" --kind code_item should find go_main: %#v", found.Results)
	}
}

func TestMapQualityTSVueViteRepo(t *testing.T) {
	root, paths := initVueFixture(t)
	_ = root

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Likely role: frontend") || !strings.Contains(overview, "Evidence:") {
		t.Fatalf("overview.md should detect frontend with evidence:\n%s", overview)
	}
	for _, want := range []string{"vite", "vue", ".vue files are present"} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview.md frontend evidence missing %q:\n%s", want, overview)
		}
	}

	structure := mustReadFile(t, paths.WikiStructure)
	if !strings.Contains(structure, ".vue files are present") {
		t.Fatalf("structure.md should mention .vue files:\n%s", structure)
	}
	areaSrc := mustReadFile(t, filepath.Join(paths.WikiAreasDir, "src.md"))
	if !strings.Contains(areaSrc, "src/App.vue") || !strings.Contains(areaSrc, "src/components/Foo.vue") {
		t.Fatalf("areas/src.md should mention .vue files:\n%s", areaSrc)
	}

	commands := mustReadFile(t, paths.WikiCommands)
	if !strings.Contains(commands, "vite build") {
		t.Fatalf("commands.md should include package.json scripts:\n%s", commands)
	}

	entrypoints := mustReadFile(t, paths.WikiEntrypoints)
	if !strings.Contains(entrypoints, "src/main.ts") {
		t.Fatalf("entrypoints.md should include src/main.ts:\n%s", entrypoints)
	}

	config := mustReadFile(t, paths.WikiConfig)
	if !strings.Contains(config, "vite.config.ts") {
		t.Fatalf("config.md should include vite.config.ts:\n%s", config)
	}

	for _, item := range readCodeItems(t, paths.CodeItemsJSONL) {
		if strings.HasSuffix(item.Path, ".vue") {
			t.Fatalf(".vue files should not produce code items: %#v", item)
		}
	}
}

func TestMapQualityPythonRepo(t *testing.T) {
	_, paths := initPythonFixture(t)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Likely role: Python") || !strings.Contains(overview, "pyproject.toml present") {
		t.Fatalf("overview.md should detect Python with evidence:\n%s", overview)
	}

	tests := mustReadFile(t, paths.WikiTests)
	if !strings.Contains(tests, "tests/test_module.py") {
		t.Fatalf("tests.md should detect the test file:\n%s", tests)
	}

	commands := mustReadFile(t, paths.WikiCommands)
	if !strings.Contains(commands, "pytest") {
		t.Fatalf("commands.md should include evidence-backed Python hints (pytest):\n%s", commands)
	}
	// No Go/Make/package.json in this repo: those command groups must report
	// nothing rather than guessing.
	for _, unwanted := range []string{"make ", "go build", "npm run"} {
		if strings.Contains(commands, unwanted) {
			t.Fatalf("commands.md should not guess %q for a Python-only repo:\n%s", unwanted, commands)
		}
	}
}

func TestMapQualityDotNetRepo(t *testing.T) {
	root, paths := initDotNetFixture(t)
	app := testApp(root)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Likely role: C# / .NET") {
		t.Fatalf("overview.md should detect a .NET ecosystem:\n%s", overview)
	}
	for _, want := range []string{"solution file present", "global.json present"} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview.md .NET evidence missing %q:\n%s", want, overview)
		}
	}

	config := mustReadFile(t, paths.WikiConfig)
	for _, want := range []string{"src/MyApp/MyApp.csproj", "MyApp.sln", "global.json"} {
		if !strings.Contains(config, want) {
			t.Fatalf("config.md missing .NET file %q:\n%s", want, config)
		}
	}

	commands := mustReadFile(t, paths.WikiCommands)
	for _, want := range []string{"dotnet build", "dotnet test"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("commands.md missing .NET command %q:\n%s", want, commands)
		}
	}

	entrypoints := mustReadFile(t, paths.WikiEntrypoints)
	if !strings.Contains(entrypoints, "src/MyApp/Program.cs") || !strings.Contains(entrypoints, "Program.cs") {
		t.Fatalf("entrypoints.md should list Program.cs as a candidate:\n%s", entrypoints)
	}

	// C# is a supported language: definitions are extracted.
	if !hasCodeItemKind(readCodeItems(t, paths.CodeItemsJSONL), "cs_class") {
		t.Fatalf("expected C# code items (cs_class) from Program.cs")
	}

	// using/namespace markers stay out of code_item search.
	found := wikiSearch(t, app, "Greeter", "--kind", "code_item").Results
	if !hasCodeKind(found, "cs_class") {
		t.Fatalf("search \"Greeter\" --kind code_item should find the cs_class: %#v", found)
	}
}

func TestMapQualityMavenRepo(t *testing.T) {
	_, paths := initMavenFixture(t)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Likely role: Java (Maven)") || !strings.Contains(overview, "pom.xml present") {
		t.Fatalf("overview.md should detect Java/Maven with evidence:\n%s", overview)
	}

	config := mustReadFile(t, paths.WikiConfig)
	if !strings.Contains(config, "pom.xml") {
		t.Fatalf("config.md should list pom.xml:\n%s", config)
	}

	commands := mustReadFile(t, paths.WikiCommands)
	for _, want := range []string{"mvn test", "mvn package"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("commands.md missing Maven command %q:\n%s", want, commands)
		}
	}

	tests := mustReadFile(t, paths.WikiTests)
	if !strings.Contains(tests, "src/test/java/AppTest.java") {
		t.Fatalf("tests.md should recognize src/test/java:\n%s", tests)
	}

	// Java symbols are not parsed.
	if items := readCodeItems(t, paths.CodeItemsJSONL); len(items) != 0 {
		t.Fatalf("Java repo should have zero code items, got %#v", items)
	}
}

func TestMapQualityGradleRepo(t *testing.T) {
	_, paths := initGradleFixture(t)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Likely role: JVM (Gradle)") {
		t.Fatalf("overview.md should detect a Gradle/JVM ecosystem:\n%s", overview)
	}

	config := mustReadFile(t, paths.WikiConfig)
	if !strings.Contains(config, "build.gradle") {
		t.Fatalf("config.md should list build.gradle:\n%s", config)
	}

	commands := mustReadFile(t, paths.WikiCommands)
	// gradlew is present, so wrapper commands are preferred.
	for _, want := range []string{"./gradlew test", "./gradlew build"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("commands.md missing Gradle wrapper command %q:\n%s", want, commands)
		}
	}
}

func TestMapQualityRustRepo(t *testing.T) {
	_, paths := initRustFixture(t)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Likely role: Rust") {
		t.Fatalf("overview.md should detect Rust:\n%s", overview)
	}

	entrypoints := mustReadFile(t, paths.WikiEntrypoints)
	if !strings.Contains(entrypoints, "src/main.rs") {
		t.Fatalf("entrypoints.md should list src/main.rs:\n%s", entrypoints)
	}
	if !strings.Contains(entrypoints, "Rust binary entrypoint") || !strings.Contains(entrypoints, "Cargo.toml present") {
		t.Fatalf("entrypoints.md should cite Cargo.toml and the Rust binary convention:\n%s", entrypoints)
	}

	// Rust symbols are not parsed.
	if items := readCodeItems(t, paths.CodeItemsJSONL); len(items) != 0 {
		t.Fatalf("Rust repo should have zero code items, got %#v", items)
	}
}

func initExpressFixture(t *testing.T) (string, artifacts.Paths) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "package.json"), `{
  "name": "express-like",
  "main": "index.js",
  "scripts": { "test": "mocha" }
}
`)
	mustWriteFile(t, filepath.Join(root, "index.js"), `const express = require("express")

function createApplication() {}

module.exports = createApplication
module.exports.Router = require("router")
`)
	mustWriteFile(t, filepath.Join(root, "lib", "application.js"), `const express = require("express")

function createApplication() {}

module.exports = createApplication
`)
	mustWriteFile(t, filepath.Join(root, "lib", "router", "index.js"), `const debug = require("debug")

function Router() {}

module.exports = Router
`)
	app := testApp(root)
	wikiRunOK(t, app, "init")
	return root, artifacts.New(root)
}

func TestMapQualityCommonJSExpress(t *testing.T) {
	root, paths := initExpressFixture(t)
	app := testApp(root)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, "Node.js / JavaScript") {
		t.Fatalf("overview.md should detect Node.js:\n%s", overview)
	}

	entrypoints := mustReadFile(t, paths.WikiEntrypoints)
	if !strings.Contains(entrypoints, "index.js") {
		t.Fatalf("entrypoints.md should include index.js:\n%s", entrypoints)
	}

	if items := readCodeItems(t, paths.CodeItemsJSONL); len(items) == 0 {
		t.Fatal("CommonJS repo should produce non-zero code items")
	}

	// require(...) is searchable as an import.
	imports := wikiSearch(t, app, "express", "--kind", "import").Results
	if !hasCodeKind(imports, "js_import") {
		t.Fatalf("search \"express\" --kind import should find a js_import: %#v", imports)
	}

	// module.exports identifier is searchable as a code_item.
	defs := wikiSearch(t, app, "createApplication", "--kind", "code_item").Results
	if !hasCodeKind(defs, "js_export") {
		t.Fatalf("search \"createApplication\" --kind code_item should find a js_export: %#v", defs)
	}

	// require items must not pollute code_item search.
	for _, r := range wikiSearch(t, app, "express", "--kind", "code_item").Results {
		if r.Kind == "import" || r.CodeKind == "js_import" {
			t.Fatalf("--kind code_item should not return require/js_import entries: %#v", r)
		}
	}
}

func TestMapQualityConfigHeavyRepo(t *testing.T) {
	_, paths := initConfigHeavyFixture(t)

	if items := readCodeItems(t, paths.CodeItemsJSONL); len(items) != 0 {
		t.Fatalf("config-heavy repo should have zero code items, got %#v", items)
	}

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, ".github/") {
		t.Fatalf("overview.md should still list areas without code items:\n%s", overview)
	}

	config := mustReadFile(t, paths.WikiConfig)
	for _, want := range []string{"Dockerfile", "docker-compose.yml", ".github/workflows/ci.yml", "Makefile"} {
		if !strings.Contains(config, want) {
			t.Fatalf("config.md missing %q:\n%s", want, config)
		}
	}

	commands := mustReadFile(t, paths.WikiCommands)
	for _, want := range []string{"make build", "make deploy"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("commands.md missing %q:\n%s", want, commands)
		}
	}
	// CI workflow `run:` command is surfaced.
	if !strings.Contains(commands, "## CI workflow commands") {
		t.Fatalf("commands.md should have a CI workflow section:\n%s", commands)
	}

	// tests.md must still be generated and usable even with no tests.
	tests := mustReadFile(t, paths.WikiTests)
	if !strings.Contains(tests, "# Tests") {
		t.Fatalf("tests.md should still be generated:\n%s", tests)
	}
}

// --- Part B: search quality ----------------------------------------------

func TestSearchPrefersDefinitionsAndWikiOverImports(t *testing.T) {
	root, _ := initGoCLIFixture(t)
	app := testApp(root)

	results := wikiSearch(t, app, "main").Results
	if len(results) == 0 {
		t.Fatal("search \"main\" returned no results")
	}
	if results[0].Kind == "import" {
		t.Fatalf("top result for \"main\" should not be an import: %#v", results)
	}

	firstGood, firstImport := -1, -1
	for i, r := range results {
		if firstGood < 0 && (r.Kind == "code_item" || r.Kind == "wiki") {
			firstGood = i
		}
		if firstImport < 0 && r.Kind == "import" {
			firstImport = i
		}
	}
	if firstGood < 0 {
		t.Fatalf("expected a code_item or wiki result for \"main\": %#v", results)
	}
	if firstImport >= 0 && firstImport < firstGood {
		t.Fatalf("import-like result ranked above definitions/wiki for \"main\": %#v", results)
	}
}

func TestSearchKindCodeItemExcludesImports(t *testing.T) {
	root, _ := initGoCLIFixture(t)
	app := testApp(root)

	results := wikiSearch(t, app, "main", "--kind", "code_item").Results
	if len(results) == 0 {
		t.Fatal("search \"main\" --kind code_item returned nothing")
	}
	for _, r := range results {
		if r.Kind != "code_item" {
			t.Fatalf("--kind code_item returned non-code_item: %#v", r)
		}
		if r.CodeKind == "go_import" || r.CodeKind == "go_package" {
			t.Fatalf("--kind code_item should exclude import-like entries: %#v", r)
		}
	}
}

func TestSearchKindImportReturnsImports(t *testing.T) {
	root, _ := initGoCLIFixture(t)
	app := testApp(root)

	results := wikiSearch(t, app, "main", "--kind", "import").Results
	if len(results) == 0 {
		t.Fatal("search \"main\" --kind import returned nothing")
	}
	for _, r := range results {
		if r.Kind != "import" {
			t.Fatalf("--kind import returned non-import: %#v", r)
		}
	}
}

func TestSearchViteKindWikiReturnsWikiHits(t *testing.T) {
	root, _ := initVueFixture(t)
	app := testApp(root)

	results := wikiSearch(t, app, "vite", "--kind", "wiki").Results
	if len(results) == 0 {
		t.Fatal("search \"vite\" --kind wiki returned nothing")
	}
	relevant := false
	for _, r := range results {
		if r.Kind != "wiki" {
			t.Fatalf("--kind wiki returned non-wiki: %#v", r)
		}
		if strings.Contains(r.Path, "wiki/config") || strings.Contains(r.Path, "wiki/entrypoints") || strings.Contains(r.Path, "wiki/commands") {
			relevant = true
		}
	}
	if !relevant {
		t.Fatalf("search \"vite\" --kind wiki should surface config/entrypoints/commands: %#v", results)
	}
}

func TestSearchTestSurfacesWikiTestsPage(t *testing.T) {
	root, _ := initPythonFixture(t)
	app := testApp(root)

	results := wikiSearch(t, app, "test").Results
	if len(results) == 0 {
		t.Fatal("search \"test\" returned nothing")
	}

	testsRank := -1
	for i, r := range results {
		if r.Kind == "wiki" && strings.HasSuffix(r.Path, "wiki/tests.md") {
			testsRank = i
			break
		}
	}
	if testsRank < 0 {
		t.Fatalf("search \"test\" should surface wiki/tests.md: %#v", results)
	}
	// No clearly-unrelated file (not a test, not config) should outrank the
	// tests wiki page.
	for i := 0; i < testsRank; i++ {
		r := results[i]
		if r.Kind == "file" && !looksLikeTestPath(r.Path) && r.CodeKind != "config" {
			t.Fatalf("unrelated file %q ranked above wiki/tests.md: %#v", r.Path, results)
		}
	}
}

func hasCodeKind(results []wikiSearchResult, codeKind string) bool {
	for _, r := range results {
		if r.CodeKind == codeKind {
			return true
		}
	}
	return false
}

func hasCodeItemKind(items []codeindex.Item, kind string) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func looksLikeTestPath(path string) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "test") || strings.Contains(lower, "spec") {
		return true
	}
	return false
}
