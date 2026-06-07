# Contract Draft

## Goal
Add agent-assisted contract drafting (Phase 1, slice 2): a read-only agent reads the contract goal + the human's answered clarifications + repo context and proposes updated contract fields (in/out-of-scope, acceptance criteria, validation commands, assumptions) into a draft-proposal artifact; a separate accept step applies the proposal to the contract. The agent never answers clarification questions, changes the goal, or edits code

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260606_205654
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
