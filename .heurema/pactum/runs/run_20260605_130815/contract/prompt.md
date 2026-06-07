# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260605_130815
- Approval: approved
- Contract hash: 7ed961c096375eacaf925547f01a8d15394478a5974325d3c0c1d20e84e4cd50

## Goal
Add per-stage model[:effort] config for the reviewer agent with pin-or-inherit semantics, symmetric to the existing executor model[:effort]; reuse ParseModelSpec and the model-spec application logic rather than duplicating it

## In scope
- Add a reviewer model[:effort] config field (agents.reviewer_model) to pactum.config.v1, defaulting to empty
- Apply the reviewer override to the reviewer command (codex '-c model=' / '-c model_reasoning_effort='; claude '--model' / '--effort'), reusing ParseModelSpec
- Generalize/reuse the existing executor model-spec application so executor and reviewer share the flag-emission logic (no duplication)
- Unit tests + reviewer dry-run tests, plus a docs/agents.md note

## Out of scope
- Changing executor model[:effort] behavior
- Resolved-config visibility header
- Native LLM API or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- With empty reviewer_model, reviewer commands are unchanged
- With reviewer_model set, the reviewer command gets the model/effort flags while PRESERVING the read-only reviewer sandbox (codex exec --sandbox read-only, claude -p)
- Executor and reviewer reuse the same flag-emission logic; covered by tests

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
