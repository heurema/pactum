# Review Fixer Context

## Run
- Run id: run_20260612_230148
- Run status: contract_approved

## Approved contract
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

## Current review findings
- Summary: findings=3 open=3 resolved=0 blocking_open=3
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: parseCodexACPUsageMeta accepts any non-empty total_token_usage object as valid; unknown-only or missing-field objects unmarshal to zero counts and return Captured=true instead of treating malformed metadata as a miss.
    location: internal/agents/acp_transport.go:376
  - f_002 severity=medium category=correctness blocking=true status=open: ACP runs do not emit the required no-usage warning when Codex ACP metadata is absent or malformed.
    location: internal/agents/acp_transport.go:137
  - f_003 severity=medium category=quality blocking=true status=open: The malformed metadata tests do not cover a non-empty total_token_usage object with missing/wrong token fields; the parser only checks that the object has at least one field and then unmarshals absent int fields as zero, so malformed metadata such as {"foo":1} is captured as real zero usage instead of preserving captured=false.
    location: internal/agents/acp_transport_test.go:336
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - none

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
