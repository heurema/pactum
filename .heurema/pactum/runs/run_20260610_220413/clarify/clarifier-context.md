# Clarifier Context

## Run
- Run id: run_20260610_220413
- Run status: clarifying

## Contract draft
- Goal: Add an export command that dumps a run's full record as a single archive
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
- Total: 11
- Open: 9
- Blocking open: 7
- Converged: false
- Coverage by dimension (a zero-question dimension is unprobed, not necessarily settled):
  - terminology: total 2, answered 0, open 2, blocking open 2
  - scope: total 2, answered 1, open 1, blocking open 1
  - acceptance: total 1, answered 0, open 1, blocking open 0
  - edge_case: total 5, answered 1, open 4, blocking open 3
  - assumption: total 1, answered 0, open 1, blocking open 1
- Questions:
  - q_001 blocking=true status=open: What should 'full record' include for export: only `.heurema/pactum/runs/<run_id>/`, that run directory plus filtered workspace-level ledger events, generated map/context artifacts, raw `.log` transcripts, or accepted project memory outside the run?
    kind: terminology
    rationale: The repo has run-local paths in `contractRunPaths`, but lifecycle events are written to `.heurema/pactum/ledger/events.jsonl`. The actual workspace `.gitignore` ignores generated `runs/*/context/` and `*.log`, while structured execute/review/ledger artifacts are versionable. This changes archive contents, size, and privacy.
    recommended answer (confidence medium): Define 'full record' as all files currently under `.heurema/pactum/runs/<run_id>/`, including raw `.log` files if present, plus a generated export-only filtered copy of `.heurema/pactum/ledger/events.jsonl` containing only events for that run. Exclude project map, cache, tmp, locks, and global accepted memory unless the file already exists inside the run directory.
  - q_002 blocking=true status=open: Where should the export command live and what CLI shape should it expose: a top-level `pactum export`, a `pactum task export`, or another command group?
    kind: scope
    rationale: The existing CLI has top-level groups for lifecycle stages and no `run` group. The goal says 'Add an export command' but does not name the command path, arguments, or output flag.
    recommended answer (confidence medium): Add a top-level read-oriented command `pactum export [run_id] --output <path> [--json]`. Resolve omitted `run_id` using the same current-run/sole-active behavior as other read-only run commands.
  - q_003 blocking=true status=open: What archive format and internal path layout should 'single archive' mean: `.zip`, `.tar.gz`, or another format, and should paths be rooted at the run id or copied exactly relative to `.heurema/pactum/`?
    kind: terminology
    rationale: The repository currently ignores `.zip` and `.tar` in project-map scans but has no archive implementation. Format and path layout affect cross-platform behavior and tests.
    recommended answer (confidence medium): Produce a ZIP archive with deterministic, slash-separated relative entries rooted at `pactum-run-<run_id>/`, with stable sorted file order and no absolute local paths introduced by the exporter.
    depends on: q_001
    blocked: waiting on unanswered prerequisites
  - q_004 blocking=true status=open: Should exporting mutate Pactum state, such as appending an `export_*` event to the ledger or updating `run.json`, or should it be read-only apart from writing the archive file?
    kind: assumption
    rationale: Existing read-only commands do not append ledger events, but export creates an external artifact. Mutating state would make export part of the audited run record; read-only export keeps repeated exports from changing the record being exported.
    recommended answer (confidence high): Treat export as read-only Pactum state: do not update `run.json`, contract files, approval, or ledgers. The only write is the requested archive output path, and JSON/human output reports what was exported.
    depends on: q_002
    blocked: waiting on unanswered prerequisites
  - q_005 blocking=true status=open: For the concrete scenario where the requested output archive already exists, should `pactum export` overwrite it, fail, or require a force flag?
    kind: edge_case
    rationale: Archive creation is a filesystem write outside normal Pactum state. Overwriting could destroy a prior export, while adding `--force` expands scope.
    recommended answer (confidence high): Fail if the output path already exists. Do not add `--force` in this change unless explicitly requested later.
    depends on: q_002
    blocked: waiting on unanswered prerequisites
  - q_006 blocking=true status=open: For the concrete scenario where the output path is inside `.heurema/pactum/runs/<run_id>/`, should export reject it to avoid including the archive inside itself?
    kind: edge_case
    rationale: If the archive is created inside the directory being walked, the export can recursively include itself or produce nondeterministic contents.
    recommended answer (confidence high): Reject output paths inside the exported run directory. Write via a temporary sibling file and atomically rename on success when possible.
    depends on: q_002, q_003
    blocked: waiting on unanswered prerequisites
  - q_007 blocking=true status=open: For the concrete scenario where a run is still in progress or a file disappears while export is reading it, should export allow a partial snapshot, wait/lock, or fail?
    kind: edge_case
    rationale: Runs can be active and agent/review attempts may write files over time. The repo has lock directories but no current export semantics.
    recommended answer (confidence medium): Allow exporting any existing run status as a best-effort current on-disk snapshot, but fail the command if any file selected for export cannot be read during archive creation; remove the partial archive on failure.
    depends on: q_001, q_004
    blocked: waiting on unanswered prerequisites
  - q_008 blocking=false status=open: What acceptance checks should define completion for this export feature?
    kind: acceptance
    rationale: The contract draft has no acceptance criteria or validation commands. The repo standard requires `make check` before reporting changes.
    recommended answer (confidence high): Acceptance requires tests proving `pactum export [run_id] --output <path>` creates a ZIP containing the expected run files and filtered run events, supports `--json`, fails for missing runs, fails when output exists, rejects output inside the exported run directory, and leaves Pactum run/ledger state unchanged. Validation command: `make check`.
    depends on: q_001, q_002, q_003, q_004, q_005, q_006, q_007
    blocked: waiting on unanswered prerequisites
  - q_009 blocking=true status=answered: For the concrete scenario where `.heurema/pactum/runs/<run_id>/` contains a symlink or non-regular filesystem entry such as a FIFO, socket, or device file, should `pactum export` follow it, preserve it, skip it, or fail?
    kind: edge_case
    rationale: The current questions define archive contents broadly, but the repo does not define archive behavior for symlinks or special files. Following a symlink could leak files outside the run record, while preserving such entries in ZIP is platform-sensitive.
    recommended answer (confidence high): Export only regular files and directories. Do not follow symlinks, and fail the export if any symlink or non-regular filesystem entry is encountered under the selected run record.
    answer: Export only regular files and directories. Do not follow symlinks, and fail the export if any symlink or non-regular filesystem entry is encountered under the selected run record.
  - q_010 blocking=false status=answered: How should `--output <path>` be interpreted when the user passes a relative path from a subdirectory: relative to the invocation working directory, relative to the repository root, or relative to `.heurema/pactum/`?
    kind: scope
    rationale: The existing command-shape question names `--output <path>` but does not specify path resolution. Pactum has repo-relative path globs for contract scope, while normal file output flags usually resolve relative to the process working directory.
    recommended answer (confidence high): Accept absolute output paths. Resolve relative output paths against the process working directory where `pactum export` was invoked, and allow output outside the repository except for the explicit rejection of paths inside the exported run directory.
    answer: Accept absolute output paths. Resolve relative output paths against the process working directory where `pactum export` was invoked, and allow output outside the repository except for the explicit rejection of paths inside the exported run directory.
  - q_011 blocking=false status=open: For the concrete scenario where `--output reports/run.zip` is requested but the `reports/` parent directory does not exist, should export create the parent directories or fail?
    kind: edge_case
    rationale: Pactum's internal store creates parent directories for workspace artifacts, but this command writes a user-requested external artifact. Auto-creating arbitrary external directories could hide path typos.
    recommended answer (confidence medium): Fail if the output path's parent directory does not already exist. The command may create only its temporary archive file in that existing directory before atomically renaming it to the requested output path.

## Repository context
# Repository Context

Generated: 2026-06-10T22:04:13Z

Map run: map_20260610_211052
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-10T21:10:52Z

Repository root: `.`

## Summary

- Indexed files: 142
- Ignored files/directories: 1177
- Detected languages: 6
- Code items (best-effort hints): 1547

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

- Go: 114 file(s)
- Markdown: 22 file(s)
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
- `assets/agent-skills/pactum/...`
- `cmd/pactum/main.go`
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
- `internal/app/agent_attempt.go`
- `internal/app/agent_attempt_timeout_test.go`
- `internal/app/agent_attempt_transport_test.go`
- `internal/app/agent_output.go`
- `internal/app/agent_output_test.go`
- `internal/app/agent_resolve.go`
- `internal/app/agents_doctor.go`
- `internal/app/agents_doctor_test.go`
- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/app/attempt_paths_test.go`
- `internal/app/clarify.go`
- `internal/app/clarify_loop.go`
- `internal/app/clarify_loop_test.go`
- `internal/app/clarify_suggest.go`
- `internal/app/clarify_suggest_test.go`
- `internal/app/clarify_test.go`
- `internal/app/cli.go`
- `internal/app/cli_v2_test.go`
- `internal/app/commands.go`
- `internal/app/config.go`
- `internal/app/config_test.go`
- `internal/app/confirm.go`
- `internal/app/contract.go`
- `internal/app/contract_draft.go`
- `internal/app/contract_draft_test.go`
- `internal/app/dogfood_hardening_test.go`
- `internal/app/errors.go`
- `internal/app/execute.go`
- `internal/app/execute_report.go`
- `internal/app/execute_report_test.go`
- `internal/app/execute_test.go`
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
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientNilActivityIsSafe`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyDeniesWrites`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyRefusesPermissionRequests`
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
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestClarifySuggestCompletedDespiteTimeoutRunsAfterSuccess`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestExecuteRunCompletedDespiteTimeoutTakesSuccessPath`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestExecuteRunPlainTimeoutStillFailsWithTimeoutMessage`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifySuggestExplicitTimeoutOverridesConfigIdleDefault`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifySuggestMarksAttemptReadOnlyAndPassesModelSpec`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifySuggestOmittedTimeoutUsesConfigIdleDefault`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestContractDraftMarksAttemptReadOnlyAndPassesModelSpec`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestExecuteRunNegativeTimeoutIsRejected`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestExecuteRunPassesModelSpecAndStaysWriteStage`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestReviewFixPassesModelSpecAndStaysWriteStage`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestReviewRunMarksAttemptReadOnlyAndPassesModelSpec`
- `internal/app/agent_output_test.go`: `go_func` `app.TestAgentMessageTextEmptyExtractionFallsBackToRaw`
- `internal/app/agent_output_test.go`: `go_func` `app.TestAgentMessageTextExtractsClaudeResult`
- `internal/app/agent_output_test.go`: `go_func` `app.TestAgentMessageTextExtractsCodexAgentMessages`
- `internal/app/agent_output_test.go`: `go_func` `app.TestAgentMessageTextFallsBackToRawOutput`
- `internal/app/agent_output_test.go`: `go_func` `app.TestAgentMessageTextSeparatesGluedFenceFromProgressMessage`
- `internal/app/agent_output_test.go`: `go_func` `app.TestReadStageParsersExtractFromJSONWrappedAgentOutput`
- `internal/app/agents_doctor.go`: `go_method` `App.AgentsDoctor`
- `internal/app/agents_doctor_test.go`: `go_func` `app.TestAgentsDoctorBeforeInitPrintsGuidance`

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
  "query": "Add an export command that dumps a run's full record as a single archive",
  "queries": [
    "export",
    "command",
    "dumps",
    "full",
    "record",
    "single",
    "archive"
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
