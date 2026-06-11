# Clarifier Context

## Run
- Run id: run_20260611_080712
- Run status: clarifying

## Contract draft
- Goal: Normalize the CLI command grammar for agent-first use: every stage exposes a uniform verb set, duplicates and aliases are removed, hyphenated pseudo-subcommands become nested subcommands. Renames (hard, no deprecation aliases — the project has no users yet): agents doctor -> doctor; clarify ask -> clarify add; clarify loop -> clarify run; clarify status (and its list alias) -> clarify show; contract show-draft -> contract show --draft; contract accept-draft -> contract accept; execute dry-run -> execute plan; execute status merges into execute show; review dry-run -> review plan; review add-finding -> review finding add; review resolve -> review finding resolve; review accept-proposal -> review proposal accept; review reject-proposal -> review proposal reject; review propose-findings -> review proposal collect; review fix -> review fix run; review apply-fix-outcomes -> review fix apply; task current is dropped (pactum status already reports the current run). All human-output Next: hints, error messages, and docs (flow.md, agents.md, agent-skill.md, README if it lists commands) must reference the new names. Existing JSON output schemas keep their names. Tests updated to invoke the new grammar.
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
- Total: 10
- Open: 8
- Blocking open: 7
- Converged: false
- Coverage by dimension (a zero-question dimension is unprobed, not necessarily settled):
  - terminology: total 3, answered 0, open 3, blocking open 3
  - scope: total 2, answered 1, open 1, blocking open 1
  - acceptance: total 1, answered 0, open 1, blocking open 0
  - edge_case: total 3, answered 0, open 3, blocking open 3
  - assumption: total 1, answered 1, open 0, blocking open 0
- Questions:
  - q_001 blocking=true status=open: What concrete scope is intended by “every stage exposes a uniform verb set”: only the explicit rename/drop list in the goal, or also redesign unlisted command groups such as `task`, `prompt`, `gate`, `memory`, `map`, `search`, `status`, `usage`, `version`, and `export`?
    kind: terminology
    rationale: The current CLI defines many stages in `internal/app/cli.go`; the goal lists specific renames but the phrase “every stage” could expand the change into a full CLI taxonomy redesign.
    recommended answer (confidence medium): Limit this run to the explicit rename/drop list in the goal and the directly affected help, human output, errors, docs, and tests; do not redesign unlisted command groups.
  - q_002 blocking=true status=open: For “duplicates and aliases are removed,” should that mean only removing old command spellings such as `clarify list`, `execute dry-run`, and `review add-finding`, or should flexible positional forms like `pactum review resolve f_001` vs `pactum review resolve <run_id> f_001` also be removed?
    kind: terminology
    rationale: The repo has a Kong alias for `clarify list`, old command paths, and flexible `[run_id]` parsing via helpers such as `splitLeadingRunID`; these are different kinds of “alias.”
    recommended answer (confidence medium): Remove only command-name aliases and old renamed command spellings; keep current-run resolution and flexible optional `[run_id]` forms because they are argument resolution behavior, not duplicate command grammar.
  - q_003 blocking=true status=answered: Should durable artifact names and JSON field names containing old terms, such as `execute/dry-run.json`, `review/reviewer-dry-run.json`, `review/fix/fixer-dry-run.json`, and `dry_run` fields, remain unchanged?
    kind: assumption
    rationale: The draft explicitly says existing JSON output schemas keep their names, but the repo also has durable artifact filenames and JSON fields with `dry-run` terminology; renaming those would be a storage-format migration, not just CLI grammar normalization.
    recommended answer (confidence high): Keep durable artifact filenames, JSON schema names, and JSON field names unchanged; update only command spellings and human-facing text that presents commands or current workflow instructions.
    answer: Keep durable artifact filenames, JSON schema names, and JSON field names unchanged; update only command spellings and human-facing text that presents commands or current workflow instructions.
  - q_004 blocking=true status=open: In the concrete scenario where a run has a built prompt but no execution attempts, what should `pactum execute show` do after `execute status` is removed: show the old status summary, or keep the old `execute show` behavior of saying no attempts were found?
    kind: edge_case
    rationale: `execute status` currently reports prompt readiness, plan artifact existence, attempt count, and last result; `execute show` currently focuses on a specific/latest attempt and prints “No execution attempts found” when none exist.
    recommended answer (confidence medium): `pactum execute show` with no attempt id should absorb the old status summary behavior; `pactum execute show [run_id] <attempt_id>` should keep the old attempt-detail behavior, with `--logs` applying only when attempt details are shown.
    depends on: q_001
    blocked: waiting on unanswered prerequisites
  - q_005 blocking=true status=open: Which documentation should be updated: only README, `docs/flow.md`, `docs/agents.md`, and `docs/agent-skill.md`, or also current workflow docs in `assets/agent-skills/pactum/`, `docs/install.md`, `docs/skill-install.md`, and historical files such as dogfood reports and backlog notes?
    kind: scope
    rationale: Repository search finds old commands in current user docs, the bundled Pactum agent skill, install docs, backlog, and dogfood reports. Historical reports may intentionally preserve past commands, while skill/install docs are active instructions.
    recommended answer (confidence medium): Update all current user-facing instructions and bundled skill workflow files, including README, `docs/flow.md`, `docs/agents.md`, `docs/agent-skill.md`, `docs/install.md`, `docs/skill-install.md`, and `assets/agent-skills/pactum/**`; leave explicitly historical dogfood/backlog text unchanged unless it is presented as current guidance.
    depends on: q_001, q_003
    blocked: waiting on unanswered prerequisites
  - q_006 blocking=false status=open: For acceptance, should tests verify only that the new command spellings work, or also that every removed old spelling fails and no help/usage output advertises the old names?
    kind: acceptance
    rationale: The draft says tests are updated and aliases are hard-removed, but does not state whether negative coverage is required. Existing tests heavily invoke old commands, so the test update scope matters.
    recommended answer (confidence high): Require positive tests for the new spellings, negative parser/help tests for the removed old spellings and `clarify list`, and assertions that human `Next:` hints and usage strings advertise only the new command names; validate with `make check`.
    depends on: q_002
    blocked: waiting on unanswered prerequisites
  - q_007 blocking=true status=open: When a user invokes an old removed command such as `pactum execute dry-run` or `pactum contract show-draft`, may the parser error mention the invalid old token, or must all error output avoid old command names entirely?
    kind: edge_case
    rationale: The goal says all error messages must reference new names, but parser diagnostics often echo the invalid token. Treating that echo as a violation would require custom error handling beyond removing command definitions.
    recommended answer (confidence medium): Allow parser diagnostics to echo the invalid old token as the cause of failure, but ensure usage text, suggestions, `Next:` hints, and hand-written errors list only the new command names.
    depends on: q_002
    blocked: waiting on unanswered prerequisites
  - q_008 blocking=true status=open: After `execute dry-run -> execute plan` and `review dry-run -> review plan`, should human-facing labels/headings continue to use “dry-run” as a workflow term, or should “dry-run” be reserved only for durable artifact/schema names such as `execute/dry-run.json` and `dry_run` JSON fields?
    kind: terminology
    rationale: The answered artifact/schema clarification preserves `dry-run` filenames and JSON fields, but current human output and docs also say things like `Execution dry-run prepared`, `Dry-run:`, `Reviewer dry-run prepared`, and `dry-run vs run`; this changes visible UX and test expectations.
    recommended answer (confidence medium): Use `plan` in current human-facing command/workflow labels, headings, `Next:` hints, usage text, and current docs; allow `dry-run` only when referring to preserved artifact paths, JSON fields/schema names, historical records, or explanatory storage details.
  - q_009 blocking=true status=open: If `pactum execute show` absorbs the old `execute status` behavior, what JSON shape should `pactum execute show --json` with no `attempt_id` return when attempts may or may not exist?
    kind: edge_case
    rationale: `execute status --json` currently returns prompt/plan/attempt summary fields, while `execute show --json` returns a specific attempt’s request/result/log excerpts. Merging the commands without a rule would make JSON consumers and tests ambiguous, especially because existing JSON schemas are meant to keep their names.
    recommended answer (confidence medium): `pactum execute show --json` with no `attempt_id` should always return the old execute-status summary shape; `pactum execute show --json [run_id] <attempt_id>` should return the old execute-show attempt-detail shape, with log excerpts only when `--logs` is passed.
  - q_010 blocking=false status=answered: Should active command consumers outside the named docs, specifically `AGENTS.md` and `scripts/smoke.sh`, be updated to the new grammar, while historical files such as `CHANGELOG.md` remain unchanged?
    kind: scope
    rationale: Repository search finds old command spellings in active agent guidance and the smoke script. `scripts/smoke.sh` currently invokes `pactum agents doctor`, which will break after hard removal, while changelog/history entries may intentionally describe past behavior.
    recommended answer (confidence high): Update active agent guidance and executable/dev workflow scripts such as `AGENTS.md` and `scripts/smoke.sh` to the new grammar; leave changelog and explicitly historical reports unchanged unless they present current instructions.
    answer: Update active agent guidance and executable/dev workflow scripts such as `AGENTS.md` and `scripts/smoke.sh` to the new grammar; leave changelog and explicitly historical reports unchanged unless they present current instructions.

## Repository context
# Repository Context

Generated: 2026-06-11T08:07:12Z

Map run: map_20260611_063009
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-11T06:30:09Z

Repository root: `.`

## Summary

- Indexed files: 142
- Ignored files/directories: 1219
- Detected languages: 6
- Code items (best-effort hints): 1554

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
  "query": "Normalize the CLI command grammar for agent-first use: every stage exposes a uniform verb set, duplicates and aliases are removed, hyphenated pseudo-subcommands become nested subcommands. Renames (hard, no deprecation aliases — the project has no users yet): agents doctor -\u003e doctor; clarify ask -\u003e clarify add; clarify loop -\u003e clarify run; clarify status (and its list alias) -\u003e clarify show; contract show-draft -\u003e contract show --draft; contract accept-draft -\u003e contract accept; execute dry-run -\u003e execute plan; execute status merges into execute show; review dry-run -\u003e review plan; review add-finding -\u003e review finding add; review resolve -\u003e review finding resolve; review accept-proposal -\u003e review proposal accept; review reject-proposal -\u003e review proposal reject; review propose-findings -\u003e review proposal collect; review fix -\u003e review fix run; review apply-fix-outcomes -\u003e review fix apply; task current is dropped (pactum status already reports the current run). All human-output Next: hints, error messages, and docs (flow.md, agents.md, agent-skill.md, README if it lists commands) must reference the new names. Existing JSON output schemas keep their names. Tests updated to invoke the new grammar.",
  "queries": [
    "flow.md",
    "agents.md",
    "agent-skill.md",
    "agent-first",
    "pseudo-subcommands",
    "show-draft",
    "accept-draft",
    "dry-run"
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
