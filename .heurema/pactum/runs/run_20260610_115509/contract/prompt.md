# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_115509
- Approval: approved
- Contract hash: 621209ce29895140bc09ca00dc6396ba77c42dd3c8dcc3f44039b888ea32b04a

## Goal
Bake the house style into the write-stage prompts per docs/review-prompt-design.md slice 2. The executor prompt (renderApprovedPromptMD) and the fixer prompt (renderReviewFixPrompt) carry contract/scope discipline but no code style or engineering discipline — while the M19.0 reviewer now enforces exactly that ruleset, so the writer must be told the rules the reviewer will hold it to. Add ONE shared house-style section used by both write-stage prompts.

## In scope
- A shared section builder in internal/app (e.g. a writeHouseStyleSection helper) emitting the condensed house style from the design note: match the surrounding code — idiom, naming, comment density, with comments only where the code is not self-explanatory and no narration of the obvious; search for and reuse existing helpers before writing new ones; keep the diff small and focused — change only what the contract requires; simplicity first — no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization, with the over-engineering catalog as DON'Ts (wrappers that add nothing, factories/abstractions for a single case, unused extension points, dual implementations, silent fallbacks); no dead code, no commented-out code, no unused parameters; error handling per the project's existing convention with no silent failures; tests verify behavior (not implementation details) and cover error paths, with the fake-test catalog as DON'Ts (always-pass tests, hardcoded-value checks, assertions on mock behavior, ignored errors, commented-out cases).
- Wire the shared section into renderApprovedPromptMD (internal/app/prompt.go) and renderReviewFixPrompt (internal/app/review_fix.go) at a sensible position; the fixer variant may add one line noting the reviewer will re-check fixes against the same ruleset.
- Prompt-content tests for BOTH prompts guarding the section heading and its key lines (idiom-matching, reuse-before-new, small focused diffs, the over-engineering DON'Ts, no dead code, the fake-test DON'Ts), so the section cannot be silently dropped from either prompt; assert both prompts share the same section text.
- Docs: docs/agents.md briefly notes the write-stage house-style section; docs/review-prompt-design.md and docs/backlog.md mark slice 2 shipped (leaving the per-panel-member lens follow-up recorded).

## Out of scope
- No reviewer prompt changes (M19.0 shipped); no per-panel-member lenses; no config knobs for the ruleset (it lives in the templates per the earlier decision); no behavior changes outside prompt text and tests; no clarifier/drafter prompt changes (read-only stages do not write code).

## Paths in scope
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- Both write-stage prompts (executor and fixer) contain the identical shared house-style section with the idiom/reuse/small-diff/simplicity/no-dead-code/test-discipline rules; prompt-content tests pin the section in both and its key lines.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the design note and backlog mark slice 2 shipped.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Writer and reviewer must share one ruleset: the house-style section is the write-side mirror of the M19.0 reviewer lenses, so violations the reviewer flags are rules the writer was explicitly given.

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
