# Clarifier Context

## Run
- Run id: run_20260610_082523
- Run status: clarifying

## Contract draft
- Goal: Add progress indicators so long operations feel responsive
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
- Total: 9
- Open: 8
- Blocking open: 3
- Converged: false
- Coverage by dimension (a zero-question dimension is unprobed, not necessarily settled):
  - terminology: total 1, answered 0, open 1, blocking open 1
  - scope: total 2, answered 0, open 2, blocking open 1
  - acceptance: total 1, answered 0, open 1, blocking open 1
  - edge_case: total 3, answered 0, open 3, blocking open 0
  - assumption: total 2, answered 1, open 1, blocking open 0
- Questions:
  - q_001 blocking=true status=open: Which concrete form should 'progress indicators' take? The repo supports several distinct interpretations: (a) periodic plain-text heartbeat lines on operator stderr while an agent attempt runs (stage, attempt id, elapsed time), emitted by the shared lifecycle in internal/app/agent_attempt.go; (b) a TTY-only animated spinner/elapsed-time display; (c) step-boundary announcements (e.g. 'clarify loop: round 2/5 starting', 'attempt exec_0002 started') in clarify_loop.go/review_loop.go, which today print only an end-of-run summary; (d) richer parsed agent activity surfaced from the ACP stream (e.g. 'agent is reading internal/app/run.go'). Which of (a)-(d), or which combination, is intended?
    kind: terminology
    rationale: Each interpretation lands in different code (transport vs lifecycle vs loop renderers) and has very different cost. Today the only liveness signal is the agent's own teed output (LiveOutput = r.Stderr in commands.go), which can be silent for minutes — the goal 'feel responsive' does not pick among these by itself.
    recommended answer (confidence medium): Combine (a) and (c): a periodic plain-text heartbeat line on stderr during every agent attempt, implemented once in runAgentAttemptLifecycle, plus round/attempt boundary announcements in clarify loop and review loop. No ANSI animation and no ACP event parsing in this contract.
  - q_002 blocking=true status=open: Which operations are 'long operations' in scope? Candidates: (1) only the agent-attempt stages that run subprocesses for minutes (contract draft, clarify suggest, clarify loop, execute, review, review fix, review loop — all via runAgentAttemptLifecycle); (2) also the multi-round loop commands' round boundaries; (3) also deterministic commands like 'pactum map refresh' (internal/app/map.go), which scans/indexes the repo but typically finishes in seconds on this repo (133 files).
    kind: scope
    rationale: The contract's in-scope list is empty. Scoping to the shared agent-attempt lifecycle means one implementation point covers seven commands; pulling map refresh in adds per-file progress plumbing through internal/projectmap for an operation that is rarely slow here, though it could be slow on large repos.
    recommended answer (confidence medium): In scope: agent-attempt stages via the shared lifecycle, plus round-boundary announcements in clarify loop and review loop. Out of scope: map refresh, search, and other deterministic commands.
    depends on: q_001
    blocked: waiting on unanswered prerequisites
  - q_003 blocking=false status=open: Should progress output go exclusively to stderr, and should it still be emitted when --json is set? Every relevant command has a JSONOutput flag ('Print machine-readable JSON output', internal/app/cli.go) writing to stdout, and liveOutput is already wired to r.Stderr in commands.go.
    kind: assumption
    rationale: Progress on stdout would corrupt --json consumers and the human-readable end summaries. The existing convention (agent live output on stderr) strongly suggests stderr, but whether --json runs should stay progress-free entirely is a product choice for scripted callers.
    recommended answer (confidence high): All progress goes to stderr only, never stdout. Keep emitting progress under --json (stdout stays clean JSON); scripted callers that want full silence can redirect stderr.
    depends on: q_001
    blocked: waiting on unanswered prerequisites
  - q_004 blocking=false status=open: Silent-agent scenario: 'pactum execute' runs with --timeout 600s and the agent emits nothing for 5+ minutes while the idle watchdog (startIdleTimeout in internal/agents) counts toward killing the attempt. What should the operator see during the silence, and should the heartbeat expose watchdog state (time since last agent output, configured idle timeout) so an impending kill is predictable?
    kind: edge_case
    rationale: This is the documented pain case: claude -p barely streams, so attempts look hung and the watchdog can fire at the default timeout with no warning. Note the heartbeat must be written outside the activityWriter path — it is Pactum's own output and must not reset the agent's idle timer.
    recommended answer (confidence medium): Heartbeat every ~15s including stage, attempt id, total elapsed, time since last agent output, and the idle timeout (e.g. 'execute exec_0002: 4m10s elapsed, 3m50s since last agent output, idle timeout 10m'). The heartbeat must not feed the idle-watchdog activity channel.
    depends on: q_001
    blocked: waiting on unanswered prerequisites
  - q_005 blocking=false status=open: Non-TTY scenario: a CI job (.github/workflows/ci.yml) or a parent process like Claude Code runs 'pactum execute' with stderr captured to a log. A 10-minute attempt at a 15s heartbeat produces ~40 log lines. Should non-TTY stderr get the same cadence, a reduced cadence, or no heartbeat at all — and is any TTY-only rendering (carriage-return spinner) wanted when stderr is a terminal?
    kind: edge_case
    rationale: The contract is silent on TTY detection. Plain lines are safe everywhere but noisy in logs; ANSI rewriting is pleasant interactively but garbles CI logs. This choice decides whether the implementation needs isatty detection at all.
    recommended answer (confidence medium): Emit the same plain heartbeat lines regardless of TTY (no ANSI, no isatty detection) but at a reduced ~60s cadence in addition to boundary announcements; defer any TTY-only spinner to a later contract.
    depends on: q_001, q_003
    blocked: waiting on unanswered prerequisites
  - q_006 blocking=false status=open: Should progress be operator-controllable — a global --no-progress/--quiet CLI flag, a workspace config key, or always-on? M16.0 deliberately made the workspace config stage-centric and strict with dead keys removed, so adding a config key is a real surface change.
    kind: scope
    rationale: The contract says nothing about configurability. Always-on keeps the M16.0 config surface untouched and the CLI flag set stable, but parent tools that already parse stderr might want to opt out.
    recommended answer (confidence medium): Always-on with no new flag or config key in this contract; revisit opt-out only if real usage shows the stderr noise is harmful.
    depends on: q_001, q_002
    blocked: waiting on unanswered prerequisites
  - q_007 blocking=true status=open: How should 'feel responsive' be accepted and verified? Propose concrete acceptance criteria, e.g.: (1) during any agent attempt, the first progress line appears on stderr within 15s and at least every N seconds until the attempt exits; (2) clarify loop and review loop announce each round before its first attempt starts; (3) stdout under --json remains byte-identical to today; verified via unit tests with an injected clock/writer plus 'make check'. Is that the intended bar, and what is N?
    kind: acceptance
    rationale: The contract has no acceptance criteria or validation commands, and 'feel responsive' is unverifiable as written. The repo already injects io.Writer everywhere and has a Makefile check target, so time-based criteria are testable without real agents.
    recommended answer (confidence medium): Adopt the three criteria with N=30s interactive cadence: first line within 15s, then at least every 30s; round announcements precede each loop round; --json stdout unchanged. Validation commands: 'go test ./...' and 'make check'.
    depends on: q_001, q_002
    blocked: waiting on unanswered prerequisites
  - q_008 blocking=false status=answered: Should progress indicators remain ephemeral operator output only, or also be persisted? runAgentAttemptLifecycle (internal/app/agent_attempt.go) already appends typed started/finished events to events.jsonl via ledger.Append, so attempt boundaries are durably recorded today. Persisting a 15-30s heartbeat as ledger events would add hundreds of entries per long run; a new run artifact (e.g. progress.log per attempt) is a third option. Which is intended: stderr-only, ledger events, an artifact, or some combination?
    kind: assumption
    rationale: The contract and the seven open questions only discuss where progress is displayed (stderr, q_003), not whether it is recorded. The repo settles that attempt boundaries are already durable (StartedEvent/FinishedEvent in agent_attempt.go), which argues heartbeats add no durable value, but ledger bloat vs. post-hoc debuggability is a product trade-off the repo cannot decide. The answer changes scope: ledger persistence touches the versioned event schema and store, stderr-only touches nothing but writers.
    recommended answer (confidence high): Progress is ephemeral stderr-only: no new ledger event types, no changes to events.jsonl, no new run artifacts. The existing started/finished ledger events remain the durable record of attempt boundaries.
    answer: Progress is ephemeral stderr-only: no new ledger event types, no changes to events.jsonl, no new run artifacts. The existing started/finished ledger events remain the durable record of attempt boundaries.
  - q_009 blocking=false status=open: Chatty-agent interleaving scenario: agent live output is teed to the operator stderr as raw byte chunks (io.MultiWriter + lockedWriter in internal/agents/runner.go and acp_transport.go), so a chunk can end mid-line. A heartbeat written at that moment splices onto a partial agent line, and nothing distinguishes Pactum's progress lines from the agent's own output for a parent process (CI job, Claude Code) reading stderr. Should every progress line (a) be emitted as a single atomic Write through the same lockedWriter, and (b) carry a stable machine-recognizable prefix (e.g. 'pactum <stage> <attempt_id>:') that is documented as the greppable identifier — or is occasional ambiguous interleaving acceptable?
    kind: edge_case
    rationale: Presupposes line-based progress on stderr (open q_001's leading interpretation). q_004 covers the silent agent and q_005 covers cadence, but neither addresses the opposite case — an agent actively streaming when the heartbeat fires. The repo confirms writes are chunk-level, not line-buffered, so splicing is real; and once a prefix exists, parent tools will match on it, making the format a quasi-API that is costly to change later. That makes the prefix wording a product decision, not just an implementation detail.
    recommended answer (confidence medium): Yes to both: each progress line is one atomic Write through the shared lockedWriter, prefixed 'pactum <stage> <attempt_id>:' (e.g. 'pactum execute exec_0002: 4m10s elapsed'). Do not track the agent's trailing-newline state — accept that a heartbeat may occasionally follow a partial agent line; the stable prefix keeps progress lines unambiguous and greppable. Document the prefix as stable for parent tools.

## Repository context
# Repository Context

Generated: 2026-06-10T08:25:23Z

Map run: map_20260610_074529
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-10T07:45:29Z

Repository root: `.`

## Summary

- Indexed files: 133
- Ignored files/directories: 936
- Detected languages: 6
- Code items (best-effort hints): 1428

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

- Go: 106 file(s)
- Markdown: 21 file(s)
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
- `internal/agents/model.go`
- `internal/agents/process_unix.go`
- `internal/agents/process_windows.go`
- `internal/agents/runner.go`
- `internal/agents/transport.go`
- `internal/agents/types.go`
- `internal/agents/usage.go`
- `internal/agents/usage_test.go`
- `internal/app/agent_attempt.go`
- `internal/app/agent_attempt_transport_test.go`
- `internal/app/agent_output.go`
- `internal/app/agent_output_test.go`
- `internal/app/agents_doctor.go`
- `internal/app/agents_doctor_test.go`
- `internal/app/app.go`
- `internal/app/app_test.go`
- `internal/app/attempt_paths_test.go`
- `internal/app/clarify.go`
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
- `internal/app/review_test.go`
- `internal/app/run.go`
- `internal/app/run_context_test.go`
- `internal/app/status.go`
- `internal/app/store.go`
- `internal/app/store_swap_test.go`
- `internal/app/store_test.go`
- `internal/app/task.go`
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
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyDeniesWrites`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientReadOnlyRefusesPermissionRequests`
- `internal/agents/acp_transport_test.go`: `go_func` `agents.TestACPClientTokenUsage`
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
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessIdleTimeoutResetsOnOutput`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessTeesLiveOutput`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessTimesOutAfterIdleOutputGap`
- `internal/agents/executor_test.go`: `go_func` `agents.TestRunSubprocessWithoutLiveOutputIsCaptureOnly`
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
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseClaudeUsageNormalizesCacheAdditiveInput`
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseCodexUsageTakesLastCompletedEvent`
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseUsageMalformedOrEmptyOutputIsUncaptured`
- `internal/agents/usage_test.go`: `go_func` `agents.TestParseUsageSkippedWhenStructuredOutputIsNotEnabled`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestClarifySuggestMarksAttemptReadOnlyAndPassesModelSpec`
- `internal/app/agent_attempt_transport_test.go`: `go_func` `app.TestContractDraftMarksAttemptReadOnlyAndPassesModelSpec`
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
- `internal/app/agents_doctor_test.go`: `go_func` `app.TestAgentsDoctorDefaultConfig`
- `internal/app/agents_doctor_test.go`: `go_func` `app.TestAgentsDoctorJSON`
- `internal/app/agents_doctor_test.go`: `go_func` `app.TestAgentsDoctorMissingAgentFails`
- `internal/app/agents_doctor_test.go`: `go_func` `app.TestAgentsDoctorReadOnlyLedger`
- `internal/app/agents_doctor_test.go`: `go_func` `app.TestAgentsDoctorSelectedBuiltIn`
- `internal/app/app.go`: `go_func` `app.Run`
- `internal/app/app.go`: `go_method` `App.Run`
- `internal/app/app.go`: `go_method` `App.Search`
- `internal/app/app.go`: `go_method` `App.Status`
- `internal/app/app.go`: `go_type` `app.App`
- `internal/app/app_test.go`: `go_func` `app.TestClarifyAfterApprovedContractResetsApproval`
- `internal/app/app_test.go`: `go_func` `app.TestClarifyAnswerQuestionUpdatesArtifacts`
- `internal/app/app_test.go`: `go_func` `app.TestClarifyArtifactsUseRepoRelativePaths`
- `internal/app/app_test.go`: `go_func` `app.TestClarifyAskBlockingQuestionUpdatesArtifacts`

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
  "query": "Add progress indicators so long operations feel responsive",
  "queries": [
    "progress",
    "indicators",
    "long",
    "operations",
    "feel",
    "responsive"
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
