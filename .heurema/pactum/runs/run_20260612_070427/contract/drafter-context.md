# Contract Drafter Context

## Run
- Run id: run_20260612_070427
- Run status: contract_draft

## Contract goal
Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-navigation-design.md). Make search results symbol-addressable so an agent's first read of a large file is a ranged read instead of a grep-window-rewindow loop. (1) Plumb the per-symbol data the code-items index already stores — StartLine, EndLine, and Signature (see codeindex.Item) — through the search layer into code_item results: extend the FTS5 document metadata or result hydration so search.Result carries start_line, end_line, and signature for kind=code_item hits (other kinds leave them empty), expose them in pactum search human output as path:start-end and the signature, and in search --json. (2) Add a --symbol <name> filter to pactum search that restricts results to code_item hits whose symbol name matches (exact match preferred, case-insensitive; document the matching rule), so an agent can resolve a known identifier straight to its address. (3) Render the same addresses in the executor context: renderExecutorContext search-result lines gain the line range and signature for code_item hits (path:start-end — signature), and the prompt guidance text tells the agent to read symbol ranges directly instead of scanning whole files. (4) The map/search index rebuild stays deterministic; index schema or stored-shape changes must keep pactum map refresh reproducible — same tree in, same index out; bump internal schema markers if the stored shape changes rather than silently rereading stale rows. Tests pin: a code_item search result carries the correct range and signature; --symbol returns exactly the matching symbols; executor-context rendering shows ranged addresses; non-code_item results are unchanged.

## Current contract fields
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

## Answered clarifications
- q_001 [blocking]: For `pactum search --symbol <name>`, should "symbol name" mean only `codeindex.Item.Name` / `search.Result.Title`, or should it also match qualified forms such as `Parent.Name`, package-qualified names, `Signature`, or `CodeKind`?
  Rationale: The repository's code-item documents carry several identifier-like fields (`Name`, `Parent`, `Package`, `Signature`, `Kind`). The contract says exact case-insensitive symbol name matching is preferred, but does not name which field is authoritative.
  Answer: `--symbol <name>` matches only `codeindex.Item.Name` as exposed in `search.Result.Title`, using exact case-insensitive comparison. It does not match `Parent`, `Package`, `Signature`, `CodeKind`, or synthesized qualified names.
- q_002 [blocking]: Should `--symbol` be usable as a standalone lookup (`pactum search --symbol Runner`) even though the current CLI requires a positional query, or only as a filter on an existing query (`pactum search Runner --symbol Runner`)?
  Rationale: The design doc and contract phrase the feature as `pactum search --symbol <name>` so agents can resolve a known identifier directly, but the current `searchCmd` requires `Query string arg`. This changes CLI grammar and query execution.
  Answer: Make the positional query optional when `--symbol` is provided. With only `--symbol`, return matching `code_item` symbols directly. When both query and `--symbol` are present, keep the lexical query and then restrict results to the exact symbol match.
- q_003 [blocking]: What should happen for an incompatible kind filter such as `pactum search Runner --symbol Runner --kind file` or `--kind import`?
  Rationale: The existing CLI supports `--kind any|repo_map|llms|wiki|file|code_item|import`, while the contract says `--symbol` restricts results to `code_item` hits. Silent empty results and implicit override would be different user-facing semantics.
  Answer: Allow `--symbol` only with omitted/default `--kind any` or explicit `--kind code_item`. Reject other `--kind` values with a clear usage error explaining that `--symbol` only applies to `code_item` results.
- q_004: If multiple files or parents define the same symbol name, for example two `Runner` code items in different packages, should `--symbol Runner` return all exact matches or treat the duplicate as ambiguous?
  Rationale: The code index can contain repeated `Name` values across paths, packages, methods, and tests. The contract says `--symbol` returns exactly matching symbols, but does not state whether duplicates are an error, all results, or only the top-ranked result.
  Answer: Return all exact case-insensitive `code_item` matches, ordered by the existing deterministic ranking and tie-breakers, capped by `--limit`. Do not report duplicates as an error; users can raise `--limit` if needed.
- q_005 [blocking]: For `pactum search --json`, should `start_line`, `end_line`, and `signature` be omitted from non-`code_item` results to keep their JSON shape unchanged, or included with zero/empty values because `search.Result` now has those fields?
  Rationale: The contract says only `kind=code_item` hits carry symbol metadata and that non-code_item results are unchanged, but it also says other kinds leave the new fields empty. The current `search.Result` JSON shape emits fixed fields, so adding non-omitempty fields would visibly change file/wiki/import JSON results.
  Answer: In JSON output, include `start_line`, `end_line`, and `signature` only for `kind=code_item` hits with valid symbol metadata. Omit those fields for `repo_map`, `llms`, `wiki`, `file`, and `import` results so non-code_item JSON results remain unchanged.
- q_006: If a `code_item` hit has incomplete range metadata, for example `StartLine=5` and `EndLine=0` in a fixture or an incompatible `search.sqlite` built before the new stored columns, what should search and executor-context rendering do?
  Rationale: Tree-sitter extraction normally sets 1-based start/end lines, but current tests and future fixtures can construct partial `codeindex.Item` values, and the existing search index has no schema marker today. The contract requires deterministic rebuilds and says stored-shape changes should not silently reread stale rows.
  Answer: Treat an incompatible/legacy search index schema as stale and tell the user to run `pactum map refresh`. For an individual `code_item` row with missing or invalid range data, still return the result but omit the ranged address and absent signature; never render `path:0-0` or a dangling separator.
- q_007: If two distinct `code_item` hits have the same `path`, `title`, and `code_kind` but different parents or ranges, for example `Cache.Start` and `Worker.Start` methods in the same file, should executor-context search results keep both ranged addresses or dedupe them as one result?
  Rationale: `runContextSearch` currently dedupes by `(kind, path, title, code_kind)`, while `codeindex.Item` can represent same-name methods/functions distinguished by `Parent`, `StartLine`, and `EndLine`. The contract's goal is symbol-addressable navigation, so collapsing distinct ranges would hide a valid symbol address from the executor context.
  Answer: Treat distinct `code_item` document IDs or distinct valid `start_line`/`end_line` ranges as distinct results in run-context search and executor-context rendering. Dedupe only the same underlying result resurfaced by multiple targeted queries.

## Repository context
# Repository Context

Generated: 2026-06-12T07:04:27Z

Map run: map_20260611_191857
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-11T19:18:57Z

Repository root: `.`

## Summary

- Indexed files: 150
- Ignored files/directories: 1790
- Detected languages: 6
- Code items (best-effort hints): 1637

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

- Go: 120 file(s)
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
- `internal/search/types.go`
- `internal/store/store.go`
- `internal/version/version.go`
- `scripts/smoke.sh`

## Code surface (best-effort code hints)

- `cmd/pactum/main.go`: `go_main` `main`
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
- `internal/app/affordances_test.go`: `go_func` `app.TestNextFallsBackToInspectionOnUnreadableClarifications`
- `internal/app/affordances_test.go`: `go_func` `app.TestNextSwitchesToProposeForStaleMemoryCandidate`
- `internal/app/affordances_test.go`: `go_func` `app.TestTaskNewClarifyLoopFailureJSONEnvelope`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestClarifierRoundCompletedDespiteTimeoutRunsAfterSuccess`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestExecuteRunCompletedDespiteTimeoutTakesSuccessPath`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestExecuteRunPlainTimeoutStillFailsWithTimeoutMessage`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifierRoundExplicitTimeoutOverridesConfigIdleDefault`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifierRoundMarksAttemptReadOnlyAndPassesModelSpec`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifierRoundOmittedTimeoutUsesConfigIdleDefault`

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
  "query": "Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-navigation-design.md). Make search results symbol-addressable so an agent's first read of a large file is a ranged read instead of a grep-window-rewindow loop. (1) Plumb the per-symbol data the code-items index already stores — StartLine, EndLine, and Signature (see codeindex.Item) — through the search layer into code_item results: extend the FTS5 document metadata or result hydration so search.Result carries start_line, end_line, and signature for kind=code_item hits (other kinds leave them empty), expose them in pactum search human output as path:start-end and the signature, and in search --json. (2) Add a --symbol \u003cname\u003e filter to pactum search that restricts results to code_item hits whose symbol name matches (exact match preferred, case-insensitive; document the matching rule), so an agent can resolve a known identifier straight to its address. (3) Render the same addresses in the executor context: renderExecutorContext search-result lines gain the line range and signature for code_item hits (path:start-end — signature), and the prompt guidance text tells the agent to read symbol ranges directly instead of scanning whole files. (4) The map/search index rebuild stays deterministic; index schema or stored-shape changes must keep pactum map refresh reproducible — same tree in, same index out; bump internal schema markers if the stored shape changes rather than silently rereading stale rows. Tests pin: a code_item search result carries the correct range and signature; --symbol returns exactly the matching symbols; executor-context rendering shows ranged addresses; non-code_item results are unchanged.",
  "queries": [
    "docs/agent-file-navigation-design.md",
    "codeindex.Item",
    "map/search",
    "file-navigation",
    "symbol-addressable",
    "grep-window-rewindow",
    "per-symbol",
    "code-items"
  ],
  "query_source": "task",
  "results": [],
  "warnings": [
    "Search index is stale. Run: pactum map refresh."
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
