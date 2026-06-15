# Contract Draft

## Goal
Run a no-edit Pactum execution through the rebuilt Pactum binary and local codex-acp adapter to verify captured and coherent ACP PromptResponse.Usage accounting.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260613_090236
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- No source changes; the executor should only return a brief confirmation.

## Out of scope
- Editing source, documentation, tests, or manually editing run ledgers.

## Acceptance criteria
- pactum usage for this smoke run reports one captured Codex execute call and zero uncaptured calls.
- The captured usage reports total_tokens at least input_tokens + output_tokens after ACP normalization.

## Validation commands
- git diff --check

## Assumptions
TBD

## Open questions
- None
