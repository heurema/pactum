# Contract Drafter Context

## Run
- Run id: run_20260617_182429
- Run status: contract_draft

## Contract goal
Add the plan-DAG schema and structural validation to the contract — slice 1 of the plan-DAG arc (see docs/contract-plan-dag-design.md). SCHEMA + VALIDATION ONLY: no drafter emission, no execution change, no plan-rendering command, no tasks-state, no execution loop (all later slices).

Add an optional 'plan' object to the contract (pactum.contract.v1alpha1), INSIDE the hashed contract (extend draftContract in internal/app/run.go, do not duplicate). plan.tasks is a list; each task: { id (string, required, non-empty), title (string), depends_on ([]string of task ids), context ([] of structured evidence selectors, each {path (string), lines (string, optional e.g. "60-100"), symbol (string, optional), why (string)}), expected_files ([]string, advisory), acceptance ([]string), validation ([]string) }. A contract MAY carry no plan (optional for now); when present, plan is part of the hashed contract and is preserved through contract show/revise like the other fields.

Add structural validation of the plan, enforced at contract load AND on 'contract revise', rejecting with a clear actionable error: (a) duplicate task id; (b) a depends_on entry referencing a task id that does not exist in plan.tasks; (c) a cycle in the depends_on DAG; (d) an expected_files entry outside paths_in_scope when paths_in_scope is non-empty; (e) an empty acceptance list or empty validation list on any task; (f) an empty or missing task id. A plan with zero tasks is allowed (treat as no plan).

Do NOT: change the drafter or any prompt (no auto-emission of plan.tasks), change execute/prompt build behaviour, add a 'plan show' command, add execute/tasks-state.json, or add the topological execution loop. Those are explicitly later slices.

Add focused Go tests: a valid plan is accepted and survives the contract hash + show/revise round-trip; and one test per rejection case (duplicate id, missing dependency, cycle, out-of-scope expected_file, empty acceptance, empty validation, empty id).

Validation: go test ./internal/app -run 'Plan', go test ./internal/app -run 'Contract', go test ./..., go build ./..., make check.

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

Generated: 2026-06-17T18:24:29Z

Map run: map_20260617_182427
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-17T18:24:27Z

Repository root: `.`

## Summary

- Indexed files: 164
- Ignored files/directories: 3941
- Detected languages: 6
- Code items (best-effort hints): 1831

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
- Markdown: 29 file(s)
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
- `docs/contract-plan-dag-design.md`
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
  "query": "Add the plan-DAG schema and structural validation to the contract — slice 1 of the plan-DAG arc (see docs/contract-plan-dag-design.md). SCHEMA + VALIDATION ONLY: no drafter emission, no execution change, no plan-rendering command, no tasks-state, no execution loop (all later slices).\n\nAdd an optional 'plan' object to the contract (pactum.contract.v1alpha1), INSIDE the hashed contract (extend draftContract in internal/app/run.go, do not duplicate). plan.tasks is a list; each task: { id (string, required, non-empty), title (string), depends_on ([]string of task ids), context ([] of structured evidence selectors, each {path (string), lines (string, optional e.g. \"60-100\"), symbol (string, optional), why (string)}), expected_files ([]string, advisory), acceptance ([]string), validation ([]string) }. A contract MAY carry no plan (optional for now); when present, plan is part of the hashed contract and is preserved through contract show/revise like the other fields.\n\nAdd structural validation of the plan, enforced at contract load AND on 'contract revise', rejecting with a clear actionable error: (a) duplicate task id; (b) a depends_on entry referencing a task id that does not exist in plan.tasks; (c) a cycle in the depends_on DAG; (d) an expected_files entry outside paths_in_scope when paths_in_scope is non-empty; (e) an empty acceptance list or empty validation list on any task; (f) an empty or missing task id. A plan with zero tasks is allowed (treat as no plan).\n\nDo NOT: change the drafter or any prompt (no auto-emission of plan.tasks), change execute/prompt build behaviour, add a 'plan show' command, add execute/tasks-state.json, or add the topological execution loop. Those are explicitly later slices.\n\nAdd focused Go tests: a valid plan is accepted and survives the contract hash + show/revise round-trip; and one test per rejection case (duplicate id, missing dependency, cycle, out-of-scope expected_file, empty acceptance, empty validation, empty id).\n\nValidation: go test ./internal/app -run 'Plan', go test ./internal/app -run 'Contract', go test ./..., go build ./..., make check.",
  "queries": [
    "docs/contract-plan-dag-design.md",
    "internal/app/run.go",
    "plan.tasks",
    "e.g",
    "show/revise",
    "execute/prompt",
    "execute/tasks-state.json",
    "/internal/app"
  ],
  "query_source": "task",
  "results": [
    {
      "rank": 1,
      "id": "file:docs/contract-plan-dag-design.md",
      "kind": "file",
      "path": "docs/contract-plan-dag-design.md",
      "title": "contract-plan-dag-design.md",
      "language": "Markdown",
      "code_kind": "doc",
      "score": -40.43069053643914,
      "snippet": "path: docs/contract-plan-dag-design.md\nlanguage: Markdown\nkind: doc\ntop_level: docs",
      "source_query": "docs/contract-plan-dag-design.md"
    },
    {
      "rank": 2,
      "id": "code_item:internal/app/app.go:go_func:Run:44",
      "kind": "code_item",
      "path": "internal/app/app.go",
      "title": "Run",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 44,
      "end_line": 51,
      "signature": "func Run(args []string, stdout, stderr io.Writer) int",
      "score": -6.339965053317829,
      "snippet": "...func Run(args []string, stdout, stderr io.Writer) int\npath: internal/app/app.go",
      "source_query": "internal/app/run.go"
    },
    {
      "rank": 3,
      "id": "wiki:tests.md",
      "kind": "wiki",
      "path": "map/wiki/tests.md",
      "title": "Tests",
      "language": "",
      "code_kind": "",
      "score": -1.8569774033924689,
      "snippet": "...test.go`\n- `internal/app/execute_report_test.go`\n- `internal/app/execute_test.go`\n- `internal/app/export...",
      "source_query": "execute/prompt"
    },
    {
      "rank": 4,
      "id": "wiki:areas/docs.md",
      "kind": "wiki",
      "path": "map/wiki/areas/docs.md",
      "title": "Area: docs",
      "language": "",
      "code_kind": "",
      "score": -23.156783113100296,
      "snippet": "...design.md`\n- `docs/agents.md`\n- `docs/backlog.md`\n- `docs/contract-plan-dag-design.md`\n- `docs/contract...",
      "source_query": "docs/contract-plan-dag-design.md"
    },
    {
      "rank": 5,
      "id": "file:internal/app/run.go",
      "kind": "file",
      "path": "internal/app/run.go",
      "title": "run.go",
      "language": "Go",
      "code_kind": "source",
      "score": -6.326864935683925,
      "snippet": "path: internal/app/run.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "internal/app/run.go"
    },
    {
      "rank": 6,
      "id": "wiki:areas/internal.md",
      "kind": "wiki",
      "path": "map/wiki/areas/internal.md",
      "title": "Area: internal",
      "language": "",
      "code_kind": "",
      "score": -1.4635259994487044,
      "snippet": "...execute.go`\n- `internal/app/execute_report.go`\n- `internal/app/execute_report_test.go`\n- `internal/app/execute...",
      "source_query": "execute/prompt"
    },
    {
      "rank": 7,
      "id": "file:internal/app/app.go",
      "kind": "file",
      "path": "internal/app/app.go",
      "title": "app.go",
      "language": "Go",
      "code_kind": "source",
      "score": -0.0000033136944725024485,
      "snippet": "path: internal/app/app.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "/internal/app"
    },
    {
      "rank": 8,
      "id": "wiki:structure.md",
      "kind": "wiki",
      "path": "map/wiki/structure.md",
      "title": "Project structure",
      "language": "",
      "code_kind": "",
      "score": -17.932480711982198,
      "snippet": "...design.md`\n- `docs/agents.md`\n- `docs/backlog.md`\n- `docs/contract-plan-dag-design.md`\n- `docs/contract...",
      "source_query": "docs/contract-plan-dag-design.md"
    },
    {
      "rank": 9,
      "id": "code_item:internal/app/app.go:go_method:Run:59",
      "kind": "code_item",
      "path": "internal/app/app.go",
      "title": "Run",
      "language": "go",
      "code_kind": "go_method",
      "start_line": 59,
      "end_line": 146,
      "signature": "func (a App) Run(args []string, stdout, stderr io.Writer) (code int)",
      "score": -6.083064388760952,
      "snippet": "...a App) Run(args []string, stdout, stderr io.Writer) (code int)\npath: internal/app/app.go",
      "source_query": "internal/app/run.go"
    },
    {
      "rank": 10,
      "id": "repo-map.md",
      "kind": "repo_map",
      "path": "map/repo-map.md",
      "title": "Repository map",
      "language": "",
      "code_kind": "",
      "score": -0.7940160776716785,
      "snippet": "...execute.go`\n- `internal/app/execute_report.go`\n- `internal/app/execute_report_test.go`\n- `internal/app/execute...",
      "source_query": "execute/prompt"
    }
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
