# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_212924
- Approval: approved
- Contract hash: 15c20ad8e74afabee11fcdd5cc9eb1827971f9e4baccd78f05f30b5a69c46554

## Goal
Behavior-preserving refactor: extract the duplicated agent-run attempt lifecycle shared by execute run, review run, review fix, clarify suggest, and contract draft into ONE shared helper, eliminating the 5-way duplication. No change to artifacts, schemas, output text, ledger events, exit codes, or CLI

## In scope
- Introduce a shared helper for the common agent-attempt lifecycle: --yes/confirm gate, ensure prompt/dry-run artifacts, allocate next attempt id, write request doc, ledger 'started' event, RunSubprocess (agent/prompt/artifact-dir/timeout/live-output), runErr+StartedAt early return, write result + last-result, ledger 'finished' event, JSON/human output + processExitError handling
- Parameterize it by per-command specifics (resolved agent, prompt path, attempt/last-result paths, ledger event names, request/result doc builders, output renderer, exit Kind, optional post-run step like parsing output into proposals)
- Refactor all five commands to use the helper; replace duplicated nextX/attemptPaths/resultFromRunResult/resultTimestamp helpers with shared generic versions (preserving exact attempt-id formats and paths per command)
- Behavior-preserving: existing command/integration tests must pass WITHOUT modification

## Out of scope
- Any behavior change, new feature, or CLI change
- Changing artifact paths, schemas, JSON shapes, or human output text
- Fixing backlogged edge cases (all-empty draft dead-end, etc.) — pure refactor only
- Native LLM API or model/provider abstraction; touching generated .heurema artifacts

## Acceptance criteria
- The five commands share one agent-attempt lifecycle helper; per-command duplication is removed; net lines of code are reduced
- ALL existing tests pass UNCHANGED; artifacts, schemas, output, ledger events, and exit codes are identical to before
- go test ./... and go test -race ./... are green

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
