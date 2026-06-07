# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_195132
- Approval: approved
- Contract hash: f732ec1ddd893d599d65e47511ed7d3fc1ca835c23b3d9722e5190777ed1bd8f

## Goal
Make 'pactum review loop' safe for long autonomous runs by adding two stop conditions beyond max_rounds: stalemate detection (stop when the fixer stops changing the working tree) and K-consecutive-clean (require K clean review rounds before declaring convergence), each with a distinct terminal reason

## In scope
- Stalemate-by-fingerprint: after each round compute a fingerprint of the working tree (reuse the gate's file hashing / a hash of changed files + HEAD). If N consecutive rounds in which a fix ran leave the fingerprint unchanged, terminate with terminal_reason 'stalemate'. N from config (e.g. limits.review.patience) or a flag, with a sane default (e.g. 2)
- K-consecutive-clean: require K clean review rounds (no created proposals, no warnings) in a row before terminating as 'clean_round'; a non-clean round resets the streak. K from config/flag, default 1 (preserving current L3a behavior)
- Record the per-round signals in the loop summary (e.g. unchanged-fingerprint streak, clean streak); add a docs/agents.md note
- Tests with fake agents: stalemate triggers after N unchanged fix rounds; K-clean requires K consecutive clean rounds; default behavior unchanged

## Out of scope
- Budget/cost stop (needs token/cost accounting from the agent CLIs — a separate slice)
- Rebuttal channel, dedup findings across rounds, severity composition, multi-reviewer panel
- Native LLM API or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- When a fix runs but the working tree is unchanged for N consecutive rounds, the loop stops with terminal_reason 'stalemate' instead of grinding to max_rounds
- With K>1 the loop requires K consecutive clean review rounds before terminal_reason 'clean_round'; a non-clean round resets the clean streak
- Default behavior (no new config/flags) is unchanged from L3a; covered by tests

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
