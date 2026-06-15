# Contract Draft

## Goal
Capture Codex token usage from ACP usage_update metadata and add per-engine ACP adapter command overrides.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260612_175403
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 — How should the override env var value (PACTUM_CLAUDE_ACP_COMMAND / PACTUM_CODEX_ACP_COMMAND) be interpreted — as a single literal executable (argv[0], no whitespace/arg parsing), or as a whitespace-split command line that can carry its own arguments? This matters because the contract's primary motivation is supply-chain pinning, and pinning a version via `npx -y @zed-industries/codex-acp@1.2.3` requires arguments, whereas a single literal path forces the operator to point at a locally-installed/wrapper binary instead.
  Rationale: The contract says the override is 'an executable path that replaces only the npx+package invocation' and that 'pactum still appends the same computed arguments' — a literal singular reading. But the stated supply-chain goal (pin a specific adapter version) is naturally a versioned npx command with args, which a single literal path cannot express. The choice changes how acpAdapterCommand parses the env var and what the byte-identical-args test asserts. The repo (acp_transport.go:181-207) does not constrain this.
  Answer: Treat the value as a single literal executable resolved like any exec command (absolute path or PATH lookup) — no shell parsing, no whitespace arg-splitting. It replaces cmd ('npx') and drops the '-y <package>@latest' package args; pactum then appends only its computed args (codex -c pins) and env (claude model/effort) exactly as today. Document that version pinning is achieved by pointing the override at a locally-installed or vendored adapter binary (or a wrapper script), which is the supply-chain control SECURITY.md advises.
- q_002 — When the override env var is set but empty or whitespace-only (e.g. PACTUM_CODEX_ACP_COMMAND=""), should pactum ignore it and fall back to the npx+package default, or error?
  Rationale: The contract describes the override only for the non-empty case and is silent on empty/whitespace values, which are common when an env var is exported-but-unset in CI. acpAdapterCommand currently has no override branch at all, so the empty-value behavior must be specified.
  Answer: An unset, empty, or whitespace-only override is ignored: pactum uses the existing npx '-y <package>@latest' default. Only a non-empty (trimmed) value activates the override. No error is raised for an empty value.
- q_003 — Confirm the exact external contract emitted by the heurema/codex-acp fork: the _meta key is literally the string 'codex/token_usage', and the TokenUsageInfo fields are snake_case (total_token_usage, last_token_usage, model_context_window; each with input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens). The implementation hardcodes this key and casing, and the scripted test reproduces it — so a mismatch with the real fork would pass tests yet silently capture nothing in production.
  Rationale: The meta key and field names are an external dependency this repo cannot verify (no fork source here; the search index returned no hits and is stale). Because both the parser and its test derive from the contract's stated shape, a key/casing drift is invisible to the test suite. The human owns the heurema fork and can confirm the wire shape authoritatively.
  Answer: Confirmed from the patched local heurema/codex-acp diff and the Codex TokenUsageInfo source: the ACP usage_update metadata key is exactly codex/token_usage, and the payload uses snake_case fields total_token_usage, last_token_usage, model_context_window, with token usage fields input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens. Treat this wire shape as an explicit integration assumption.
- q_004 — If a usage_update carries the codex/token_usage meta but the total_token_usage object is absent (e.g. only last_token_usage is present) or decodes to all-zero counts, should the attempt capture zeros (Captured=true), or treat it as missing/malformed and keep Captured=false with the warning?
  Rationale: The contract says to 'retain the latest seen total_token_usage' and that 'malformed or missing meta is ignored', but a JSON object that parses successfully with total_token_usage simply omitted falls between those two rules — it isn't a parse error, yet there is no usable per-attempt figure. The chosen behavior determines whether a captured=true record with all-zero totals can be emitted.
  Answer: Require a present, non-empty total_token_usage to capture. If the meta key is present but total_token_usage is absent, treat it as malformed/missing for that notification — do not capture from it. An object that is present but reports genuine zeros is captured as-is (Captured=true).
- q_005 — When usage_update notifications repeat (last-cumulative-wins), and a LATER notification's codex/token_usage meta is malformed or missing after an EARLIER one parsed valid totals, should pactum keep the last successfully-parsed totals, or discard them because the most recent notification could not be parsed?
  Rationale: The contract pairs 'last one wins' with 'malformed/missing meta is ignored', which conflict when the last notification is the malformed one. Whether a late malformed notification clobbers an earlier valid capture changes whether codex usage is recorded at all for that attempt.
  Answer: Retain the last successfully-parsed total_token_usage. A later malformed or missing meta is ignored (warning at most) and does NOT overwrite or clear an earlier valid capture; 'last one wins' applies only among successfully-parsed notifications.
- q_006 — What are the explicit validation commands and acceptance criteria for this change? The contract embeds a detailed test list in prose, but contract.json leaves validation.commands and acceptance_criteria empty.
  Rationale: The acceptance and validation dimensions have zero entries in the contract (clarifier context confirms acceptance: none, validation: none). The prose tests imply Go unit tests in internal/agents plus doc edits, but the machine-checkable validation gate is unset.
  Answer: Set validation.commands to `go build ./...` and `go test ./internal/agents/... ./internal/app/...` (or `go test ./...`), and add acceptance criteria mirroring the prose tests: scripted ACP usage_update meta yields the exact mapped TokenUsage; repeated notifications keep the last cumulative totals; resp.Usage wins over meta when both present; absent/malformed meta preserves captured=false plus warning; override changes only the command with computed args and env byte-identical for both engines; docs/agents.md and docs/cost-budget-design.md updated.

## In scope
- Update the ACP transport adapter launch path so PACTUM_CLAUDE_ACP_COMMAND and PACTUM_CODEX_ACP_COMMAND can replace only the default npx executable/package invocation while preserving computed adapter args and environment.
- Parse codex/token_usage metadata on ACP usage_update notifications, retain the latest successfully parsed total_token_usage for the attempt, and use it as a fallback when PromptResponse.Usage is absent.
- Add focused unit tests for adapter command overrides and ACP usage metadata normalization/precedence/error behavior.
- Document the ACP adapter override supply-chain use case and the forked Codex ACP usage metadata path.

## Out of scope
- Do not change the ACP protocol dependency, agent registry model inference, CLI transport usage parsers, review scheduling, or the sibling codex-acp repository in this run.
- Do not commit Pactum run-record churn as part of the feature change.

## Paths in scope
- internal/agents/acp_transport.go
- internal/agents/acp_transport_test.go
- docs/agents.md
- docs/cost-budget-design.md


## Acceptance criteria
- Default ACP adapter commands remain unchanged when override env vars are unset, empty, or whitespace-only.
- Non-empty PACTUM_CLAUDE_ACP_COMMAND and PACTUM_CODEX_ACP_COMMAND values are treated as single executable paths, without shell splitting, replacing only the default npx/package prefix while preserving computed args and env for both engines.
- ACP usage_update metadata at _meta.codex/token_usage maps total_token_usage to TokenUsage with InputTokens including cached input, CacheReadTokens from cached_input_tokens, CacheCreationTokens zero, OutputTokens including reasoning, ReasoningTokens from reasoning_output_tokens, TotalTokens from total_tokens, and Captured true.
- Repeated valid usage_update notifications keep the latest cumulative totals, while later missing or malformed metadata does not clear an earlier valid capture.
- PromptResponse.Usage remains authoritative when present; metadata-derived Codex usage is only the fallback.
- Absent, malformed, or total_token_usage-missing metadata preserves captured=false with the existing no-usage warning, without failing the attempt; an explicitly present zero total_token_usage is captured as a real zero.
- docs/agents.md explains the override and Codex ACP usage metadata behavior; docs/cost-budget-design.md notes the forked adapter usage path until upstream support exists.

## Validation commands
- go test ./internal/agents/... ./internal/app/...
- make check

## Assumptions
- The forked Codex ACP adapter emits usage_update metadata under the literal key codex/token_usage using the confirmed snake_case TokenUsageInfo shape.

## Open questions
- None
