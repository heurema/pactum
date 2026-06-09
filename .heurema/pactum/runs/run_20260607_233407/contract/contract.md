# Contract Draft

## Goal
Behavior-preserving refactor: split the oversized internal/app/app.go (about 1454 lines) into several cohesive files within the SAME Go package app, by MOVING top-level declarations verbatim. No change to behavior, output text, JSON/YAML schemas, CLI grammar, ledger events, exit codes, exported API, or tests. This is pure file reorganization to improve readability.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260607_233407
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- Create new files in internal/app and move declarations into them verbatim (identical signatures and bodies): cli.go = the kong CLI grammar (the cli struct and every *Cmd struct definition); commands.go = the thin command dispatchers (every func (c *xCmd) Run(r *runner) error); config.go = the config types (workspaceManifest, configFile, projectMapConfig, gateConfig, limitsConfig, iterationLimits, executeLimits, reviewLimits, budgetConfig, memoryConfig) plus their read/write/normalize helpers (defaultConfigFile, writeDefaultConfigIfMissing, readConfig, readWorkspaceManifest, writeYAML, normalizeGateScopeEnforcement, normalizeBudgetMode); workspace.go = init/workspace plumbing (Init, ensureInitialized, resolveInitRoot, resolveStatusRoot, requireWorkspace, notInitialized, findUp, ensureDirs, writeStaticWorkspaceFiles); jsonio.go = the shared JSON I/O helpers (writeJSON, writeJSONResponse, readJSON, isDir, readMapManifest).
- app.go retains the package declaration and imports it still needs, the App struct and its const block, the Run entrypoint, kongExit, the runner struct, App.Run dispatch, nowUTC, agentRegistry, and the small core (Status/writeWorkspaceStatus/Search may stay in app.go or move to the existing status.go/search.go ONLY if a clearly-matching topic file already exists). Fix the import lists of every touched file so each imports exactly what it uses (no unused imports).
- Group only by cohesion; do not split a single declaration across files; keep all code in package app (no new packages, no import-cycle risk since it is one package).

## Out of scope
- ANY behavior, logic, signature, name, or output change. Renaming declarations, changing function bodies, altering struct tags or kong annotations, reordering CLI commands.
- Editing or adding tests; changing any *_test.go file; touching files other than internal/app/app.go and the new internal/app/*.go files (do not refactor review.go, gate.go, etc.).
- New features, new helpers, dead-code removal beyond what the move requires, or merging logic.

## Paths in scope
- internal/app/*.go


## Acceptance criteria
- app.go is substantially smaller (well under 500 lines) and the moved declarations live in cohesive new files; every moved declaration appears exactly once across the package (none dropped, none duplicated).
- go build succeeds; make check passes (go test ./..., go vet, the deadcode gate, git diff --check); make test-race passes.
- ALL existing tests pass WITHOUT any modification; CLI behavior, output text, JSON/YAML schemas, ledger events, and exit codes are byte-for-byte unchanged.

## Validation commands
- make build
- make check
- make test-race

## Assumptions
- Everything stays in package app, so moving declarations between files cannot create import cycles and cannot change semantics; the deadcode gate and the unchanged test suite are the regression guards.
- No backward-compatibility concerns; this is internal package organization with no exported-API change.

## Open questions
- None
