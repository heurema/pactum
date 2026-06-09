# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_081747
- Approval: approved
- Contract hash: 17d836372c9d20b9399c905adfaf11fba99febc9b8d97687a4f02e6d2a33514e

## Goal
Two small, safe, behavior-preserving cleanups. (A) Consolidate the redundant reviewLoopLimits struct (internal/app/review_loop.go) into the identical reviewLimits config type — both are exactly {MaxRounds, Patience, CleanRounds}. (B) Remove the unused RunResult.Command and RunResult.Args fields (internal/agents/types.go); they are json:"-" (never serialized) and read by nothing in production — the only reader is one redundant test assertion already covered by a sibling assertion. No public behavior or artifact change.

## In scope
- A: Delete the reviewLoopLimits type and make reviewLoopSettings.Limits use the existing reviewLimits type (config.go) instead; update the construction in resolveReviewLoopSettings to build a reviewLimits{MaxRounds, Patience, CleanRounds}. All reads (settings.Limits.MaxRounds/.Patience/.CleanRounds, e.g. review_loop.go:150) stay unchanged because the field names are identical.
- B: Remove the Command and Args fields from the RunResult struct in internal/agents/types.go.
- B: Remove the Command: and Args: entries from the RunResult{...} literals in internal/agents/runner.go and internal/agents/acp_transport.go. IMPORTANT: do NOT touch the processSpec{Command, Args} literal in runner.go (the one passed to runner.Run) — that is unrelated and must stay; cloneArgs must remain (still used by processSpec).
- B: Update the single reader, TestRunSubprocessCodexUsesTypedRunnerStdinAndEnv in internal/agents/executor_test.go, by dropping the 'result.Command != ...' and 'result.Args ...' terms from the if-condition and keeping 'if result.ExitCode != 0'. Command/args construction stays fully verified by the existing runner.spec.Command / runner.spec.Args assertions immediately below it.

## Out of scope
- Any change to RunResult, processSpec, reviewLimits, reviewLoopSettings, or the config schema beyond the two named edits; any other test change besides the one executor_test.go assertion; anything outside internal/agents/*.go and internal/app/review_loop.go.
- Any behavior or serialized-artifact change — RunResult.Command/Args are json:"-" so no result.json artifact changes; the review-loop limits values and resolution logic must be identical.

## Paths in scope
- internal/agents/*.go
- internal/app/review_loop.go


## Acceptance criteria
- reviewLoopLimits no longer exists; reviewLoopSettings.Limits is of type reviewLimits; the review-loop limit values and resolution behavior are unchanged.
- RunResult has no Command/Args fields; no production code references them; the one test reader asserts command construction via the captured processSpec (runner.spec), not RunResult; cloneArgs remains used by processSpec.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the ONLY test change is the noted executor_test.go assertion.

## Validation commands
- go build ./...
- go test ./internal/agents/... ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- reviewLoopLimits and reviewLimits have byte-identical field sets {MaxRounds, Patience, CleanRounds}, so reusing the config type for the resolved settings is purely a de-duplication.
- RunResult.Command/Args are json:"-" and unread except by the one redundant test assertion; removing them changes no serialized artifact and no production behavior.

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
