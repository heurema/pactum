# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_230208
- Approval: approved
- Contract hash: 936fdacd63f6ec0ad634c7f0c13e8f69fdab4034b12c357e0f7a41bcb6a11af4

## Goal
Add mechanical path-scope enforcement to the gate. The contract gains optional path-glob lists (paths_in_scope / paths_out_of_scope); the gate compares the run's changed and new files against them and surfaces files that are outside the declared scope. This is deterministic (no LLM) and opt-in: a contract with no path globs behaves exactly as today. Goal is to catch the executor touching files the task never authorized

## In scope
- Contract data model: add paths_in_scope []string and paths_out_of_scope []string (repo-relative, slash-separated glob patterns) to the contract; persist and load them with the existing contract artifact; 'contract show' displays them
- CLI: add 'contract revise' flags --add-path-in-scope and --add-path-out-of-scope mirroring the existing --add-in-scope/--add-out-of-scope (append semantics, sep:none)
- Glob matcher: a small, thoroughly unit-tested matcher over repo-relative slash paths supporting '*' (matches a run of non-separator chars within one path segment) and '**' (matches any number of segments including zero, e.g. internal/app/** matches internal/app/x.go and internal/app/sub/y.go). Prefer a small in-repo matcher over adding a dependency unless a vetted library is clearly cleaner
- Gate: when path globs are declared, compare the gate change report's changed files AND new files against them. Add a scope section to the gate report (gateReportDocument, JSON + human output) listing: 'undeclared' files (match no paths_in_scope pattern, only when paths_in_scope is non-empty) and 'out_of_scope' files (match a paths_out_of_scope pattern). Surface these as WARNINGS
- Tests: matcher unit tests (*, **, segment boundaries, no-match); gate tests for declared-globs-with-violation, clean-within-scope, out-of-scope match, and the opt-in skip (no globs => no scope section, identical to prior behavior)
- Docs: update the gate docs and docs/backlog.md (move/annotate the 'Mechanical scope enforcement' item; note that hard-fail/blocking is a deliberate follow-up)

## Out of scope
- Hard-failing the gate (non-zero exit) on scope violations — this slice is advisory/warnings only; promotion to blocking is a separate follow-up
- Any semantic/LLM scope judging (variant B); the drafter agent auto-suggesting path globs
- Changing the meaning or handling of the existing prose in_scope/out_of_scope fields
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Acceptance criteria
- The contract carries paths_in_scope/paths_out_of_scope; 'contract revise' can append them; 'contract show' displays them
- With globs declared, the gate report (JSON + human) lists undeclared and out-of-scope changed/new files; '*' and '**' matching is correct per the specified semantics
- With NO path globs declared, the gate behaves exactly as before (no scope section) and still exits success; with violations present the gate also still exits success (warnings only)
- make check is green (includes the deadcode gate); go test ./... passes

## Validation commands
- make check

## Assumptions
TBD

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
