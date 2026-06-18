# Contract Drafter Context

## Run
- Run id: run_20260618_110809
- Run status: contract_draft

## Contract goal
Plan-DAG slice 4a: the per-task execution NODE primitives, WITHOUT the topological loop. This is slice 4a of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 4 and the "Slice 4, finalized" section). Build and unit-test the building blocks the 4b loop will drive, on a hand-written single-task plan. NO topological scheduler, NO workspace snapshot/restore, NO commits, NO tasks-state execution loop, NO agent execution change (all slice 4b).

Context: slice 1 added plan.tasks[] to the hashed contract + structural validation; slice 2 has the drafter emit plans + `pactum plan show`; slice 3 added non-vacuous validation + single-pass `plan_review`. Each plan task has: id, title, depends_on[], context[] (evidence selectors: each with path + optional lines like "60-100" and/or symbol, plus why), expected_files[] (advisory), acceptance[], validation[] (frozen commands). This slice builds the deterministic node primitives.

In scope:
1. Context-pack builder: a deterministic function that resolves one task's context[] selectors into an evidence pack (markdown). For a path+lines selector, read the repo-relative file and emit the line-range slice in a fenced block tagged path:start-end; for a symbol selector, resolve the symbol to a path:start-end via the existing search/code-index (the same mechanism `pactum search --symbol` uses) and emit that slice; carry each selector's `why`. Append the task's acceptance, the task's frozen validation commands, and the relevant constitution constraints (goal, scope, paths_in_scope). Bound the output: cap each slice and the whole pack with an explicit truncation marker and a `pactum search` pointer (never silently truncate). The pack is deterministic given (contract, working tree). Compute its SHA-256. Write it to execute/context/<task_id>.md and return {path, sha256}. Use os.Lstat/read for file content; do not go through the ACP text-write path.

2. Per-task frozen-validation runner: run a task's validation[] commands via the same mechanism the gate already uses (sh -c with cwd=root, timeout, captured stdout/stderr/exit) and report pass (all commands exit 0) or fail with the failing command + output. Reuse the gate's command-runner; do not invent a new one.

3. Baseline-red checker: run a task's validation against the UNCHANGED working tree (before any executor attempt) and classify the result. A validation that is test-shaped (a test command, e.g. `go test ... -run ...`) and already passes (green) is reported as baseline_green = a hard block candidate, since a check that passes before the work is not a real check (this also catches the Go `go test -run NoSuchTest` exit-0 trap — a -run pattern matching no test exits 0). A build/lint validation that is green-before is reported as a softer signal (advisory, recorded), not an auto-block. Return a structured baseline result {status: red|green, shape: test|other, command, recommendation: proceed|block|signal}. This is a pure classification step in 4a; the 4b loop decides what to do with it.

4. execute.loop config gate: make the existing-but-unused pipeline.execute.loop slot valid and bounded. Add `execute` to the set of stages that accept a loop block, but for execute REJECT non-default patience/settle at config load (a binary node only has a meaningful `max`; settle is fixed at 1 and patience at 0). Only `execute.loop.max` is configurable; a config with execute.loop.patience or execute.loop.settle set to a non-default value is a clear load error.

5. Tests (Go, internal/app): context-pack resolves path+lines and symbol selectors deterministically (same input -> same bytes -> same sha256), includes acceptance/validation/why, and truncates with the marker when over the cap; the validation runner reports pass and fail correctly; baseline-red classifies a red test validation as proceed, an already-green test validation as block (including the `-run NoSuchTest` exit-0 case), and a green build/lint as signal; the execute.loop config gate accepts execute.loop.max and rejects execute.loop.patience/settle. Use hand-written single-task plans / fixtures; do not run real agents.

Out of scope (slice 4b and later): the topological scheduler / graph-drain; per-node loop.Run wiring + retry-with-feedback; the workspace content snapshot + restore-on-block + the two guards (whole-tree out-of-scope detector, clean-in-scope precondition); tasks-state.json execution state + the execution loop that mutates it; per-task ACP attempts / actually running the executor over the DAG; commit/contain semantics; the drain -> constitution gate integration; GO/NO-GO instrumentation; out-of-scope structured blockers. Do NOT change execute run's current single-shot behavior, gate, code_review, or memory.

Validation: go test ./internal/app -run 'Plan|Context|Pack|Baseline|Validation|Config|Execute', go test ./..., go build ./..., make check.

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

Generated: 2026-06-18T11:08:09Z

Map run: map_20260618_074126
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-18T07:41:26Z

Repository root: `.`

## Summary

- Indexed files: 167
- Ignored files/directories: 4652
- Detected languages: 6
- Code items (best-effort hints): 1882

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

- Go: 132 file(s)
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
- `internal/app/contract_plan_test.go`
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
- `internal/app/plan.go`
- `internal/app/plan_test.go`
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
  "query": "Plan-DAG slice 4a: the per-task execution NODE primitives, WITHOUT the topological loop. This is slice 4a of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 4 and the \"Slice 4, finalized\" section). Build and unit-test the building blocks the 4b loop will drive, on a hand-written single-task plan. NO topological scheduler, NO workspace snapshot/restore, NO commits, NO tasks-state execution loop, NO agent execution change (all slice 4b).\n\nContext: slice 1 added plan.tasks[] to the hashed contract + structural validation; slice 2 has the drafter emit plans + `pactum plan show`; slice 3 added non-vacuous validation + single-pass `plan_review`. Each plan task has: id, title, depends_on[], context[] (evidence selectors: each with path + optional lines like \"60-100\" and/or symbol, plus why), expected_files[] (advisory), acceptance[], validation[] (frozen commands). This slice builds the deterministic node primitives.\n\nIn scope:\n1. Context-pack builder: a deterministic function that resolves one task's context[] selectors into an evidence pack (markdown). For a path+lines selector, read the repo-relative file and emit the line-range slice in a fenced block tagged path:start-end; for a symbol selector, resolve the symbol to a path:start-end via the existing search/code-index (the same mechanism `pactum search --symbol` uses) and emit that slice; carry each selector's `why`. Append the task's acceptance, the task's frozen validation commands, and the relevant constitution constraints (goal, scope, paths_in_scope). Bound the output: cap each slice and the whole pack with an explicit truncation marker and a `pactum search` pointer (never silently truncate). The pack is deterministic given (contract, working tree). Compute its SHA-256. Write it to execute/context/\u003ctask_id\u003e.md and return {path, sha256}. Use os.Lstat/read for file content; do not go through the ACP text-write path.\n\n2. Per-task frozen-validation runner: run a task's validation[] commands via the same mechanism the gate already uses (sh -c with cwd=root, timeout, captured stdout/stderr/exit) and report pass (all commands exit 0) or fail with the failing command + output. Reuse the gate's command-runner; do not invent a new one.\n\n3. Baseline-red checker: run a task's validation against the UNCHANGED working tree (before any executor attempt) and classify the result. A validation that is test-shaped (a test command, e.g. `go test ... -run ...`) and already passes (green) is reported as baseline_green = a hard block candidate, since a check that passes before the work is not a real check (this also catches the Go `go test -run NoSuchTest` exit-0 trap — a -run pattern matching no test exits 0). A build/lint validation that is green-before is reported as a softer signal (advisory, recorded), not an auto-block. Return a structured baseline result {status: red|green, shape: test|other, command, recommendation: proceed|block|signal}. This is a pure classification step in 4a; the 4b loop decides what to do with it.\n\n4. execute.loop config gate: make the existing-but-unused pipeline.execute.loop slot valid and bounded. Add `execute` to the set of stages that accept a loop block, but for execute REJECT non-default patience/settle at config load (a binary node only has a meaningful `max`; settle is fixed at 1 and patience at 0). Only `execute.loop.max` is configurable; a config with execute.loop.patience or execute.loop.settle set to a non-default value is a clear load error.\n\n5. Tests (Go, internal/app): context-pack resolves path+lines and symbol selectors deterministically (same input -\u003e same bytes -\u003e same sha256), includes acceptance/validation/why, and truncates with the marker when over the cap; the validation runner reports pass and fail correctly; baseline-red classifies a red test validation as proceed, an already-green test validation as block (including the `-run NoSuchTest` exit-0 case), and a green build/lint as signal; the execute.loop config gate accepts execute.loop.max and rejects execute.loop.patience/settle. Use hand-written single-task plans / fixtures; do not run real agents.\n\nOut of scope (slice 4b and later): the topological scheduler / graph-drain; per-node loop.Run wiring + retry-with-feedback; the workspace content snapshot + restore-on-block + the two guards (whole-tree out-of-scope detector, clean-in-scope precondition); tasks-state.json execution state + the execution loop that mutates it; per-task ACP attempts / actually running the executor over the DAG; commit/contain semantics; the drain -\u003e constitution gate integration; GO/NO-GO instrumentation; out-of-scope structured blockers. Do NOT change execute run's current single-shot behavior, gate, code_review, or memory.\n\nValidation: go test ./internal/app -run 'Plan|Context|Pack|Baseline|Validation|Config|Execute', go test ./..., go build ./..., make check.",
  "queries": [
    "docs/contract-plan-dag-design.md",
    "snapshot/restore",
    "plan.tasks",
    "and/or",
    "search/code-index",
    "execute/context/\u003ctask_id\u003e.md",
    "os.Lstat/read",
    "stdout/stderr/exit"
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
