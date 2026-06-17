# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260617_150710/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260617_150710/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260617_150710/review/review.json, .heurema/pactum/runs/run_20260617_150710/review/findings.jsonl, .heurema/pactum/runs/run_20260617_150710/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=2 open=2 resolved=0 blocking_open=1
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=quality blocking=true status=open: New cache/effective usage fields are not tested for --all or the non-stage --by modes.
    location: internal/app/usage_test.go:297
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_002 severity=low category=quality blocking=false status=open: `docs/token-efficiency-research.md` still says `pactum usage` shows raw input instead of cache-adjusted cost and that `cache_read` is not surfaced, but the implementation now exposes cache fields, cache ratio, and effective units in both JSON and human output.
    location: docs/token-efficiency-research.md:51

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
  "schema": "pactum.review_fix_outcomes.v1alpha1",
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
