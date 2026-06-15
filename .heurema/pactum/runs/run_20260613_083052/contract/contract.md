# Contract Draft

## Goal
Dogfood Pactum with a local codex-acp adapter that returns official ACP PromptResponse.Usage, and align Pactum's ACP usage code/docs with response usage as the primary source.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260613_083015
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- ACP usage handling and tests in internal/agents/acp_transport.go and internal/agents/acp_transport_test.go.
- User-facing ACP/cost documentation in docs/agents.md and docs/cost-budget-design.md.

## Out of scope
- Changing ACP schema dependencies or codex-acp source from this Pactum repository task.
- Persisting local absolute adapter paths in source, docs, or committed run records.
- Removing the legacy codex/token_usage metadata parser unless it conflicts with official PromptResponse.Usage.

## Paths in scope
- internal/agents/acp_transport.go
- internal/agents/acp_transport_test.go
- docs/agents.md
- docs/cost-budget-design.md


## Acceptance criteria
- Pactum treats ACP PromptResponse.Usage as authoritative for token accounting and preserves the existing legacy codex/token_usage metadata path only as a fallback when prompt usage is absent.
- Docs describe official ACP PromptResponse.Usage as the primary Codex-over-ACP usage source and describe codex/token_usage metadata only as legacy/fork fallback compatibility.
- The run is executed with a locally built codex-acp adapter via PACTUM_CODEX_ACP_COMMAND so Pactum dogfoods the official usage response path.
- After execution, Pactum usage reporting for this run records a captured Codex call rather than an 'acp prompt returned no usage' warning.
- No source or docs file contains an absolute local filesystem path to the adapter binary.

## Validation commands
- make check

## Assumptions
- The local shell environment supplies PACTUM_CODEX_ACP_COMMAND for dogfood execution; the concrete machine path is not part of the repository change.

## Open questions
- None
