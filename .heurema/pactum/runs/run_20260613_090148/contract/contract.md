# Contract Draft

## Goal
Run a no-edit Pactum execution through the rebuilt Pactum binary and local codex-acp adapter to verify ACP PromptResponse.Usage is captured.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260613_083015
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- None

## In scope
- No source changes; the executor should only inspect the prompt and return a brief confirmation.

## Out of scope
- Editing any source, documentation, test, or .heurema run-record file.
- Persisting the local adapter filesystem path in source or docs.

## Acceptance criteria
- pactum execute run completes with the local codex-acp adapter selected via PACTUM_CODEX_ACP_COMMAND.
- pactum usage for this smoke run reports at least one captured Codex call.
- The smoke execution introduces no source/doc/test file changes beyond the pre-existing working tree.

## Validation commands
- git diff --check

## Assumptions
TBD

## Open questions
- None
