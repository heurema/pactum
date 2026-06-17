# Contract Review Fixer Prompt

You are fixing a software change contract to address blocking review findings.

Current contract version: 1f3a86d1705b4f120c4f2e05ed31e21015de32ea399f2d78dae7bb8ba71ae7aa

## Current Contract

**Goal**: Surface cache reuse and effective cost in the existing 'pactum usage' command. The data is already captured and computed — it is just not shown.

Today 'pactum usage' reports input/output/total tokens + 'captured N/M' per group (and in --json: input_tokens, output_tokens, total_tokens, captured_only, lower_bound). The usage records also carry cache_read_tokens and cache_creation_tokens, and internal/app/usage.go already computes effective_units (the provider-weighted cost proxy: fresh×1.0 + cache_write×{1.25 anthropic | 1.0 openai} + cache_read×0.1 + output×5 via recordEffectiveUnits) — but the summary response and the human table do not expose them.

Add to BOTH the human table and the --json output, per group AND for the TOTAL:
- cache_read_tokens and cache_creation_tokens (sum per group/total).
- cache_read_ratio = cache_read_tokens / input_tokens (0 when input is 0). In the human table render it as a percent column (e.g. 'cache 94%').
- effective_units = the already-computed provider-weighted proxy, summed per group and total. Render as a column in the human table and a field in JSON.

The motivation: raw input is misleading because most of it is cheap cache-read (0.1x), so effective_units is the number that actually bills and cache_read_ratio explains why raw input overstates cost. The human table should make this legible at a glance — suggested columns: STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED. Keep the existing TOTAL row + coverage line + lower-bound behavior. Uncaptured records contribute no tokens, no cache, no effective units (consistent with today).

Constraints: do NOT change the usage-recording path, the effective_units formula/multipliers, or any other command; do NOT add dollar cost; keep --by stage|model|agent|provider, --all, --json, and the coverage/lower-bound behavior intact; bump the JSON schema additively (still pactum.usage_summary.v1alpha1, additive fields only). Validation: go test ./internal/app -run Usage, go test ./..., go build ./..., make check.

**Scope in**:
  - Update the existing pactum usage summary path in internal/app/usage.go to aggregate cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units for totals and for every --by group.
  - Update pactum usage human output to show cache read/create information, cache read percentage, effective units, and captured counts for each group and the TOTAL row while preserving coverage and lower-bound reporting.
  - Update pactum usage --json output additively so totals and each group include cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units without removing or renaming existing pactum.usage_summary.v1alpha1 fields.
  - Add or update internal/app usage tests covering run-level, --all, --by stage|model|agent|provider, uncaptured records, zero-input cache ratio, JSON fields, and human table output.

**Scope out**:
  - Changing the usage-recording path, ledger schema, TokenUsage capture, or appendUsageRecord behavior.
  - Changing effective_units multipliers or formula.
  - Adding dollar costs, price lookup, budget logic, or provider pricing features.
  - Changing commands other than pactum usage, including status output.
  - Changing existing --all, run_id resolution, --by, coverage, warning, group sorting, or lower-bound semantics except as needed to display the new fields.

**Acceptance criteria**:
  - pactum usage --json emits schema pactum.usage_summary.v1alpha1 and preserves existing top-level, totals, group, coverage, and warnings fields.
  - In JSON output, totals and every group include numeric cache_read_tokens, cache_creation_tokens, cache_read_ratio, and effective_units fields.
  - cache_read_tokens and cache_creation_tokens are summed from captured records only for each group and TOTAL; uncaptured records contribute zero cache, token, and effective-unit values.
  - cache_read_ratio equals cache_read_tokens / input_tokens for each group and TOTAL, and is 0 when input_tokens is 0.
  - effective_units for each group and TOTAL is the sum of the existing recordEffectiveUnits result for captured records, using the current provider multipliers and formula.
  - Human pactum usage output shows a cache-read percentage (cache_read_ratio rendered as a percent column, e.g. 'cache 94%'), effective_units, and captured counts for every group row and the TOTAL row.
  - Human output retains the existing TOTAL row, Coverage line, LOWER BOUND marker, and uncaptured provider/stage details when records are uncaptured.
  - --by stage, --by model, --by agent, --by provider, --all, and default current-run behavior continue to work.

**Validation commands**:
  - go test ./internal/app -run Usage
  - go test ./...
  - go build ./...
  - make check

**Assumptions**:
  - The existing recordEffectiveUnits implementation is the authoritative source for effective_units.
  - Exact human table spacing and column labels are flexible as long as the required values are visible per row and for TOTAL.
  - JSON cache_read_ratio and effective_units are numeric values, not formatted strings.

## Blocking Findings to Address

1. [opus/completeness] The Goal requires the human table to show raw cache_read_tokens AND cache_creation_tokens, but the suggested columns (STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED) and acceptance criterion 6 only require the derived cache% ratio, effective_units, and captured counts. cache_creation_tokens has no representation in any human-table column or acceptance criterion. An implementer can pass all acceptance criteria without showing the raw cache tokens the Goal demands, so the human-table requirement is ungatable. Resolve by either removing raw cache-token columns from the Goal's human-table requirement or adding an acceptance criterion that names them.
   Evidence: Goal: 'Add to BOTH the human table and the --json output ... cache_read_tokens and cache_creation_tokens (sum per group/total).' vs. Acceptance criterion 6: 'Human pactum usage output shows a cache-read percentage ..., effective_units, and captured counts' (no raw cache tokens), and suggested columns 'STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED'.
2. [opus/completeness] The Goal requires the human table to show raw cache_read_tokens AND cache_creation_tokens, but the suggested columns (STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED) and acceptance criterion 6 only require the derived cache% ratio, effective_units, and captured counts. cache_creation_tokens has no representation in any human-table column or acceptance criterion. An implementer can pass all acceptance criteria without showing the raw cache tokens the Goal demands, so the human-table requirement is ungatable. Resolve by either removing raw cache-token columns from the Goal's human-table requirement or adding an acceptance criterion that names them.
   Evidence: Goal: 'Add to BOTH the human table and the --json output ... cache_read_tokens and cache_creation_tokens (sum per group/total).' vs. Acceptance criterion 6: 'Human pactum usage output shows a cache-read percentage ..., effective_units, and captured counts' (no raw cache tokens), and suggested columns 'STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED'.
3. [codex-xhigh/completeness] The acceptance criteria do not explicitly require human output to show cache_read_tokens and cache_creation_tokens, even though the goal requires adding both to the human table. This leaves the human-output requirement under-gated.
   Evidence: Goal: "Add to BOTH the human table and the --json output... cache_read_tokens and cache_creation_tokens"; Acceptance criteria: "Human pactum usage output shows a cache-read percentage..., effective_units, and captured counts..."
4. [codex-xhigh/assumptions-surfaced] The contract should explicitly state whether the human table must show raw cache_read_tokens and cache_creation_tokens, or whether cache_read_ratio alone satisfies the cache display requirement. As written, this assumption is buried across conflicting guidance.
   Evidence: Goal says: "Add to BOTH the human table and the --json output, per group AND for the TOTAL: - cache_read_tokens and cache_creation_tokens" but suggested columns are "STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED" and acceptance only requires "Human pactum usage output shows a cache-read percentage..."

## Fixer Instructions

- Address each blocking finding by updating the relevant contract field.
- Do NOT change the goal field — it is out of scope for the fixer.
- Only include the contract fields you are changing in the output.
- base_version must exactly match the version shown above.

## Output

Output your reasoning, then a single JSON block with the revise payload:

```json
{
  "schema": "pactum.contract_revise.v1alpha1",
  "base_version": "1f3a86d1705b4f120c4f2e05ed31e21015de32ea399f2d78dae7bb8ba71ae7aa",
  "contract": {
    "acceptance_criteria": ["...updated criteria..."],
    "validation": {"commands": ["...updated commands..."]}
  }
}
```

Omit any contract field you are not changing. Do not include the goal field.
