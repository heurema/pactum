# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260612_230148/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260612_230148/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260612_230148/review/review.json, .heurema/pactum/runs/run_20260612_230148/review/findings.jsonl, .heurema/pactum/runs/run_20260612_230148/review/resolutions.jsonl

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

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
