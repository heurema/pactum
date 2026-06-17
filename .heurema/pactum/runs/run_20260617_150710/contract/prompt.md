# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260617_150710
- Approval: approved
- Contract hash: 5bd2474457984bd7ffe712b4a61526d97b6642044899d1865fa96a2bd60b6081

## Goal
Surface cache reuse and effective cost in the existing 'pactum usage' command. The data is already captured and computed — it is just not shown.

Today 'pactum usage' reports input/output/total tokens + 'captured N/M' per group (and in --json: input_tokens, output_tokens, total_tokens, captured_only, lower_bound). The usage records also carry cache_read_tokens and cache_creation_tokens, and internal/app/usage.go already computes effective_units (the provider-weighted cost proxy: fresh×1.0 + cache_write×{1.25 anthropic | 1.0 openai} + cache_read×0.1 + output×5 via recordEffectiveUnits) — but the summary response and the human table do not expose them.

Add to BOTH the human table and the --json output, per group AND for the TOTAL:
- cache_read_tokens and cache_creation_tokens (sum per group/total).
- cache_read_ratio = cache_read_tokens / input_tokens (0 when input is 0). In the human table render it as a percent column (e.g. 'cache 94%').
- effective_units = the already-computed provider-weighted proxy, summed per group and total. Render as a column in the human table and a field in JSON.

The motivation: raw input is misleading because most of it is cheap cache-read (0.1x), so effective_units is the number that actually bills and cache_read_ratio explains why raw input overstates cost. The human table should make this legible at a glance — suggested columns: STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED. Keep the existing TOTAL row + coverage line + lower-bound behavior. Uncaptured records contribute no tokens, no cache, no effective units (consistent with today).

Constraints: do NOT change the usage-recording path, the effective_units formula/multipliers, or any other command; do NOT add dollar cost; keep --by stage|model|agent|provider, --all, --json, and the coverage/lower-bound behavior intact; bump the JSON schema additively (still pactum.usage_summary.v1alpha1, additive fields only). Validation: go test ./internal/app -run Usage, go test ./..., go build ./..., make check.

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
- Human pactum usage output shows cache_read_tokens, cache_creation_tokens, a cache-read percentage (cache_read_ratio rendered as a percent column, e.g. 'cache 94%'), effective_units, and captured counts for every group row and the TOTAL row.
- Human output retains the existing TOTAL row, Coverage line, LOWER BOUND marker, and uncaptured provider/stage details when records are uncaptured.
- --by stage, --by model, --by agent, --by provider, --all, and default current-run behavior continue to work.

## Validation commands
- go test ./internal/app -run Usage
- go test ./...
- go build ./...
- make check

## Assumptions
- The existing recordEffectiveUnits implementation is the authoritative source for effective_units.
- The human table must display both raw cache_read_tokens and cache_creation_tokens values (not only the derived cache% ratio) for every group row and the TOTAL row; the cache% column satisfies the ratio display requirement but does not substitute for the raw token columns.
- Exact human table spacing and column labels are flexible as long as the required values (cache_read_tokens, cache_creation_tokens, cache_read_ratio as %, effective_units, captured counts) are visible per row and for TOTAL.
- JSON cache_read_ratio and effective_units are numeric values, not formatted strings.

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 5
- fresh: 4
- stale: 1
- unknown: 0

Items:
- mem_018 [fresh] score=57 — Add a new top-level CLI command 'pactum usage [run_id]' that summarizes agent...
- mem_002 [stale] score=37 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go
- mem_016 [fresh] score=30 — Port the code-review loop (internal/app/review_loop.go) onto the existing int...
- mem_012 [fresh] score=30 — Capture Codex token usage from ACP usage_update metadata and add per-engine A...
- mem_005 [fresh] score=28 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Use `pactum search "<term>"` and inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing code items when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.

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
