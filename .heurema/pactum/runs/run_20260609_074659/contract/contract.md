# Contract Draft

## Goal
Remove the duplicated run-context loading prefix shared by the six load*Context functions in internal/app (loadMemoryContext, loadClarifyContext, loadContractContext, loadGateContext, loadReviewContext, loadExecuteReportContext). Each currently inlines the identical sequence: requireWorkspace -> runDir = filepath.Join(paths.RunsDir, runID) -> storeDirExists -> 'run not found: <id>' error -> contractRunPaths(runDir) -> readContractRunState(runPaths.RunJSON). Extract this into a shared unexported helper returning the common base (root, paths, runPaths, state) and rewrite all six functions to call it. Strictly behavior-preserving; no public API change; no test changes.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_074659
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- Add an unexported helper (e.g. loadRunStateContext(stdout, runID, jsonOutput)) that performs the identical prefix and returns the common base — the run root, artifacts paths, contractRunPaths set, and contractRunState — together with the (ok, err) signaling. It must preserve the exact semantics: requireWorkspace failure or !ok returns the zero base with ok=false and requireWorkspace's err; storeDirExists error propagates; a missing run dir returns the identical fmt.Errorf('run not found: %s', runID); ok is true only on full success.
- Rewrite all six load*Context functions to call the helper and build their specific context struct from the returned base: loadClarifyContext->clarifyContext and loadReviewContext->reviewContext map base{Root,Paths,RunPaths,State}; loadExecuteReportContext->executeReportContext maps only base.RunPaths and base.State (it still discards root/paths, exactly as today).
- Since loadMemoryContext, loadContractContext, and loadGateContext build the IDENTICAL runContext (base + readDraftContract + readApprovalState), also extract that shared tail (e.g. a helper that loads the full runContext) so those three become thin wrappers differing only in the jsonOutput argument they pass to requireWorkspace (memory/contract take the caller's jsonOutput; gate passes false).
- Preserve each function's existing jsonOutput argument exactly: loadMemoryContext, loadClarifyContext, loadContractContext take a jsonOutput parameter and forward it; loadGateContext, loadReviewContext, loadExecuteReportContext pass false.

## Out of scope
- Do NOT use Go generics — a plain non-generic base-struct helper is required (it is more readable here and cleanly handles the memory/contract/gate extra fallible reads that a build-callback generic cannot express).
- Do not modify requireWorkspace, storeDirExists, contractRunPaths, runDirFor, readContractRunState, readDraftContract, readApprovalState, or any context struct definition (runContext, clarifyContext, reviewContext, executeReportContext).
- Do not touch other inlined readContractRunState call sites that do NOT perform the run-not-found check (e.g. in task.go, execute.go); leave them as-is. No changes outside internal/app.

## Paths in scope
- internal/app/*.go


## Acceptance criteria
- None of the six load*Context functions still inlines the requireWorkspace -> storeDirExists -> run-not-found -> contractRunPaths -> readContractRunState prefix; all call the shared helper.
- Behavior is byte-identical: the same error values (including 'run not found: <id>'), the same ok/err propagation in every branch, the same jsonOutput argument per function, and the same context struct contents returned by each function.
- No test changes are required; go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- The shared prefix is byte-identical across all six functions (verified by reading each); the only inter-function differences are the jsonOutput argument and the per-function tail that builds the specific context struct.
- A non-generic base-struct helper is the lowest-risk, most readable form and is preferred over Go generics for this extraction.

## Open questions
- None
