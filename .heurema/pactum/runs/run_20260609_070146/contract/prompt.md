# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_070146
- Approval: approved
- Contract hash: b578a183174ac36fea64cfc3271d38ded6fcbe7091559bfcac0fe6fee56d2f21

## Goal
Remove the copy-paste duplication between CLITransport (internal/agents/runner.go) and ACPTransport (internal/agents/acp_transport.go) by extracting the shared attempt plumbing — request validation, attempt-artifact path layout, and attempt log-file creation — into shared helpers in the agents package. The two transports MUST keep producing byte-identical attempt artifacts and RunResult contents; the extraction is the mechanism that prevents them from drifting. Behavior-preserving only — no public API change.

## In scope
- Extract a shared request-validation helper covering the duplicated RepoRoot/RunID/AttemptID/PromptRepoPath emptiness checks (with the same error messages) and call it from both RunSubprocess (runSubprocessWithRunner) and ACPTransport.Run.
- Extract a shared attempt-artifact layout helper that computes the artifactDir default ('execute/attempts'), the absolute attemptDir under .heurema runs, the slash-relative stdout/stderr artifact paths, and the absolute stdout.log/stderr.log paths, performing the MkdirAll — and use it from both transports.
- Extract a shared helper that creates the two attempt log files (stdout.log, stderr.log) and use it from both transports; the caller retains responsibility for deferring Close.
- Update both runner.go and acp_transport.go to call the new helpers, deleting the duplicated inline code.

## Out of scope
- Removing or renaming any exported type, function, or struct field (RunRequest, RunResult, Transport, AgentDescriptor, etc.) — including the test-coupled RunResult.Command/Args fields.
- Any change to ACP protocol logic, idle-timeout watchdog, usage parsing/normalization, process-group handling, or the writer-wrapping (live-tee/activity) setup — leave those as-is even if similar.
- Any change to files outside internal/agents, and any consolidation of reviewLoopLimits/reviewLimits (separate concern).

## Paths in scope
- internal/agents/*.go


## Acceptance criteria
- runner.go and acp_transport.go no longer contain duplicated request-validation, attempt-path-layout, or log-file-creation code; both call the shared helpers.
- Behavior is unchanged: the attempt directory layout, the slash-relative artifact paths, the absolute log paths, the validation error messages, and every RunResult field are identical to before the refactor.
- All existing tests pass with NO test changes required (including internal/agents/executor_test.go); go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./internal/agents/... passes.

## Validation commands
- go build ./...
- go test ./internal/agents/...
- go vet ./...
- go test -race ./internal/agents/...

## Assumptions
- The CLI and ACP transports are required to produce identical attempt artifacts; the shared helpers are the single source of truth that enforces this, so extracting them is a net reduction in drift risk, not just line count.
- No exported API changes are needed; the helpers are unexported package-internal functions in internal/agents.

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
