# Contract Drafter Context

## Run
- Run id: run_20260617_150710
- Run status: contract_draft

## Contract goal
Surface cache reuse and effective cost in the existing 'pactum usage' command. The data is already captured and computed — it is just not shown.

Today 'pactum usage' reports input/output/total tokens + 'captured N/M' per group (and in --json: input_tokens, output_tokens, total_tokens, captured_only, lower_bound). The usage records also carry cache_read_tokens and cache_creation_tokens, and internal/app/usage.go already computes effective_units (the provider-weighted cost proxy: fresh×1.0 + cache_write×{1.25 anthropic | 1.0 openai} + cache_read×0.1 + output×5 via recordEffectiveUnits) — but the summary response and the human table do not expose them.

Add to BOTH the human table and the --json output, per group AND for the TOTAL:
- cache_read_tokens and cache_creation_tokens (sum per group/total).
- cache_read_ratio = cache_read_tokens / input_tokens (0 when input is 0). In the human table render it as a percent column (e.g. 'cache 94%').
- effective_units = the already-computed provider-weighted proxy, summed per group and total. Render as a column in the human table and a field in JSON.

The motivation: raw input is misleading because most of it is cheap cache-read (0.1x), so effective_units is the number that actually bills and cache_read_ratio explains why raw input overstates cost. The human table should make this legible at a glance — suggested columns: STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED. Keep the existing TOTAL row + coverage line + lower-bound behavior. Uncaptured records contribute no tokens, no cache, no effective units (consistent with today).

Constraints: do NOT change the usage-recording path, the effective_units formula/multipliers, or any other command; do NOT add dollar cost; keep --by stage|model|agent|provider, --all, --json, and the coverage/lower-bound behavior intact; bump the JSON schema additively (still pactum.usage_summary.v1alpha1, additive fields only). Validation: go test ./internal/app -run Usage, go test ./..., go build ./..., make check.

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
- None

## Repository context
# Repository Context

Generated: 2026-06-17T15:07:10Z

Map run: map_20260617_150709
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-17T15:07:09Z

Repository root: `.`

## Summary

- Indexed files: 163
- Ignored files/directories: 3781
- Detected languages: 6
- Code items (best-effort hints): 1828

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

- Go: 129 file(s)
- Markdown: 28 file(s)
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
- `docs/agent-tool-trace-design.md`
- `docs/agents.md`
- `docs/backlog.md`
- `docs/contract-review-design.md`
- `docs/contract-revise-cli-design.md`
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
- `docs/token-efficiency-research.md`
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
- `internal/agents/dryrun_test.go`
- `internal/agents/executor_test.go`
- `internal/agents/infer.go`
- `internal/agents/infer_test.go`
- `internal/agents/model.go`
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
- `internal/app/cli_test.go`
- `internal/app/commands.go`
- `internal/app/config.go`
- `internal/app/config_test.go`
- `internal/app/contract.go`
- `internal/app/contract_draft.go`
- `internal/app/contract_draft_test.go`
- `internal/app/contract_review.go`
- `internal/app/contract_review_test.go`
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
- `internal/app/gate_unix.go`
- `internal/app/gate_windows.go`
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
- `internal/app/review_stagger_test.go`
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
- `internal/loop/loop.go`
- `internal/loop/loop_test.go`
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
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommandOverrideIgnoresEmptyAndWhitespace`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommandOverrideIsSingleExecutable`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommandOverrideReplacesOnlyDefaultPrefix`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommandReadOnly`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPAdapterCommandThreadsModelPin`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientActivityTicksOnAllInboundCalls`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientCodexACPMetadataCapturesExplicitZeroTotal`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientCodexACPMetadataKeepsLatestValidTotals`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientCodexACPMetadataMissesPreserveNoUsageWarning`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientFiresFirstOutputOnFirstAgentMessage`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientNeverSeparatesIdlessChunks`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientNilActivityIsSafe`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientNilFirstOutputIsSafe`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientPromptResponseUsageWinsOverCodexACPMetadata`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyDeniesWrites`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyRefusesPermissionRequests`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientSeparatesMessageBoundariesWithNewline`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientTokenUsage`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientTokenUsageFallsBackToCodexACPMetadata`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientTokenUsageFallsBackToCodexACPMetadataWithoutReasoning`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientTurnCompleted`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientWriteTextFileScopeGuard`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPTransportWritesNoUsageWarning`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestDriveACPSessionCapturesPromptResponseUsage`
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
- `internal/agents/dryrun.go`: `go_func` `agents.BuildACPWouldRun`
- `internal/agents/dryrun.go`: `go_func` `agents.BuildDryRunPlan`
- `internal/agents/dryrun_test.go`: `go_func` `agents.TestBuildACPWouldRun`
- `internal/agents/dryrun_test.go`: `go_func` `agents.TestBuildDryRunPlan_HomePath`
- `internal/agents/dryrun_test.go`: `go_func` `agents.TestSanitizeHomePath`
- `internal/agents/executor_test.go`: `go_func` `agents.TestApplyModelSpecClaudeIsNoOp`
- `internal/agents/executor_test.go`: `go_func` `agents.TestApplyModelSpecCodexIsNoOp`
- `internal/agents/executor_test.go`: `go_func` `agents.TestClaudeDescriptorHasNoCLIArgs`
- `internal/agents/executor_test.go`: `go_func` `agents.TestReviewerBuiltinsHaveNoWriteBypassArgs`
- `internal/agents/infer.go`: `go_func` `agents.InferAgentFromModel`
- `internal/agents/infer_test.go`: `go_func` `agents.TestInferAgentFromModel`
- `internal/agents/model.go`: `go_func` `agents.ApplyModelSpec`
- `internal/agents/model.go`: `go_type` `agents.ModelSpec`
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
- `internal/app/affordances_test.go`: `go_func` `app.TestConditionalNextBranches`
- `internal/app/affordances_test.go`: `go_func` `app.TestEmittedAffordancesUseCurrentGrammar`
- `internal/app/affordances_test.go`: `go_func` `app.TestJSONErrorEnvelopePinnedPreconditions`
- `internal/app/affordances_test.go`: `go_func` `app.TestMemoryProposeNextMatchesHumanHints`
- `internal/app/affordances_test.go`: `go_func` `app.TestNextAffordancesAcrossLifecycleStages`

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
  "query": "Surface cache reuse and effective cost in the existing 'pactum usage' command. The data is already captured and computed — it is just not shown.\n\nToday 'pactum usage' reports input/output/total tokens + 'captured N/M' per group (and in --json: input_tokens, output_tokens, total_tokens, captured_only, lower_bound). The usage records also carry cache_read_tokens and cache_creation_tokens, and internal/app/usage.go already computes effective_units (the provider-weighted cost proxy: fresh×1.0 + cache_write×{1.25 anthropic | 1.0 openai} + cache_read×0.1 + output×5 via recordEffectiveUnits) — but the summary response and the human table do not expose them.\n\nAdd to BOTH the human table and the --json output, per group AND for the TOTAL:\n- cache_read_tokens and cache_creation_tokens (sum per group/total).\n- cache_read_ratio = cache_read_tokens / input_tokens (0 when input is 0). In the human table render it as a percent column (e.g. 'cache 94%').\n- effective_units = the already-computed provider-weighted proxy, summed per group and total. Render as a column in the human table and a field in JSON.\n\nThe motivation: raw input is misleading because most of it is cheap cache-read (0.1x), so effective_units is the number that actually bills and cache_read_ratio explains why raw input overstates cost. The human table should make this legible at a glance — suggested columns: STAGE/KEY, INPUT, cache%, OUTPUT, EFFECTIVE, CAPTURED. Keep the existing TOTAL row + coverage line + lower-bound behavior. Uncaptured records contribute no tokens, no cache, no effective units (consistent with today).\n\nConstraints: do NOT change the usage-recording path, the effective_units formula/multipliers, or any other command; do NOT add dollar cost; keep --by stage|model|agent|provider, --all, --json, and the coverage/lower-bound behavior intact; bump the JSON schema additively (still pactum.usage_summary.v1alpha1, additive fields only). Validation: go test ./internal/app -run Usage, go test ./..., go build ./..., make check.",
  "queries": [
    "input/output/total",
    "N/M",
    "internal/app/usage.go",
    "group/total",
    "/",
    "e.g",
    "STAGE/KEY",
    "formula/multipliers"
  ],
  "query_source": "task",
  "results": [
    {
      "rank": 1,
      "id": "code_item:internal/app/usage.go:go_method:Usage:169",
      "kind": "code_item",
      "path": "internal/app/usage.go",
      "title": "Usage",
      "language": "go",
      "code_kind": "go_method",
      "start_line": 169,
      "end_line": 193,
      "signature": "func (a App) Usage(stdout io.Writer, runID string, by string, jsonOutput bool) error",
      "score": -5.542922800872789,
      "snippet": "...internal/app/usage.go",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 2,
      "id": "file:internal/app/usage.go",
      "kind": "file",
      "path": "internal/app/usage.go",
      "title": "usage.go",
      "language": "Go",
      "code_kind": "source",
      "score": -5.385583309969612,
      "snippet": "path: internal/app/usage.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 3,
      "id": "file:internal/app/usage_test.go",
      "kind": "file",
      "path": "internal/app/usage_test.go",
      "title": "usage_test.go",
      "language": "Go",
      "code_kind": "source",
      "score": -5.252754884910349,
      "snippet": "path: internal/app/usage_test.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 4,
      "id": "code_item:internal/app/usage.go:go_type:UsageRecord:49",
      "kind": "code_item",
      "path": "internal/app/usage.go",
      "title": "UsageRecord",
      "language": "go",
      "code_kind": "go_type",
      "start_line": 49,
      "end_line": 73,
      "signature": "UsageRecord struct",
      "score": -3.8870844941772784,
      "snippet": "...internal/app/usage.go",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 5,
      "id": "code_item:internal/app/usage_test.go:go_func:TestExecuteRunUsageRecordsRegistryAgentName:45",
      "kind": "code_item",
      "path": "internal/app/usage_test.go",
      "title": "TestExecuteRunUsageRecordsRegistryAgentName",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 45,
      "end_line": 74,
      "signature": "func TestExecuteRunUsageRecordsRegistryAgentName(t *testing.T)",
      "score": -3.622614194903602,
      "snippet": "...internal/app/usage_test.go",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 6,
      "id": "code_item:internal/app/usage_test.go:go_func:TestRunAgentAttemptLifecycleAppendsUsageRecord:16",
      "kind": "code_item",
      "path": "internal/app/usage_test.go",
      "title": "TestRunAgentAttemptLifecycleAppendsUsageRecord",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 16,
      "end_line": 43,
      "signature": "func TestRunAgentAttemptLifecycleAppendsUsageRecord(t *testing.T)",
      "score": -3.622614194903602,
      "snippet": "...internal/app/usage_test.go",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 7,
      "id": "code_item:internal/app/usage_test.go:go_func:TestStatusAndUsageCommandSumRunUsage:79",
      "kind": "code_item",
      "path": "internal/app/usage_test.go",
      "title": "TestStatusAndUsageCommandSumRunUsage",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 79,
      "end_line": 162,
      "signature": "func TestStatusAndUsageCommandSumRunUsage(t *testing.T)",
      "score": -3.622614194903602,
      "snippet": "...internal/app/usage_test.go",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 8,
      "id": "code_item:internal/app/usage_test.go:go_func:TestStatusAndUsageDegradeOnCorruptLedger:167",
      "kind": "code_item",
      "path": "internal/app/usage_test.go",
      "title": "TestStatusAndUsageDegradeOnCorruptLedger",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 167,
      "end_line": 205,
      "signature": "func TestStatusAndUsageDegradeOnCorruptLedger(t *testing.T)",
      "score": -3.622614194903602,
      "snippet": "...internal/app/usage_test.go",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 9,
      "id": "code_item:internal/app/usage_test.go:go_func:TestUsageAllRejectsRunIDArg:207",
      "kind": "code_item",
      "path": "internal/app/usage_test.go",
      "title": "TestUsageAllRejectsRunIDArg",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 207,
      "end_line": 218,
      "signature": "func TestUsageAllRejectsRunIDArg(t *testing.T)",
      "score": -3.622614194903602,
      "snippet": "...internal/app/usage_test.go",
      "source_query": "internal/app/usage.go"
    },
    {
      "rank": 10,
      "id": "code_item:internal/app/usage_test.go:go_func:TestUsageSummaryAll:297",
      "kind": "code_item",
      "path": "internal/app/usage_test.go",
      "title": "TestUsageSummaryAll",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 297,
      "end_line": 348,
      "signature": "func TestUsageSummaryAll(t *testing.T)",
      "score": -3.622614194903602,
      "snippet": "...internal/app/usage_test.go",
      "source_query": "internal/app/usage.go"
    }
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
