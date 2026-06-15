# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260613_090426
- Approval: approved
- Contract hash: 2277545d3977900eee60427874c1cdaca2b4becd79f1cce8f307e7c1738f7a7a

## Goal
Run a no-edit Pactum execution through the rebuilt Pactum binary and local codex-acp adapter to verify captured and coherent ACP PromptResponse.Usage accounting.

## In scope
- No source changes; the executor should only return a brief confirmation.

## Out of scope
- Editing source, documentation, tests, or manually editing run ledgers.

## Acceptance criteria
- pactum usage for this smoke run reports one captured Codex execute call and zero uncaptured calls.
- The captured usage reports total_tokens at least input_tokens + output_tokens after ACP normalization.

## Validation commands
- git diff --check

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
- total: 5
- fresh: 4
- stale: 1
- unknown: 0

Items:
- mem_012 [fresh] score=28 — Capture Codex token usage from ACP usage_update metadata and add per-engine A...
- mem_010 [fresh] score=14 — Combined config and usage polish slice. (1) Hide the unfinished budget surfac...
- mem_003 [stale] score=11 — Remove the interactive confirmation layer from the CLI: the consumer is an AI...
  reason: missing file internal/app/confirm.go
- mem_007 [fresh] score=10 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_001 [fresh] score=7 — Add an export command that dumps a run's full record as a single archive

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

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.
