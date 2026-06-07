# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260605_120057
- Approval: approved
- Contract hash: 0db5549dadac4e3487c29f787d44ef5f31810706c16f82a09742dcaaf11e5fa8

## Goal
Add per-stage model[:effort] config for the executor agent with pin-or-inherit semantics: empty inherits the CLI's own config; a set value emits the override to the agent command

## In scope
- Add an executor model[:effort] config field to pactum.config.v1, defaulting to empty
- Parse the model[:effort] spec (empty, model-only, :effort-only, model:effort)
- Emit overrides to the executor command: codex via '-c model=' and '-c model_reasoning_effort='; claude via '--model' and '--effort'
- Unit tests for the parser and the emitted executor command args, plus a brief docs/agents.md note

## Out of scope
- Reviewer model[:effort] (executor only in this slice)
- Dry-run resolved-config visibility header
- Any native LLM API call or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- With empty config, executor commands are unchanged (no model/effort flags emitted)
- With model[:effort] set, codex gets '-c model=' / '-c model_reasoning_effort=' and claude gets '--model' / '--effort'
- Parser handles empty, model-only, ':effort'-only, and 'model:effort'; covered by tests

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
