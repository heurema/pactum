# Clarifier Context

## Run
- Run id: run_20260612_161619
- Run status: clarifying

## Contract draft
- Goal: Combined config and usage polish slice. (1) Hide the unfinished budget surface: review.budget (mode/max_tokens) gates nothing real — remove it from the config surface entirely: writeDefaultConfigIfMissing stops emitting it, readConfig rejects a leftover review.budget key with a loud configuration error naming the key (the same pattern as the removed agent: key), and the warn-mode budget plumbing in the review loop is deleted. Token accounting in the usage ledger and the usage command are untouched by the removal; budget enforcement returns later as a designed feature whose home is docs/cost-budget-design.md. (2) Usage display polish: pactum usage --all leads with the workspace summary, sorts the per-run rows by total tokens descending, and gains a --top N flag to cap the run list; uncaptured calls stop rendering as zero-valued rows — by-agent and by-run breakdowns annotate them honestly (for example: codex — N calls, usage not reported by the agent) and zero-token captured rows remain distinguishable from uncaptured ones. (3) Effective cost units: usage output (per-run and --all, human and JSON) adds an effective-units metric computed per provider with documented multipliers — for anthropic: fresh input x1.0, cache write x1.25, cache read x0.1, output x5.0; for codex/openai: fresh input x1.0 (cache writes are free and count as fresh), cached read x0.1, output x5.0 — the multipliers live as named constants with a comment citing the standard price ratios, and a per-stage/per-attempt cache hit-rate (cache_read / (fresh + cache_write + cache_read)) is shown where cache fields exist. (4) Map staleness pin narrowed: the map manifest currently pins the SHA-256 of the whole config.yaml, so editing agents or review.panel falsely invalidates the map; pin a deterministic hash of only the canonicalized map: config section instead — map-parameter changes still invalidate, other config edits do not; a legacy manifest holding the old whole-file hash is treated as stale once (one final refresh migrates it). (5) docs/cost-budget-design.md gains a verified cache-economics section recording the researched facts the future budget feature must account in: per-provider write/read multipliers, cache scoping (anthropic org+workspace, machine+directory effective scope for Claude Code; openai machine-local routing with prompt_cache_key), the concurrent cold-start write race and the staggered-launch savings model for panel fan-out (planned as its own slice), and the rule that budgets must be denominated in effective units rather than raw tokens. Usage docs (flow.md or wherever usage is documented) and CHANGELOG updated; tests pin the budget-key rejection, the sorted/top output, the uncaptured annotation, the effective-units math per provider, the hit-rate, and the map-pin behavior (agents-edit keeps map fresh, map-edit invalidates, legacy manifest migrates).
- In scope:
  - none
- Out of scope:
  - none
- Acceptance criteria:
  - none
- Validation commands:
  - none
- Assumptions:
  - none

## Existing clarifications
- Total: 9
- Open: 5
- Blocking open: 3
- Converged: false
- Coverage by dimension (a zero-question dimension is unprobed, not necessarily settled):
  - terminology: total 3, answered 2, open 1, blocking open 1
  - scope: total 1, answered 0, open 1, blocking open 0
  - acceptance: total 1, answered 0, open 1, blocking open 1
  - edge_case: total 3, answered 1, open 2, blocking open 1
  - assumption: total 1, answered 1, open 0, blocking open 0
- Questions:
  - q_001 blocking=true status=answered: What concrete JSON and human-output shape is intended for the new "effective-units" metric: a floating-point `effective_units` field on totals and each usage breakdown, or a different name/type?
    kind: terminology
    rationale: The repo currently has `usageCounts` embedded in per-run and workspace usage responses with integer token fields only (`input_tokens`, `output_tokens`, `total_tokens`, cache fields). The requested multipliers include 1.25 and 0.1, so effective units can be fractional and the field shape affects tests and downstream JSON compatibility.
    recommended answer (confidence high): Add `effective_units` as a JSON number (float64) to the total usage counts and to each usage breakdown that reports token counts; render it in human output with two decimal places. Do not persist it in `UsageRecord`; compute it on demand from ledger records.
    answer: Add `effective_units` as a JSON number (float64) to the total usage counts and to each usage breakdown that reports token counts; render it in human output with two decimal places. Do not persist it in `UsageRecord`; compute it on demand from ledger records.
  - q_002 blocking=true status=open: For the requested "per-stage/per-attempt cache hit-rate", does "per-attempt" mean adding a new per-run `by_attempt` breakdown keyed by `attempt_id`, or only showing the existing aggregate cache ratio on stage rows?
    kind: terminology
    rationale: The current usage command has per-run `by_stage` and `by_agent`, and workspace `by_run`, `by_stage`, `by_agent`, and `by_model`; it does not expose an attempt-level breakdown even though `UsageRecord` has `AttemptID`. The wording names both stage and attempt, so silently choosing one would change the API surface.
    recommended answer (confidence medium): Keep the ledger unchanged, retain the existing `cache_read_ratio` JSON field for compatibility, label it as cache hit rate in human output, and add per-run `by_attempt` rows keyed by `attempt_id` with stage, agent, provider, token counts, effective_units, and cache_read_ratio. Workspace output does not need `by_attempt`.
    depends on: q_001
  - q_003 blocking=false status=open: Should `status` output also gain `effective_units` and the updated cache-hit-rate wording, or is this slice limited to `pactum usage` output?
    kind: scope
    rationale: The repo currently surfaces usage in both `status` and `pactum usage`, but the draft specifically says usage output per-run and `--all`, human and JSON. Extending `status` would broaden tests and user-facing JSON beyond the named command.
    recommended answer (confidence high): Limit this slice to `pactum usage` per-run and `pactum usage --all`, in both human and JSON output. Leave `status` usage fields unchanged except for any compile-time fallout from shared helper refactors.
    depends on: q_001, q_002
    blocked: waiting on unanswered prerequisites
  - q_004 blocking=true status=answered: For `pactum usage --all --top N`, should `N` cap only the sorted `by_run` list while workspace totals and other breakdowns still include all runs, and should non-positive values be rejected?
    kind: edge_case
    rationale: The current CLI has no `--top` flag. The goal says to lead with the workspace summary and cap the run list, which implies totals should remain workspace-wide, but JSON behavior and invalid values are not pinned. Existing local limit handling is mixed: some limits default on non-positive, while loop limits reject negatives.
    recommended answer (confidence high): `--top N` is valid only with `--all`; reject `N <= 0` with a usage error naming `--top`. Sort all run rows by total tokens descending, then cap only `by_run` in human and JSON output. Workspace totals, run count, calls, and by-stage/by-agent/by-model breakdowns continue to aggregate every run.
    answer: `--top N` is valid only with `--all`; reject `N <= 0` with a usage error naming `--top`. Sort all run rows by total tokens descending, then cap only `by_run` in human and JSON output. Workspace totals, run count, calls, and by-stage/by-agent/by-model breakdowns continue to aggregate every run.
  - q_005 blocking=true status=open: If an uncaptured ledger record has non-zero token fields, should those token fields contribute to token totals and effective_units, or should `captured=false` always mean usage is unknown?
    kind: edge_case
    rationale: Current aggregation sums token fields even when `Captured` is false, and an existing test fixture has an uncaptured record with non-zero totals. The draft says uncaptured calls should stop rendering as zero-valued rows and be annotated honestly, while zero-token captured rows remain distinguishable.
    recommended answer (confidence medium): Treat `captured=false` as unknown usage regardless of token fields: count the call under `uncaptured_calls`, exclude it from token totals and effective_units, and annotate it as usage not reported. A `captured=true` record with zero tokens remains a real zero and appears in totals as zero.
    depends on: q_001
  - q_006 blocking=false status=open: How should effective_units handle records whose provider is empty, `unknown`, or a future custom provider rather than `anthropic`, `codex`, or `openai`?
    kind: edge_case
    rationale: The existing provider resolver can yield non-standard provider strings for unknown agents or old/custom ledger records. The requested multipliers are defined only for Anthropic and Codex/OpenAI, so computing a neutral value versus omitting it changes totals.
    recommended answer (confidence medium): For unknown or unsupported providers, do not add to effective_units; keep raw token totals and captured/uncaptured counts. In human output, annotate unsupported-provider rows as `effective units unavailable`; in JSON, expose `effective_units` as 0 for those rows and include an `effective_units_unavailable_calls` count at the relevant aggregate levels.
    depends on: q_001
  - q_007 blocking=true status=answered: What exactly should "canonicalized map: config section" mean for the map staleness hash: raw YAML subtree bytes, or the normalized `mapConfig` struct after `readConfig`?
    kind: terminology
    rationale: The current map manifest stores a SHA-256 of the whole `.heurema/pactum/config.yaml`. The repo has a typed `mapConfig` with `max_file_bytes` and `code_index`, and YAML comments/order should not make the project map stale. The chosen canonicalization affects whether formatting-only changes invalidate the map.
    recommended answer (confidence high): Hash a deterministic serialization of the normalized `config.Map` struct after `readConfig`, including `max_file_bytes` and normalized `code_index`, not raw YAML bytes. Comments, key order, and unrelated config sections must not affect the hash.
    answer: Hash a deterministic serialization of the normalized `config.Map` struct after `readConfig`, including `max_file_bytes` and normalized `code_index`, not raw YAML bytes. Comments, key order, and unrelated config sections must not affect the hash.
  - q_008 blocking=true status=open: Should the map manifest add an explicit hash-scope marker when changing `config_hash` from whole-file to map-section semantics?
    kind: acceptance
    rationale: The current manifest has only `config_hash`, so a reader cannot tell from artifact shape whether the value is the old whole-file hash or the new map-section hash. The draft requires legacy manifests to be stale once and then migrate, which is easier and more testable with an explicit marker.
    recommended answer (confidence medium): Keep `config_hash` for the hash value and add a manifest field such as `config_hash_scope: "map"` on refresh. Treat manifests missing `config_hash_scope` as legacy whole-file pins: report stale once, and after refresh write the map-section hash plus the scope marker.
    depends on: q_007
  - q_009 blocking=true status=answered: For the "verified cache-economics" docs section, what source standard should count as verified: current official provider docs with citations, or the facts already listed in the contract draft without external citations?
    kind: assumption
    rationale: The repo’s `docs/cost-budget-design.md` contains older draft cost and budget text, but it does not verify the new cache-scope and multiplier facts requested in the draft. Provider cache behavior is external and can change, so the contract should say what verification means.
    recommended answer (confidence high): Verify against current official OpenAI/Codex and Anthropic/Claude documentation where available, cite the provider URLs in `docs/cost-budget-design.md`, and record any unsupported or inferred facts explicitly as implementation assumptions. Do not make tests depend on live network access.
    answer: Verify against current official OpenAI/Codex and Anthropic/Claude documentation where available, cite the provider URLs in `docs/cost-budget-design.md`, and record any unsupported or inferred facts explicitly as implementation assumptions. Do not make tests depend on live network access.

## Repository context
# Repository Context

Generated: 2026-06-12T16:16:19Z

Map run: map_20260612_073609
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-12T07:36:09Z

Repository root: `.`

## Summary

- Indexed files: 154
- Ignored files/directories: 1881
- Detected languages: 6
- Code items (best-effort hints): 1705

## How to navigate this map

- Start with the wiki: read `wiki/overview.md` first.
- The wiki is generated from deterministic facts (file inventory and manifests).
- Code items are best-effort navigation hints, not complete semantic truth.
- Unsupported languages/framework files may have no code items.
- Imports are not treated as primary code surface.
- Source files remain the source of truth.

## Wiki pages

- `wiki/overview.md` — Project map overview
- `wiki/structure.md` — Project structure
- `wiki/commands.md` — Commands
- `wiki/entrypoints.md` — Candidate entrypoints
- `wiki/config.md` — Configuration
- `wiki/tests.md` — Tests
- Area pages:
  - `wiki/areas/.github.md`
  - `wiki/areas/assets.md`
  - `wiki/areas/cmd.md`
  - `wiki/areas/docs.md`
  - `wiki/areas/internal.md`
  - `wiki/areas/scripts.md`

## Project map artifacts

- `files.jsonl` — deterministic per-file metadata.
- `hashes.jsonl` — per-file content hashes.
- `code-items.jsonl` — best-effort symbol hints (incomplete by design).
- `search.sqlite` — local full-text search index.
- `manifest.json` — map manifest listing all artifacts.

## Files / areas

### Detected languages

- Go: 124 file(s)
- Markdown: 24 file(s)
- Go module: 2 file(s)
- Make: 1 file(s)
- Shell: 1 file(s)
- YAML: 1 file(s)

### Top-level directories

- `.github/` (see `wiki/areas/.github.md`)
- `assets/` (see `wiki/areas/assets.md`)
- `cmd/` (see `wiki/areas/cmd.md`)
- `docs/` (see `wiki/areas/docs.md`)
- `internal/` (see `wiki/areas/internal.md`)
- `scripts/` (see `wiki/areas/scripts.md`)

### Important files

- `README.md`
- `go.mod`
- `Makefile`

### File tree

- `.github/workflows/ci.yml`
- `.gitignore`
- `AGENTS.md`
- `CHANGELOG.md`
- `Makefile`
- `README.md`
- `SECURITY.md`
- `assets/agent-skills/pactum/...`
- `cmd/heurema-hygiene/main.go`
- `cmd/heurema-hygiene/main_test.go`
- `cmd/pactum/main.go`
- `docs/agent-file-navigation-design.md`
- `docs/agent-skill.md`
- `docs/agents.md`
- `docs/backlog.md`
- `docs/cost-budget-design.md`
- `docs/dogfood-run-context-retrieval.md`
- `docs/dogfood-second-repo.md`
- `docs/flow.md`
- `docs/install.md`
- `docs/loop-architecture-design.md`
- `docs/map-evaluation.md`
- `docs/memory.md`
- `docs/real-agent-execution-dogfood.md`
- `docs/review-prompt-design.md`
- `docs/skill-install.md`
- `docs/workspace.md`
- `go.mod`
- `go.sum`
- `internal/agents/acp_transport.go`
- `internal/agents/acp_transport_other.go`
- `internal/agents/acp_transport_test.go`
- `internal/agents/acp_transport_unix.go`
- `internal/agents/attempt.go`
- `internal/agents/config.go`
- `internal/agents/doctor.go`
- `internal/agents/dryrun.go`
- `internal/agents/executor.go`
- `internal/agents/executor_test.go`
- `internal/agents/infer.go`
- `internal/agents/infer_test.go`
- `internal/agents/model.go`
- `internal/agents/process_unix.go`
- `internal/agents/process_windows.go`
- `internal/agents/runner.go`
- `internal/agents/transport.go`
- `internal/agents/types.go`
- `internal/agents/usage.go`
- `internal/agents/usage_test.go`
- `internal/app/affordances_test.go`
- `internal/app/agent_attempt.go`
- `internal/app/agent_attempt_timeout_test.go`
- `internal/app/agent_attempt_transport_test.go`
- `internal/app/agent_output.go`
- `internal/app/agent_output_test.go`
- `internal/app/agent_resolve.go`
- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/app/attempt_paths_test.go`
- `internal/app/clarify.go`
- `internal/app/clarify_loop.go`
- `internal/app/clarify_loop_test.go`
- `internal/app/clarify_round.go`
- `internal/app/clarify_round_test.go`
- `internal/app/clarify_test.go`
- `internal/app/cli.go`
- `internal/app/cli_grammar_test.go`
- `internal/app/cli_v2_test.go`
- `internal/app/commands.go`
- `internal/app/config.go`
- `internal/app/config_test.go`
- `internal/app/contract.go`
- `internal/app/contract_draft.go`
- `internal/app/contract_draft_test.go`
- `internal/app/doctor.go`
- `internal/app/doctor_test.go`
- `internal/app/dogfood_hardening_test.go`
- `internal/app/errors.go`
- `internal/app/execute.go`
- `internal/app/execute_report.go`
- `internal/app/execute_report_test.go`
- `internal/app/execute_test.go`
- `internal/app/export.go`
- `internal/app/export_test.go`
- `internal/app/export_unix.go`
- `internal/app/export_windows.go`
- `internal/app/gate.go`
- `internal/app/gate_test.go`
- `internal/app/jsonio.go`
- `internal/app/map.go`
- `internal/app/map_quality_test.go`
- `internal/app/memory.go`
- `internal/app/memory_freshness.go`
- `internal/app/memory_freshness_test.go`
- `internal/app/memory_prompt_boundary_test.go`
- `internal/app/memory_selection.go`
- `internal/app/memory_selection_test.go`
- `internal/app/memory_test.go`
- `internal/app/path_scope.go`
- `internal/app/path_scope_test.go`
- `internal/app/principal.go`
- `internal/app/process.go`
- `internal/app/prompt.go`
- `internal/app/prompt_test.go`
- `internal/app/readiness.go`
- `internal/app/resolve.go`
- `internal/app/review.go`
- `internal/app/review_fix.go`
- `internal/app/review_fix_outcomes.go`
- `internal/app/review_loop.go`
- `internal/app/review_loop_test.go`
- `internal/app/review_proposals.go`
- `internal/app/review_proposals_test.go`
- `internal/app/review_test.go`
- `internal/app/run.go`
- `internal/app/run_context_test.go`
- `internal/app/status.go`
- `internal/app/store.go`
- `internal/app/store_swap_test.go`
- `internal/app/store_test.go`
- `internal/app/symbol_search_test.go`
- `internal/app/task.go`
- `internal/app/task_clarify_test.go`
- `internal/app/transport_selection_test.go`
- `internal/app/usage.go`
- `internal/app/usage_test.go`
- `internal/app/wiki_test.go`
- `internal/app/workspace.go`
- `internal/artifacts/paths.go`
- `internal/codeindex/extract.go`
- `internal/codeindex/extract_test.go`
- `internal/codeindex/registry.go`
- `internal/codeindex/treesitter.go`
- `internal/codeindex/types.go`
- `internal/docs/docs_test.go`
- `internal/docs/packaging_test.go`
- `internal/docs/skill_test.go`
- `internal/ledger/events.go`
- `internal/projectmap/render.go`
- `internal/projectmap/scan.go`
- `internal/projectmap/scan_test.go`
- `internal/projectmap/wiki.go`
- `internal/projectmap/wiki_test.go`
- `internal/search/index.go`
- `internal/search/query.go`
- `internal/search/search_test.go`
- `internal/search/symbol_test.go`
- `internal/search/types.go`
- `internal/store/store.go`
- `internal/version/version.go`
- `scripts/smoke.sh`

## Code surface (best-effort code hints)

- `cmd/heurema-hygiene/main.go`: `go_main` `main`
- `cmd/pactum/main.go`: `go_main` `main`
- `cmd/heurema-hygiene/main.go`: `go_main` `main.main`
- `cmd/heurema-hygiene/main_test.go`: `go_func` `main.TestHeuremaFiles`
- `cmd/heurema-hygiene/main_test.go`: `go_func` `main.TestScanFlagsLeaks`
- `cmd/heurema-hygiene/main_test.go`: `go_func` `main.TestScanIgnoresBareExamples`
- `cmd/heurema-hygiene/main_test.go`: `go_func` `main.TestScanIndexCountsFindingsForNonzeroExit`
- `cmd/heurema-hygiene/main_test.go`: `go_func` `main.TestScanIndexReadsStagedContent`
- `cmd/heurema-hygiene/main_test.go`: `go_func` `main.TestScanReportsEveryFindingWithLineNumbers`
- `cmd/heurema-hygiene/main_test.go`: `go_func` `main.TestScanSkipsBinaryContent`
- `cmd/pactum/main.go`: `go_main` `main.main`
- `internal/agents/acp_transport.go`: `go_method` `ACPTransport.Run`
- `internal/agents/acp_transport.go`: `go_type` `agents.ACPTransport`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommand`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommandReadOnly`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommandThreadsModelPin`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientActivityTicksOnAllInboundCalls`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientNeverSeparatesIdlessChunks`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientNilActivityIsSafe`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyDeniesWrites`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyRefusesPermissionRequests`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientSeparatesMessageBoundariesWithNewline`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientTokenUsage`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientTurnCompleted`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientWriteTextFileScopeGuard`
- `internal/agents/config.go`: `go_func` `agents.DefaultExecutor`
- `internal/agents/config.go`: `go_func` `agents.DefaultReviewer`
- `internal/agents/config.go`: `go_func` `agents.ListBuiltins`
- `internal/agents/config.go`: `go_method` `BuiltinRegistry.DefaultExecutor`
- `internal/agents/config.go`: `go_method` `BuiltinRegistry.DefaultReviewer`
- `internal/agents/config.go`: `go_method` `BuiltinRegistry.ListBuiltins`
- `internal/agents/config.go`: `go_method` `BuiltinRegistry.ResolveExecutor`
- `internal/agents/config.go`: `go_method` `BuiltinRegistry.ResolveReviewer`
- `internal/agents/config.go`: `go_type` `agents.BuiltinRegistry`
- `internal/agents/doctor.go`: `go_func` `agents.DiagnoseAgents`
- `internal/agents/doctor.go`: `go_type` `agents.AgentDoctor`
- `internal/agents/doctor.go`: `go_type` `agents.DoctorReport`
- `internal/agents/dryrun.go`: `go_func` `agents.BuildDryRunPlan`
- `internal/agents/executor.go`: `go_func` `agents.BuildCommand`
- `internal/agents/executor_test.go`: `go_func` `agents.TestApplyModelSpecEmitsBuiltInAgentArgs`
- `internal/agents/executor_test.go`: `go_func` `agents.TestBuildCommandUsesStdinForBuiltInAgents`
- `internal/agents/executor_test.go`: `go_func` `agents.TestReviewerBuiltinsAreReadOnly`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessClaudeFiltersNestedAgentMarker`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessCodexUsesTypedRunnerStdinAndEnv`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessIdleTimeoutAfterTerminalMarkerCompletesWithWarning`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessIdleTimeoutResetsOnOutput`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessTeesLiveOutput`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessTimesOutAfterIdleOutputGap`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessWithoutLiveOutputIsCaptureOnly`
- `internal/agents/infer.go`: `go_func` `agents.InferAgentFromModel`
- `internal/agents/infer_test.go`: `go_func` `agents.TestInferAgentFromModel`
- `internal/agents/model.go`: `go_func` `agents.ApplyModelSpec`
- `internal/agents/model.go`: `go_type` `agents.ModelSpec`
- `internal/agents/runner.go`: `go_func` `agents.RunSubprocess`
- `internal/agents/transport.go`: `go_method` `CLITransport.Run`
- `internal/agents/transport.go`: `go_type` `agents.CLITransport`
- `internal/agents/transport.go`: `go_type` `agents.Transport`
- `internal/agents/types.go`: `go_type` `agents.AgentDescriptor`
- `internal/agents/types.go`: `go_type` `agents.DryRunArtifacts`
- `internal/agents/types.go`: `go_type` `agents.DryRunChecks`
- `internal/agents/types.go`: `go_type` `agents.DryRunCommand`
- `internal/agents/types.go`: `go_type` `agents.DryRunPlan`
- `internal/agents/types.go`: `go_type` `agents.Registry`
- `internal/agents/types.go`: `go_type` `agents.RunRequest`
- `internal/agents/types.go`: `go_type` `agents.RunResult`
- `internal/agents/types.go`: `go_type` `agents.TokenUsage`
- `internal/agents/usage_test.go`: `go_func` `agents.TestAgentRunCompleted`
- `internal/agents/usage_test.go`: `go_func` `agents.TestFinalizeTimedOutAttemptEmptyPathSkipsDetection`
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseClaudeUsageNormalizesCacheAdditiveInput`
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseCodexUsageTakesLastCompletedEvent`
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseUsageMalformedOrEmptyOutputIsUncaptured`
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseUsageSkippedWhenStructuredOutputIsNotEnabled`
- `internal/app/affordances_test.go`: `go_func` `app.TestConditionalNextBranches`
- `internal/app/affordances_test.go`: `go_func` `app.TestEmittedAffordancesUseCurrentGrammar`
- `internal/app/affordances_test.go`: `go_func` `app.TestJSONErrorEnvelopePinnedPreconditions`
- `internal/app/affordances_test.go`: `go_func` `app.TestMemoryProposeNextMatchesHumanHints`
- `internal/app/affordances_test.go`: `go_func` `app.TestNextAffordancesAcrossLifecycleStages`
- `internal/app/affordances_test.go`: `go_func` `app.TestNextArraysMirrorStageAffordances`
- `internal/app/affordances_test.go`: `go_func` `app.TestNextCommandSwitchesToProposeForStaleMemoryCandidate`
- `internal/app/affordances_test.go`: `go_func` `app.TestNextDoesNotAdvertiseApproveForFailedGate`

## Language support

- File metadata is collected for common source, config, and documentation files.
- Best-effort code hints are extracted for the starter language pack: Go, Python, JavaScript, TypeScript/TSX/JSX, and C#.
- Code items are best-effort navigation hints; imports are not treated as primary code surface.
- Unsupported languages/framework files may have no code items but still appear in the wiki and file inventory.
- Pactum does not perform LSP, references, call graph, or semantic analysis in this phase.
- The map is a navigation aid, not complete semantic truth.
- Source files remain the source of truth.

## Agent guidance

- Read the wiki first (`wiki/overview.md`), then drill into the relevant area page.
- Before adding new code, search/read relevant files and code items.
- Prefer existing exported functions/types when applicable.
- If ownership is unclear, ask for clarification instead of guessing.

## Search results
{
  "query": "Combined config and usage polish slice. (1) Hide the unfinished budget surface: review.budget (mode/max_tokens) gates nothing real — remove it from the config surface entirely: writeDefaultConfigIfMissing stops emitting it, readConfig rejects a leftover review.budget key with a loud configuration error naming the key (the same pattern as the removed agent: key), and the warn-mode budget plumbing in the review loop is deleted. Token accounting in the usage ledger and the usage command are untouched by the removal; budget enforcement returns later as a designed feature whose home is docs/cost-budget-design.md. (2) Usage display polish: pactum usage --all leads with the workspace summary, sorts the per-run rows by total tokens descending, and gains a --top N flag to cap the run list; uncaptured calls stop rendering as zero-valued rows — by-agent and by-run breakdowns annotate them honestly (for example: codex — N calls, usage not reported by the agent) and zero-token captured rows remain distinguishable from uncaptured ones. (3) Effective cost units: usage output (per-run and --all, human and JSON) adds an effective-units metric computed per provider with documented multipliers — for anthropic: fresh input x1.0, cache write x1.25, cache read x0.1, output x5.0; for codex/openai: fresh input x1.0 (cache writes are free and count as fresh), cached read x0.1, output x5.0 — the multipliers live as named constants with a comment citing the standard price ratios, and a per-stage/per-attempt cache hit-rate (cache_read / (fresh + cache_write + cache_read)) is shown where cache fields exist. (4) Map staleness pin narrowed: the map manifest currently pins the SHA-256 of the whole config.yaml, so editing agents or review.panel falsely invalidates the map; pin a deterministic hash of only the canonicalized map: config section instead — map-parameter changes still invalidate, other config edits do not; a legacy manifest holding the old whole-file hash is treated as stale once (one final refresh migrates it). (5) docs/cost-budget-design.md gains a verified cache-economics section recording the researched facts the future budget feature must account in: per-provider write/read multipliers, cache scoping (anthropic org+workspace, machine+directory effective scope for Claude Code; openai machine-local routing with prompt_cache_key), the concurrent cold-start write race and the staggered-launch savings model for panel fan-out (planned as its own slice), and the rule that budgets must be denominated in effective units rather than raw tokens. Usage docs (flow.md or wherever usage is documented) and CHANGELOG updated; tests pin the budget-key rejection, the sorted/top output, the uncaptured annotation, the effective-units math per provider, the hit-rate, and the map-pin behavior (agents-edit keeps map fresh, map-edit invalidates, legacy manifest migrates).",
  "queries": [
    "mode/max_tokens",
    "docs/cost-budget-design.md",
    "codex/openai",
    "per-stage/per-attempt",
    "/",
    "config.yaml",
    "review.panel",
    "write/read"
  ],
  "query_source": "task",
  "results": [],
  "warnings": [
    "Search index is stale. Run: pactum map refresh."
  ]
}

## Clarifier guidance
- Propose only questions that need a human answer to improve the contract.
- Do not answer existing or proposed questions.
- Do not modify files.
- Avoid duplicates of existing clarification questions.
- Treat repository map/search context as navigation hints, not semantic truth.
