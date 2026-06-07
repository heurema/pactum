# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260605_132959
- Approval: approved
- Contract hash: 5e8ef42af44233b81c7d8277d5bab6448d1eac215e7c2d61514d238980ab5924

## Goal
Surface the resolved per-stage model/effort once per run in execute and review output, so the operator can see what was applied: show the pinned model[:effort] when set, or 'inherit' (the agent CLI's own default) when empty

## In scope
- Add a 'Resolved' summary (agent, model, effort, pinned-or-inherit) to execute dry-run and execute run human output
- Add the same Resolved summary to reviewer dry-run and review run output
- Reuse the already-parsed ModelSpec from the execute/reviewer preparation — do not re-read config or re-parse
- Tests for the rendered Resolved output, plus a docs/agents.md note

## Out of scope
- Reading the agent CLI's own config (e.g. ~/.codex/config.toml) to resolve an inherited model — show 'inherit', not the actual CLI default
- Changing model/effort/sandbox behavior
- Native LLM API or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- When model[:effort] is pinned, the Resolved block shows the pinned model and effort
- When empty, the Resolved block shows 'inherit' for model and effort
- Shown for both the executor (execute) and the reviewer (review); reuses the existing parsed ModelSpec; covered by tests

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
