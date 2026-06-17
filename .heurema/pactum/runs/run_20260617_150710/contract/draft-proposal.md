# Contract Draft Proposal

## Status
- Run id: run_20260617_150710
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-17T15:08:58Z

## In scope
- Update the existing pactum usage summary path in internal/app/usage.go to aggregate cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units for totals and for every --by group.
- Update pactum usage human output to show cache read/create information, cache read percentage, effective units, and captured counts for each group and the TOTAL row while preserving coverage and lower-bound reporting.
- Update pactum usage --json output additively so totals and each group include cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units without removing or renaming existing pactum.usage_summary.v1alpha1 fields.
- Add or update internal/app usage tests covering run-level, --all, --by stage|model|agent|provider, uncaptured records, zero-input cache ratio, JSON fields, and human table output.

## Out of scope
- Changing the usage-recording path, ledger schema, TokenUsage capture, or appendUsageRecord behavior.
- Changing effective_units multipliers or formula.
- Adding dollar costs, price lookup, budget logic, or provider pricing features.
- Changing commands other than pactum usage, including status output.
- Changing existing --all, run_id resolution, --by, coverage, warning, group sorting, or lower-bound semantics except as needed to display the new fields.

## Acceptance criteria
- pactum usage --json emits schema pactum.usage_summary.v1alpha1 and preserves existing top-level, totals, group, coverage, and warnings fields.
- In JSON output, totals and every group include numeric cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units fields.
- cache_read_tokens and cache_creation_tokens are summed from captured records only for each group and TOTAL; uncaptured records contribute zero cache, token, and effective-unit values.
- cache_read_ratio equals cache_read_tokens / input_tokens for each group and TOTAL, and is 0 when input_tokens is 0.
- effective_units for each group and TOTAL is the sum of the existing recordEffectiveUnits result for captured records, using the current provider multipliers and formula.
- Human pactum usage output shows cache read/create values, a cache percentage, effective units, and captured counts for every group row and the TOTAL row.
- Human output retains the existing TOTAL row, Coverage line, LOWER BOUND marker, and uncaptured provider/stage details when records are uncaptured.
- --by stage, --by model, --by agent, --by provider, --all, and default current-run behavior continue to work.

## Validation commands
- go test ./internal/app -run Usage
- go test ./...
- go build ./...
- make check

## Assumptions
- The existing recordEffectiveUnits implementation is the authoritative source for effective_units.
- Exact human table spacing and column labels are flexible as long as the required values are visible per row and for TOTAL.
- JSON cache_read_ratio and effective_units are numeric values, not formatted strings.

