# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260613_083052
- Approval: approved
- Contract hash: 8c4bee1985d1f5c6eb63cb6aaf933e8813a7b74bc45c4ffe3f8742985a7ae488

## Goal
Dogfood Pactum with a local codex-acp adapter that returns official ACP PromptResponse.Usage, and align Pactum's ACP usage code/docs with response usage as the primary source.

## In scope
- ACP usage handling and tests in internal/agents/acp_transport.go and internal/agents/acp_transport_test.go.
- User-facing ACP/cost documentation in docs/agents.md and docs/cost-budget-design.md.

## Out of scope
- Changing ACP schema dependencies or codex-acp source from this Pactum repository task.
- Persisting local absolute adapter paths in source, docs, or committed run records.
- Removing the legacy codex/token_usage metadata parser unless it conflicts with official PromptResponse.Usage.

## Paths in scope
- internal/agents/acp_transport.go
- internal/agents/acp_transport_test.go
- docs/agents.md
- docs/cost-budget-design.md


## Acceptance criteria
- Pactum treats ACP PromptResponse.Usage as authoritative for token accounting and preserves the existing legacy codex/token_usage metadata path only as a fallback when prompt usage is absent.
- Docs describe official ACP PromptResponse.Usage as the primary Codex-over-ACP usage source and describe codex/token_usage metadata only as legacy/fork fallback compatibility.
- The run is executed with a locally built codex-acp adapter via PACTUM_CODEX_ACP_COMMAND so Pactum dogfoods the official usage response path.
- After execution, Pactum usage reporting for this run records a captured Codex call rather than an 'acp prompt returned no usage' warning.
- No source or docs file contains an absolute local filesystem path to the adapter binary.

## Validation commands
- make check

## Assumptions
- The local shell environment supplies PACTUM_CODEX_ACP_COMMAND for dogfood execution; the concrete machine path is not part of the repository change.

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
- fresh: 5
- stale: 0
- unknown: 0

Items:
- mem_012 [fresh] score=53 — Capture Codex token usage from ACP usage_update metadata and add per-engine A...
- mem_004 [fresh] score=37 — Tell the security truth in the user-facing docs and add a security policy. RE...
- mem_009 [fresh] score=32 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_010 [fresh] score=22 — Combined config and usage polish slice. (1) Hide the unfinished budget surfac...
- mem_005 [fresh] score=21 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...

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
