# Contract Drafter Context

## Run
- Run id: run_20260617_060708
- Run status: contract_draft

## Contract goal
Port the code-review loop (internal/app/review_loop.go) onto the existing internal/loop engine, behaviour-preserving. The engine internal/loop.Run(ctx, Limits{Max,Patience,Settle}, Step) (Outcome, error) ALREADY EXISTS and is already used by contract_review.go — reuse it, do NOT modify or recreate it. Replace review_loop.go's hand-rolled 'for round := 1; round <= maxRounds; round++' loop with a loop.Run call whose Step closure runs the existing per-round work unchanged (reviewer fan-out across lenses, finding proposal/dedup/accept, fixer apply, gate run, stop-signal computation). Stop semantics must match today EXACTLY: max rounds -> existing max_rounds terminal, stalemate/patience -> existing terminal, clean_rounds convergence -> existing resolved terminal. Map review_loop's streak fields to the engine's signals exactly as contract_review did: RoundResult.Clean = (blocking findings this round == 0); RoundResult.Progress = the fixer ran AND the working tree changed (mirror contract_review's content-hash progress signal, adapted to the code-review fixer/working-tree). A clean round must leave the stale streak UNCHANGED (the engine already enforces this). Preserve ALL existing review_loop behaviour, the public JSON response shape and field names, ledger events, and existing tests. Constraints: do NOT change internal/loop/*, do NOT change contract_review.go or clarify_loop.go, do NOT change any config or config schema, no multi-model/best-of-N. Validation: go test ./internal/loop, go test ./internal/app -run TestReviewLoop, go test ./internal/app -run TestContractReview, go build ./..., go test ./..., make check.

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

Generated: 2026-06-17T06:07:08Z

Map run: map_20260617_060707
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-17T06:07:07Z

Repository root: `.`

## Summary

- Indexed files: 162
- Ignored files/directories: 3212
- Detected languages: 6
- Code items (best-effort hints): 1819

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
- Markdown: 27 file(s)
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
  "query": "Port the code-review loop (internal/app/review_loop.go) onto the existing internal/loop engine, behaviour-preserving. The engine internal/loop.Run(ctx, Limits{Max,Patience,Settle}, Step) (Outcome, error) ALREADY EXISTS and is already used by contract_review.go — reuse it, do NOT modify or recreate it. Replace review_loop.go's hand-rolled 'for round := 1; round \u003c= maxRounds; round++' loop with a loop.Run call whose Step closure runs the existing per-round work unchanged (reviewer fan-out across lenses, finding proposal/dedup/accept, fixer apply, gate run, stop-signal computation). Stop semantics must match today EXACTLY: max rounds -\u003e existing max_rounds terminal, stalemate/patience -\u003e existing terminal, clean_rounds convergence -\u003e existing resolved terminal. Map review_loop's streak fields to the engine's signals exactly as contract_review did: RoundResult.Clean = (blocking findings this round == 0); RoundResult.Progress = the fixer ran AND the working tree changed (mirror contract_review's content-hash progress signal, adapted to the code-review fixer/working-tree). A clean round must leave the stale streak UNCHANGED (the engine already enforces this). Preserve ALL existing review_loop behaviour, the public JSON response shape and field names, ledger events, and existing tests. Constraints: do NOT change internal/loop/*, do NOT change contract_review.go or clarify_loop.go, do NOT change any config or config schema, no multi-model/best-of-N. Validation: go test ./internal/loop, go test ./internal/app -run TestReviewLoop, go test ./internal/app -run TestContractReview, go build ./..., go test ./..., make check.",
  "queries": [
    "internal/app/review_loop.go",
    "internal/loop",
    "internal/loop.Run(ctx",
    "contract_review.go",
    "loop.Run",
    "proposal/dedup/accept",
    "stalemate/patience",
    "RoundResult.Clean"
  ],
  "query_source": "task",
  "results": [
    {
      "rank": 1,
      "id": "file:internal/app/review_loop.go",
      "kind": "file",
      "path": "internal/app/review_loop.go",
      "title": "review_loop.go",
      "language": "Go",
      "code_kind": "source",
      "score": -5.220135845141998,
      "snippet": "path: internal/app/review_loop.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "internal/app/review_loop.go"
    },
    {
      "rank": 2,
      "id": "file:internal/loop/loop.go",
      "kind": "file",
      "path": "internal/loop/loop.go",
      "title": "loop.go",
      "language": "Go",
      "code_kind": "source",
      "score": -4.584033323419715,
      "snippet": "path: internal/loop/loop.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "internal/loop"
    },
    {
      "rank": 3,
      "id": "code_item:internal/loop/loop.go:go_func:Run:68",
      "kind": "code_item",
      "path": "internal/loop/loop.go",
      "title": "Run",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 68,
      "end_line": 114,
      "signature": "func Run(ctx context.Context, limits Limits, step Step) (Outcome, error)",
      "score": -16.834172409724523,
      "snippet": "...func Run(ctx context.Context, limits Limits, step Step) (Outcome, error)\npath: internal/loop/loop.go",
      "source_query": "internal/loop.Run(ctx"
    },
    {
      "rank": 4,
      "id": "file:internal/app/contract_review.go",
      "kind": "file",
      "path": "internal/app/contract_review.go",
      "title": "contract_review.go",
      "language": "Go",
      "code_kind": "source",
      "score": -5.77102041566601,
      "snippet": "path: internal/app/contract_review.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "contract_review.go"
    },
    {
      "rank": 5,
      "id": "file:internal/app/review_loop_test.go",
      "kind": "file",
      "path": "internal/app/review_loop_test.go",
      "title": "review_loop_test.go",
      "language": "Go",
      "code_kind": "source",
      "score": -5.094496044915179,
      "snippet": "path: internal/app/review_loop_test.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "internal/app/review_loop.go"
    },
    {
      "rank": 6,
      "id": "file:internal/loop/loop_test.go",
      "kind": "file",
      "path": "internal/loop/loop_test.go",
      "title": "loop_test.go",
      "language": "Go",
      "code_kind": "source",
      "score": -4.500230016764984,
      "snippet": "path: internal/loop/loop_test.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "internal/loop"
    },
    {
      "rank": 7,
      "id": "file:internal/app/contract_review_test.go",
      "kind": "file",
      "path": "internal/app/contract_review_test.go",
      "title": "contract_review_test.go",
      "language": "Go",
      "code_kind": "source",
      "score": -5.6321217749464285,
      "snippet": "path: internal/app/contract_review_test.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "contract_review.go"
    },
    {
      "rank": 8,
      "id": "wiki:tests.md",
      "kind": "wiki",
      "path": "map/wiki/tests.md",
      "title": "Tests",
      "language": "",
      "code_kind": "",
      "score": -1.983430608451597,
      "snippet": "...go`\n- `internal/docs/skill_test.go`\n- `internal/loop/loop_test.go`\n- `internal/projectmap/scan_test.go...",
      "source_query": "loop.Run"
    },
    {
      "rank": 9,
      "id": "code_item:internal/app/review_loop_test.go:go_func:TestResolveReviewLoopReviewersPanelAllowsSameBuiltInTwice:564",
      "kind": "code_item",
      "path": "internal/app/review_loop_test.go",
      "title": "TestResolveReviewLoopReviewersPanelAllowsSameBuiltInTwice",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 564,
      "end_line": 590,
      "signature": "func TestResolveReviewLoopReviewersPanelAllowsSameBuiltInTwice(t *testing.T)",
      "score": -3.539967325219018,
      "snippet": "...internal/app/review_loop_test.go",
      "source_query": "internal/app/review_loop.go"
    },
    {
      "rank": 10,
      "id": "code_item:internal/loop/loop.go:go_type:HumanExit:21",
      "kind": "code_item",
      "path": "internal/loop/loop.go",
      "title": "HumanExit",
      "language": "go",
      "code_kind": "go_type",
      "start_line": 21,
      "end_line": 23,
      "signature": "HumanExit struct",
      "score": -4.419435448217817,
      "snippet": "...internal/loop/loop.go",
      "source_query": "internal/loop"
    }
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
