# Contract Draft Proposal

## Status
- Run id: run_20260612_161619
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-12T16:25:00Z

## In scope
- Remove `review.budget` from the default config, typed config surface, config normalization, and review-loop budget checks while preserving usage ledger recording and `pactum usage` aggregation.
- Make `readConfig` reject any leftover `review.budget` key with a loud configuration error that names `review.budget`.
- Update `pactum usage --all` human and JSON output so workspace totals are shown before run breakdowns, `by_run` rows sort by total captured tokens descending, and `--top N` caps only the sorted `by_run` list.
- Add `effective_units` as a computed float64 JSON number to usage totals and token-count breakdowns, render it in human output with two decimal places, and keep it out of persisted `UsageRecord` records.
- Compute effective units with named provider multiplier constants for Anthropic, Codex, and OpenAI, with source-ratio comments near the constants.
- Treat `captured=false` usage records as unknown usage regardless of token fields: count them as uncaptured calls, exclude them from token totals and effective units, and annotate them in human output as usage not reported.
- Keep captured zero-token records visible as real zero-token records distinct from uncaptured calls.
- Retain `cache_read_ratio` in JSON for compatibility, label it as cache hit rate in human output, and add per-run `by_attempt` rows keyed by `attempt_id` with stage, agent, provider, token counts, `effective_units`, and `cache_read_ratio`.
- For unsupported or unknown providers, preserve raw token totals but report `effective_units` as 0, add `effective_units_unavailable_calls` at relevant aggregate levels, and annotate affected human rows as effective units unavailable.
- Change map staleness detection to hash a deterministic serialization of the normalized `config.Map` struct only, add `config_hash_scope: "map"` to refreshed manifests, and treat manifests missing that scope marker as legacy stale once.
- Update `docs/cost-budget-design.md`, usage documentation, and `CHANGELOG.md` for the removed budget config surface, usage output changes, effective-unit semantics, cache-hit wording, and map hash scope behavior.

## Out of scope
- Do not implement a new budget enforcement feature, dollar-cost budgeting, cost estimation, or panel fan-out staggering.
- Do not change the persisted `UsageRecord` schema to store `effective_units`.
- Do not add `effective_units`, `by_attempt`, or cache-hit wording changes to `pactum status` except for unavoidable compile-time sharing fallout.
- Do not make tests depend on live network access or live provider documentation.
- Do not run real Pactum agent execution or review commands as part of this slice.

## Acceptance criteria
- A newly written default config contains no `review.budget` block or budget mode/max_tokens keys.
- A config file containing `review.budget` fails strict config loading with an error message that names `review.budget`.
- Review execution no longer stops, warns, records budget summaries, or emits `budget_exceeded` because of `review.budget`; usage records are still appended normally.
- `pactum usage --all` human output starts with the workspace summary before per-run rows.
- `pactum usage --all --json` returns `by_run` sorted by total captured tokens descending, and `--top N` returns only the first N sorted run rows while totals and other breakdowns still aggregate all runs.
- `pactum usage --top N` without `--all` fails with a usage error, and `--top 0` or a negative top value fails with a usage error naming `--top`.
- Per-run and workspace usage JSON include `effective_units` on totals and token-count breakdowns; human output renders effective units with exactly two decimal places.
- Anthropic effective units use fresh input x1.0, cache write x1.25, cache read x0.1, and output x5.0; Codex/OpenAI effective units use fresh input x1.0, cached read x0.1, and output x5.0 with cache writes counted as fresh/free according to the clarified rule.
- Uncaptured calls contribute to call counts and `uncaptured_calls`, do not contribute to token totals or effective units, and render as usage not reported rather than as zero-token rows.
- Captured zero-token calls remain distinguishable from uncaptured calls in JSON counts and human output.
- Per-run usage JSON includes `by_attempt` rows keyed by `attempt_id` with stage, agent, provider, token counts, `effective_units`, and `cache_read_ratio`; workspace usage JSON does not add `by_attempt`.
- Human usage output labels the existing cache ratio as cache hit rate, while JSON keeps the `cache_read_ratio` field name.
- Unsupported-provider captured rows keep raw token totals, report `effective_units: 0`, increment `effective_units_unavailable_calls`, and human output marks effective units unavailable.
- Editing non-map config sections such as agents or review panel settings does not make the project map stale, while editing `map.max_file_bytes` or `map.code_index` does.
- A legacy map manifest without `config_hash_scope` is reported stale once, and the next map refresh writes `config_hash_scope: "map"` with the map-section hash.
- `docs/cost-budget-design.md` cites current official provider documentation where available and explicitly labels any unsupported cache-scope or routing facts as implementation assumptions.

## Validation commands
- go test ./internal/app ./internal/agents ./internal/docs
- make check

## Assumptions
- Official provider documentation available during implementation is sufficient to verify the documented cache multipliers and scoping facts; any fact not directly supported by official docs will be recorded as an implementation assumption rather than treated as verified.
- Sorting `by_run` by total tokens means captured `total_tokens` only, with ties allowed to use the existing stable deterministic secondary ordering.
- The new `effective_units_unavailable_calls` field may be added to usage aggregate JSON without a schema version bump beyond the existing additive-response compatibility expectations.

