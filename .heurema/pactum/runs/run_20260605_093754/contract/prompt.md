# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260605_093754
- Approval: approved
- Contract hash: 2a16bc5e1bbb48fee56f15bf13221770514d4f554606f1c0827d313b7df8f11c

## Goal
Add one sentence to docs/agents.md clarifying that real agent execution (pactum execute run) runs unsandboxed

## In scope
- Make a tiny bounded docs-only change

## Out of scope
- Large refactor
- Dependency changes
- Touching generated .heurema artifacts

## Acceptance criteria
- Change is small and reviewable

## Validation commands
- make check

## Assumptions
TBD

## Clarifications
- q_001 [blocking] Should this real execution dogfood keep the change tiny and reversible?
  Decision: Yes. The task should be small, reversible, and validated by make check.

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
