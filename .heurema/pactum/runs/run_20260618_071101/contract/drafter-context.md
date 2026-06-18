# Contract Drafter Context

## Run
- Run id: run_20260618_071101
- Run status: contract_draft

## Contract goal
Plan-DAG slice 3: the plan immune system (entry) — static non-vacuous validation + a single-pass `plan_review` pipeline stage. This is slice 3 of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 3, and the "Validation is the immune system" section). It is the blocking prerequisite that makes a plan DAG trustworthy BEFORE any unattended execution loop (slice 4). STATIC ENFORCEMENT + REVIEW HOOK ONLY: no execution change, no topological loop, no tasks-state, no context-pack.

Context: slice 1 put plan.tasks[] on the hashed contract and validates it structurally (duplicate ids, cycles, unresolved depends_on, expected_files outside paths_in_scope, empty acceptance/validation) at contract load and revise. Slice 2 lets the drafter emit a plan and added `pactum plan show`. The plan's per-task validation is the most exploitable seam: a weak executor under retry pressure can fake green by weakening a check rather than doing the work. This slice adds the parts of the immune system that are enforceable statically (now), and a review hook over the DAG.

In scope:
1. Non-vacuous per-task validation (extend validateContractPlan, enforced at contract load AND revise like the existing slice-1 rules): a task whose expected_files is non-empty must have at least one validation command that is SCOPED to its expected_files — i.e. at least one validation command string references one of the task's expected_files by path or by an enclosing directory/package segment of that path. Reject with a new actionable issue code (e.g. VACUOUS_VALIDATION) a task whose validation commands are all unscoped/global (none reference any expected_file path or its parent dir), since a check that a global no-op like `go build ./...` or `make check` satisfies regardless is not a real check for that task. A task with empty expected_files is exempt from this rule (cannot be checked). Keep the rule a conservative substring/path-segment match (low false positives); a task MAY also carry extra global commands as long as at least one scoped command exists. Add focused tests: scoped validation accepted; all-global validation rejected with VACUOUS_VALIDATION; mixed scoped+global accepted; empty expected_files exempt.

2. New `plan_review` pipeline stage, mirroring `contract_review`: add a PlanReview pipelineStage field to the pipeline config (yaml: plan_review) reusing the existing stageBy (by: scalar-or-list of agent names); empty/absent by means the stage is a human-gate-only no-op (no automated plan review). Add a `pactum plan review [run_id] [--json]` command that runs a SINGLE-PASS reviewer panel (NOT a convergence loop, no fixer) over the contract's plan.tasks[] DAG. Reviewer lenses for the plan: granularity (the DAG earns nodes only on real intra-contract fan-in or independently-validatable surfaces; target 3-10 leaves; a leaf is one independently reviewable patch, not one edit), dependency-correctness (depends_on edges are sensible and acyclic — structural cycle/missing-dep is already hard-rejected, this lens judges logical correctness), testability (each task's validation is falsifiable and scoped to its expected_files), non-vacuity (no task validation is a global no-op), and scope-fidelity (the plan's expected_files stay within paths_in_scope and cover the contract's goal). Reuse the EXISTING reviewer-findings capture machinery (the mandatory pactum.reviewer_findings.v1alpha1 block + parse-miss + corrective-retry path shipped in the reviewer-findings-capture change) so plan-review findings are captured structurally, never silently dropped. Persist plan-review findings as artifacts under the run (e.g. plan-review/). When the contract has no plan, `plan review` is a clear no-op (prints that there is no plan to review, exits 0). Single-pass: collect and report structured findings; do NOT auto-fix or loop — the operator addresses findings by revising the plan and re-approving.

3. Tests: non-vacuous validation cases (above); plan review on a contract with a plan produces structured findings artifacts; plan review on a plan-less contract is a clear no-op exiting 0; an absent/empty plan_review.by makes automated plan review a no-op; the plan-review reviewer prompt documents the lenses and the mandatory findings block; plan-review uses the structured capture (a reviewer that omits the block triggers the existing parse-miss/corrective-retry path rather than silently passing).

Out of scope (explicitly later slices): baseline-red enforcement (running each task's validation against the pre-change tree to confirm it fails — this is runtime and lands with the executor loop in slice 4); frozen-edit detection (auto-blocking a node whose validation was edited in the same commit as its implementation — runtime, slice 4); the topological execute loop; execute.loop.max config; context-pack resolution; tasks-state.json; single-writer lease; --task. Do not change execute, gate, code_review, or memory behavior. Do not make plan_review a convergence loop with a fixer (single-pass only this slice).

Validation: go test ./internal/app -run 'Contract|Plan|Config|Review', go test ./..., go build ./..., make check.

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

Generated: 2026-06-18T07:11:01Z

Map run: map_20260618_060341
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-18T06:03:41Z

Repository root: `.`

## Summary

- Indexed files: 165
- Ignored files/directories: 4467
- Detected languages: 6
- Code items (best-effort hints): 1855

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

- Go: 130 file(s)
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
  "query": "Plan-DAG slice 3: the plan immune system (entry) — static non-vacuous validation + a single-pass `plan_review` pipeline stage. This is slice 3 of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 3, and the \"Validation is the immune system\" section). It is the blocking prerequisite that makes a plan DAG trustworthy BEFORE any unattended execution loop (slice 4). STATIC ENFORCEMENT + REVIEW HOOK ONLY: no execution change, no topological loop, no tasks-state, no context-pack.\n\nContext: slice 1 put plan.tasks[] on the hashed contract and validates it structurally (duplicate ids, cycles, unresolved depends_on, expected_files outside paths_in_scope, empty acceptance/validation) at contract load and revise. Slice 2 lets the drafter emit a plan and added `pactum plan show`. The plan's per-task validation is the most exploitable seam: a weak executor under retry pressure can fake green by weakening a check rather than doing the work. This slice adds the parts of the immune system that are enforceable statically (now), and a review hook over the DAG.\n\nIn scope:\n1. Non-vacuous per-task validation (extend validateContractPlan, enforced at contract load AND revise like the existing slice-1 rules): a task whose expected_files is non-empty must have at least one validation command that is SCOPED to its expected_files — i.e. at least one validation command string references one of the task's expected_files by path or by an enclosing directory/package segment of that path. Reject with a new actionable issue code (e.g. VACUOUS_VALIDATION) a task whose validation commands are all unscoped/global (none reference any expected_file path or its parent dir), since a check that a global no-op like `go build ./...` or `make check` satisfies regardless is not a real check for that task. A task with empty expected_files is exempt from this rule (cannot be checked). Keep the rule a conservative substring/path-segment match (low false positives); a task MAY also carry extra global commands as long as at least one scoped command exists. Add focused tests: scoped validation accepted; all-global validation rejected with VACUOUS_VALIDATION; mixed scoped+global accepted; empty expected_files exempt.\n\n2. New `plan_review` pipeline stage, mirroring `contract_review`: add a PlanReview pipelineStage field to the pipeline config (yaml: plan_review) reusing the existing stageBy (by: scalar-or-list of agent names); empty/absent by means the stage is a human-gate-only no-op (no automated plan review). Add a `pactum plan review [run_id] [--json]` command that runs a SINGLE-PASS reviewer panel (NOT a convergence loop, no fixer) over the contract's plan.tasks[] DAG. Reviewer lenses for the plan: granularity (the DAG earns nodes only on real intra-contract fan-in or independently-validatable surfaces; target 3-10 leaves; a leaf is one independently reviewable patch, not one edit), dependency-correctness (depends_on edges are sensible and acyclic — structural cycle/missing-dep is already hard-rejected, this lens judges logical correctness), testability (each task's validation is falsifiable and scoped to its expected_files), non-vacuity (no task validation is a global no-op), and scope-fidelity (the plan's expected_files stay within paths_in_scope and cover the contract's goal). Reuse the EXISTING reviewer-findings capture machinery (the mandatory pactum.reviewer_findings.v1alpha1 block + parse-miss + corrective-retry path shipped in the reviewer-findings-capture change) so plan-review findings are captured structurally, never silently dropped. Persist plan-review findings as artifacts under the run (e.g. plan-review/). When the contract has no plan, `plan review` is a clear no-op (prints that there is no plan to review, exits 0). Single-pass: collect and report structured findings; do NOT auto-fix or loop — the operator addresses findings by revising the plan and re-approving.\n\n3. Tests: non-vacuous validation cases (above); plan review on a contract with a plan produces structured findings artifacts; plan review on a plan-less contract is a clear no-op exiting 0; an absent/empty plan_review.by makes automated plan review a no-op; the plan-review reviewer prompt documents the lenses and the mandatory findings block; plan-review uses the structured capture (a reviewer that omits the block triggers the existing parse-miss/corrective-retry path rather than silently passing).\n\nOut of scope (explicitly later slices): baseline-red enforcement (running each task's validation against the pre-change tree to confirm it fails — this is runtime and lands with the executor loop in slice 4); frozen-edit detection (auto-blocking a node whose validation was edited in the same commit as its implementation — runtime, slice 4); the topological execute loop; execute.loop.max config; context-pack resolution; tasks-state.json; single-writer lease; --task. Do not change execute, gate, code_review, or memory behavior. Do not make plan_review a convergence loop with a fixer (single-pass only this slice).\n\nValidation: go test ./internal/app -run 'Contract|Plan|Config|Review', go test ./..., go build ./..., make check.",
  "queries": [
    "docs/contract-plan-dag-design.md",
    "plan.tasks",
    "acceptance/validation",
    "i.e",
    "directory/package",
    "e.g",
    "unscoped/global",
    "/"
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
