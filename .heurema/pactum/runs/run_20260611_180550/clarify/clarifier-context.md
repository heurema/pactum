# Clarifier Context

## Run
- Run id: run_20260611_180550
- Run status: clarifying

## Contract draft
- Goal: Fix three valid external review findings. (1) pactum export must preserve its no-overwrite guarantee at rename time: today two concurrent exports targeting the same --output can both pass the early existence check and the final os.Rename silently replaces the file that appeared in the window; finalize through a no-replace path (e.g. os.Link from the staged temp file to the output then remove the temp, treating link EEXIST as the existing 'output path already exists' error; keep behavior correct on Windows where os.Rename already refuses to replace) so a concurrent winner is never clobbered. (2) pactum export must reject run-record relative paths containing backslashes before writing ZIP entry names: on Unix a backslash is a valid filename character and filepath.ToSlash does not rewrite it, which would emit a non-portable entry name that some Windows extractors treat as a separator; fail the export with a clear error naming the offending path. (3) memory accept must not accept a stale candidate: memory propose records a freshness pin of the review state (for example the review document's updated_at or a content hash) inside memory-candidate.json; memory accept verifies the pin against the current review document and fails with a clear error plus error.fix pactum memory propose <run_id> when the review changed after the candidate was generated; the next-affordance selection that advertises memory accept applies the same staleness check and advertises memory propose instead when stale. Tests cover: concurrent-window overwrite refusal (simulate by creating the output file between staging and finalize), backslash rejection, stale-candidate refusal with the fix affordance, and the next selector switching to propose on staleness.
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
- Total: 4
- Open: 2
- Blocking open: 2
- Converged: false
- Coverage by dimension (a zero-question dimension is unprobed, not necessarily settled):
  - terminology: total 2, answered 2, open 0, blocking open 0
  - scope: total 0, answered 0, open 0, blocking open 0
  - acceptance: total 0, answered 0, open 0, blocking open 0
  - edge_case: total 2, answered 0, open 2, blocking open 2
  - assumption: total 0, answered 0, open 0, blocking open 0
- Questions:
  - q_001 blocking=true status=answered: For the memory-staleness finding, should "review state" mean only `review/review.json` and its `updated_at`, or the full review state assembled by `buildReviewStateWithProposals` from `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl`, `review/proposals.jsonl`, and `review/proposal-decisions.jsonl`?
    kind: terminology
    rationale: `memory propose` currently builds the candidate from the full review state, including findings, resolutions, proposals, and proposal decisions. Some relevant review changes, such as proposal collection or rejection, are stored in JSONL artifacts and may not be represented by `review/review.json` alone, so pinning only `updated_at` could miss stale candidates.
    recommended answer (confidence high): Define the freshness pin as a deterministic content hash over the full review state used to build the memory candidate: `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl`, `review/proposals.jsonl`, and `review/proposal-decisions.jsonl`; `memory accept` and next-affordance selection compare that pin to the current full-state hash.
    answer: Define the freshness pin as a deterministic content hash over the full review state used to build the memory candidate: `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl`, `review/proposals.jsonl`, and `review/proposal-decisions.jsonl`; `memory accept` and next-affordance selection compare that pin to the current full-state hash.
  - q_002 blocking=true status=open: How should `pactum memory accept` handle an existing `memory/memory-candidate.json` that was generated before this change and therefore has no freshness pin?
    kind: edge_case
    rationale: Existing candidates in the current schema have no field that proves which review state they were generated from. Accepting such a candidate would bypass the new guarantee; rejecting it may affect backwards compatibility for already-proposed but unaccepted runs.
    recommended answer (confidence medium): Treat a missing or unrecognized freshness pin as stale/unverifiable: do not append a memory item, fail with a clear stale-candidate error, and direct the user to regenerate with `pactum memory propose <run_id>`.
    depends on: q_001
  - q_003 blocking=true status=answered: For export path validation, should "run-record relative paths containing backslashes" reject only literal `\` characters that remain in the ZIP entry name after platform separator conversion, rather than rejecting normal Windows path separators?
    kind: terminology
    rationale: On Unix, a backslash can be part of a filename and `filepath.ToSlash` leaves it unchanged, creating a non-portable ZIP entry. On Windows, backslash is the normal path separator and `filepath.ToSlash` should convert it to `/`; rejecting all relative paths containing `\` before conversion would break normal Windows exports.
    recommended answer (confidence high): Validate the final archive entry name after `filepath.ToSlash`; reject any entry name that still contains `\`, and report the offending run-relative path. This rejects Unix filenames with literal backslashes while preserving normal Windows separator conversion.
    answer: Validate the final archive entry name after `filepath.ToSlash`; reject any entry name that still contains `\`, and report the offending run-relative path. This rejects Unix filenames with literal backslashes while preserving normal Windows separator conversion.
  - q_004 blocking=true status=open: If a memory candidate is stale and the current review state also makes `pactum memory propose <run_id>` illegal, for example a new blocking finding reset review approval or a pending proposal was added, should `memory accept` still return `error.fix: pactum memory propose <run_id>` or should it point to review inspection first?
    kind: edge_case
    rationale: The finding asks for `error.fix pactum memory propose <run_id>` when the review changed. The existing precondition model uses `fix` only when one exact command remedies the failure; when review approval is reset or proposals are pending, `memory propose` itself will fail and existing lifecycle affordances point at `pactum review show <run_id>`.
    recommended answer (confidence medium): Use a context-sensitive affordance: if the current review state is still approved and proposal-clean, stale `memory accept` fails with `error.fix: pactum memory propose <run_id>`; if review approval or proposal preconditions are no longer satisfied, fail without accepting and expose `next: ["pactum review show <run_id>"]` so the user can repair the review state first.
    depends on: q_001

## Repository context
# Repository Context

Generated: 2026-06-11T18:05:50Z

Map run: map_20260611_173351
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-11T17:33:51Z

Repository root: `.`

## Summary

- Indexed files: 150
- Ignored files/directories: 1679
- Detected languages: 6
- Code items (best-effort hints): 1623

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
- `internal/app/affordances_test.go`: `go_func` `app.TestNextDoesNotAdvertiseApproveForFailedGate`
- `internal/app/affordances_test.go`: `go_func` `app.TestNextFallsBackToInspectionOnUnreadableClarifications`
- `internal/app/affordances_test.go`: `go_func` `app.TestTaskNewClarifyLoopFailureJSONEnvelope`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestClarifierRoundCompletedDespiteTimeoutRunsAfterSuccess`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestExecuteRunCompletedDespiteTimeoutTakesSuccessPath`
- `internal/app/agent_attempt_timeout_test.go`: `go_func` `app.TestExecuteRunPlainTimeoutStillFailsWithTimeoutMessage`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifierRoundExplicitTimeoutOverridesConfigIdleDefault`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifierRoundMarksAttemptReadOnlyAndPassesModelSpec`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifierRoundOmittedTimeoutUsesConfigIdleDefault`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestContractDraftMarksAttemptReadOnlyAndPassesModelSpec`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestExecuteRunNegativeTimeoutIsRejected`

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
  "query": "Fix three valid external review findings. (1) pactum export must preserve its no-overwrite guarantee at rename time: today two concurrent exports targeting the same --output can both pass the early existence check and the final os.Rename silently replaces the file that appeared in the window; finalize through a no-replace path (e.g. os.Link from the staged temp file to the output then remove the temp, treating link EEXIST as the existing 'output path already exists' error; keep behavior correct on Windows where os.Rename already refuses to replace) so a concurrent winner is never clobbered. (2) pactum export must reject run-record relative paths containing backslashes before writing ZIP entry names: on Unix a backslash is a valid filename character and filepath.ToSlash does not rewrite it, which would emit a non-portable entry name that some Windows extractors treat as a separator; fail the export with a clear error naming the offending path. (3) memory accept must not accept a stale candidate: memory propose records a freshness pin of the review state (for example the review document's updated_at or a content hash) inside memory-candidate.json; memory accept verifies the pin against the current review document and fails with a clear error plus error.fix pactum memory propose \u003crun_id\u003e when the review changed after the candidate was generated; the next-affordance selection that advertises memory accept applies the same staleness check and advertises memory propose instead when stale. Tests cover: concurrent-window overwrite refusal (simulate by creating the output file between staging and finalize), backslash rejection, stale-candidate refusal with the fix affordance, and the next selector switching to propose on staleness.",
  "queries": [
    "e.g",
    "os.Link",
    "memory-candidate.json",
    "error.fix",
    "no-overwrite",
    "no-replace",
    "run-record",
    "filepath.ToSlash"
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
