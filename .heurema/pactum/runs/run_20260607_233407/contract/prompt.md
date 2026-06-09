# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260607_233407
- Approval: approved
- Contract hash: 4231d6f57188c7ab143572b5e259701897baa842d7cf14dfb7ff7f534390b439

## Goal
Behavior-preserving refactor: split the oversized internal/app/app.go (about 1454 lines) into several cohesive files within the SAME Go package app, by MOVING top-level declarations verbatim. No change to behavior, output text, JSON/YAML schemas, CLI grammar, ledger events, exit codes, exported API, or tests. This is pure file reorganization to improve readability.

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

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 0
- fresh: 0
- stale: 0
- unknown: 0

Items:
- none

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Use `pactum search "<term>"` and inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing code items when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.
