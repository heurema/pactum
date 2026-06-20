# Contract Drafter Context

## Run
- Run id: run_20260620_114301
- Run status: contract_draft

## Contract goal
De-hardcode `--agent codex` from pactum's lifecycle `next` affordances and human "Next:" output so the suggested execute-plan command respects the user's CONFIGURED executor instead of steering every user — including a claude-only user — to codex.

Background: the skill-install slice (#214) shipped a SKILL.md that correctly uses an `<agent>` placeholder and tells the agent to use the configured executor, but pactum's own machine/human surfaces still emit `pactum execute plan <run> --agent codex` literally. This was flagged by that slice's two non-blocking review findings (f_005: the lifecycle `next` affordance hardcodes codex; f_007: the human-facing output hardcodes it). Because an agent driving pactum runs the `next` array VERBATIM, a claude-only user is silently steered to `--agent codex`, contradicting the cross-agent guidance. The fix is to DROP the hardcoded `--agent codex` so the command defers to execute-plan's existing default-agent resolution (an omitted `--agent` already resolves the configured executor via `prepareExecution`). This is the pattern the codebase already uses elsewhere: `nextCommandForStatus` and `resolve.go`'s `nextCommandForStatus` sibling already emit the bare `pactum execute plan` form, and `execute plan` with no `--agent` is already runnable.

In scope (exactly three production affordance sites + their tests; change ONLY the emitted command string, not resolution logic):
1. internal/app/resolve.go — the `prompt_built` case in the `next`-affordance builder currently returns `[]string{"pactum execute plan " + runID + " --agent codex"}`. Drop ` --agent codex` so it returns `"pactum execute plan " + runID`.
2. internal/app/errors.go — `noExecutionAttemptError` sets `next: []string{"pactum execute plan " + runID + " --agent codex"}`. Drop ` --agent codex` likewise.
3. internal/app/prompt.go — the human "Next:" line in the prompt-build writer prints `"  pactum execute plan %s --agent codex\n"`. Drop ` --agent codex` so it prints `"  pactum execute plan %s\n"`.
4. Update the tests that assert the OLD string to assert the new bare form: internal/app/prompt_test.go (the `pactum execute plan <run> --agent codex` expectation) and internal/app/affordances_test.go (all three: the `wantNext` builder, the `assertNext` line, and the human-output `Next:` substring check).

The result: every lifecycle affordance for the execute-plan step emits `pactum execute plan <run>` (no `--agent`), so it respects `pipeline.execute.by` (the configured executor) for whoever is running — a claude user gets their claude executor, a codex user gets codex — instead of a hardcoded codex. This is consistent with the SKILL.md `<agent>` guidance and with the already-bare affordances elsewhere in resolve.go.

Out of scope (do NOT do here):
- Do NOT change execute-plan's default-agent RESOLUTION logic (`prepareExecution` / `ExecutePlan`); only the emitted affordance/human strings change. The omitted-`--agent` default already works.
- Do NOT touch internal/app/skill.go's "no agent targets detected; use --agent claude, --agent codex, or --agent all" help text — it legitimately lists all install targets and is not a lifecycle affordance.
- Do NOT introduce a new `--agent` placeholder token in the machine `next` array (an agent must be able to run it verbatim; a literal `<agent>` would not run). Dropping the flag is the correct machine-runnable form.
- No schema change, no new flag, no change to the contract-review "no nitpicks" calibration (a separate slice), no docs/SKILL.md changes (the skill is already correct).

Tests: the updated affordance/prompt tests assert the bare `pactum execute plan <run>` form (no `--agent codex`) in both the machine `next` arrays (resolve.go and errors.go paths) and the human "Next:" output (prompt.go). Verify no other test still asserts the hardcoded `--agent codex` for these lifecycle affordances (the skill_test.go references to `--agent codex` are for `skill install` and must stay).

Validation: go build ./..., go vet ./..., go test ./internal/..., go test ./..., make check.

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

Generated: 2026-06-20T11:43:01Z

Map run: map_20260620_101240
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-20T10:12:40Z

Repository root: `.`

## Summary

- Indexed files: 168
- Ignored files/directories: 5736
- Detected languages: 6
- Code items (best-effort hints): 1904

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
- YAML: 2 file(s)
- Make: 1 file(s)
- Shell: 1 file(s)

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
- `.github/workflows/release.yml`
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
- `internal/agents/acp_transport_wallclock_test.go`
- `internal/agents/acp_transport_wallclock_unix_test.go`
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
- `internal/app/contract_review.go`
- `internal/app/contract_review_resolve_test.go`
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
- `internal/agents/acp_transport_wallclock_test.go`: `go_func` `agents.TestACPTransportFastExitNoWallClockFlag`
- `internal/agents/acp_transport_wallclock_test.go`: `go_func` `agents.TestACPTransportIdleTimeoutDistinctFromWallClockCap`
- `internal/agents/acp_transport_wallclock_test.go`: `go_func` `agents.TestACPTransportWallClockCapKillsHangingProcess`
- `internal/agents/acp_transport_wallclock_test.go`: `go_func` `agents.TestMain`
- `internal/agents/acp_transport_wallclock_test.go`: `go_func` `agents.TestStartWallClockTimeoutFiresAndSetsFlag`
- `internal/agents/acp_transport_wallclock_test.go`: `go_func` `agents.TestStartWallClockTimeoutStopPreventsCancel`
- `internal/agents/acp_transport_wallclock_test.go`: `go_func` `agents.TestWallClockCapFiresWhileIdleTimerKeptAlive`
- `internal/agents/acp_transport_wallclock_unix_test.go`: `go_func` `agents.TestACPTransportWallClockCapReapsChildProcess`
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
  "query": "De-hardcode `--agent codex` from pactum's lifecycle `next` affordances and human \"Next:\" output so the suggested execute-plan command respects the user's CONFIGURED executor instead of steering every user — including a claude-only user — to codex.\n\nBackground: the skill-install slice (#214) shipped a SKILL.md that correctly uses an `\u003cagent\u003e` placeholder and tells the agent to use the configured executor, but pactum's own machine/human surfaces still emit `pactum execute plan \u003crun\u003e --agent codex` literally. This was flagged by that slice's two non-blocking review findings (f_005: the lifecycle `next` affordance hardcodes codex; f_007: the human-facing output hardcodes it). Because an agent driving pactum runs the `next` array VERBATIM, a claude-only user is silently steered to `--agent codex`, contradicting the cross-agent guidance. The fix is to DROP the hardcoded `--agent codex` so the command defers to execute-plan's existing default-agent resolution (an omitted `--agent` already resolves the configured executor via `prepareExecution`). This is the pattern the codebase already uses elsewhere: `nextCommandForStatus` and `resolve.go`'s `nextCommandForStatus` sibling already emit the bare `pactum execute plan` form, and `execute plan` with no `--agent` is already runnable.\n\nIn scope (exactly three production affordance sites + their tests; change ONLY the emitted command string, not resolution logic):\n1. internal/app/resolve.go — the `prompt_built` case in the `next`-affordance builder currently returns `[]string{\"pactum execute plan \" + runID + \" --agent codex\"}`. Drop ` --agent codex` so it returns `\"pactum execute plan \" + runID`.\n2. internal/app/errors.go — `noExecutionAttemptError` sets `next: []string{\"pactum execute plan \" + runID + \" --agent codex\"}`. Drop ` --agent codex` likewise.\n3. internal/app/prompt.go — the human \"Next:\" line in the prompt-build writer prints `\"  pactum execute plan %s --agent codex\\n\"`. Drop ` --agent codex` so it prints `\"  pactum execute plan %s\\n\"`.\n4. Update the tests that assert the OLD string to assert the new bare form: internal/app/prompt_test.go (the `pactum execute plan \u003crun\u003e --agent codex` expectation) and internal/app/affordances_test.go (all three: the `wantNext` builder, the `assertNext` line, and the human-output `Next:` substring check).\n\nThe result: every lifecycle affordance for the execute-plan step emits `pactum execute plan \u003crun\u003e` (no `--agent`), so it respects `pipeline.execute.by` (the configured executor) for whoever is running — a claude user gets their claude executor, a codex user gets codex — instead of a hardcoded codex. This is consistent with the SKILL.md `\u003cagent\u003e` guidance and with the already-bare affordances elsewhere in resolve.go.\n\nOut of scope (do NOT do here):\n- Do NOT change execute-plan's default-agent RESOLUTION logic (`prepareExecution` / `ExecutePlan`); only the emitted affordance/human strings change. The omitted-`--agent` default already works.\n- Do NOT touch internal/app/skill.go's \"no agent targets detected; use --agent claude, --agent codex, or --agent all\" help text — it legitimately lists all install targets and is not a lifecycle affordance.\n- Do NOT introduce a new `--agent` placeholder token in the machine `next` array (an agent must be able to run it verbatim; a literal `\u003cagent\u003e` would not run). Dropping the flag is the correct machine-runnable form.\n- No schema change, no new flag, no change to the contract-review \"no nitpicks\" calibration (a separate slice), no docs/SKILL.md changes (the skill is already correct).\n\nTests: the updated affordance/prompt tests assert the bare `pactum execute plan \u003crun\u003e` form (no `--agent codex`) in both the machine `next` arrays (resolve.go and errors.go paths) and the human \"Next:\" output (prompt.go). Verify no other test still asserts the hardcoded `--agent codex` for these lifecycle affordances (the skill_test.go references to `--agent codex` are for `skill install` and must stay).\n\nValidation: go build ./..., go vet ./..., go test ./internal/..., go test ./..., make check.",
  "queries": [
    "SKILL.md",
    "machine/human",
    "internal/app/resolve.go",
    "internal/app/errors.go",
    "internal/app/prompt.go",
    "internal/app/prompt_test.go",
    "internal/app/affordances_test.go",
    "pipeline.execute.by"
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
