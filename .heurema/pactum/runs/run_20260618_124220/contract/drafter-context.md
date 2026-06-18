# Contract Drafter Context

## Run
- Run id: run_20260618_124220
- Run status: contract_draft

## Contract goal
Plan-DAG slice 4b: the minimal topological execute loop — run a contract's plan.tasks[] DAG node-by-node, sequentially, unattended. This is slice 4b of the plan-DAG arc and the pivotal slice; honor the finalized design in docs/contract-plan-dag-design.md ("Execution: a topological scheduler over internal/loop.Run", "Slice 4, finalized", "State split, gates, safety"). It drives the per-task node primitives shipped in slice 4a (context-pack builder, per-task frozen-validation runner, baseline-red classifier, the execute.loop config gate — all in internal/app/execute_node.go).

Context: slices 1-3 added the hashed plan.tasks[] + validation + drafter emission + plan_review. Slice 4a built the node primitives (buildContextPack, the per-task validation runner reusing the gate command runner, baseline-red classification, and the pipeline.execute.loop config gate that accepts only max). This slice wires them into the topological loop with the workspace boundary and execution state.

In scope:
1. Topological scheduler (outer, app-level graph-drain — NOT loop.Run): when the approved contract has plan.tasks[], execute run loads/creates execute/tasks-state.json (verify its contract_sha256 matches the prepared contract), then repeatedly picks the first ready task (all depends_on done) in deterministic task order, runs it, and continues; a blocked task stalls only its own subtree (mark unreached descendants blocked-upstream); independent branches keep running. If plan.tasks is absent, preserve the current single-shot execute run unchanged.

2. Per-node execution via internal/loop.Run(Limits{Max: execute.loop.max, Settle: 1, Patience: 0}): one Step = build the task prompt (stable contract prefix + the slice-4a context-pack as a late volatile suffix) + run one fresh ACP executor attempt (reuse the existing attempt machinery, namespaced per task under execute/tasks/<task_id>/attempts/) + run the task's frozen validation; RoundResult.Clean iff validation passes. Retry carries feedback: the prior attempt's validation-failure output is included in the next attempt's prompt suffix (a fresh-session retry without it just thrashes). loop.Run reason settled => task done; max => task blocked.

3. Baseline-red gating per node (using slice 4a's classifier): before the first executor attempt, classify the task's validation against the unchanged tree; an already-green test-shaped validation blocks the task immediately (no executor attempt); record the baseline result.

4. Workspace boundary — scoped content snapshot, git-independent (per the finalized design): at task start snapshot bytes+mode+type of files under paths_in_scope minus paths_out_of_scope into gitignored .heurema/pactum/tmp/; attribute changed/new/deleted by re-walk after the attempt. On pass: keep the changes (they accumulate in the working tree; no per-task commit). On block: preserve the failing diff as an artifact under the run, then restore the snapshot (modified->prior bytes+mode, new->delete, deleted->recreate); restore touches ONLY the in-scope set, never .heurema or out-of-scope paths. Handle binaries, exec-bit, symlinks (lstat). Restore must be crash-tolerant (write-temp-then-rename; idempotent replay from the manifest). Reject an empty/over-broad paths_in_scope before snapshotting.

5. Two guards (both required): (a) whole-tree out-of-scope detector — after each attempt, reuse buildGateChangeReport (whole-tree hash diff) and if any changed file is outside paths_in_scope, record an out-of-scope structured blocker (see 7) and do not propagate; (b) clean-in-scope precondition — refuse to start the loop if the working tree is dirty within paths_in_scope (exclude .heurema and unrelated dirt; not a global clean requirement).

6. execute/tasks-state.json (unhashed, single-writer whole-file rewrite, tied to contract_sha256, structurally incapable of holding nodes/edges/validation-definitions — those stay in the hashed contract): per task status (pending/ready/running/done/blocked/blocked-upstream), attempts, by, files_touched, snapshot ref, baseline result, validation result, blocker {reason, files, why, proposed} (strings only, human-facing). Plus a run-level terminal {state, ...}.

7. Out-of-scope and other blocks are STRUCTURED, not crashes; the loop is non-interactive and the human is reached via the driving agent. The node prompt instructs the executor to stay in scope and, if the task genuinely needs an out-of-scope file, to stop and declare a blocker (file + why) rather than force it. A block records blocker {reason: "out_of_scope"|"validation_unmet"|"baseline_green"|..., files, why, proposed: {paths_in_scope_add}} plus next commands, in tasks-state and the --json output, with the attempted diff as an artifact. Scope is never auto-widened; resolution is a human-approved contract revision relayed by the driving agent.

8. Drain + constitution gate + terminal states: when all tasks are done, run the contract's top-level validation.commands (constitution gate) via the existing gate machinery, then the existing gate/review flow consumes it. The run-level execution verdict is synthesized from tasks-state (all-done vs any-blocked), NOT from the chronologically-last attempt. If any task is blocked/blocked-upstream when no ready tasks remain, stop with terminal state blocked and do NOT run the constitution gate (a final gate over a partial tree is meaningless). Terminal states: completed, blocked, gate_failed, human, error.

9. GO/NO-GO instrumentation: record real, trustworthy signals — per-node retries, blocked nodes + blocker reasons, baseline-green rate, context-pack bytes, per-attempt token usage (from the existing ACP usage ledger) — into tasks-state and an execute/loop-summary.json, plus an execution_drained ledger event. Do NOT claim to measure handoff/context-loss as a loop metric (ACP exposes no file-read telemetry); that classification is a human/reviewer judgment.

Tests (Go, internal/app, no real agents — use the existing helper-process executor/fixtures): a linear 2-task plan runs both to done and the constitution gate runs; a fan-in plan (t3 depends on t1+t2) schedules in dependency order; a node whose validation never passes blocks after Max attempts and its subtree is blocked-upstream while an independent branch still completes; retry feeds the prior failure output into the next attempt; on block the in-scope snapshot is restored (modified/new/deleted, incl. mode/symlink) and .heurema + out-of-scope files are untouched; an out-of-scope change is detected and recorded as a structured out_of_scope blocker, not a crash; a dirty-in-scope tree is refused at start; an already-green test-shaped task is baseline-blocked with no executor attempt; tasks-state round-trips and a re-run resumes from it; a blocked run does not run the constitution gate; plan-less contracts still run single-shot unchanged.

Out of scope (later slices): single-writer lease + execute run --task <id> / --by <agent> (slice 6 — no second writer yet); parallel branch execution / worktrees; contract revisioning + bounded blocked-node auto-replan (slice 7); usage rows gaining task_id/role (slice 5); codex-executor out-of-scope auto-rollback + sandbox_mode=workspace-write (follow-on — for now codex/validation escapes are detected and hard-blocked, not auto-rolled-back). Do not change code_review or memory behavior.

Validation: go test ./internal/app -run 'Execute|Plan|Task|Loop|Gate|Context|Baseline', go test ./..., go build ./..., make check.

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

Generated: 2026-06-18T12:42:20Z

Map run: map_20260618_112443
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-18T11:24:43Z

Repository root: `.`

## Summary

- Indexed files: 169
- Ignored files/directories: 4768
- Detected languages: 6
- Code items (best-effort hints): 1922

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

- Go: 134 file(s)
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
- `internal/app/plan_review.go`
- `internal/app/plan_review_test.go`
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
  "query": "Plan-DAG slice 4b: the minimal topological execute loop — run a contract's plan.tasks[] DAG node-by-node, sequentially, unattended. This is slice 4b of the plan-DAG arc and the pivotal slice; honor the finalized design in docs/contract-plan-dag-design.md (\"Execution: a topological scheduler over internal/loop.Run\", \"Slice 4, finalized\", \"State split, gates, safety\"). It drives the per-task node primitives shipped in slice 4a (context-pack builder, per-task frozen-validation runner, baseline-red classifier, the execute.loop config gate — all in internal/app/execute_node.go).\n\nContext: slices 1-3 added the hashed plan.tasks[] + validation + drafter emission + plan_review. Slice 4a built the node primitives (buildContextPack, the per-task validation runner reusing the gate command runner, baseline-red classification, and the pipeline.execute.loop config gate that accepts only max). This slice wires them into the topological loop with the workspace boundary and execution state.\n\nIn scope:\n1. Topological scheduler (outer, app-level graph-drain — NOT loop.Run): when the approved contract has plan.tasks[], execute run loads/creates execute/tasks-state.json (verify its contract_sha256 matches the prepared contract), then repeatedly picks the first ready task (all depends_on done) in deterministic task order, runs it, and continues; a blocked task stalls only its own subtree (mark unreached descendants blocked-upstream); independent branches keep running. If plan.tasks is absent, preserve the current single-shot execute run unchanged.\n\n2. Per-node execution via internal/loop.Run(Limits{Max: execute.loop.max, Settle: 1, Patience: 0}): one Step = build the task prompt (stable contract prefix + the slice-4a context-pack as a late volatile suffix) + run one fresh ACP executor attempt (reuse the existing attempt machinery, namespaced per task under execute/tasks/\u003ctask_id\u003e/attempts/) + run the task's frozen validation; RoundResult.Clean iff validation passes. Retry carries feedback: the prior attempt's validation-failure output is included in the next attempt's prompt suffix (a fresh-session retry without it just thrashes). loop.Run reason settled =\u003e task done; max =\u003e task blocked.\n\n3. Baseline-red gating per node (using slice 4a's classifier): before the first executor attempt, classify the task's validation against the unchanged tree; an already-green test-shaped validation blocks the task immediately (no executor attempt); record the baseline result.\n\n4. Workspace boundary — scoped content snapshot, git-independent (per the finalized design): at task start snapshot bytes+mode+type of files under paths_in_scope minus paths_out_of_scope into gitignored .heurema/pactum/tmp/; attribute changed/new/deleted by re-walk after the attempt. On pass: keep the changes (they accumulate in the working tree; no per-task commit). On block: preserve the failing diff as an artifact under the run, then restore the snapshot (modified-\u003eprior bytes+mode, new-\u003edelete, deleted-\u003erecreate); restore touches ONLY the in-scope set, never .heurema or out-of-scope paths. Handle binaries, exec-bit, symlinks (lstat). Restore must be crash-tolerant (write-temp-then-rename; idempotent replay from the manifest). Reject an empty/over-broad paths_in_scope before snapshotting.\n\n5. Two guards (both required): (a) whole-tree out-of-scope detector — after each attempt, reuse buildGateChangeReport (whole-tree hash diff) and if any changed file is outside paths_in_scope, record an out-of-scope structured blocker (see 7) and do not propagate; (b) clean-in-scope precondition — refuse to start the loop if the working tree is dirty within paths_in_scope (exclude .heurema and unrelated dirt; not a global clean requirement).\n\n6. execute/tasks-state.json (unhashed, single-writer whole-file rewrite, tied to contract_sha256, structurally incapable of holding nodes/edges/validation-definitions — those stay in the hashed contract): per task status (pending/ready/running/done/blocked/blocked-upstream), attempts, by, files_touched, snapshot ref, baseline result, validation result, blocker {reason, files, why, proposed} (strings only, human-facing). Plus a run-level terminal {state, ...}.\n\n7. Out-of-scope and other blocks are STRUCTURED, not crashes; the loop is non-interactive and the human is reached via the driving agent. The node prompt instructs the executor to stay in scope and, if the task genuinely needs an out-of-scope file, to stop and declare a blocker (file + why) rather than force it. A block records blocker {reason: \"out_of_scope\"|\"validation_unmet\"|\"baseline_green\"|..., files, why, proposed: {paths_in_scope_add}} plus next commands, in tasks-state and the --json output, with the attempted diff as an artifact. Scope is never auto-widened; resolution is a human-approved contract revision relayed by the driving agent.\n\n8. Drain + constitution gate + terminal states: when all tasks are done, run the contract's top-level validation.commands (constitution gate) via the existing gate machinery, then the existing gate/review flow consumes it. The run-level execution verdict is synthesized from tasks-state (all-done vs any-blocked), NOT from the chronologically-last attempt. If any task is blocked/blocked-upstream when no ready tasks remain, stop with terminal state blocked and do NOT run the constitution gate (a final gate over a partial tree is meaningless). Terminal states: completed, blocked, gate_failed, human, error.\n\n9. GO/NO-GO instrumentation: record real, trustworthy signals — per-node retries, blocked nodes + blocker reasons, baseline-green rate, context-pack bytes, per-attempt token usage (from the existing ACP usage ledger) — into tasks-state and an execute/loop-summary.json, plus an execution_drained ledger event. Do NOT claim to measure handoff/context-loss as a loop metric (ACP exposes no file-read telemetry); that classification is a human/reviewer judgment.\n\nTests (Go, internal/app, no real agents — use the existing helper-process executor/fixtures): a linear 2-task plan runs both to done and the constitution gate runs; a fan-in plan (t3 depends on t1+t2) schedules in dependency order; a node whose validation never passes blocks after Max attempts and its subtree is blocked-upstream while an independent branch still completes; retry feeds the prior failure output into the next attempt; on block the in-scope snapshot is restored (modified/new/deleted, incl. mode/symlink) and .heurema + out-of-scope files are untouched; an out-of-scope change is detected and recorded as a structured out_of_scope blocker, not a crash; a dirty-in-scope tree is refused at start; an already-green test-shaped task is baseline-blocked with no executor attempt; tasks-state round-trips and a re-run resumes from it; a blocked run does not run the constitution gate; plan-less contracts still run single-shot unchanged.\n\nOut of scope (later slices): single-writer lease + execute run --task \u003cid\u003e / --by \u003cagent\u003e (slice 6 — no second writer yet); parallel branch execution / worktrees; contract revisioning + bounded blocked-node auto-replan (slice 7); usage rows gaining task_id/role (slice 5); codex-executor out-of-scope auto-rollback + sandbox_mode=workspace-write (follow-on — for now codex/validation escapes are detected and hard-blocked, not auto-rolled-back). Do not change code_review or memory behavior.\n\nValidation: go test ./internal/app -run 'Execute|Plan|Task|Loop|Gate|Context|Baseline', go test ./..., go build ./..., make check.",
  "queries": [
    "plan.tasks",
    "docs/contract-plan-dag-design.md",
    "internal/loop.Run",
    "execute.loop",
    "internal/app/execute_node.go",
    "pipeline.execute.loop",
    "loop.Run",
    "loads/creates"
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
