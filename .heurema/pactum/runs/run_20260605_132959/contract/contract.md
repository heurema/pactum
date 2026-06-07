# Contract Draft

## Goal
Surface the resolved per-stage model/effort once per run in execute and review output, so the operator can see what was applied: show the pinned model[:effort] when set, or 'inherit' (the agent CLI's own default) when empty

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260605_132959
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
