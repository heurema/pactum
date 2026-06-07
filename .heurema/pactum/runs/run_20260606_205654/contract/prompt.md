# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_205654
- Approval: approved
- Contract hash: f78905bd164c8ec8af5b9f1f7f1324643f18be092ec99999f8664a8f1bd28abc

## Goal
Add agent-assisted contract drafting (Phase 1, slice 2): a read-only agent reads the contract goal + the human's answered clarifications + repo context and proposes updated contract fields (in/out-of-scope, acceptance criteria, validation commands, assumptions) into a draft-proposal artifact; a separate accept step applies the proposal to the contract. The agent never answers clarification questions, changes the goal, or edits code

## In scope
- A 'contract draft <run_id>' command (--reviewer, --yes, --timeout, --json): build a drafting prompt from the goal + answered clarifications + repo context; run a READ-ONLY agent; parse its structured output into a proposed contract draft (in_scope, out_of_scope, acceptance, validation, assumptions) recorded as a draft-proposal artifact — NOT applied to the contract
- A command to show the proposed draft and a command to ACCEPT it (apply the proposed fields to the contract via the existing contract-revision path); acceptance is the human gate
- The agent must not answer clarification questions, change the contract goal, or edit code
- Require --yes; reuse cross-model selection / model[:effort] / live output; tests with a fake agent; docs/agents.md note

## Out of scope
- The full clarify loop driver (suggest -> answer -> draft -> repeat) — a later slice
- The agent answering clarification questions or rewriting the goal
- Native LLM API or model/provider abstraction; touching generated .heurema artifacts

## Acceptance criteria
- 'contract draft' runs a read-only agent and records a proposed draft (scope/acceptance/validation/assumptions) WITHOUT applying it, answering questions, changing the goal, or editing code
- Accepting the draft applies its fields to the contract; the human still approves separately
- Requires --yes; covered by tests with a fake agent

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
