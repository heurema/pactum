# Memory Candidate

## Run
- Run id: run_20260612_230148
- Source: deterministic

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

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - docs/agents.md
  - docs/cost-budget-design.md
  - internal/agents/acp_transport.go
  - internal/agents/acp_transport_test.go
- New files: none
- Missing files: none

## Clarifications
- q_001: How should the override env var value (PACTUM_CLAUDE_ACP_COMMAND / PACTUM_CODEX_ACP_COMMAND) be interpreted — as a single literal executable (argv[0], no whitespace/arg parsing), or as a whitespace-split command line that can carry its own arguments? This matters because the contract's primary motivation is supply-chain pinning, and pinning a version via `npx -y @zed-industries/codex-acp@1.2.3` requires arguments, whereas a single literal path forces the operator to point at a locally-installed/wrapper binary instead.
  Answer: Treat the value as a single literal executable resolved like any exec command (absolute path or PATH lookup) — no shell parsing, no whitespace arg-splitting. It replaces cmd ('npx') and drops the '-y <package>@latest' package args; pactum then appends only its computed args (codex -c pins) and env (claude model/effort) exactly as today. Document that version pinning is achieved by pointing the override at a locally-installed or vendored adapter binary (or a wrapper script), which is the supply-chain control SECURITY.md advises.
- q_002: When the override env var is set but empty or whitespace-only (e.g. PACTUM_CODEX_ACP_COMMAND=""), should pactum ignore it and fall back to the npx+package default, or error?
  Answer: An unset, empty, or whitespace-only override is ignored: pactum uses the existing npx '-y <package>@latest' default. Only a non-empty (trimmed) value activates the override. No error is raised for an empty value.
- q_003: Confirm the exact external contract emitted by the heurema/codex-acp fork: the _meta key is literally the string 'codex/token_usage', and the TokenUsageInfo fields are snake_case (total_token_usage, last_token_usage, model_context_window; each with input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens). The implementation hardcodes this key and casing, and the scripted test reproduces it — so a mismatch with the real fork would pass tests yet silently capture nothing in production.
  Answer: Confirmed from the patched local heurema/codex-acp diff and the Codex TokenUsageInfo source: the ACP usage_update metadata key is exactly codex/token_usage, and the payload uses snake_case fields total_token_usage, last_token_usage, model_context_window, with token usage fields input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens. Treat this wire shape as an explicit integration assumption.
- q_004: If a usage_update carries the codex/token_usage meta but the total_token_usage object is absent (e.g. only last_token_usage is present) or decodes to all-zero counts, should the attempt capture zeros (Captured=true), or treat it as missing/malformed and keep Captured=false with the warning?
  Answer: Require a present, non-empty total_token_usage to capture. If the meta key is present but total_token_usage is absent, treat it as malformed/missing for that notification — do not capture from it. An object that is present but reports genuine zeros is captured as-is (Captured=true).
- q_005: When usage_update notifications repeat (last-cumulative-wins), and a LATER notification's codex/token_usage meta is malformed or missing after an EARLIER one parsed valid totals, should pactum keep the last successfully-parsed totals, or discard them because the most recent notification could not be parsed?
  Answer: Retain the last successfully-parsed total_token_usage. A later malformed or missing meta is ignored (warning at most) and does NOT overwrite or clear an earlier valid capture; 'last one wins' applies only among successfully-parsed notifications.
- q_006: What are the explicit validation commands and acceptance criteria for this change? The contract embeds a detailed test list in prose, but contract.json leaves validation.commands and acceptance_criteria empty.
  Answer: Set validation.commands to `go build ./...` and `go test ./internal/agents/... ./internal/app/...` (or `go test ./...`), and add acceptance criteria mirroring the prose tests: scripted ACP usage_update meta yields the exact mapped TokenUsage; repeated notifications keep the last cumulative totals; resp.Usage wins over meta when both present; absent/malformed meta preserves captured=false plus warning; override changes only the command with computed args and env byte-identical for both engines; docs/agents.md and docs/cost-budget-design.md updated.

## Review Decisions
- f_001 [medium] resolved internal/agents/acp_transport.go:376: parseCodexACPUsageMeta accepts any non-empty total_token_usage object as valid; unknown-only or missing-field objects unmarshal to zero counts and return Captured=true instead of treating malformed metadata as a miss.
  Resolution: Updated internal/agents/acp_transport.go so Codex ACP total_token_usage must contain all expected token fields before capture; unknown-only or missing-field objects now preserve captured=false.
- f_002 [medium] resolved internal/agents/acp_transport.go:137: ACP runs do not emit the required no-usage warning when Codex ACP metadata is absent or malformed.
  Resolution: Updated ACPTransport.Run in internal/agents/acp_transport.go to call writeUsageWarning for ACP usage results before returning; added a transport-level test that verifies stderr.log receives the no-usage warning.
- f_003 [medium] resolved internal/agents/acp_transport_test.go:336: The malformed metadata tests do not cover a non-empty total_token_usage object with missing/wrong token fields; the parser only checks that the object has at least one field and then unmarshals absent int fields as zero, so malformed metadata such as {"foo":1} is captured as real zero usage instead of preserving captured=false.
  Resolution: Extended internal/agents/acp_transport_test.go malformed metadata coverage for unknown-only, missing-field, and wrong-type total_token_usage objects. Validation passed: go test ./internal/agents/... ./internal/app/... and make check.
- Proposal summary: pending=0 accepted=3 rejected=0

## Reusable Project Knowledge
- scope: in scope: Update the ACP transport adapter launch path so PACTUM_CLAUDE_ACP_COMMAND and PACTUM_CODEX_ACP_COMMAND can replace only the default npx executable/package invocation while preserving computed adapter args and environment.
- scope: in scope: Parse codex/token_usage metadata on ACP usage_update notifications, retain the latest successfully parsed total_token_usage for the attempt, and use it as a fallback when PromptResponse.Usage is absent.
- scope: in scope: Add focused unit tests for adapter command overrides and ACP usage metadata normalization/precedence/error behavior.
- scope: in scope: Document the ACP adapter override supply-chain use case and the forked Codex ACP usage metadata path.
- scope: out of scope: Do not change the ACP protocol dependency, agent registry model inference, CLI transport usage parsers, review scheduling, or the sibling codex-acp repository in this run.
- scope: out of scope: Do not commit Pactum run-record churn as part of the feature change.
- clarification: q_001: How should the override env var value (PACTUM_CLAUDE_ACP_COMMAND / PACTUM_CODEX_ACP_COMMAND) be interpreted — as a single literal executable (argv[0], no whitespace/arg parsing), or as a whitespace-split command line that can carry its own arguments? This matters because the contract's primary motivation is supply-chain pinning, and pinning a version via `npx -y @zed-industries/codex-acp@1.2.3` requires arguments, whereas a single literal path forces the operator to point at a locally-installed/wrapper binary instead. Answer: Treat the value as a single literal executable resolved like any exec command (absolute path or PATH lookup) — no shell parsing, no whitespace arg-splitting. It replaces cmd ('npx') and drops the '-y <package>@latest' package args; pactum then appends only its computed args (codex -c pins) and env (claude model/effort) exactly as today. Document that version pinning is achieved by pointing the override at a locally-installed or vendored adapter binary (or a wrapper script), which is the supply-chain control SECURITY.md advises.
- clarification: q_002: When the override env var is set but empty or whitespace-only (e.g. PACTUM_CODEX_ACP_COMMAND=""), should pactum ignore it and fall back to the npx+package default, or error? Answer: An unset, empty, or whitespace-only override is ignored: pactum uses the existing npx '-y <package>@latest' default. Only a non-empty (trimmed) value activates the override. No error is raised for an empty value.
- clarification: q_003: Confirm the exact external contract emitted by the heurema/codex-acp fork: the _meta key is literally the string 'codex/token_usage', and the TokenUsageInfo fields are snake_case (total_token_usage, last_token_usage, model_context_window; each with input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens). The implementation hardcodes this key and casing, and the scripted test reproduces it — so a mismatch with the real fork would pass tests yet silently capture nothing in production. Answer: Confirmed from the patched local heurema/codex-acp diff and the Codex TokenUsageInfo source: the ACP usage_update metadata key is exactly codex/token_usage, and the payload uses snake_case fields total_token_usage, last_token_usage, model_context_window, with token usage fields input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens. Treat this wire shape as an explicit integration assumption.
- clarification: q_004: If a usage_update carries the codex/token_usage meta but the total_token_usage object is absent (e.g. only last_token_usage is present) or decodes to all-zero counts, should the attempt capture zeros (Captured=true), or treat it as missing/malformed and keep Captured=false with the warning? Answer: Require a present, non-empty total_token_usage to capture. If the meta key is present but total_token_usage is absent, treat it as malformed/missing for that notification — do not capture from it. An object that is present but reports genuine zeros is captured as-is (Captured=true).
- clarification: q_005: When usage_update notifications repeat (last-cumulative-wins), and a LATER notification's codex/token_usage meta is malformed or missing after an EARLIER one parsed valid totals, should pactum keep the last successfully-parsed totals, or discard them because the most recent notification could not be parsed? Answer: Retain the last successfully-parsed total_token_usage. A later malformed or missing meta is ignored (warning at most) and does NOT overwrite or clear an earlier valid capture; 'last one wins' applies only among successfully-parsed notifications.
- clarification: q_006: What are the explicit validation commands and acceptance criteria for this change? The contract embeds a detailed test list in prose, but contract.json leaves validation.commands and acceptance_criteria empty. Answer: Set validation.commands to `go build ./...` and `go test ./internal/agents/... ./internal/app/...` (or `go test ./...`), and add acceptance criteria mirroring the prose tests: scripted ACP usage_update meta yields the exact mapped TokenUsage; repeated notifications keep the last cumulative totals; resp.Usage wins over meta when both present; absent/malformed meta preserves captured=false plus warning; override changes only the command with computed args and env byte-identical for both engines; docs/agents.md and docs/cost-budget-design.md updated.
- review_resolution: f_001 resolved: parseCodexACPUsageMeta accepts any non-empty total_token_usage object as valid; unknown-only or missing-field objects unmarshal to zero counts and return Captured=true instead of treating malformed metadata as a miss.; resolution: Updated internal/agents/acp_transport.go so Codex ACP total_token_usage must contain all expected token fields before capture; unknown-only or missing-field objects now preserve captured=false.
- review_resolution: f_002 resolved: ACP runs do not emit the required no-usage warning when Codex ACP metadata is absent or malformed.; resolution: Updated ACPTransport.Run in internal/agents/acp_transport.go to call writeUsageWarning for ACP usage results before returning; added a transport-level test that verifies stderr.log receives the no-usage warning.
- review_resolution: f_003 resolved: The malformed metadata tests do not cover a non-empty total_token_usage object with missing/wrong token fields; the parser only checks that the object has at least one field and then unmarshals absent int fields as zero, so malformed metadata such as {"foo":1} is captured as real zero usage instead of preserving captured=false.; resolution: Extended internal/agents/acp_transport_test.go malformed metadata coverage for unknown-only, missing-field, and wrong-type total_token_usage objects. Validation passed: go test ./internal/agents/... ./internal/app/... and make check.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- validation: go test ./internal/agents/... ./internal/app/... passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
