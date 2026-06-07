# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_124529
- Approval: approved
- Contract hash: 3021d4b0160f11966e90aac2988fb2bf058dbced72759e9e1ee103516cf67266

## Goal
Add opt-in cross-model review: when enabled, the reviewer defaults to a different built-in agent (hence a different model) than the one that executed the run, so a model does not review its own output

## In scope
- Add an agents.cross_model_review bool config field to pactum.config.v1 (default false; when false, reviewer selection is unchanged)
- When cross_model_review is true and no explicit --reviewer is given, default the reviewer to the built-in agent that differs from the run's executor (codex<->claude), determined from the latest execution attempt
- Explicit --reviewer always wins; fall back to the configured default reviewer when the executor cannot be determined or is not a built-in
- Reflect the chosen reviewer in the existing Resolved block; unit tests + reviewer dry-run tests; docs/agents.md note

## Out of scope
- Multi-reviewer panel and findings aggregation (a follow-up)
- Changing reviewer model[:effort] behavior or reviewer least-privilege (read-only)
- Native LLM API or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- With cross_model_review=false, the default reviewer is unchanged
- With cross_model_review=true and a codex execution, the default reviewer resolves to claude (and vice versa)
- An explicit --reviewer overrides cross-model selection; falls back to the configured default reviewer when the executor is unknown

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
