# Contract Draft

## Goal
Add per-stage model[:effort] config for the executor agent with pin-or-inherit semantics: empty inherits the CLI's own config; a set value emits the override to the agent command

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260605_120031
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (2 result(s))

## Clarifications
- None

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

## Open questions
- None
