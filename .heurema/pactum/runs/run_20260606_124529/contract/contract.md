# Contract Draft

## Goal
Add opt-in cross-model review: when enabled, the reviewer defaults to a different built-in agent (hence a different model) than the one that executed the run, so a model does not review its own output

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260606_124529
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
