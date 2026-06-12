# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260612_161619
- Approval: approved
- Contract hash: e1a955baf9203b9456859cc7a5811657e95a69a04e3f1c57ea861ec4f2a6fa41

## Goal
Combined config and usage polish slice. (1) Hide the unfinished budget surface: review.budget (mode/max_tokens) gates nothing real — remove it from the config surface entirely: writeDefaultConfigIfMissing stops emitting it, readConfig rejects a leftover review.budget key with a loud configuration error naming the key (the same pattern as the removed agent: key), and the warn-mode budget plumbing in the review loop is deleted. Token accounting in the usage ledger and the usage command are untouched by the removal; budget enforcement returns later as a designed feature whose home is docs/cost-budget-design.md. (2) Usage display polish: pactum usage --all leads with the workspace summary, sorts the per-run rows by total tokens descending, and gains a --top N flag to cap the run list; uncaptured calls stop rendering as zero-valued rows — by-agent and by-run breakdowns annotate them honestly (for example: codex — N calls, usage not reported by the agent) and zero-token captured rows remain distinguishable from uncaptured ones. (3) Effective cost units: usage output (per-run and --all, human and JSON) adds an effective-units metric computed per provider with documented multipliers — for anthropic: fresh input x1.0, cache write x1.25, cache read x0.1, output x5.0; for codex/openai: fresh input x1.0 (cache writes are free and count as fresh), cached read x0.1, output x5.0 — the multipliers live as named constants with a comment citing the standard price ratios, and a per-stage/per-attempt cache hit-rate (cache_read / (fresh + cache_write + cache_read)) is shown where cache fields exist. (4) Map staleness pin narrowed: the map manifest currently pins the SHA-256 of the whole config.yaml, so editing agents or review.panel falsely invalidates the map; pin a deterministic hash of only the canonicalized map: config section instead — map-parameter changes still invalidate, other config edits do not; a legacy manifest holding the old whole-file hash is treated as stale once (one final refresh migrates it). (5) docs/cost-budget-design.md gains a verified cache-economics section recording the researched facts the future budget feature must account in: per-provider write/read multipliers, cache scoping (anthropic org+workspace, machine+directory effective scope for Claude Code; openai machine-local routing with prompt_cache_key), the concurrent cold-start write race and the staggered-launch savings model for panel fan-out (planned as its own slice), and the rule that budgets must be denominated in effective units rather than raw tokens. Usage docs (flow.md or wherever usage is documented) and CHANGELOG updated; tests pin the budget-key rejection, the sorted/top output, the uncaptured annotation, the effective-units math per provider, the hit-rate, and the map-pin behavior (agents-edit keeps map fresh, map-edit invalidates, legacy manifest migrates).

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

## Clarifications
- q_001 [blocking] What concrete JSON and human-output shape is intended for the new "effective-units" metric: a floating-point `effective_units` field on totals and each usage breakdown, or a different name/type?
  Rationale: The repo currently has `usageCounts` embedded in per-run and workspace usage responses with integer token fields only (`input_tokens`, `output_tokens`, `total_tokens`, cache fields). The requested multipliers include 1.25 and 0.1, so effective units can be fractional and the field shape affects tests and downstream JSON compatibility.
  Decision: Add `effective_units` as a JSON number (float64) to the total usage counts and to each usage breakdown that reports token counts; render it in human output with two decimal places. Do not persist it in `UsageRecord`; compute it on demand from ledger records.
- q_002 [blocking] For the requested "per-stage/per-attempt cache hit-rate", does "per-attempt" mean adding a new per-run `by_attempt` breakdown keyed by `attempt_id`, or only showing the existing aggregate cache ratio on stage rows?
  Rationale: The current usage command has per-run `by_stage` and `by_agent`, and workspace `by_run`, `by_stage`, `by_agent`, and `by_model`; it does not expose an attempt-level breakdown even though `UsageRecord` has `AttemptID`. The wording names both stage and attempt, so silently choosing one would change the API surface.
  Decision: Keep the ledger unchanged, retain the existing `cache_read_ratio` JSON field for compatibility, label it as cache hit rate in human output, and add per-run `by_attempt` rows keyed by `attempt_id` with stage, agent, provider, token counts, effective_units, and cache_read_ratio. Workspace output does not need `by_attempt`.
- q_003 Should `status` output also gain `effective_units` and the updated cache-hit-rate wording, or is this slice limited to `pactum usage` output?
  Rationale: The repo currently surfaces usage in both `status` and `pactum usage`, but the draft specifically says usage output per-run and `--all`, human and JSON. Extending `status` would broaden tests and user-facing JSON beyond the named command.
  Decision: Limit this slice to `pactum usage` per-run and `pactum usage --all`, in both human and JSON output. Leave `status` usage fields unchanged except for any compile-time fallout from shared helper refactors.
- q_004 [blocking] For `pactum usage --all --top N`, should `N` cap only the sorted `by_run` list while workspace totals and other breakdowns still include all runs, and should non-positive values be rejected?
  Rationale: The current CLI has no `--top` flag. The goal says to lead with the workspace summary and cap the run list, which implies totals should remain workspace-wide, but JSON behavior and invalid values are not pinned. Existing local limit handling is mixed: some limits default on non-positive, while loop limits reject negatives.
  Decision: `--top N` is valid only with `--all`; reject `N <= 0` with a usage error naming `--top`. Sort all run rows by total tokens descending, then cap only `by_run` in human and JSON output. Workspace totals, run count, calls, and by-stage/by-agent/by-model breakdowns continue to aggregate every run.
- q_005 [blocking] If an uncaptured ledger record has non-zero token fields, should those token fields contribute to token totals and effective_units, or should `captured=false` always mean usage is unknown?
  Rationale: Current aggregation sums token fields even when `Captured` is false, and an existing test fixture has an uncaptured record with non-zero totals. The draft says uncaptured calls should stop rendering as zero-valued rows and be annotated honestly, while zero-token captured rows remain distinguishable.
  Decision: Treat `captured=false` as unknown usage regardless of token fields: count the call under `uncaptured_calls`, exclude it from token totals and effective_units, and annotate it as usage not reported. A `captured=true` record with zero tokens remains a real zero and appears in totals as zero.
- q_006 How should effective_units handle records whose provider is empty, `unknown`, or a future custom provider rather than `anthropic`, `codex`, or `openai`?
  Rationale: The existing provider resolver can yield non-standard provider strings for unknown agents or old/custom ledger records. The requested multipliers are defined only for Anthropic and Codex/OpenAI, so computing a neutral value versus omitting it changes totals.
  Decision: For unknown or unsupported providers, do not add to effective_units; keep raw token totals and captured/uncaptured counts. In human output, annotate unsupported-provider rows as `effective units unavailable`; in JSON, expose `effective_units` as 0 for those rows and include an `effective_units_unavailable_calls` count at the relevant aggregate levels.
- q_007 [blocking] What exactly should "canonicalized map: config section" mean for the map staleness hash: raw YAML subtree bytes, or the normalized `mapConfig` struct after `readConfig`?
  Rationale: The current map manifest stores a SHA-256 of the whole `.heurema/pactum/config.yaml`. The repo has a typed `mapConfig` with `max_file_bytes` and `code_index`, and YAML comments/order should not make the project map stale. The chosen canonicalization affects whether formatting-only changes invalidate the map.
  Decision: Hash a deterministic serialization of the normalized `config.Map` struct after `readConfig`, including `max_file_bytes` and normalized `code_index`, not raw YAML bytes. Comments, key order, and unrelated config sections must not affect the hash.
- q_008 [blocking] Should the map manifest add an explicit hash-scope marker when changing `config_hash` from whole-file to map-section semantics?
  Rationale: The current manifest has only `config_hash`, so a reader cannot tell from artifact shape whether the value is the old whole-file hash or the new map-section hash. The draft requires legacy manifests to be stale once and then migrate, which is easier and more testable with an explicit marker.
  Decision: Keep `config_hash` for the hash value and add a manifest field such as `config_hash_scope: "map"` on refresh. Treat manifests missing `config_hash_scope` as legacy whole-file pins: report stale once, and after refresh write the map-section hash plus the scope marker.
- q_009 [blocking] For the "verified cache-economics" docs section, what source standard should count as verified: current official provider docs with citations, or the facts already listed in the contract draft without external citations?
  Rationale: The repo’s `docs/cost-budget-design.md` contains older draft cost and budget text, but it does not verify the new cache-scope and multiplier facts requested in the draft. Provider cache behavior is external and can change, so the contract should say what verification means.
  Decision: Verify against current official OpenAI/Codex and Anthropic/Claude documentation where available, cite the provider URLs in `docs/cost-budget-design.md`, and record any unsupported or inferred facts explicitly as implementation assumptions. Do not make tests depend on live network access.

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
- mem_002 [stale] score=59 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go
- mem_009 [fresh] score=53 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_007 [fresh] score=49 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_005 [fresh] score=40 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_006 [fresh] score=33 — Smooth the pipeline so no command is pure ritual, then compress the agent ski...

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
