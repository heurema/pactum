# Reviewer Context

## Run
- Run id: run_20260612_161619
- Run status: contract_approved

## Contract
- Goal: Combined config and usage polish slice. (1) Hide the unfinished budget surface: review.budget (mode/max_tokens) gates nothing real — remove it from the config surface entirely: writeDefaultConfigIfMissing stops emitting it, readConfig rejects a leftover review.budget key with a loud configuration error naming the key (the same pattern as the removed agent: key), and the warn-mode budget plumbing in the review loop is deleted. Token accounting in the usage ledger and the usage command are untouched by the removal; budget enforcement returns later as a designed feature whose home is docs/cost-budget-design.md. (2) Usage display polish: pactum usage --all leads with the workspace summary, sorts the per-run rows by total tokens descending, and gains a --top N flag to cap the run list; uncaptured calls stop rendering as zero-valued rows — by-agent and by-run breakdowns annotate them honestly (for example: codex — N calls, usage not reported by the agent) and zero-token captured rows remain distinguishable from uncaptured ones. (3) Effective cost units: usage output (per-run and --all, human and JSON) adds an effective-units metric computed per provider with documented multipliers — for anthropic: fresh input x1.0, cache write x1.25, cache read x0.1, output x5.0; for codex/openai: fresh input x1.0 (cache writes are free and count as fresh), cached read x0.1, output x5.0 — the multipliers live as named constants with a comment citing the standard price ratios, and a per-stage/per-attempt cache hit-rate (cache_read / (fresh + cache_write + cache_read)) is shown where cache fields exist. (4) Map staleness pin narrowed: the map manifest currently pins the SHA-256 of the whole config.yaml, so editing agents or review.panel falsely invalidates the map; pin a deterministic hash of only the canonicalized map: config section instead — map-parameter changes still invalidate, other config edits do not; a legacy manifest holding the old whole-file hash is treated as stale once (one final refresh migrates it). (5) docs/cost-budget-design.md gains a verified cache-economics section recording the researched facts the future budget feature must account in: per-provider write/read multipliers, cache scoping (anthropic org+workspace, machine+directory effective scope for Claude Code; openai machine-local routing with prompt_cache_key), the concurrent cold-start write race and the staggered-launch savings model for panel fan-out (planned as its own slice), and the rule that budgets must be denominated in effective units rather than raw tokens. Usage docs (flow.md or wherever usage is documented) and CHANGELOG updated; tests pin the budget-key rejection, the sorted/top output, the uncaptured annotation, the effective-units math per provider, the hit-rate, and the map-pin behavior (agents-edit keeps map fresh, map-edit invalidates, legacy manifest migrates).
- In scope:
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
- Out of scope:
  - Do not implement a new budget enforcement feature, dollar-cost budgeting, cost estimation, or panel fan-out staggering.
  - Do not change the persisted `UsageRecord` schema to store `effective_units`.
  - Do not add `effective_units`, `by_attempt`, or cache-hit wording changes to `pactum status` except for unavoidable compile-time sharing fallout.
  - Do not make tests depend on live network access or live provider documentation.
  - Do not run real Pactum agent execution or review commands as part of this slice.
- Acceptance criteria:
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
- Validation commands:
  - go test ./internal/app ./internal/agents ./internal/docs
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: passed
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app ./internal/agents ./internal/docs (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
- Change summary:
  - changed files:
    - none
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
