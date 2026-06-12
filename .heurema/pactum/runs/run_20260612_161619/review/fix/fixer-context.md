# Review Fixer Context

## Run
- Run id: run_20260612_161619
- Run status: contract_approved

## Approved contract
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

## Current review findings
- Summary: findings=16 open=16 resolved=0 blocking_open=7
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: by_attempt rows are keyed only by attempt_id, but attempt IDs collide across stages: execute (execute.go:85-87) and review-fix (review_fix.go:110-112) both use the "attempt" prefix with numbering restarting at 1 in separate attempts directories, so a run with an execute attempt and a fixer round has two usage records with attempt_id="attempt_001" (stages execute/fix). accumulateUsageByAttempt merges them into one row — tokens, effective_units, and cache_read_ratio summed across two distinct attempts — labeled with the stage/agent/provider of whichever record is first in the ledger. usageRecordID already hashes runID+attemptID+stage because of this exact collision. Key the by_attempt map by stage+attempt_id instead.
    location: internal/app/usage.go:407
  - f_008 severity=medium category=correctness blocking=true status=open: Mixed captured/uncaptured usage breakdown rows are not annotated as having unreported usage.
    location: internal/app/usage.go:613
  - f_009 severity=medium category=correctness blocking=true status=open: Map config hashing uses raw map.code_index instead of the normalized map config.
    location: internal/app/map.go:34
  - f_010 severity=medium category=correctness blocking=true status=open: Per-run by_attempt aggregation keys only by attempt_id, so execute attempt_001 and fix attempt_001 collapse into one row with misleading stage/agent/provider metadata.
    location: internal/app/usage.go:407
  - f_011 severity=medium category=correctness blocking=true status=open: Human breakdown rows with a mix of captured and uncaptured calls do not annotate that some calls had usage not reported.
    location: internal/app/usage.go:614
  - f_015 severity=medium category=quality blocking=true status=open: The backlog still documents the removed token budget stop as shipped/current, including `budget_exceeded`, and later still lists this exact config/usage polish slice as next work.
    location: docs/backlog.md:122
  - f_016 severity=medium category=quality blocking=true status=open: The loop architecture design still tells readers that `review.budget.mode` / `review.budget.max_tokens` are the Phase 3 budget stop config, even though this change removes and rejects `review.budget`.
    location: docs/loop-architecture-design.md:193
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_002 severity=low category=correctness blocking=false status=open: mapConfigHash hashes the raw decoded code_index string, but the contract (scope item and q_007) requires hashing the normalized config.Map; the effective mode is normalized downstream by codeindex.NormalizeMode, which folds "" and unknown values into "auto". Deleting the code_index: auto line (raw value becomes "") therefore changes the hash and falsely reports the map stale even though the effective scan behavior is unchanged. False staleness only — one extra refresh; no false-freshness path.
    location: internal/app/map.go:34
  - f_003 severity=low category=correctness blocking=false status=open: mapConfigHash hashes the raw, un-normalized code_index string. readConfig does not normalize config.Map (codeindex.NormalizeMode runs only inside projectmap.Scan, where "" and unrecognized values fold to "auto"), but contract clarification q_007 requires hashing the normalized config.Map struct including normalized code_index. Deleting the default `code_index: auto` line, or writing any alias NormalizeMode folds to auto, leaves scan behavior identical yet changes the hash, so status falsely reports the map stale — the false-invalidation class this slice targeted, now confined to the map section. Self-heals after one refresh; no literal acceptance criterion breaks.
    location: internal/app/map.go:34
  - f_004 severity=low category=quality blocking=false status=open: No test exercises the "openai" provider in effective-units math. recordEffectiveUnits has `case "codex", "openai"` but all fixtures use only codex and anthropic, so acceptance criterion 8's OpenAI multipliers (fresh x1.0, cached read x0.1, output x5.0, cache writes as fresh) are unverified; a regression splitting the shared case arm would pass the suite.
    location: internal/app/usage_test.go:515
  - f_005 severity=low category=quality blocking=false status=open: Workspace (usage --all) effective_units is never asserted, in JSON or human output. Acceptance criterion 7 requires workspace JSON to include effective_units on totals and breakdowns; all EffectiveUnits assertions are in per-run tests. TestUsageAllAggregatesAcrossRuns and TestUsageAllSortsByRunDescendingWithTop check token totals and by_run ordering but not effective units, and the workspace human-output substring checks omit "effective units:".
    location: internal/app/usage_test.go:282
  - f_006 severity=low category=quality blocking=false status=open: inspectProjectMap takes a configHash parameter that is now a pure derivation of the config parameter passed in the same call: workspaceStatus computes mapConfigHash(config.Map) and threads it through, but inspectProjectMap already receives config and is the hash's only consumer. The caller-side computation was justified when the hash required file I/O (storeFileSHA256); after this change it is leftover plumbing — compute the hash inside inspectProjectMap and drop the parameter.
    location: internal/app/status.go:235
  - f_007 severity=low category=quality blocking=false status=open: docs/loop-architecture-design.md still presents review.budget.mode / review.budget.max_tokens as declared config surface (lines 14, 193, 263, 289) after this change removed the keys and made readConfig reject them with a loud error. The doc's claim 'the values are declared but unenforced — not new config surface' is now wrong: adding those keys fails config loading. The doc is maintained when config changes (its table annotates clarify.max_rounds with M16.0/M17.0 history) and now contradicts docs/cost-budget-design.md, the declared home of the future budget feature.
    location: docs/loop-architecture-design.md:263
  - f_012 severity=medium category=quality blocking=false status=open: `usage --all` human ordering and `--top` behavior are not tested; only JSON sorting/capping is pinned.
    location: internal/app/usage_test.go:397
  - f_013 severity=medium category=quality blocking=false status=open: Workspace effective-units aggregation is not asserted in tests.
    location: internal/app/usage_test.go:355
  - f_014 severity=low category=quality blocking=false status=open: Legacy map manifest migration test does not assert the refreshed map status is fresh.
    location: internal/app/app_test.go:728

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
