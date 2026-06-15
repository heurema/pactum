# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260613_090148
- Approval: approved
- Contract hash: d697d81a5fc484dbd1884bea758cad27cad390e1993ab1c4cb71d65bd6e29f4a

## Goal
Run a no-edit Pactum execution through the rebuilt Pactum binary and local codex-acp adapter to verify ACP PromptResponse.Usage is captured.

## In scope
- No source changes; the executor should only inspect the prompt and return a brief confirmation.

## Out of scope
- Editing any source, documentation, test, or .heurema run-record file.
- Persisting the local adapter filesystem path in source or docs.

## Acceptance criteria
- pactum execute run completes with the local codex-acp adapter selected via PACTUM_CODEX_ACP_COMMAND.
- pactum usage for this smoke run reports at least one captured Codex call.
- The smoke execution introduces no source/doc/test file changes beyond the pre-existing working tree.

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
- mem_012 [fresh] score=32 — Capture Codex token usage from ACP usage_update metadata and add per-engine A...
- mem_001 [fresh] score=15 — Add an export command that dumps a run's full record as a single archive
- mem_009 [fresh] score=13 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_006 [fresh] score=13 — Smooth the pipeline so no command is pure ritual, then compress the agent ski...
- mem_002 [stale] score=13 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go

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
