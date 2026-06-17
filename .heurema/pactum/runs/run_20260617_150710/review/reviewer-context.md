# Reviewer Context

## Run
- Run id: run_20260617_150710
- Run status: contract_approved

## Contract
- Goal: Surface cache reuse and effective cost in the existing 'pactum usage' command. The data is already captured and computed — it is just not shown.

Today 'pactum usage' reports input/output/total tokens + 'captured N/M' per group (and in --json: input_tokens, output_tokens, total_tokens, captured_only, lower_bound). The usage records also carry cache_read_tokens and cache_creation_tokens, and internal/app/usage.go already computes effective_units (the provider-weighted cost proxy: fresh×1.0 + cache_write×{1.25 anthropic | 1.0 openai} + cache_read×0.1 + output×5 via recordEffectiveUnits) — but the summary response and the human table do not expose them.

Add to BOTH the human table and the --json output, per group AND for the TOTAL:
- cache_read_tokens and cache_creation_tokens (sum per group/total).
- cache_read_ratio = cache_read_tokens / input_tokens (0 when input is 0). In the human table render it as a percent column (e.g. 'cache 94%').
- effective_units = the already-computed provider-weighted proxy, summed per group and total. Render as a column in the human table and a field in JSON.

The motivation: raw input is misleading because most of it is cheap cache-read (0.1x), so effective_units is the number that actually bills and cache_read_ratio explains why raw input overstates cost. The human table should make this legible at a glance — suggested columns: STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED. Keep the existing TOTAL row + coverage line + lower-bound behavior. Uncaptured records contribute no tokens, no cache, no effective units (consistent with today).

Constraints: do NOT change the usage-recording path, the effective_units formula/multipliers, or any other command; do NOT add dollar cost; keep --by stage|model|agent|provider, --all, --json, and the coverage/lower-bound behavior intact; bump the JSON schema additively (still pactum.usage_summary.v1alpha1, additive fields only). Validation: go test ./internal/app -run Usage, go test ./..., go build ./..., make check.
- In scope:
  - Update the existing pactum usage summary path in internal/app/usage.go to aggregate cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units for totals and for every --by group.
  - Update pactum usage human output to show cache read/create information, cache read percentage, effective units, and captured counts for each group and the TOTAL row while preserving coverage and lower-bound reporting.
  - Update pactum usage --json output additively so totals and each group include cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units without removing or renaming existing pactum.usage_summary.v1alpha1 fields.
  - Add or update internal/app usage tests covering run-level, --all, --by stage|model|agent|provider, uncaptured records, zero-input cache ratio, JSON fields, and human table output.
- Out of scope:
  - Changing the usage-recording path, ledger schema, TokenUsage capture, or appendUsageRecord behavior.
  - Changing effective_units multipliers or formula.
  - Adding dollar costs, price lookup, budget logic, or provider pricing features.
  - Changing commands other than pactum usage, including status output.
  - Changing existing --all, run_id resolution, --by, coverage, warning, group sorting, or lower-bound semantics except as needed to display the new fields.
- Acceptance criteria:
  - pactum usage --json emits schema pactum.usage_summary.v1alpha1 and preserves existing top-level, totals, group, coverage, and warnings fields.
  - In JSON output, totals and every group include numeric cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units fields.
  - cache_read_tokens and cache_creation_tokens are summed from captured records only for each group and TOTAL; uncaptured records contribute zero cache, token, and effective-unit values.
  - cache_read_ratio equals cache_read_tokens / input_tokens for each group and TOTAL, and is 0 when input_tokens is 0.
  - effective_units for each group and TOTAL is the sum of the existing recordEffectiveUnits result for captured records, using the current provider multipliers and formula.
  - Human pactum usage output shows cache_read_tokens, cache_creation_tokens, a cache-read percentage (cache_read_ratio rendered as a percent column, e.g. 'cache 94%'), effective_units, and captured counts for every group row and the TOTAL row.
  - Human output retains the existing TOTAL row, Coverage line, LOWER BOUND marker, and uncaptured provider/stage details when records are uncaptured.
  - --by stage, --by model, --by agent, --by provider, --all, and default current-run behavior continue to work.
- Validation commands:
  - go test ./internal/app -run Usage
  - go test ./...
  - go build ./...
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
  - command_001: go test ./internal/app -run Usage (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./... (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: go build ./... (exit 0, timed out: false, result: gate/validation/command_003/result.json)
  - command_004: make check (exit 0, timed out: false, result: gate/validation/command_004/result.json)
- Change summary:
  - changed files:
    - internal/app/usage.go
    - internal/app/usage_test.go
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
