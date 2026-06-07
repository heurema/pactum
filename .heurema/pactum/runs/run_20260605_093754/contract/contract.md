# Contract Draft

## Goal
Add one sentence to docs/agents.md clarifying that real agent execution (pactum execute run) runs unsandboxed

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260605_093743
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- q_001 [blocking] — Should this real execution dogfood keep the change tiny and reversible?
  Answer: Yes. The task should be small, reversible, and validated by make check.

## In scope
- Make a tiny bounded docs-only change

## Out of scope
- Large refactor
- Dependency changes
- Touching generated .heurema artifacts

## Acceptance criteria
- Change is small and reviewable

## Validation commands
- make check

## Assumptions
TBD

## Open questions
- None
