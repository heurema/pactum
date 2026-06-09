# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_185752
- Approval: approved
- Contract hash: 5f8a48da0312f17f161e96880b323469d9fb1b5d9bc3f70a29bcdeb66de1d143

## Goal
Add a coverage / convergence signal to clarify (grill-me slice 5, completing the arc). Report, per contract dimension (the question kinds from slice 3), how many questions are open vs answered, surface an overall Converged flag (no open blocking questions, mirroring review-loop 'resolved'), and nudge the clarifier to cover all material dimensions before concluding. This gives the human — and a future autonomous clarify loop — a real 'is interrogation done / where is ambiguity left' signal rather than a flat open count. Additive (no schema-version bump); the signal is computed and surfaced (not enforced) — no autonomous loop / auto-stop is built in this slice.

## In scope
- Add a clarifyKindCoverage struct {Kind string; Total, Open, Answered, BlockingOpen int} and add Coverage []clarifyKindCoverage and Converged bool to clarifyStatusResponse (clarify.go).
- In buildClarificationStatus (clarify.go): compute Coverage with one entry for EACH of the five canonical dimensions — terminology, scope, acceptance, edge_case, assumption — in that fixed order even when the count is 0 (so an unprobed dimension is visible), plus an 'other' entry only when it has at least one question. Tally Total/Open/Answered/BlockingOpen per kind consistently with the existing overall counters (an answered question is answered; an open question is open, and blocking-open when its Blocking is true). Set Converged = (response.BlockingOpen == 0).
- Display: writeClarifyStatus shows a 'Coverage by dimension' section listing each kind with its counts, and a 'converged: yes/no' line; renderClarifierContext surfaces the same per-dimension coverage so the clarifier sees which dimensions are already probed.
- renderClarifierPrompt: add guidance to seek coverage across the material dimensions — before concluding, ensure each relevant dimension (scope, acceptance, terminology, edge_case, assumption) has been considered; do not leave a material dimension unprobed, but do not manufacture questions for dimensions the contract or repository already settles (explore-first still applies).
- Tests (clarify_suggest_test.go and/or a clarify_test.go): cover the per-kind tally (including a 0-count canonical dimension and an answered-vs-open split), Converged reflecting BlockingOpen, the five canonical dimensions always present in order, 'other' present only when used, and the coverage/converged display. Update docs/backlog.md to mark grill-me slice 5 shipped and note the clarification arc (slices 1-5) complete.

## Out of scope
- Do NOT build or change an autonomous clarify loop or any auto-stop / convergence-driven control flow — Converged and Coverage are computed and surfaced only, for the human and a future loop to consume.
- Do not change the meaning of the existing Total/Answered/Open/BlockingOpen counters or the depends_on/Blocked/kind/recommended_answer/confidence behavior; do not bump the clarification schema-version constants; do not change files outside internal/app/clarify.go, internal/app/clarify_suggest.go (and their _test.go), and docs/backlog.md.

## Paths in scope
- internal/app/clarify.go
- internal/app/clarify_suggest.go
- internal/app/clarify_suggest_test.go
- docs/backlog.md


## Acceptance criteria
- clarifyStatusResponse carries Coverage (the five canonical dimensions always present in fixed order, 'other' only when used) and Converged (true iff BlockingOpen==0); the per-kind tallies sum consistently with the overall Total/Answered/Open/BlockingOpen.
- clarify status displays a coverage-by-dimension breakdown and a converged line; the clarifier context surfaces the coverage.
- renderClarifierPrompt instructs covering the material dimensions before concluding without manufacturing questions the repo/contract already settles.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes with tests covering the coverage tally, the Converged flag, the canonical-dimensions-always-present rule, and the display; docs/backlog.md marks slice 5 shipped.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Convergence for clarification is signaled, not enforced, this slice: Converged mirrors the existing no-open-blocking condition, while the per-dimension coverage reveals unprobed dimensions a flat open count hides; a future autonomous clarify loop can consume both.
- The five canonical dimensions (terminology, scope, acceptance, edge_case, assumption) are the contract aspects worth covering; 'other' is a catch-all, not a dimension to ensure coverage of, so it appears only when used.

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
