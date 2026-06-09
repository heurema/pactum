# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260608_213111
- Approval: approved
- Contract hash: bf17239e6d0d581b17a4b12e43809f1b4700d1d2b90cdb9a4681f451cd931966

## Goal
Create a new file docs/acp-smoke.md containing exactly one line of text: ACP transport smoke test ok. Do not modify any other files.

## In scope
- Create docs/acp-smoke.md with the single specified line.

## Out of scope
TBD

## Paths in scope
- docs/*.md


## Acceptance criteria
- docs/acp-smoke.md exists with the one specified line.

## Validation commands
TBD

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
