# Contract Drafter Context

## Run
- Run id: run_20260611_113834
- Run status: contract_draft

## Contract goal
Make the CLI announce legal moves so an agent never guesses the pipeline state machine: (1) structured error envelopes — when a command fails on a recognizable precondition (workspace not initialized, project map stale, contract not approved, prompt not built, no execution attempt, gate report missing, review not prepared, open blocking clarifications, run not found, pending proposals), the --json output emits a versioned error envelope carrying a stable machine-readable reason code, the human message, and a fix field holding the exact remedial pactum command when one exists; human output keeps the existing Suggested:/guidance text; exit codes stay nonzero and unchanged. (2) next affordances — every mutating command's --json response gains a next array of full pactum command strings mirroring the human Next: block (commands that already print Next: hints must emit the same set in JSON), and pactum status --json gains a next array for the current run's stage. There is precedent to build on: the pactum.not_ready.v1 envelope with suggested_command, next_command fields in run resolution, and the human Next: blocks — unify these into one consistent affordance convention rather than inventing a parallel one. The bundled skill and docs describe the convention briefly (agents should read next/fix instead of memorizing stage order). Tests pin: a representative precondition failure per stage emits the envelope with the right reason and fix; next arrays match the human hints; non-error and non-mutating outputs are unchanged.

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
- q_001 [blocking]: What exactly counts as a 'mutating command' for the required `next` array: every command that writes anything (`init`, `map refresh`, `task use`, `export`, `memory refresh`, etc.), or only workflow/run-stage commands such as `task new`, `contract approve`, `prompt build`, `execute plan/run`, `gate run`, `review ...`, and `memory propose/accept`?
  Rationale: The repo has many commands that write files, but only a few currently print human `Next:` blocks (`task new`, `task show`, `contract accept`, `prompt build`, `memory propose`). The implementation and tests differ substantially depending on whether non-stage mutations like `export` and `memory refresh` must gain `next`.
  Answer: Define `mutating command` as commands that create or update Pactum workflow state and already support `--json`; exclude pure read-only commands and exclude `export` because it writes an archive but does not advance the Pactum state machine. For in-scope mutating commands with no meaningful next action, emit `next: []`.
- q_002 [blocking]: For the JSON error envelope, should the stable machine-readable field be named `error.code` as in the existing `pactum.error.v1` implementation, or `error.reason` as described in the backlog phrase '`reason` + `fix`'?
  Rationale: The current code has `pactum.error.v1` with `error.message` and `error.code`; the draft says 'reason code' and the backlog says `reason`. This is an external schema decision for agents.
  Answer: Keep `error.code` as the canonical reason-code field for compatibility with the existing `pactum.error.v1` envelope, and add `error.fix` when an exact remedial command exists. Do not add a parallel `error.reason` field in this slice.
- q_003 [blocking]: Should existing JSON affordance fields such as `suggested_command` and `next_command` remain for compatibility while the new `fix` and `next` fields are added, or should they be removed/renamed immediately?
  Rationale: The repo currently exposes `suggested_command` in `pactum.not_ready.v1` and prompt-not-built responses, plus `next_command` in task/status responses. Removing them would break existing tests and consumers; keeping them temporarily creates two conventions.
  Answer: Preserve existing `suggested_command` and `next_command` fields where they already exist, add the new `fix` and `next` fields, and update the bundled skill/docs to prefer `fix` and `next`.
- q_004 [blocking]: How should read-only not-ready responses behave, such as `gate show --json` before a gate report exists or `review show --json` before review is prepared: keep the current exit-0 `pactum.not_ready.v1` response, or convert them to nonzero `pactum.error.v1` envelopes?
  Rationale: The draft says error envelopes apply when a command fails and exit codes stay unchanged. The repo currently treats these read-only guidance cases as successful `pactum.not_ready.v1` JSON with `ready:false` and `suggested_command`.
  Answer: Keep read-only not-ready responses exit 0 and keep `pactum.not_ready.v1`; add `fix` mirroring the exact remedial command while preserving `suggested_command` for compatibility.
- q_005 [blocking]: For the 'no completed execution attempt' precondition on `pactum gate run --json`, should `fix` ever suggest `pactum execute run`, even though real agent execution is unsandboxed and requires explicit human approval?
  Rationale: The repository and AGENTS.md safety rules say not to run real agents without explicit approval. But the exact remedial command for a missing execution attempt could be interpreted as `pactum execute run`, which is unsafe for an agent to follow automatically.
  Answer: Do not put `pactum execute run` in `fix`. For this precondition, omit `fix` and use the human message to say a completed execution attempt is required; `next` may point to the safe preparation command `pactum execute plan <run_id> --agent codex`.
- q_006 [blocking]: When there is no single exact remedial command, such as multiple open blocking clarification questions, multiple pending review proposals, or no current run with multiple active runs, should `fix` be omitted, set to an empty string, or contain a command with placeholders?
  Rationale: The draft says `fix` holds the exact remedial pactum command when one exists. Some current guidance is multi-step or requires choosing IDs, so placeholder commands could make agents guess again.
  Answer: Make `fix` optional and omit it when no single exact runnable command exists. Never emit placeholder commands in `fix`; keep multi-step guidance in the human-readable message and expose concrete legal commands through `next` only when they are directly runnable.
- q_007: Which precondition failures must be pinned by acceptance tests for this slice?
  Rationale: The draft says 'a representative precondition failure per stage' but does not name the test matrix. The repo already has partial coverage for `run_not_found`, `contract_not_approved`, readiness responses, and status next commands.
  Answer: Pin at least: `not_initialized`, `project_map_stale`, `contract_not_approved`, `blocking_clarifications_open`, `prompt_not_built`, `no_execution_attempt`, `gate_report_missing`, `review_not_prepared`, `pending_review_proposals`, and `run_not_found`; each test should assert schema, code, message, optional fix, unchanged exit code, and empty stderr in `--json` mode.
- q_008 [blocking]: Besides mutating commands, should the new plural `next` array be added to read-only JSON responses that already expose next-step affordances, specifically `pactum task show --json`, `pactum task list --json`, and `pactum status --json`?
  Rationale: The draft says non-error and non-mutating outputs are unchanged, but also says commands already printing human `Next:` hints must emit the same set in JSON, and explicitly names `pactum status --json`. In the repo, `task show` prints a human `Next:` block and has `next_command`; `task list` has per-run `next_command` in JSON but no human `Next:` block; `status` has `runs.next_command` and human `next:` output.
  Answer: Add plural `next` to `pactum status --json` and `pactum task show --json` because they are current-run guidance surfaces; leave `pactum task list --json` unchanged except for preserving existing per-run `next_command` fields because it is an inventory view and does not print a human `Next:` block.
- q_009 [blocking]: Where should the new plural `next` array live in JSON responses: as a top-level `next` field on each command response, or nested beside existing affordance fields such as `runs.next_command` and per-run `next_command`?
  Rationale: The repo currently stores next-step affordances in different places: `status.runs.next_command`, `task show` top-level `next_command`, and `task list` item-level `next_command`. The draft says each response 'gains a `next` array' and says `status --json` gains one, but does not pin the field location.
  Answer: Use a top-level `next: []` array on each in-scope command response, including `pactum status --json`; preserve existing `next_command` fields for compatibility where they already exist.
- q_010 [blocking]: For JSON `next`, should Pactum mirror the human `Next:` block byte-for-byte, or normalize it into concrete directly runnable commands when the human hint is bare, multi-step, or contains placeholders such as `<question_id>` and `"<answer>"`?
  Rationale: Current human hints vary: `prompt build` prints `pactum execute plan --agent codex` without a run id, `task show` uses bare commands from `nextCommandForStatus`, `memory propose` prints two concrete commands with run ids, and `task new --clarify` may print placeholder guidance for answering clarification questions. The draft simultaneously says 'full pactum command strings', 'mirroring the human Next block', and that agents should not guess.
  Answer: Define JSON `next` as concrete directly runnable command strings. It may fill known run ids and omit placeholder-only templates; human output can keep its existing explanatory `Next:` text. Preserve exact set/order only where the human hints are already concrete commands.
- q_011: When the current stage has no safe automatic next command because human input is required, for example open blocking clarification questions after `task new --clarify`, should `next` be empty or contain a safe inspection command such as `pactum clarify status <run_id>`?
  Rationale: The contract wants agents to read legal moves from `next`, but answering clarification questions requires human-authored content and cannot be represented as an exact runnable command. The repo already distinguishes safe preparation commands from unsandboxed or human-decision commands in the existing clarification around `execute run`.
  Answer: Emit only safe, concrete commands in `next`; for open blocking clarifications, prefer `next: ["pactum clarify status <run_id>"]` and keep answer templates in the human-readable message, not in `next`.
- q_012 [blocking]: Where should the new JSON `fix` field live: inside the existing `error` object as `error.fix`, or as a top-level sibling to `schema` and `error`?
  Rationale: The current repo defines `pactum.error.v1` as `{schema, error: {message, code}}` in `internal/app/errors.go`. Existing questions cover `code` versus `reason` and optional fixes, but not the field location. This affects the public schema and tests.
  Answer: Add `fix` as `error.fix` inside the existing `error` object, alongside `error.message` and `error.code`. Omit `error.fix` when no exact runnable remedial command exists.
- q_013 [blocking]: Should this slice cover only the named precondition list, or also secondary artifact-boundary failures such as `approved contract hash does not match current contract`, `executor prompt was built for a different project map`, `memory context changed after prompt build`, and `executor prompt does not match current approved contract`?
  Rationale: The contract lists specific recognizable preconditions, but the repo has additional boundary checks in `internal/app/execute.go`, `internal/app/gate.go`, and `internal/app/contract.go`. Including all of them expands the reason-code taxonomy and fix-command matrix; excluding them leaves some agent-facing failures as generic `command_failed`.
  Answer: For this slice, require stable codes and `fix` only for the named preconditions in the contract. Leave secondary artifact-integrity and boundary-mismatch errors as `command_failed` unless they already match an existing code such as `project_map_stale`; handle a fuller taxonomy in a follow-up.
- q_014 [blocking]: For the partial-success scenario where `pactum task new --clarify --json` creates a run but the clarify loop fails before a normal JSON response, should the command emit an error envelope, a partial success response, or both?
  Rationale: `internal/app/task.go` explicitly preserves the created run when the clarify loop fails and returns an error telling the user to re-run `pactum clarify run <run_id>`. In `--json` mode, the new envelope convention needs to say whether agents get structured recovery data or a normal task response.
  Answer: Keep the nonzero exit and emit a `pactum.error.v1` envelope, not the normal task response. Use a stable code such as `clarify_loop_failed`, keep the human message indicating the run was created, and set `error.fix` to `pactum clarify run <run_id>`.
- q_015: What concrete files count as the 'bundled skill and docs' that must briefly describe `next` and `fix`: only `assets/agent-skills/pactum/SKILL.md`, the reference files under `assets/agent-skills/pactum/references/`, or also human docs such as `docs/agent-skill.md` and `docs/flow.md`?
  Rationale: The repo guidance names `assets/agent-skills/pactum/` as the canonical skill package and `docs/agent-skill.md` as the human overview. The draft says 'bundled skill and docs' without naming files, so acceptance could vary from one small skill update to a wider documentation pass.
  Answer: Update `assets/agent-skills/pactum/SKILL.md` and `assets/agent-skills/pactum/references/workflow.md` as the required bundled skill docs, and update `docs/agent-skill.md` as the human overview. Do not require broad README, `docs/flow.md`, or backlog rewrites for this slice.

## Repository context
# Repository Context

Generated: 2026-06-11T11:38:34Z

Map run: map_20260611_111353
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-11T11:13:53Z

Repository root: `.`

## Summary

- Indexed files: 148
- Ignored files/directories: 1463
- Detected languages: 6
- Code items (best-effort hints): 1596

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

- Go: 119 file(s)
- Markdown: 23 file(s)
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
- `internal/app/clarify_suggest.go`
- `internal/app/clarify_suggest_test.go`
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
  "query": "Make the CLI announce legal moves so an agent never guesses the pipeline state machine: (1) structured error envelopes — when a command fails on a recognizable precondition (workspace not initialized, project map stale, contract not approved, prompt not built, no execution attempt, gate report missing, review not prepared, open blocking clarifications, run not found, pending proposals), the --json output emits a versioned error envelope carrying a stable machine-readable reason code, the human message, and a fix field holding the exact remedial pactum command when one exists; human output keeps the existing Suggested:/guidance text; exit codes stay nonzero and unchanged. (2) next affordances — every mutating command's --json response gains a next array of full pactum command strings mirroring the human Next: block (commands that already print Next: hints must emit the same set in JSON), and pactum status --json gains a next array for the current run's stage. There is precedent to build on: the pactum.not_ready.v1 envelope with suggested_command, next_command fields in run resolution, and the human Next: blocks — unify these into one consistent affordance convention rather than inventing a parallel one. The bundled skill and docs describe the convention briefly (agents should read next/fix instead of memorizing stage order). Tests pin: a representative precondition failure per stage emits the envelope with the right reason and fix; next arrays match the human hints; non-error and non-mutating outputs are unchanged.",
  "queries": [
    "Suggested:/guidance",
    "next/fix",
    "machine-readable",
    "pactum.not_ready.v1",
    "suggested_command",
    "next_command",
    "non-error",
    "non-mutating"
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
