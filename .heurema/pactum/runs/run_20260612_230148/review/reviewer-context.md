# Reviewer Context

## Run
- Run id: run_20260612_230148
- Run status: contract_approved

## Contract
- Goal: Capture Codex token usage from ACP usage_update metadata and add per-engine ACP adapter command overrides.
- In scope:
  - Update the ACP transport adapter launch path so PACTUM_CLAUDE_ACP_COMMAND and PACTUM_CODEX_ACP_COMMAND can replace only the default npx executable/package invocation while preserving computed adapter args and environment.
  - Parse codex/token_usage metadata on ACP usage_update notifications, retain the latest successfully parsed total_token_usage for the attempt, and use it as a fallback when PromptResponse.Usage is absent.
  - Add focused unit tests for adapter command overrides and ACP usage metadata normalization/precedence/error behavior.
  - Document the ACP adapter override supply-chain use case and the forked Codex ACP usage metadata path.
- Out of scope:
  - Do not change the ACP protocol dependency, agent registry model inference, CLI transport usage parsers, review scheduling, or the sibling codex-acp repository in this run.
  - Do not commit Pactum run-record churn as part of the feature change.
- Acceptance criteria:
  - Default ACP adapter commands remain unchanged when override env vars are unset, empty, or whitespace-only.
  - Non-empty PACTUM_CLAUDE_ACP_COMMAND and PACTUM_CODEX_ACP_COMMAND values are treated as single executable paths, without shell splitting, replacing only the default npx/package prefix while preserving computed args and env for both engines.
  - ACP usage_update metadata at _meta.codex/token_usage maps total_token_usage to TokenUsage with InputTokens including cached input, CacheReadTokens from cached_input_tokens, CacheCreationTokens zero, OutputTokens including reasoning, ReasoningTokens from reasoning_output_tokens, TotalTokens from total_tokens, and Captured true.
  - Repeated valid usage_update notifications keep the latest cumulative totals, while later missing or malformed metadata does not clear an earlier valid capture.
  - PromptResponse.Usage remains authoritative when present; metadata-derived Codex usage is only the fallback.
  - Absent, malformed, or total_token_usage-missing metadata preserves captured=false with the existing no-usage warning, without failing the attempt; an explicitly present zero total_token_usage is captured as a real zero.
  - docs/agents.md explains the override and Codex ACP usage metadata behavior; docs/cost-budget-design.md notes the forked adapter usage path until upstream support exists.
- Validation commands:
  - go test ./internal/agents/... ./internal/app/...
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/agents/... ./internal/app/... (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
- Change summary:
  - changed files:
    - docs/agents.md
    - docs/cost-budget-design.md
    - internal/agents/acp_transport.go
    - internal/agents/acp_transport_test.go
  - new files:
    - none
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If you are not certain an issue is real after verification, do not flag it.
