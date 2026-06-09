# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260608_221737
- Approval: approved
- Contract hash: 138ca95acd3f8751baeddb394e6a8d0dd2df079b22c496076a26c3b5718964b8

## Goal
Create two files. First, docs/acp-gate-inscope.md with one line: in scope ok. Second, internal/acp-gate-denied.txt with one line: should be denied. Create both files; report which succeeded.

## In scope
- docs/acp-gate-inscope.md

## Out of scope
TBD

## Paths in scope
- docs/*.md


## Acceptance criteria
- docs/acp-gate-inscope.md created.

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
