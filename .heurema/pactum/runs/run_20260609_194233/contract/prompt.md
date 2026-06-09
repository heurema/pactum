# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_194233
- Approval: approved
- Contract hash: f2ddecb60b61709218c892db3c1a68ff821949c0c51d8cc33700a8785a65887d

## Goal
Fix two valid Codex review findings on the clarification slices. (1) depends_on resolution (M15.2): recordClarifierSuggestions numbers question positions globally across all fenced suggestion blocks, but the clarifier prompt instructs depends_on to reference earlier questions 'in this same block'. With more than one emitted block, a block-2 depends_on:[1] wrongly resolves to block-1's first question. Reset the position counter and the position->id map per block so depends_on is block-local, matching the prompt. (2) kind validation (M15.3): a clarifier suggestion that omits the new kind field is skipped entirely, silently recording zero questions for a producer following the previously documented v1 shape. Default a missing/empty kind to 'other' (v1-compatible) while still rejecting a non-empty invalid kind, and document the structured suggestion fields.

## In scope
- In recordClarifierSuggestions (internal/app/clarify_suggest.go): reset position to 0 and positionToID to a fresh map at the START of each block iteration (for _, block := range blocks), so depends_on positions are numbered within the block they appear in and a reference in a later block can never resolve to an earlier block's question. The global q_NNN id assignment (len(questions)+len(created)+1) is unchanged.
- In clarificationQuestionFromSuggestion (clarify_suggest.go): when the trimmed kind is empty, default it to 'other' (a missing kind no longer skips the question — restores v1 compatibility); a non-empty kind that is not in the allowed set is still rejected with the existing 'question skipped: kind must be one of ...' warning.
- Tests (clarify_suggest_test.go): add a test that two fenced suggestion blocks each get block-local depends_on — block 2's depends_on:[1] resolves to block 2's first question id (not block 1's); update the kind test so an empty/whitespace kind is recorded with kind='other' (not skipped) while a non-empty invalid kind (e.g. 'Terminology', 'domain') is still rejected.
- docs/agents.md: briefly document that the clarifier suggestion's per-question object carries text, blocking, rationale, recommended_answer, confidence, kind (one of terminology/scope/acceptance/edge_case/assumption/other, defaulting to other when omitted), and an optional depends_on of 1-based earlier-in-block positions.

## Out of scope
- Do NOT relax the recommended_answer or confidence required-ness (those are the core grill-me content and were not part of the finding). Do not touch the ledger repo_root leak (already scrubbed in #87). Do not change other clarify behavior (Blocked, coverage, terminology/edge-case prompt sections).
- Do not change files outside internal/app/clarify_suggest.go, internal/app/clarify_suggest_test.go, and docs/agents.md. Do not bump the clarification schema-version constants.

## Paths in scope
- internal/app/clarify_suggest.go
- internal/app/clarify_suggest_test.go
- docs/agents.md


## Acceptance criteria
- depends_on positions are block-local: with two suggestion blocks, a depends_on referencing position 1 in the second block resolves to that block's first question id, never the first block's; a single-block emission is unaffected.
- A clarifier suggestion that omits kind is recorded with kind 'other' (not skipped); a non-empty invalid kind is still rejected with the existing warning.
- docs/agents.md documents the structured per-question suggestion fields including kind's default-to-other behavior.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes with tests covering block-local depends_on and the kind default; no unrelated behavior changes.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- The clarifier prompt's 'this same block' wording is the intended semantics, so per-block position numbering (no cross-block depends_on) is the correct resolution; cross-block dependencies are not needed.
- kind is a categorization with a valid catch-all ('other'), so defaulting a missing one is safe and preserves v1 compatibility; recommended_answer/confidence are core content and stay required.

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
