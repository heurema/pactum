package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
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
	_, paths := initGoCLIFixture(t)

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
	_, paths := initDotNetFixture(t)

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

	// JS files are indexed and searchable via FTS.
	results := wikiSearch(t, app, "index").Results
	if len(results) == 0 {
		t.Fatalf("search \"index\" returned no results")
	}
	found := false
	for _, r := range results {
		if strings.Contains(r.Path, "index.js") {
			found = true
		}
	}
	if !found {
		t.Fatalf("search \"index\" should surface index.js: %#v", results)
	}
}

func TestMapQualityMonorepoEntrypointsSearchable(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "package.json"), "{\n  \"name\": \"monorepo\",\n  \"workspaces\": [\"apps/*\", \"packages/*\"]\n}\n")
	mustWriteFile(t, filepath.Join(root, "apps", "admin", "src", "main.ts"), "export const admin = 1\n")
	mustWriteFile(t, filepath.Join(root, "packages", "ui", "src", "index.ts"), "export const ui = 1\n")
	app := testApp(root)
	wikiRunOK(t, app, "init")
	paths := artifacts.New(root)

	entrypoints := mustReadFile(t, paths.WikiEntrypoints)
	if !strings.Contains(entrypoints, "apps/admin/src/main.ts") {
		t.Fatalf("entrypoints.md should include the monorepo app entrypoint:\n%s", entrypoints)
	}

	// Wiki pages are indexed: monorepo entrypoints are reachable via search.
	admin := wikiSearch(t, app, "admin", "--kind", "wiki").Results
	if len(admin) == 0 {
		t.Fatal("search \"admin\" --kind wiki returned nothing")
	}
	main := wikiSearch(t, app, "main", "--kind", "wiki").Results
	foundEntrypoints := false
	for _, r := range main {
		if strings.HasSuffix(r.Path, "wiki/entrypoints.md") {
			foundEntrypoints = true
		}
	}
	if !foundEntrypoints {
		t.Fatalf("search \"main\" --kind wiki should surface entrypoints.md: %#v", main)
	}
}

func TestMapQualityConfigHeavyRepo(t *testing.T) {
	_, paths := initConfigHeavyFixture(t)

	overview := mustReadFile(t, paths.WikiOverview)
	if !strings.Contains(overview, ".github/") {
		t.Fatalf("overview.md should still list areas:\n%s", overview)
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

func TestSearchPrefersWikiResults(t *testing.T) {
	root, _ := initGoCLIFixture(t)
	app := testApp(root)

	results := wikiSearch(t, app, "main").Results
	if len(results) == 0 {
		t.Fatal("search \"main\" returned no results")
	}

	foundWiki := false
	for _, r := range results {
		if r.Kind == "wiki" {
			foundWiki = true
		}
	}
	if !foundWiki {
		t.Fatalf("search \"main\" should surface wiki results: %#v", results)
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

func looksLikeTestPath(path string) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "test") || strings.Contains(lower, "spec") {
		return true
	}
	return false
}
