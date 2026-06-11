# Contract Drafter Context

## Run
- Run id: run_20260611_160606
- Run status: contract_draft

## Contract goal
Smooth the pipeline so no command is pure ritual, then compress the agent skill against the final grammar. This is the last grammar break; hard removals, no aliases. (1) review run absorbs the loop: pactum review run becomes today's review loop semantics — implicit prepare when the gate report exists, panel-times-lenses rounds, fixer, severity-gated convergence; the single panel round without a fixer remains available as review run --no-fix --max-rounds 1; the review loop command is removed (parser error). (2) review prepare is removed: review run prepares implicitly, and manual review mutations (review finding add) self-scaffold the review when a gate report exists; the review_not_prepared error code disappears — the remaining precondition is gate_report_missing, and affordance fix/next values must reflect that. (3) prompt build self-heals a stale project map: when the map is stale it runs the refresh itself, says so in human output, exposes it in --json, and records it in the prompt manifest; the project_map_stale failure disappears from prompt build (other commands keep it). (4) clarify suggest is removed: its semantics fold into clarify run --no-auto --max-rounds 1 (new --no-auto flag disables auto-resolution of high-confidence recommendations). (5) clarify answer gains --recommended: pactum clarify answer <q_id> --recommended records that question's stored recommended answer as the answer (error when the question has no recommendation), and pactum clarify answer --all-recommended answers every open question that carries a recommendation, skipping and reporting those without; both are decision verbs honoring --by and recording answer provenance distinguishable from a typed answer. (6) The skill is rewritten against the final grammar: assets/agent-skills/pactum/SKILL.md, references/workflow.md, references/safety.md, and docs/agent-skill.md compress to the final convention — every stage exposes run and show, decision verbs relay explicit human decisions with --by, agents read next and error.fix instead of memorizing stage order, execute run is unsandboxed per SECURITY.md. (7) All Next: hints, affordance next/fix command strings, docs (flow.md, agents.md, README if affected), and tests speak only the new surface; negative parser coverage for the removed spellings (review loop, review prepare, clarify suggest) and forbidden-phrase guards in the docs/skill tests. Existing JSON schema names and artifact paths unchanged; review plan and review fix run/apply stay as surgical commands.

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
- q_001 [blocking]: When `review prepare` is removed, which review commands should auto-create `review/review.json` when a gate report exists: only `review run` and `review finding add`, or all mutating review commands that currently call `requireReviewPrepared` such as `review approve`, `review finding resolve`, proposal decisions, and `review fix run/apply`?
  Rationale: The draft names `review run` and `review finding add`, but the repository has several mutating review commands that currently fail through `review_not_prepared` in `internal/app/review.go` and `internal/app/review_fix.go`. Since `review prepare` is being hard-removed and `review_not_prepared` disappears, these commands need a consistent replacement behavior.
  Answer: Auto-scaffold the review for `review run`, `review finding add`, and `review approve` whenever `gate/report.json` exists; keep commands that require existing records, such as `review finding resolve`, proposal accept/reject, and `review fix run/apply`, failing with ordinary record/precondition errors if there is no relevant review data. `review status/show` should not mutate; on a gated run with no review artifact they should show the derived empty pending review state rather than suggesting `review prepare`.
- q_002 [blocking]: What does `severity-gated convergence` mean in concrete repository terms: should convergence and fixer invocation be gated by the existing `finding.blocking` boolean, by `severity` values (`low`, `medium`, `high`, `critical`), or by a new severity threshold that maps severities to blocking behavior?
  Rationale: The current review model stores both `Severity` and `Blocking`, but approval and loop convergence in `summarizeReview` / `ReviewLoop` use only `BlockingOpen`. The phrase `severity-gated` is overloaded against those two existing concepts and would change loop stopping, approval, and fixer behavior.
  Answer: Keep convergence and fixer invocation gated by open blocking findings, not raw severity. Preserve severity as finding metadata and allow reviewer/proposal logic to set `blocking=true` for findings severe enough to block. Do not introduce a new severity threshold in this grammar break.
- q_003 [blocking]: For `review run --no-fix --max-rounds 1`, if the reviewer panel finds new blocking findings, should the command accept those proposals into `findings.jsonl` and exit successfully with an open-review terminal reason, or should it fail nonzero because convergence was not reached?
  Rationale: The draft preserves a single panel round without a fixer, while today's `review run` records attempts and today's `review loop` auto-accepts proposals and fixes blocking findings. The no-fix path needs defined behavior for the common case where a review round finds real issues.
  Answer: `review run --no-fix --max-rounds 1` should run the panel/lenses, parse and accept valid proposals into review findings, skip fixer execution, write the loop summary, and exit 0 with a terminal reason such as `findings_open`/`no_fix`. Its `next` should point to `pactum review show <run_id>` while blocking findings remain.
- q_004 [blocking]: For `clarify answer --all-recommended`, what should happen when the run has a mix of open questions with recommendations, open questions without recommendations, already answered questions, and questions whose `depends_on` prerequisites are still unanswered?
  Rationale: The existing clarify loop auto-resolver only answers high-confidence recommended questions and respects `depends_on`. The draft says `--all-recommended` answers every open question carrying a recommendation, skipping and reporting those without, but does not say whether dependency-blocked questions or already answered questions are skipped, revised, or errors.
  Answer: `--all-recommended` should consider only currently open questions. It should answer open questions with a non-empty stored recommendation in dependency order, skip open questions without recommendations, skip dependency-blocked questions until prerequisites are answered, report all skipped IDs in human and JSON output, and exit 0 unless no answers were recorded and at least one requested single-question operation would have been an error.
- q_005 [blocking]: How exactly should `prompt build` record that it self-healed a stale project map in machine-readable output and `contract/prompt-manifest.json` while keeping existing schema names and artifact paths unchanged?
  Rationale: The current prompt manifest has `map_run_id` and `checks.project_map_fresh`, but no field indicating that `prompt build` performed a refresh. The draft requires human output, `--json`, and the prompt manifest to expose the self-heal, and also says existing JSON schema names and artifact paths stay unchanged.
  Answer: Keep `schema: pactum.executor_prompt.v1` and all artifact paths unchanged, but add an additive `map_refresh` object to the prompt manifest and `prompt build --json` response: `{ "triggered": true, "reason": "project_map_stale", "previous_map_run_id": "...", "run_id": "..." }`; use `{ "triggered": false }` when no refresh was needed. Human output should state that the stale project map was refreshed and show the new map run id.
- q_006 [blocking]: In the skill rewrite phrase “every stage exposes run and show,” what concrete CLI scope is intended: a docs-only convention using the actual final verbs, or literal new command families such as `pactum contract run`, `pactum prompt run`, and `pactum memory run`?
  Rationale: The draft explicitly changes `review run` and `clarify run`, but the repository currently has mixed stage verbs: `prompt build/show`, `execute plan/run/show`, `gate run/show`, `review plan/run/show/fix`, and `memory propose/show/accept`. Treating “every stage exposes run and show” literally would greatly expand the grammar beyond the named removals.
  Answer: Do not add broad new `run/show` aliases or rename unrelated stage verbs in this grammar break. Scope the CLI changes to the explicitly listed surface: `review run` absorbs loop behavior, `clarify run --no-auto --max-rounds 1` replaces `clarify suggest`, removed commands become parser errors, and docs/skill text should describe each stage using its actual final commands.
- q_007 [blocking]: For the concrete replacement scenario `pactum clarify run --no-auto --max-rounds 1` where the clarifier creates one open blocking medium-confidence question, should the command report a clarify-loop summary or preserve the old `clarify suggest` top-level response shape?
  Rationale: The current `clarify suggest` command returns a suggest response with created questions, while `clarify run` writes `clarify/loop-summary.json` and returns the loop summary. The draft removes `clarify suggest` but says its semantics fold into `clarify run --no-auto --max-rounds 1`, so the user-visible and JSON acceptance surface needs to be pinned.
  Answer: `clarify run --no-auto --max-rounds 1` should use the existing clarify-loop output and artifact surface, not preserve a separate suggest response. It should run one clarifier round, record created questions, skip all auto-resolution, write `clarify/loop-summary.json`, and report `terminal_reason: needs_human` when open blocking questions remain rather than reporting `max_rounds` merely because the one-round cap was reached.
- q_008 [blocking]: What exact provenance values should `clarify answer <q_id> --recommended` and `clarify answer --all-recommended` persist in `answers.jsonl` and `decisions.jsonl`?
  Rationale: The repository already pins typed manual answers as answer `source: manual` and decision `source: manual_answer`, and auto-resolved recommendations as answer `source: auto_recommended` with decision `source: clarify_loop_auto`. The draft requires recommended-answer decisions to be distinguishable from typed answers and to honor `--by`, but does not pin the durable source strings.
  Answer: Keep typed manual and auto-loop provenance unchanged. For `--recommended`, write answer `source: manual_recommended` and decision `source: manual_recommended_answer` with normalized `decided_by` from `--by`. For `--all-recommended`, write answer `source: manual_all_recommended` and decision `source: manual_all_recommended_answer` for each recorded answer, also with normalized `decided_by` from `--by`.
- q_009 [blocking]: For `pactum review run --no-fix --max-rounds 2`, if round 1 accepts an open blocking finding, should it stop immediately because no fixer can change the tree, continue reviewer-only rounds until `max_rounds`, or reject that flag combination?
  Rationale: The existing open question covers the preserved single panel round `--no-fix --max-rounds 1`, but the new grammar exposes `--no-fix` and `--max-rounds` as independent flags. Continuing reviewer-only rounds after blocking findings are accepted can churn without any write step, while rejecting combinations may surprise users if the parser accepts both flags.
  Answer: Allow the combination, but define `--no-fix` as never invoking the fixer and stopping after the first round that leaves open blocking findings, with a terminal reason such as `findings_open` or `no_fix`. If the round has no open blocking findings, use the normal clean/resolved behavior. Document `--no-fix --max-rounds 1` as the intended manual panel-pass spelling.
- q_010 [blocking]: When `review run` absorbs `review loop`, which loop controls should move onto `review run`: the full existing loop control surface (`--agent`, `--max-rounds`, `--patience`, `--clean-rounds`, `--timeout`, `--reviewer`, `--json`) plus new `--no-fix`, or only the flags explicitly named in the draft (`--no-fix` and `--max-rounds`)?
  Rationale: The current CLI has separate `review run` reviewer-only flags and `review loop` fixer/round-control flags in `internal/app/cli.go`. The draft says `review run` becomes today's review loop semantics and removes `review loop`, but only names `--no-fix --max-rounds 1`. Whether `--agent`, `--patience`, and `--clean-rounds` survive changes CLI grammar, docs, tests, and how users tune the absorbed loop.
  Answer: Move the existing `review loop` control surface onto `review run`: support `--reviewer`, `--agent`, `--max-rounds`, `--patience`, `--clean-rounds`, `--timeout`, and `--json`, and add `--no-fix`. Remove `review loop` entirely as a parser error. Keep `review plan` as the safe reviewer-only preparation command and keep `review fix run/apply` as surgical commands.
- q_011 [blocking]: For `pactum clarify answer <q_id> --recommended`, what should happen when the target question already has an answer, or when it has a stored recommended answer but its `depends_on` prerequisites are still unanswered?
  Rationale: The draft defines the no-recommendation case as an error, and q_004 covers bulk `--all-recommended`, but the single-question path has additional edge cases. The repository currently permits answer records to append as revisions via `recordClarificationAnswer`, while clarify-loop auto-resolution skips dependency-blocked questions. The new decision verb needs deterministic behavior so provenance and status do not drift.
  Answer: `--recommended` should require the question to be currently open, have a non-empty stored `recommended_answer`, and not be blocked by unanswered `depends_on` prerequisites. If the question is already answered, return an error rather than creating a revision. If dependencies are unanswered, return an error pointing the user to `pactum clarify show <run_id>`; a typed `clarify answer` remains the escape hatch for an explicit human-authored answer.

## Repository context
# Repository Context

Generated: 2026-06-11T16:06:06Z

Map run: map_20260611_114746
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-11T11:47:46Z

Repository root: `.`

## Summary

- Indexed files: 149
- Ignored files/directories: 1545
- Detected languages: 6
- Code items (best-effort hints): 1598

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
  "query": "Smooth the pipeline so no command is pure ritual, then compress the agent skill against the final grammar. This is the last grammar break; hard removals, no aliases. (1) review run absorbs the loop: pactum review run becomes today's review loop semantics — implicit prepare when the gate report exists, panel-times-lenses rounds, fixer, severity-gated convergence; the single panel round without a fixer remains available as review run --no-fix --max-rounds 1; the review loop command is removed (parser error). (2) review prepare is removed: review run prepares implicitly, and manual review mutations (review finding add) self-scaffold the review when a gate report exists; the review_not_prepared error code disappears — the remaining precondition is gate_report_missing, and affordance fix/next values must reflect that. (3) prompt build self-heals a stale project map: when the map is stale it runs the refresh itself, says so in human output, exposes it in --json, and records it in the prompt manifest; the project_map_stale failure disappears from prompt build (other commands keep it). (4) clarify suggest is removed: its semantics fold into clarify run --no-auto --max-rounds 1 (new --no-auto flag disables auto-resolution of high-confidence recommendations). (5) clarify answer gains --recommended: pactum clarify answer \u003cq_id\u003e --recommended records that question's stored recommended answer as the answer (error when the question has no recommendation), and pactum clarify answer --all-recommended answers every open question that carries a recommendation, skipping and reporting those without; both are decision verbs honoring --by and recording answer provenance distinguishable from a typed answer. (6) The skill is rewritten against the final grammar: assets/agent-skills/pactum/SKILL.md, references/workflow.md, references/safety.md, and docs/agent-skill.md compress to the final convention — every stage exposes run and show, decision verbs relay explicit human decisions with --by, agents read next and error.fix instead of memorizing stage order, execute run is unsandboxed per SECURITY.md. (7) All Next: hints, affordance next/fix command strings, docs (flow.md, agents.md, README if affected), and tests speak only the new surface; negative parser coverage for the removed spellings (review loop, review prepare, clarify suggest) and forbidden-phrase guards in the docs/skill tests. Existing JSON schema names and artifact paths unchanged; review plan and review fix run/apply stay as surgical commands.",
  "queries": [
    "fix/next",
    "assets/agent-skills/pactum/SKILL.md",
    "references/workflow.md",
    "references/safety.md",
    "docs/agent-skill.md",
    "error.fix",
    "SECURITY.md",
    "next/fix"
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
