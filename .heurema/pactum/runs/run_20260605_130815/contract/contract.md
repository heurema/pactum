# Contract Draft

## Goal
Add per-stage model[:effort] config for the reviewer agent with pin-or-inherit semantics, symmetric to the existing executor model[:effort]; reuse ParseModelSpec and the model-spec application logic rather than duplicating it

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260605_130815
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
