package projectmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WikiPage is one generated page of the deterministic map wiki. RelPath is
// relative to the wiki directory (for example "overview.md" or
// "areas/src.md"). The wiki is generated from deterministic facts only — file
// inventory, package manifests, and conventional paths.
type WikiPage struct {
	RelPath string
	Title   string
	Content []byte
}

// roleEvidence captures an inferred, conservative role together with the
// deterministic facts that support it. A role is never asserted without
// evidence.
type roleEvidence struct {
	Role     string
	Evidence []string
}

type scriptEntry struct {
	Name    string
	Command string
}

type packageManifest struct {
	Main       string
	Bin        []string
	Scripts    []scriptEntry
	Deps       []string
	Workspaces []string
}

type entrypointFact struct {
	Path     string
	Reasons  []string
	Declared bool
	Library  bool
}

type configFact struct {
	Path string
	Role string
}

type areaFacts struct {
	Name        string
	Role        roleEvidence
	Files       []FileRecord
	Subdirs     []string
	Languages   []languageItem
	Entrypoints []entrypointFact
	Configs     []string
	Tests       []string
}

type wikiFacts struct {
	generatedAt time.Time
	scan        ScanResult

	hasGoMod        bool
	goModule        string
	hasPyProject    bool
	hasRequirements bool
	hasSetupPy      bool
	hasCargo        bool

	hasDotNet       bool
	dotnetProjects  []string
	dotnetSolutions []string
	hasGlobalJSON   bool
	hasNuGetConfig  bool

	hasPomXML      bool
	hasGradle      bool
	hasGradlew     bool
	mavenPoms      []string
	gradleBuilds   []string
	gradleWrappers []string

	hasWorkspace     bool
	cargoWorkspace   bool
	workspaceMarkers []string

	pkg         *packageManifest
	makeTargets []string
	ciWorkflows []string
	ciCommands  []string

	ecosystems  []roleEvidence
	areas       []areaFacts
	entrypoints []entrypointFact
	configs     []configFact
	tests       wikiTestFacts
	commands    wikiCommandFacts
}

type wikiTestFacts struct {
	Files      []string
	Configs    []string
	Scripts    []scriptEntry
	Validation []string
}

type wikiCommandFacts struct {
	MakeTargets    []string
	Scripts        []scriptEntry
	GoCommands     []string
	PyCommands     []string
	DotNetCommands []string
	JVMCommands    []string
	CICommands     []string
}

// RenderWiki gathers deterministic facts from the repository and renders the
// map-wiki pages. Reads of optional manifest files (package.json, Makefile, CI
// workflows) are best-effort: an unreadable or absent file simply contributes
// no facts. The returned pages never embed the absolute repository root, so the
// generated wiki is portable.
func RenderWiki(root string, generatedAt time.Time, scan ScanResult) []WikiPage {
	facts := gatherWikiFacts(root, generatedAt, scan)

	pages := []WikiPage{
		{RelPath: "overview.md", Title: "Project map overview", Content: renderOverview(facts)},
		{RelPath: "structure.md", Title: "Project structure", Content: renderStructure(facts)},
		{RelPath: "commands.md", Title: "Commands", Content: renderCommands(facts)},
		{RelPath: "entrypoints.md", Title: "Candidate entrypoints", Content: renderEntrypoints(facts)},
		{RelPath: "config.md", Title: "Configuration", Content: renderConfig(facts)},
		{RelPath: "tests.md", Title: "Tests", Content: renderTests(facts)},
	}
	for _, area := range facts.areas {
		pages = append(pages, WikiPage{
			RelPath: "areas/" + areaFileName(area.Name),
			Title:   "Area: " + area.Name,
			Content: renderArea(facts, area),
		})
	}
	return pages
}

func gatherWikiFacts(root string, generatedAt time.Time, scan ScanResult) wikiFacts {
	facts := wikiFacts{generatedAt: generatedAt, scan: scan}

	present := make(map[string]bool, len(scan.Files))
	for _, file := range scan.Files {
		present[file.Path] = true
	}
	has := func(path string) bool { return present[path] }

	facts.hasGoMod = has("go.mod")
	facts.hasPyProject = has("pyproject.toml")
	facts.hasRequirements = has("requirements.txt") || has("requirements-dev.txt")
	facts.hasSetupPy = has("setup.py") || has("setup.cfg")
	facts.hasCargo = has("Cargo.toml")

	// .NET project layout is identified by file extension (.csproj/.fsproj/
	// .vbproj/.sln) rather than a single fixed manifest name, so it is gathered
	// from the file inventory. scan.Files is already sorted by path, so the
	// collected slices stay deterministic.
	for _, file := range scan.Files {
		base := pathBase(file.Path)
		switch {
		case strings.HasSuffix(base, ".csproj"), strings.HasSuffix(base, ".fsproj"), strings.HasSuffix(base, ".vbproj"):
			facts.dotnetProjects = append(facts.dotnetProjects, file.Path)
		case strings.HasSuffix(base, ".sln"):
			facts.dotnetSolutions = append(facts.dotnetSolutions, file.Path)
		case base == "global.json":
			facts.hasGlobalJSON = true
		case strings.EqualFold(base, "nuget.config"):
			facts.hasNuGetConfig = true
		case base == "pom.xml":
			facts.mavenPoms = append(facts.mavenPoms, file.Path)
		case base == "build.gradle", base == "build.gradle.kts":
			facts.gradleBuilds = append(facts.gradleBuilds, file.Path)
		case base == "gradlew", base == "gradlew.bat":
			facts.gradleWrappers = append(facts.gradleWrappers, file.Path)
		}
	}
	facts.hasDotNet = len(facts.dotnetProjects) > 0 || len(facts.dotnetSolutions) > 0
	facts.hasPomXML = len(facts.mavenPoms) > 0
	facts.hasGradle = len(facts.gradleBuilds) > 0
	facts.hasGradlew = len(facts.gradleWrappers) > 0

	if facts.hasGoMod {
		facts.goModule = readGoModule(filepath.Join(root, "go.mod"))
	}
	if has("package.json") {
		facts.pkg = readPackageManifest(filepath.Join(root, "package.json"))
	}
	if has("Makefile") {
		facts.makeTargets = readMakeTargets(filepath.Join(root, "Makefile"))
	}

	// Shallow workspace/monorepo detection: manifest field extraction or simple
	// file presence only — no package graph.
	if facts.hasCargo {
		if data, err := os.ReadFile(filepath.Join(root, "Cargo.toml")); err == nil {
			facts.cargoWorkspace = strings.Contains(string(data), "[workspace]")
		}
	}
	if facts.pkg != nil && len(facts.pkg.Workspaces) > 0 {
		facts.workspaceMarkers = append(facts.workspaceMarkers, "package.json workspaces ("+strings.Join(facts.pkg.Workspaces, ", ")+")")
	}
	for _, marker := range []struct{ file, label string }{
		{"pnpm-workspace.yaml", "pnpm-workspace.yaml"},
		{"turbo.json", "turbo.json"},
		{"nx.json", "nx.json"},
		{"lerna.json", "lerna.json"},
	} {
		if has(marker.file) {
			facts.workspaceMarkers = append(facts.workspaceMarkers, marker.label)
		}
	}
	if facts.cargoWorkspace {
		facts.workspaceMarkers = append(facts.workspaceMarkers, "Cargo.toml [workspace]")
	}
	facts.hasWorkspace = len(facts.workspaceMarkers) > 0

	facts.ciWorkflows = ciWorkflowFiles(scan.Files)
	for _, workflow := range facts.ciWorkflows {
		facts.ciCommands = append(facts.ciCommands, readRunCommands(filepath.Join(root, filepath.FromSlash(workflow)))...)
	}
	facts.ciCommands = dedupeStrings(facts.ciCommands)

	facts.ecosystems = detectEcosystems(facts, has)
	facts.entrypoints = detectEntrypoints(facts)
	facts.configs = detectConfigs(scan.Files)
	facts.tests = detectTests(facts)
	facts.commands = detectCommands(facts)
	facts.areas = buildAreas(facts)

	return facts
}

// --- ecosystem detection -------------------------------------------------

func detectEcosystems(facts wikiFacts, has func(string) bool) []roleEvidence {
	var detected []roleEvidence

	if facts.hasGoMod {
		evidence := []string{"go.mod present"}
		if facts.goModule != "" {
			evidence = append(evidence, "module path is "+facts.goModule)
		}
		if n := facts.scan.Languages["Go"]; n > 0 {
			evidence = append(evidence, fmt.Sprintf("%d Go file(s) detected", n))
		}
		detected = append(detected, roleEvidence{Role: "Go", Evidence: evidence})
	}

	if facts.pkg != nil {
		evidence := []string{"package.json present"}
		if len(facts.pkg.Scripts) > 0 {
			evidence = append(evidence, fmt.Sprintf("%d package.json script(s)", len(facts.pkg.Scripts)))
		}
		for _, marker := range facts.workspaceMarkers {
			if marker != "Cargo.toml [workspace]" {
				evidence = append(evidence, "monorepo workspace: "+marker)
			}
		}
		detected = append(detected, roleEvidence{Role: "Node.js / JavaScript", Evidence: evidence})
	}

	if frontend := detectFrontend(facts, has); frontend != nil {
		detected = append(detected, *frontend)
	}

	if facts.hasPyProject || facts.hasRequirements || facts.hasSetupPy || facts.scan.Languages["Python"] > 0 {
		var evidence []string
		if facts.hasPyProject {
			evidence = append(evidence, "pyproject.toml present")
		}
		if facts.hasSetupPy {
			evidence = append(evidence, "setup.py/setup.cfg present")
		}
		if facts.hasRequirements {
			evidence = append(evidence, "requirements.txt present")
		}
		if n := facts.scan.Languages["Python"]; n > 0 {
			evidence = append(evidence, fmt.Sprintf("%d Python file(s) detected", n))
		}
		if len(evidence) > 0 {
			detected = append(detected, roleEvidence{Role: "Python", Evidence: evidence})
		}
	}

	if facts.hasDotNet {
		var evidence []string
		if len(facts.dotnetSolutions) > 0 {
			evidence = append(evidence, "solution file present ("+facts.dotnetSolutions[0]+")")
		}
		if len(facts.dotnetProjects) > 0 {
			evidence = append(evidence, fmt.Sprintf("%d .NET project file(s), e.g. %s", len(facts.dotnetProjects), facts.dotnetProjects[0]))
		}
		if facts.hasGlobalJSON {
			evidence = append(evidence, "global.json present")
		}
		if facts.hasNuGetConfig {
			evidence = append(evidence, "nuget.config present")
		}
		if n := facts.scan.Languages["C#"]; n > 0 {
			evidence = append(evidence, fmt.Sprintf("%d C# file(s) detected", n))
		}
		detected = append(detected, roleEvidence{Role: "C# / .NET", Evidence: evidence})
	}

	if facts.hasPomXML {
		evidence := []string{"pom.xml present (" + facts.mavenPoms[0] + ")"}
		if n := facts.scan.Languages["Java"]; n > 0 {
			evidence = append(evidence, fmt.Sprintf("%d Java file(s) detected", n))
		}
		detected = append(detected, roleEvidence{Role: "Java (Maven)", Evidence: evidence})
	}

	if facts.hasGradle {
		evidence := []string{"Gradle build file present (" + facts.gradleBuilds[0] + ")"}
		if facts.hasGradlew {
			evidence = append(evidence, "Gradle wrapper present ("+facts.gradleWrappers[0]+")")
		}
		if n := facts.scan.Languages["Java"]; n > 0 {
			evidence = append(evidence, fmt.Sprintf("%d Java file(s) detected", n))
		}
		detected = append(detected, roleEvidence{Role: "JVM (Gradle)", Evidence: evidence})
	}

	if facts.hasCargo {
		evidence := []string{"Cargo.toml present"}
		if facts.cargoWorkspace {
			evidence = append(evidence, "Cargo.toml [workspace]")
		}
		detected = append(detected, roleEvidence{Role: "Rust", Evidence: evidence})
	}

	return detected
}

// detectFrontend assigns a "frontend" role only when there is app-level
// evidence, not merely build tooling. Vite appearing as a devDependency or a
// stray .tsx test fixture is not enough: a library that uses Vite/Vitest for
// its own tests should not be called a frontend app. The signals that qualify
// are framework component files (.vue/.svelte), a conventional app entrypoint
// (src/main.*, src/App.*, index.html), a Vite config paired with such an
// entrypoint, or a framework dependency paired with app-like source structure.
func detectFrontend(facts wikiFacts, has func(string) bool) *roleEvidence {
	hasVue := facts.scan.Languages["Vue"] > 0
	hasSvelte := facts.scan.Languages["Svelte"] > 0

	viteConfig := ""
	for _, candidate := range []string{"vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.cjs"} {
		if has(candidate) {
			viteConfig = candidate
			break
		}
	}

	var frameworkDeps []string
	if facts.pkg != nil {
		for _, dep := range []string{"vue", "react", "svelte", "next", "nuxt", "@vue/cli-service"} {
			if containsString(facts.pkg.Deps, dep) {
				frameworkDeps = append(frameworkDeps, dep)
			}
		}
	}

	appEntry := frontendAppEntry(has)
	appLike := appEntry != "" || facts.hasFrontendStructure() || hasVue || hasSvelte

	isFrontend := hasVue || hasSvelte ||
		(appEntry != "" && facts.pkg != nil) ||
		(viteConfig != "" && appEntry != "") ||
		(len(frameworkDeps) > 0 && appLike)
	if !isFrontend {
		return nil
	}

	var evidence []string
	for _, dep := range frameworkDeps {
		evidence = append(evidence, "package.json depends on "+dep)
	}
	if facts.pkg != nil && containsString(facts.pkg.Deps, "vite") {
		evidence = append(evidence, "package.json depends on vite")
	}
	if viteConfig != "" {
		evidence = append(evidence, viteConfig+" exists")
	}
	if hasVue {
		evidence = append(evidence, ".vue files are present")
	}
	if hasSvelte {
		evidence = append(evidence, ".svelte files are present")
	}
	if appEntry != "" {
		evidence = append(evidence, "app entrypoint "+appEntry+" present")
	}
	if facts.hasFrontendStructure() {
		evidence = append(evidence, "component/view source structure present")
	}
	return &roleEvidence{Role: "frontend", Evidence: dedupeStrings(evidence)}
}

// frontendAppEntry returns the first conventional frontend application
// entrypoint found in the inventory, or "" if none.
func frontendAppEntry(has func(string) bool) string {
	for _, candidate := range []string{
		"src/main.ts", "src/main.tsx", "src/main.js", "src/main.jsx", "src/main.mjs",
		"src/App.vue", "src/App.tsx", "src/App.jsx", "src/App.svelte",
		"src/index.tsx", "src/index.jsx",
		"index.html", "src/index.html", "public/index.html",
	} {
		if has(candidate) {
			return candidate
		}
	}
	return ""
}

// hasFrontendStructure reports whether the repository contains conventional
// frontend component/view directories under src/.
func (facts wikiFacts) hasFrontendStructure() bool {
	for _, file := range facts.scan.Files {
		for _, prefix := range []string{"src/components/", "src/pages/", "src/views/", "src/layouts/"} {
			if strings.HasPrefix(file.Path, prefix) {
				return true
			}
		}
	}
	return false
}

// --- entrypoint detection ------------------------------------------------

func detectEntrypoints(facts wikiFacts) []entrypointFact {
	byPath := map[string]*entrypointFact{}
	add := func(path, reason string, declared, library bool) {
		fact, ok := byPath[path]
		if !ok {
			fact = &entrypointFact{Path: path}
			byPath[path] = fact
		}
		if declared {
			fact.Declared = true
		}
		if library {
			fact.Library = true
		}
		if reason != "" && !containsString(fact.Reasons, reason) {
			fact.Reasons = append(fact.Reasons, reason)
		}
	}

	for _, file := range facts.scan.Files {
		path := file.Path
		base := pathBase(path)
		switch {
		case base == "main.go" && (path == "main.go" || isCmdMainGo(path)):
			add(path, "conventional Go entrypoint (main.go)", false, false)
		case path == "src/main.ts" || path == "src/main.js" || path == "src/main.tsx" || path == "src/main.mjs":
			add(path, "conventional frontend entrypoint (src/main.*)", false, false)
		case path == "src/index.ts" || path == "src/index.js" || path == "src/index.tsx":
			add(path, "conventional source entrypoint (src/index.*)", false, false)
		case path == "src/server/index.ts" || path == "src/server/index.js":
			add(path, "conventional server entrypoint (src/server/index.*)", false, false)
		case path == "index.js" || path == "index.ts" || path == "index.mjs":
			add(path, "conventional package entrypoint (index.*)", false, false)
		case isViteConfig(base):
			add(path, "Vite config / build-related candidate", false, false)
		case base == "Program.cs":
			add(path, "conventional .NET entrypoint (Program.cs)", false, false)
		case facts.hasCargo && (path == "src/main.rs" || path == "main.rs"):
			add(path, "conventional Rust binary entrypoint", false, false)
			add(path, "Cargo.toml present", false, false)
		case facts.hasCargo && strings.HasPrefix(path, "src/bin/") && strings.HasSuffix(path, ".rs"):
			add(path, "conventional Rust binary entrypoint (src/bin/*.rs)", false, false)
			add(path, "Cargo.toml present", false, false)
		default:
			if reason, library, ok := monorepoEntrypoint(path, facts); ok {
				add(path, reason, false, library)
				if len(facts.workspaceMarkers) > 0 {
					add(path, "workspace evidence: "+strings.Join(facts.workspaceMarkers, ", "), false, library)
				}
			}
		}
	}

	if facts.pkg != nil {
		if facts.pkg.Main != "" {
			add(normalizeManifestPath(facts.pkg.Main), `declared as "main" in package.json`, true, false)
		}
		for _, bin := range facts.pkg.Bin {
			add(normalizeManifestPath(bin), "declared as a bin entry in package.json", true, false)
		}
	}

	facts2 := make([]entrypointFact, 0, len(byPath))
	for _, fact := range byPath {
		sort.Strings(fact.Reasons)
		facts2 = append(facts2, *fact)
	}
	sort.Slice(facts2, func(i, j int) bool { return facts2[i].Path < facts2[j].Path })
	return facts2
}

// monorepoEntrypoint classifies a nested workspace path as a conventional
// monorepo entrypoint. Only top-level workspace areas (apps/services/packages/
// libs/crates) qualify, so docs/ or testdata/ copies are not mis-detected. It
// returns the evidence reason, whether the path is a package/library root
// (rather than an executable entrypoint), and whether it matched.
func monorepoEntrypoint(path string, facts wikiFacts) (reason string, library bool, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return "", false, false
	}
	top := parts[0]
	base := parts[len(parts)-1]
	inner := strings.Join(parts[2:], "/") // path under <top>/<name>/

	underSrc := inner == "src/"+base
	atRoot := inner == base

	jsMain := matchesAny(base, "main.ts", "main.tsx", "main.js", "main.jsx", "main.mjs")
	jsIndex := matchesAny(base, "index.ts", "index.tsx", "index.js", "index.jsx", "index.mjs")

	switch top {
	case "apps", "services":
		switch {
		case underSrc && jsMain:
			return "conventional monorepo app entrypoint (" + top + "/*/src/main.*)", false, true
		case underSrc && jsIndex:
			return "conventional monorepo app entrypoint (" + top + "/*/src/index.*)", false, true
		case underSrc && base == "main.rs":
			return "conventional Rust binary entrypoint (" + top + "/*/src/main.rs)", false, true
		}
	case "packages", "libs":
		switch {
		case underSrc && jsIndex:
			return "conventional package/library root (" + top + "/*/src/index.*)", true, true
		case atRoot && jsIndex:
			return "conventional package/library root (" + top + "/*/index.*)", true, true
		}
	case "crates":
		switch {
		case underSrc && base == "main.rs":
			return "conventional Rust binary entrypoint (crates/*/src/main.rs)", false, true
		case underSrc && base == "lib.rs" && (facts.hasCargo || facts.cargoWorkspace):
			return "conventional Rust library root (crates/*/src/lib.rs)", true, true
		}
	}
	return "", false, false
}

func matchesAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

// --- config detection ----------------------------------------------------

func detectConfigs(files []FileRecord) []configFact {
	var configs []configFact
	add := func(path, role string) { configs = append(configs, configFact{Path: path, Role: role}) }

	for _, file := range files {
		path := file.Path
		base := pathBase(path)
		switch {
		case base == "package.json":
			add(path, "Node.js package manifest")
		case base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock":
			add(path, "JavaScript dependency lockfile")
		case base == "go.mod":
			add(path, "Go module definition")
		case base == "go.sum":
			add(path, "Go module checksums")
		case strings.HasSuffix(base, ".csproj"), strings.HasSuffix(base, ".fsproj"), strings.HasSuffix(base, ".vbproj"):
			add(path, ".NET project file")
		case strings.HasSuffix(base, ".sln"):
			add(path, ".NET solution file")
		case base == "global.json":
			add(path, ".NET SDK version pin")
		case strings.EqualFold(base, "nuget.config"):
			add(path, "NuGet configuration")
		case base == "Directory.Build.props" || base == "Directory.Build.targets":
			add(path, "MSBuild shared build configuration")
		case base == "pom.xml":
			add(path, "Maven build configuration")
		case base == "build.gradle" || base == "build.gradle.kts":
			add(path, "Gradle build configuration")
		case base == "settings.gradle" || base == "settings.gradle.kts":
			add(path, "Gradle settings")
		case base == "gradlew" || base == "gradlew.bat":
			add(path, "Gradle wrapper script")
		case base == "tsconfig.json" || (strings.HasPrefix(base, "tsconfig.") && strings.HasSuffix(base, ".json")):
			add(path, "TypeScript compiler configuration")
		case isViteConfig(base):
			add(path, "Vite build configuration")
		case strings.HasPrefix(base, "tailwind.config."):
			add(path, "Tailwind CSS configuration")
		case strings.HasPrefix(base, "postcss.config."):
			add(path, "PostCSS configuration")
		case base == ".eslintrc" || strings.HasPrefix(base, ".eslintrc.") || strings.HasPrefix(base, "eslint.config."):
			add(path, "ESLint configuration")
		case base == "pyproject.toml":
			add(path, "Python project configuration")
		case base == "setup.py" || base == "setup.cfg":
			add(path, "Python package configuration")
		case base == "Dockerfile" || strings.HasSuffix(base, ".Dockerfile"):
			add(path, "Docker image definition")
		case base == "docker-compose.yml" || base == "docker-compose.yaml" || base == "compose.yml" || base == "compose.yaml":
			add(path, "Docker Compose configuration")
		case base == "Cargo.toml":
			add(path, "Rust crate manifest")
		case base == "Makefile":
			add(path, "Make build targets")
		case isCIWorkflow(path):
			add(path, "GitHub Actions workflow")
		}
	}

	sort.Slice(configs, func(i, j int) bool { return configs[i].Path < configs[j].Path })
	return configs
}

// --- test detection ------------------------------------------------------

func detectTests(facts wikiFacts) wikiTestFacts {
	var t wikiTestFacts
	for _, file := range facts.scan.Files {
		if isTestFile(file.Path) {
			t.Files = append(t.Files, file.Path)
		}
		if isTestConfig(pathBase(file.Path)) {
			t.Configs = append(t.Configs, file.Path)
		}
	}
	sort.Strings(t.Files)
	sort.Strings(t.Configs)

	if facts.pkg != nil {
		for _, script := range facts.pkg.Scripts {
			if isTestScriptName(script.Name) {
				t.Scripts = append(t.Scripts, script)
			}
		}
	}

	t.Validation = facts.validationCandidates()
	return t
}

func (facts wikiFacts) validationCandidates() []string {
	var candidates []string
	if facts.hasGoMod {
		candidates = append(candidates, "go test ./...", "go vet ./...")
	}
	for _, target := range facts.makeTargets {
		if isTestScriptName(target) || target == "check" {
			candidates = append(candidates, "make "+target)
		}
	}
	if facts.pkg != nil {
		runner := packageRunner(facts)
		for _, script := range facts.pkg.Scripts {
			if isTestScriptName(script.Name) {
				candidates = append(candidates, runner+" "+script.Name)
			}
		}
	}
	if facts.hasPytest() {
		candidates = append(candidates, "pytest")
	}
	if facts.hasDotNet {
		candidates = append(candidates, "dotnet test")
	}
	if facts.hasPomXML {
		candidates = append(candidates, "mvn test")
	}
	if facts.hasGradle {
		if facts.hasGradlew {
			candidates = append(candidates, "./gradlew test")
		} else {
			candidates = append(candidates, "gradle test")
		}
	}
	return dedupeStrings(candidates)
}

func (facts wikiFacts) hasPytest() bool {
	if facts.hasPyProject {
		return true
	}
	for _, file := range facts.scan.Files {
		base := pathBase(file.Path)
		if base == "pytest.ini" || base == "tox.ini" || base == "conftest.py" {
			return true
		}
	}
	return false
}

// --- command detection ---------------------------------------------------

func detectCommands(facts wikiFacts) wikiCommandFacts {
	var c wikiCommandFacts
	c.MakeTargets = facts.makeTargets
	if facts.pkg != nil {
		c.Scripts = facts.pkg.Scripts
	}
	if facts.hasGoMod {
		c.GoCommands = []string{"go build ./...", "go test ./...", "go vet ./..."}
	}
	if facts.hasPytest() {
		c.PyCommands = append(c.PyCommands, "pytest")
	}
	if facts.hasPyProject || facts.hasSetupPy {
		c.PyCommands = append(c.PyCommands, "python -m build")
	}
	c.PyCommands = dedupeStrings(c.PyCommands)
	if facts.hasDotNet {
		c.DotNetCommands = []string{"dotnet build", "dotnet test"}
	}
	c.JVMCommands = facts.jvmCommands()
	c.CICommands = facts.ciCommands
	return c
}

// jvmCommands returns evidence-backed Maven/Gradle command hints. Gradle hints
// prefer the wrapper (`./gradlew`) when a gradlew script is present.
func (facts wikiFacts) jvmCommands() []string {
	var commands []string
	if facts.hasPomXML {
		commands = append(commands, "mvn test", "mvn package")
	}
	if facts.hasGradle {
		if facts.hasGradlew {
			commands = append(commands, "./gradlew test", "./gradlew build")
		} else {
			commands = append(commands, "gradle test", "gradle build")
		}
	}
	return dedupeStrings(commands)
}

// --- area construction ---------------------------------------------------

func buildAreas(facts wikiFacts) []areaFacts {
	byArea := map[string][]FileRecord{}
	for _, file := range facts.scan.Files {
		top := topLevelDir(file.Path)
		if top == "" {
			continue
		}
		byArea[top] = append(byArea[top], file)
	}

	entrypointByArea := map[string][]entrypointFact{}
	for _, ep := range facts.entrypoints {
		top := topLevelDir(ep.Path)
		if top == "" {
			continue
		}
		entrypointByArea[top] = append(entrypointByArea[top], ep)
	}

	configByArea := map[string][]string{}
	for _, cfg := range facts.configs {
		top := topLevelDir(cfg.Path)
		if top == "" {
			continue
		}
		configByArea[top] = append(configByArea[top], cfg.Path)
	}

	names := make([]string, 0, len(byArea))
	for name := range byArea {
		names = append(names, name)
	}
	sort.Strings(names)

	areas := make([]areaFacts, 0, len(names))
	for _, name := range names {
		files := byArea[name]
		area := areaFacts{
			Name:        name,
			Files:       files,
			Subdirs:     areaSubdirs(name, files),
			Languages:   areaLanguages(files),
			Entrypoints: entrypointByArea[name],
			Configs:     configByArea[name],
		}
		var tests []string
		for _, file := range files {
			if isTestFile(file.Path) || isTestConfig(pathBase(file.Path)) {
				tests = append(tests, file.Path)
			}
		}
		sort.Strings(tests)
		area.Tests = tests
		area.Role = inferAreaRole(name, files, area.Languages)
		areas = append(areas, area)
	}
	return areas
}

func inferAreaRole(name string, files []FileRecord, languages []languageItem) roleEvidence {
	dominant := ""
	if len(languages) > 0 {
		dominant = languages[0].Name
	}
	role := func(r string, evidence ...string) roleEvidence {
		return roleEvidence{Role: r, Evidence: dedupeStrings(evidence)}
	}

	switch name {
	case "cmd":
		ev := []string{"directory name 'cmd' is a conventional Go command location"}
		for _, file := range files {
			if isCmdMainGo(file.Path) {
				ev = append(ev, "contains "+file.Path)
			}
		}
		return role("command entrypoints", ev...)
	case "internal", "pkg", "lib", "libs":
		return role("library / internal packages",
			"directory name '"+name+"' conventionally holds importable code",
			dominantEvidence(dominant))
	case "src", "app", "source":
		ev := []string{"directory name '" + name + "' is a conventional source root"}
		ev = append(ev, frontendFileEvidence(files)...)
		ev = append(ev, dominantEvidence(dominant))
		return role("application source", ev...)
	case "docs", "doc", "documentation":
		return role("documentation", "contains documentation files")
	case "test", "tests", "spec", "specs", "__tests__", "e2e":
		return role("tests", "directory name '"+name+"' indicates tests")
	case "scripts", "script", "tools", "tooling", "bin":
		return role("scripts / tooling", dominantEvidence(dominant))
	case ".github":
		return role("CI/CD configuration", "contains GitHub Actions workflow files")
	case "web", "frontend", "ui", "client", "www":
		ev := []string{"directory name '" + name + "' conventionally holds frontend code"}
		ev = append(ev, frontendFileEvidence(files)...)
		return role("frontend", ev...)
	case "api", "server", "backend":
		return role("backend / server", "directory name '"+name+"' conventionally holds server code", dominantEvidence(dominant))
	case "components", "views", "pages":
		ev := []string{"directory name '" + name + "' conventionally holds UI units"}
		ev = append(ev, frontendFileEvidence(files)...)
		return role("UI components / views", ev...)
	case "public", "static", "assets":
		return role("static assets", "directory name '"+name+"' conventionally holds static assets")
	case "config", "configs", "deploy", "deployment", "infra":
		return role("configuration / deployment", "directory name '"+name+"' conventionally holds configuration")
	default:
		if dominant == "" {
			return role("mixed / unclassified files", fmt.Sprintf("%d file(s) with no dominant language", len(files)))
		}
		// Role strings are plain nouns; the "Likely role:" label already carries
		// the hedge, so avoid an awkward "likely <X> code" doubling.
		switch dominant {
		case "JSON", "YAML", "TOML":
			return role("configuration", fmt.Sprintf("%d file(s); dominant content is %s", len(files), dominant))
		case "Markdown", "Text":
			return role("documentation", fmt.Sprintf("%d file(s); dominant content is %s", len(files), dominant))
		case "Shell", "Bash", "Zsh":
			return role("scripts / tooling", fmt.Sprintf("%d file(s); dominant content is %s", len(files), dominant))
		default:
			return role(dominant+" source area", fmt.Sprintf("%d file(s); dominant language is %s", len(files), dominant))
		}
	}
}

func frontendFileEvidence(files []FileRecord) []string {
	var ev []string
	counts := map[string]int{}
	for _, file := range files {
		counts[file.Language]++
	}
	if counts["Vue"] > 0 {
		ev = append(ev, ".vue files are present")
	}
	if counts["Svelte"] > 0 {
		ev = append(ev, ".svelte files are present")
	}
	if counts["TSX"] > 0 || counts["JSX"] > 0 {
		ev = append(ev, ".tsx/.jsx files are present")
	}
	return ev
}

func dominantEvidence(dominant string) string {
	if dominant == "" {
		return ""
	}
	return "dominant language is " + dominant
}

// --- page renderers ------------------------------------------------------

func renderOverview(facts wikiFacts) []byte {
	var b bytes.Buffer
	writeWikiHeader(&b, "Project map overview", facts.generatedAt)

	fmt.Fprintln(&b, "## Detected ecosystems")
	fmt.Fprintln(&b)
	if len(facts.ecosystems) == 0 {
		fmt.Fprintln(&b, "- No ecosystem manifests detected.")
	} else {
		for _, eco := range facts.ecosystems {
			writeRoleEvidence(&b, "Likely role", eco.Role, eco.Evidence)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Top-level areas")
	fmt.Fprintln(&b)
	if len(facts.areas) == 0 {
		fmt.Fprintln(&b, "- No top-level directories detected.")
	} else {
		for _, area := range facts.areas {
			fmt.Fprintf(&b, "- `%s/` — %s (see `areas/%s`)\n", area.Name, area.Role.Role, areaFileName(area.Name))
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Important files")
	fmt.Fprintln(&b)
	if len(facts.scan.Important) == 0 {
		fmt.Fprintln(&b, "- None detected.")
	} else {
		for _, file := range facts.scan.Important {
			fmt.Fprintf(&b, "- `%s`\n", file)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## How to navigate the map")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "- `structure.md` — top-level directories and their likely roles, with evidence.")
	fmt.Fprintln(&b, "- `commands.md` — build, test, and lint commands detected from manifests.")
	fmt.Fprintln(&b, "- `entrypoints.md` — candidate program entrypoints.")
	fmt.Fprintln(&b, "- `config.md` — detected configuration files.")
	fmt.Fprintln(&b, "- `tests.md` — test files, test configs, and validation command candidates.")
	fmt.Fprintln(&b, "- `areas/<area>.md` — a focused page per top-level directory.")
	fmt.Fprintln(&b)

	writeWikiLimitations(&b)
	return b.Bytes()
}

func renderStructure(facts wikiFacts) []byte {
	var b bytes.Buffer
	writeWikiHeader(&b, "Project structure", facts.generatedAt)

	fmt.Fprintln(&b, "## Top-level directories")
	fmt.Fprintln(&b)
	if len(facts.areas) == 0 {
		fmt.Fprintln(&b, "- No top-level directories detected.")
	}
	for _, area := range facts.areas {
		fmt.Fprintf(&b, "### `%s/`\n\n", area.Name)
		writeRoleEvidence(&b, "Likely role", area.Role.Role, area.Role.Evidence)
		if len(area.Languages) > 0 {
			fmt.Fprintf(&b, "Languages: %s\n", languageInline(area.Languages))
		}
		if len(area.Subdirs) > 0 {
			fmt.Fprintf(&b, "Subdirectories: %s\n", joinCodeList(area.Subdirs))
		}
		notable := notableFiles(area.Files)
		if len(notable) > 0 {
			fmt.Fprintln(&b, "Notable files:")
			for _, file := range notable {
				fmt.Fprintf(&b, "- `%s`\n", file)
			}
		}
		fmt.Fprintf(&b, "\nSee `areas/%s` for detail.\n\n", areaFileName(area.Name))
	}

	fmt.Fprintln(&b, "## Package / workspace boundaries")
	fmt.Fprintln(&b)
	boundaries := facts.packageBoundaries()
	if len(boundaries) == 0 {
		fmt.Fprintln(&b, "- No package or workspace manifests detected.")
	} else {
		for _, line := range boundaries {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	return b.Bytes()
}

func (facts wikiFacts) packageBoundaries() []string {
	var lines []string
	if facts.hasGoMod {
		if facts.goModule != "" {
			lines = append(lines, "Go module `"+facts.goModule+"` (`go.mod`)")
		} else {
			lines = append(lines, "Go module (`go.mod`)")
		}
	}
	if facts.pkg != nil {
		lines = append(lines, "Node.js package (`package.json`)")
	}
	if facts.hasPyProject {
		lines = append(lines, "Python project (`pyproject.toml`)")
	}
	if facts.hasDotNet {
		if len(facts.dotnetSolutions) > 0 {
			lines = append(lines, ".NET solution (`"+facts.dotnetSolutions[0]+"`)")
		}
		for _, project := range facts.dotnetProjects {
			lines = append(lines, ".NET project (`"+project+"`)")
		}
	}
	if facts.hasPomXML {
		lines = append(lines, "Maven project (`"+facts.mavenPoms[0]+"`)")
	}
	if facts.hasGradle {
		lines = append(lines, "Gradle project (`"+facts.gradleBuilds[0]+"`)")
	}
	if facts.hasCargo {
		lines = append(lines, "Rust crate (`Cargo.toml`)")
	}
	for _, marker := range facts.workspaceMarkers {
		lines = append(lines, "Workspace: "+marker)
	}
	return lines
}

func renderCommands(facts wikiFacts) []byte {
	var b bytes.Buffer
	writeWikiHeader(&b, "Commands", facts.generatedAt)
	c := facts.commands

	fmt.Fprintln(&b, "All commands below are detected from manifest files. No commands are guessed.")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Makefile targets")
	fmt.Fprintln(&b)
	if len(c.MakeTargets) == 0 {
		fmt.Fprintln(&b, "- No Makefile detected.")
	} else {
		for _, target := range c.MakeTargets {
			fmt.Fprintf(&b, "- `make %s`\n", target)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## package.json scripts")
	fmt.Fprintln(&b)
	if len(c.Scripts) == 0 {
		fmt.Fprintln(&b, "- No package.json scripts detected.")
	} else {
		runner := packageRunner(facts)
		for _, script := range c.Scripts {
			fmt.Fprintf(&b, "- `%s %s` — `%s`\n", runner, script.Name, script.Command)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Go commands")
	fmt.Fprintln(&b)
	if len(c.GoCommands) == 0 {
		fmt.Fprintln(&b, "- No go.mod detected.")
	} else {
		for _, cmd := range c.GoCommands {
			fmt.Fprintf(&b, "- `%s`\n", cmd)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Python commands")
	fmt.Fprintln(&b)
	if len(c.PyCommands) == 0 {
		fmt.Fprintln(&b, "- No Python project/config detected.")
	} else {
		for _, cmd := range c.PyCommands {
			fmt.Fprintf(&b, "- `%s`\n", cmd)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## .NET commands")
	fmt.Fprintln(&b)
	if len(c.DotNetCommands) == 0 {
		fmt.Fprintln(&b, "- No .NET project/solution detected.")
	} else {
		for _, cmd := range c.DotNetCommands {
			fmt.Fprintf(&b, "- `%s`\n", cmd)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## JVM commands")
	fmt.Fprintln(&b)
	if len(c.JVMCommands) == 0 {
		fmt.Fprintln(&b, "- No Maven/Gradle build files detected.")
	} else {
		for _, cmd := range c.JVMCommands {
			fmt.Fprintf(&b, "- `%s`\n", cmd)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## CI workflow commands")
	fmt.Fprintln(&b)
	if len(c.CICommands) == 0 {
		fmt.Fprintln(&b, "- No CI workflow commands detected.")
	} else {
		for _, cmd := range c.CICommands {
			fmt.Fprintf(&b, "- `%s`\n", cmd)
		}
	}
	return b.Bytes()
}

func renderEntrypoints(facts wikiFacts) []byte {
	var b bytes.Buffer
	writeWikiHeader(&b, "Candidate entrypoints", facts.generatedAt)
	fmt.Fprintln(&b, "Entrypoints are marked as candidates unless declared by a package manifest.")
	fmt.Fprintln(&b)
	if len(facts.entrypoints) == 0 {
		fmt.Fprintln(&b, "- No candidate entrypoints detected.")
		return b.Bytes()
	}
	for _, ep := range facts.entrypoints {
		fmt.Fprintf(&b, "- `%s` (%s)\n", ep.Path, entrypointLabel(ep))
		for _, reason := range ep.Reasons {
			fmt.Fprintf(&b, "  - evidence: %s\n", reason)
		}
	}
	return b.Bytes()
}

// entrypointLabel describes how an entrypoint candidate should be presented:
// a declared manifest entry, a package/library root, or a plain candidate.
func entrypointLabel(ep entrypointFact) string {
	switch {
	case ep.Library:
		return "candidate package/library root"
	case ep.Declared:
		return "declared entrypoint"
	default:
		return "candidate entrypoint"
	}
}

func renderConfig(facts wikiFacts) []byte {
	var b bytes.Buffer
	writeWikiHeader(&b, "Configuration", facts.generatedAt)
	fmt.Fprintln(&b, "Detected package and configuration files.")
	fmt.Fprintln(&b)
	if len(facts.configs) == 0 {
		fmt.Fprintln(&b, "- No configuration files detected.")
		return b.Bytes()
	}
	for _, cfg := range facts.configs {
		fmt.Fprintf(&b, "- `%s` — %s\n", cfg.Path, cfg.Role)
	}
	return b.Bytes()
}

func renderTests(facts wikiFacts) []byte {
	var b bytes.Buffer
	writeWikiHeader(&b, "Tests", facts.generatedAt)
	t := facts.tests

	fmt.Fprintln(&b, "## Test files")
	fmt.Fprintln(&b)
	if len(t.Files) == 0 {
		fmt.Fprintln(&b, "- No test files detected.")
	} else {
		for _, file := range capStrings(t.Files, 100) {
			fmt.Fprintf(&b, "- `%s`\n", file)
		}
		if len(t.Files) > 100 {
			fmt.Fprintf(&b, "- … and %d more test file(s).\n", len(t.Files)-100)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Test configuration")
	fmt.Fprintln(&b)
	if len(t.Configs) == 0 {
		fmt.Fprintln(&b, "- No test configuration files detected.")
	} else {
		for _, file := range t.Configs {
			fmt.Fprintf(&b, "- `%s`\n", file)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Test / check / lint scripts")
	fmt.Fprintln(&b)
	if len(t.Scripts) == 0 {
		fmt.Fprintln(&b, "- No test/check/lint scripts detected.")
	} else {
		runner := packageRunner(facts)
		for _, script := range t.Scripts {
			fmt.Fprintf(&b, "- `%s %s` — `%s`\n", runner, script.Name, script.Command)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Validation command candidates")
	fmt.Fprintln(&b)
	if len(t.Validation) == 0 {
		fmt.Fprintln(&b, "- No validation command candidates detected.")
	} else {
		for _, cmd := range t.Validation {
			fmt.Fprintf(&b, "- `%s`\n", cmd)
		}
	}
	return b.Bytes()
}

func renderArea(facts wikiFacts, area areaFacts) []byte {
	var b bytes.Buffer
	writeWikiHeader(&b, "Area: "+area.Name, facts.generatedAt)

	writeRoleEvidence(&b, "Likely role", area.Role.Role, area.Role.Evidence)

	if len(area.Languages) > 0 {
		fmt.Fprintf(&b, "Languages: %s\n\n", languageInline(area.Languages))
	}

	fmt.Fprintln(&b, "## File groups")
	fmt.Fprintln(&b)
	if len(area.Subdirs) > 0 {
		fmt.Fprintln(&b, "Subdirectories:")
		for _, dir := range area.Subdirs {
			fmt.Fprintf(&b, "- `%s/`\n", dir)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b, "Files:")
	for _, file := range capRecords(area.Files, 80) {
		fmt.Fprintf(&b, "- `%s`\n", file.Path)
	}
	if len(area.Files) > 80 {
		fmt.Fprintf(&b, "- … and %d more file(s).\n", len(area.Files)-80)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Candidate entrypoints")
	fmt.Fprintln(&b)
	if len(area.Entrypoints) == 0 {
		fmt.Fprintln(&b, "- None detected in this area.")
	} else {
		for _, ep := range area.Entrypoints {
			fmt.Fprintf(&b, "- `%s` (%s)\n", ep.Path, entrypointLabel(ep))
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Tests and configuration")
	fmt.Fprintln(&b)
	if len(area.Tests) == 0 && len(area.Configs) == 0 {
		fmt.Fprintln(&b, "- No tests or configuration files detected in this area.")
	} else {
		for _, file := range area.Configs {
			fmt.Fprintf(&b, "- config: `%s`\n", file)
		}
		for _, file := range capStrings(area.Tests, 40) {
			fmt.Fprintf(&b, "- test: `%s`\n", file)
		}
	}
	return b.Bytes()
}

// --- shared rendering helpers --------------------------------------------

func writeWikiHeader(b *bytes.Buffer, title string, generatedAt time.Time) {
	fmt.Fprintf(b, "# %s\n\n", title)
	fmt.Fprintf(b, "Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	fmt.Fprintln(b, "This page is part of the deterministic map wiki. It is generated from")
	fmt.Fprintln(b, "deterministic facts (file inventory and manifests), not from an LLM.")
	fmt.Fprintln(b)
}

func writeWikiLimitations(b *bytes.Buffer) {
	fmt.Fprintln(b, "## Limitations")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "- This wiki is deterministic, not LLM-generated.")
	fmt.Fprintln(b, "- Inferred roles are conservative guesses backed by evidence; verify against the source.")
	fmt.Fprintln(b, "- Source files remain the source of truth.")
}

func writeRoleEvidence(b *bytes.Buffer, label, role string, evidence []string) {
	fmt.Fprintf(b, "%s: %s\n\n", label, role)
	fmt.Fprintln(b, "Evidence:")
	if len(evidence) == 0 {
		fmt.Fprintln(b, "- (none recorded)")
	} else {
		for _, item := range evidence {
			fmt.Fprintf(b, "- %s\n", item)
		}
	}
	fmt.Fprintln(b)
}

// --- fact-source readers -------------------------------------------------

func readGoModule(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func readPackageManifest(path string) *packageManifest {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw struct {
		Main            string            `json:"main"`
		Bin             json.RawMessage   `json:"bin"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Workspaces      json.RawMessage   `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return &packageManifest{}
	}

	manifest := &packageManifest{Main: strings.TrimSpace(raw.Main)}

	if len(raw.Workspaces) > 0 {
		var globs []string
		if err := json.Unmarshal(raw.Workspaces, &globs); err == nil {
			manifest.Workspaces = globs
		} else {
			var object struct {
				Packages []string `json:"packages"`
			}
			if err := json.Unmarshal(raw.Workspaces, &object); err == nil {
				manifest.Workspaces = object.Packages
			}
		}
	}
	for name, command := range raw.Scripts {
		manifest.Scripts = append(manifest.Scripts, scriptEntry{Name: name, Command: command})
	}
	sort.Slice(manifest.Scripts, func(i, j int) bool { return manifest.Scripts[i].Name < manifest.Scripts[j].Name })

	if len(raw.Bin) > 0 {
		var binStr string
		if err := json.Unmarshal(raw.Bin, &binStr); err == nil {
			if binStr != "" {
				manifest.Bin = append(manifest.Bin, binStr)
			}
		} else {
			var binMap map[string]string
			if err := json.Unmarshal(raw.Bin, &binMap); err == nil {
				for _, target := range binMap {
					if target != "" {
						manifest.Bin = append(manifest.Bin, target)
					}
				}
			}
		}
		sort.Strings(manifest.Bin)
		manifest.Bin = dedupeStrings(manifest.Bin)
	}

	depSet := map[string]struct{}{}
	for name := range raw.Dependencies {
		depSet[name] = struct{}{}
	}
	for name := range raw.DevDependencies {
		depSet[name] = struct{}{}
	}
	for name := range depSet {
		manifest.Deps = append(manifest.Deps, name)
	}
	sort.Strings(manifest.Deps)
	return manifest
}

func readMakeTargets(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var targets []string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || line[0] == '\t' || line[0] == ' ' || line[0] == '#' || line[0] == '.' {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		// Skip variable assignments (`NAME := value`, `NAME ::= value`), which
		// are not build targets.
		rest := line[colon+1:]
		if strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, ":=") {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "" || strings.ContainsAny(name, " =") {
			continue
		}
		if !isMakeTargetName(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		targets = append(targets, name)
	}
	sort.Strings(targets)
	return targets
}

func readRunCommands(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var commands []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "run:") {
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(trimmed, "run:"))
		if command == "" || command == "|" || command == ">" || command == "|-" {
			continue
		}
		commands = append(commands, command)
	}
	return commands
}

// --- small predicates and utilities --------------------------------------

func ciWorkflowFiles(files []FileRecord) []string {
	var workflows []string
	for _, file := range files {
		if isCIWorkflow(file.Path) {
			workflows = append(workflows, file.Path)
		}
	}
	sort.Strings(workflows)
	return workflows
}

func isCIWorkflow(path string) bool {
	if !strings.HasPrefix(path, ".github/workflows/") {
		return false
	}
	return strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")
}

func isCmdMainGo(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) >= 2 && parts[0] == "cmd" && parts[len(parts)-1] == "main.go"
}

func isViteConfig(base string) bool {
	return strings.HasPrefix(base, "vite.config.")
}

func isTestFile(path string) bool {
	base := pathBase(path)
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return true
	}
	if strings.HasSuffix(base, "_test.py") {
		return true
	}
	for _, suffix := range []string{".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	for _, part := range strings.Split(path, "/") {
		switch part {
		case "test", "tests", "__tests__", "spec", "specs", "e2e":
			return true
		}
	}
	return false
}

func isTestConfig(base string) bool {
	switch base {
	case "pytest.ini", "tox.ini", "conftest.py", ".mocharc.json", ".mocharc.yml", ".mocharc.yaml":
		return true
	}
	for _, prefix := range []string{"jest.config.", "vitest.config.", "playwright.config.", "karma.conf.", "cypress.config."} {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

func isTestScriptName(name string) bool {
	switch name {
	case "test", "tests", "lint", "check", "typecheck", "type-check", "e2e", "coverage", "vet":
		return true
	}
	return strings.HasPrefix(name, "test:") || strings.HasPrefix(name, "lint:")
}

func isMakeTargetName(name string) bool {
	for _, r := range name {
		if !(r == '-' || r == '_' || r == '/' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return name != ""
}

func packageRunner(facts wikiFacts) string {
	for _, file := range facts.scan.Files {
		switch pathBase(file.Path) {
		case "pnpm-lock.yaml":
			return "pnpm run"
		case "yarn.lock":
			return "yarn"
		}
	}
	return "npm run"
}

func normalizeManifestPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	return path
}

func areaFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	cleaned := b.String()
	if cleaned == "" {
		cleaned = "area"
	}
	return cleaned + ".md"
}

func topLevelDir(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

func areaSubdirs(area string, files []FileRecord) []string {
	seen := map[string]struct{}{}
	for _, file := range files {
		parts := strings.Split(file.Path, "/")
		if len(parts) >= 3 {
			seen[parts[1]] = struct{}{}
		}
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, area+"/"+dir)
	}
	sort.Strings(dirs)
	return dirs
}

func areaLanguages(files []FileRecord) []languageItem {
	counts := map[string]int{}
	for _, file := range files {
		if file.Language != "" && file.Language != "Unknown" {
			counts[file.Language]++
		}
	}
	return languageSummary(counts)
}

func languageInline(items []languageItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s (%d)", item.Name, item.Count))
	}
	return strings.Join(parts, ", ")
}

func notableFiles(files []FileRecord) []string {
	var notable []string
	for _, file := range files {
		base := pathBase(file.Path)
		if file.Kind == "config" || file.Kind == "doc" || isViteConfig(base) || base == "main.go" || base == "index.ts" || base == "index.js" || strings.HasPrefix(base, "main.") {
			notable = append(notable, file.Path)
		}
	}
	sort.Strings(notable)
	return capStrings(dedupeStrings(notable), 30)
}

func joinCodeList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, "`"+value+"/`")
	}
	return strings.Join(parts, ", ")
}

func pathBase(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func capStrings(values []string, limit int) []string {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func capRecords(values []FileRecord, limit int) []FileRecord {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}
