# Contract Drafter Context

## Run
- Run id: run_20260611_110706
- Run status: contract_draft

## Contract goal
Tell the security truth in the user-facing docs and add a security policy. README's built-in agents section currently describes the codex executor as plain 'codex exec' while docs/agents.md documents the real command 'codex exec --dangerously-bypass-approvals-and-sandbox' — README must state the exact command and warn that real execution is unsandboxed with direct repository and runtime access, safe only in trusted repositories. Add SECURITY.md at the repo root with: the threat model (pactum is not a sandbox; the repository and runtime environment are the real boundary; execute run / review run / clarify run / contract draft launch external agent tooling), safe-usage guidance (trusted repositories only, prefer execute plan before execute run, review the contract path scope before execution, avoid exposing long-lived credentials in the environment), private vulnerability reporting to the maintainer before any public issue, and supported versions (main only, until tagged releases exist). SECURITY.md and README must stay consistent with docs/agents.md as the detailed reference. Wire SECURITY.md into the internal/docs test file lists that pin user-facing docs (existence and forbidden stale phrases) so it cannot drift silently.

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
- q_001: Should SECURITY.md describe only the four commands named in the draft (`pactum execute run`, `pactum review run`, `pactum clarify run`, `pactum contract draft`), or all current commands that can launch external agent tooling, including `pactum clarify suggest`, `pactum review fix run`, and `pactum review loop`?
  Rationale: The draft lists four commands, but `docs/agents.md` documents additional agent-running paths. This affects whether the security policy fully matches the repository's actual execution surface.
  Answer: SECURITY.md should describe all current agent-launching commands, using the four named commands as examples but explicitly including clarify suggest/clarify run, contract draft, execute run, review run, review fix run, and review loop where applicable.
- q_002: Concrete edge case: if a user runs a read-only stage such as `pactum clarify run` or `pactum contract draft` while their shell contains long-lived credentials like `GITHUB_TOKEN`, cloud keys, or agent auth tokens, should SECURITY.md warn that those credentials are still exposed to external agent tooling?
  Rationale: The draft says to avoid exposing long-lived credentials, and `docs/agents.md` says agent subprocesses/adapters inherit the environment even for read-only stages. The policy should avoid implying that read-only stages are safe for secrets.
  Answer: Warn that every agent-launching command may expose inherited environment variables to external tooling, including read-only/reviewer/drafter/clarifier stages, and recommend running Pactum with the smallest practical environment and no long-lived credentials.
- q_003 [blocking]: What private vulnerability reporting channel should SECURITY.md name: GitHub private vulnerability reporting for `github.com/heurema/pactum`, a maintainer email/address, or a generic instruction to request a private maintainer channel before filing a public issue?
  Rationale: The repo identifies the module and GitHub owner path, but no maintainer email or enabled private-reporting channel is documented. A security policy needs a usable private path; inventing a contact would be unsafe.
  Answer: Use GitHub private vulnerability reporting at `https://github.com/heurema/pactum/security/advisories/new`; if that is not enabled, instruct reporters to contact the maintainer privately and avoid public issues until a private channel is established.
- q_004: For test coverage, should `internal/docs` only add `SECURITY.md` to the existing required user-facing doc/stale-phrase lists, or should it also assert that SECURITY.md contains the required security-policy concepts?
  Rationale: `internal/docs/docs_test.go` already pins user-facing docs by explicit file list and stale phrase checks. The draft says SECURITY.md should not drift silently, but existence plus forbidden phrases would not catch deletion of the threat model, reporting channel, or supported-version policy.
  Answer: Add `SECURITY.md` to the required user-facing docs and stale-phrase scan, and add focused positive assertions that it mentions Pactum is not a sandbox, trusted repositories, planning before running, path-scope review, credential exposure, private vulnerability reporting, and support for `main` only until tagged releases exist.
- q_005 [blocking]: When the contract says README must state the exact `codex` command, should it name the CLI executor descriptor (`codex exec --json --dangerously-bypass-approvals-and-sandbox`) while also explaining that the default transport is ACP via `npx -y @zed-industries/codex-acp@latest`, or should README only use the shorter `docs/agents.md` wording (`codex exec --dangerously-bypass-approvals-and-sandbox`)?
  Rationale: The draft uses 'exact command' and points at `docs/agents.md`, but the repository currently has two concrete command surfaces: `internal/agents/config.go` defines the CLI executor descriptor with `--json`, while `internal/app/app.go` makes ACP the default and `internal/agents/acp_transport.go` launches external npm ACP adapters via `npx`. The README already mentions ACP as default, so silently treating `codex exec ...` as the only exact command could keep the security docs technically incomplete.
  Answer: README should state the CLI executor descriptor as `codex exec --json --dangerously-bypass-approvals-and-sandbox` when describing the CLI transport / executor descriptor, and also say the default ACP transport launches external ACP adapters such as `npx -y @zed-industries/codex-acp@latest`; both paths are direct external tooling with repository and environment access. Keep `docs/agents.md` as the detailed reference and update it only if needed for consistency.
- q_006: What should 'trusted repositories' mean in SECURITY.md and README: a human-vetted repository/task boundary, Codex CLI's trusted-project approval state, or both?
  Rationale: The contract says real execution is safe only in trusted repositories. In `docs/agents.md`, 'trusted repo' also appears in the specific Codex approval-policy sense where Codex may ask no permission. Those are different security concepts, and collapsing them could make the guidance ambiguous.
  Answer: Use 'trusted repository' to mean a repository and task context the human is willing to expose to arbitrary external agent tooling, including repository files, shell commands, and inherited environment variables. If Codex's trusted-project state is mentioned, distinguish it as an agent-specific approval setting that can increase risk; it is not what makes a repository safe.
- q_007: Concrete edge case: if a user runs the default ACP transport for the first time and `npx -y @zed-industries/codex-acp@latest` or `npx -y @agentclientprotocol/claude-agent-acp@latest` downloads and executes an external adapter package, should SECURITY.md explicitly warn about that external package/runtime dependency in addition to warning about agent CLIs?
  Rationale: `docs/agents.md` and `internal/agents/acp_transport.go` show ACP is the default and runs adapter packages through `npx`; q_002 already covers inherited credentials for agent-launching commands, but not the separate supply-chain/runtime implication of executing latest external npm adapters. This affects whether the security policy fully describes the default execution surface.
  Answer: Yes. SECURITY.md should say Pactum may launch external agent tooling either through the default ACP adapters (`npx` packages) or the CLI transport, and both inherit the repository/runtime environment. Users in restricted or high-sensitivity environments should review/pin/control those tools outside Pactum before running agent-launching commands.
- q_008: Should this contract update `docs/backlog.md` to remove or mark complete the existing P0 item that says README still describes `codex` as plain `codex exec`, or is `docs/backlog.md` out of scope because the contract targets user-facing docs and `internal/docs` pins only README/install/flow/workspace/agents/memory plus the new SECURITY.md?
  Rationale: `docs/backlog.md` contains the exact security-docs task text and will become stale after the README/SECURITY.md change, but it is not part of `internal/docs/docs_test.go`'s required user-facing doc set. This changes whether the implementation touches only the user-facing docs/tests named in the contract or also updates project planning documentation.
  Answer: Keep `docs/backlog.md` out of scope for this contract; update README.md, add SECURITY.md, adjust docs/agents.md only if needed for consistency, and wire SECURITY.md into `internal/docs` tests. Treat backlog pruning or completion marking as a separate cleanup.

## Repository context
# Repository Context

Generated: 2026-06-11T11:07:06Z

Map run: map_20260611_093727
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-11T09:37:27Z

Repository root: `.`

## Summary

- Indexed files: 147
- Ignored files/directories: 1381
- Detected languages: 6
- Code items (best-effort hints): 1608

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
- `internal/app/confirm.go`
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
  "query": "Tell the security truth in the user-facing docs and add a security policy. README's built-in agents section currently describes the codex executor as plain 'codex exec' while docs/agents.md documents the real command 'codex exec --dangerously-bypass-approvals-and-sandbox' — README must state the exact command and warn that real execution is unsandboxed with direct repository and runtime access, safe only in trusted repositories. Add SECURITY.md at the repo root with: the threat model (pactum is not a sandbox; the repository and runtime environment are the real boundary; execute run / review run / clarify run / contract draft launch external agent tooling), safe-usage guidance (trusted repositories only, prefer execute plan before execute run, review the contract path scope before execution, avoid exposing long-lived credentials in the environment), private vulnerability reporting to the maintainer before any public issue, and supported versions (main only, until tagged releases exist). SECURITY.md and README must stay consistent with docs/agents.md as the detailed reference. Wire SECURITY.md into the internal/docs test file lists that pin user-facing docs (existence and forbidden stale phrases) so it cannot drift silently.",
  "queries": [
    "docs/agents.md",
    "SECURITY.md",
    "/",
    "internal/docs",
    "user-facing",
    "built-in",
    "dangerously-bypass-approvals-and-sandbox",
    "safe-usage"
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
