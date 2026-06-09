# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_184335
- Approval: approved
- Contract hash: 67e4db45225995ca7e15811de9200d24e4f02135a5594a5295b5f3a28df6a4e8

## Goal
Add systematic edge-case probing to the clarifier (grill-me slice 4). The kind=edge_case category already exists (slice 3); this slice strengthens the clarifier prompt so it does not merely 'consider edge cases' abstractly but invents CONCRETE boundary and failure scenarios derived from the contract's in-scope behaviors and acceptance criteria, and asks how the contract should behave where it is silent — tagging those questions kind=edge_case. Prompt-quality change plus prompt-content tests; no schema change.

## In scope
- Add a '## Probe edge cases' section to renderClarifierPrompt (internal/app/clarify_suggest.go) instructing the clarifier to: for each in-scope behavior and acceptance criterion, invent specific concrete scenarios the contract is silent on — empty/missing/zero/duplicate/extreme inputs, error and failure paths, partial/interrupted operations, concurrency and ordering, resource/size limits, and 'what about X' cases — and ask how it should behave, with the concrete scenario named in the question text and a recommended answer; tag every such question kind=edge_case. Emphasize concrete invented scenarios over abstract 'consider edge cases', and to prefer the cases most likely to change scope, acceptance, or implementation.
- Add a test in internal/app/clarify_suggest_test.go asserting renderClarifierPrompt contains the edge-case probing guidance (the '## Probe edge cases' heading and the instruction to tag such questions kind=edge_case), so the section cannot be silently dropped.
- Update docs/backlog.md to mark grill-me slice 4 (edge-case probing) shipped.

## Out of scope
- Do NOT add or change any schema field, validation, or the kind enum (edge_case already exists); do not change depends_on resolution, recommended_answer/confidence, the terminology section, or the status/display code. Prompt text + a prompt-content test + backlog only.
- Do not implement slice 5 (coverage / convergence signal). Do not change files outside internal/app/clarify_suggest.go, internal/app/clarify_suggest_test.go, and docs/backlog.md.

## Paths in scope
- internal/app/clarify_suggest.go
- internal/app/clarify_suggest_test.go
- docs/backlog.md


## Acceptance criteria
- renderClarifierPrompt has a dedicated edge-case probing section instructing concrete-scenario derivation (named categories like empty/extreme inputs, failure paths, concurrency, limits) tagged kind=edge_case, distinct from a vague 'consider edge cases' line.
- A test asserts the rendered clarifier prompt includes the edge-case probing guidance and the kind=edge_case tagging instruction.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; docs/backlog.md marks slice 4 shipped.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Edge-case probing is fundamentally a prompt-quality behavior; the structural support (kind=edge_case) already exists from slice 3, so this slice needs no new schema.

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
