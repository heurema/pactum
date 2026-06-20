# Contract Drafter Context

## Run
- Run id: run_20260620_063231
- Run status: contract_draft

## Contract goal
Make the pactum agent skill self-sufficient and one-command installable, so a stranger driving pactum through their coding agent (Claude Code or Codex) reliably follows the safe workflow. Two parts: (A) rewrite the skill content so an agent does not need to read the reference files first, and (B) add a `pactum skill install` command that go:embeds the skill and writes it to the correct per-agent directory.

Background (from a Claude+Codex+Gemini design review): agents reliably SKIP "read references/workflow.md before acting" — they see an inline skeleton and start running commands, or skip context entirely (the same way our executor ignored the repo map). So the skill's entry file must be self-contained for the safe path, with the reference files demoted to optional enrichment. Pactum's real strength is its machine affordances: every --json command carries a `next` array of legal next commands and failures carry error.code/error.fix. The safety boundary is already machine-enforced: `pactum execute run` is never advertised in any `next` array, so an agent that only runs commands from `next` cannot auto-cross into unsandboxed execution.

In scope:

A. Rewrite assets/agent-skills/pactum/SKILL.md into a self-sufficient, next-driven "agent card" for the safe happy path:
- Keep YAML frontmatter `name: pactum` + `description` only (stay on the Agent Skills common subset that BOTH Claude Code and Codex accept; do NOT add Claude-only fields like disable-model-invocation).
- Inline the COMPLETE safe command sequence with `--json` on every command (init/status, map refresh if stale, task new, search + read, clarify, contract show/revise/approve, prompt build, execute plan — the safe stop).
- State the core loop rule explicitly: after each command, read the `next` array and run ONLY commands it lists; if `next` is empty, STOP and report to the user; on failure, use error.fix.
- Keep the dangerous rules inline and repeat them at the boundary: never run `pactum execute run` or `pactum review run` by default; `pactum execute plan` is the stop point; and add: do NOT implement source changes by hand after `execute plan` — pactum stops there unless the user explicitly exits pactum or approves unsandboxed execution (otherwise the agent runs the plan then just edits code itself).
- Do NOT hardcode `--agent codex`; use a placeholder/`<agent>` and tell the agent to use the configured executor (detect codex vs claude or ask), since claude users must not be steered to codex.
- Make map/search non-optional where it is pactum's value: instruct the agent to use `pactum search`/read for required context discovery and not substitute raw rg/cat unless pactum directs otherwise.
- Fix the `.heurema` rule (the current "do not commit .heurema/" conflicts with pactum's own durable run-record story): say do not include `.heurema/pactum` churn in feature commits, never delete or revert it, and report it separately unless the user asks for an audit-record commit.
- Specify the exact final report format: run id, contract status (approved or not), the plan command, files likely touched, and an explicit "stopped at execute plan" statement.
- Update the SKILL.md pointers and the reference files so references/workflow.md, install.md, safety.md are clearly OPTIONAL enrichment/detail, not required reading before acting.

B. Add a `pactum skill install` command:
- go:embed the assets/agent-skills/pactum/ package into the binary (a new embed source) so the installed binary can write the skill out without the repo present. Add a test that the embedded copy is byte-identical to assets/agent-skills/pactum/ (anti-drift), extending the existing internal/docs skill sync test rather than duplicating it.
- `pactum skill install --agent <claude|codex|auto|all> --scope <user|repo>`:
  - claude → user: ~/.claude/skills/pactum/ ; repo: .claude/skills/pactum/
  - codex  → user: ~/.agents/skills/pactum/ ; repo: .agents/skills/pactum/
  - auto detects which agent skill dirs / CLIs are present; all installs to every known target.
  - DEFAULT scope is repo (.<agent>/skills) for alpha, so a global skill does not trigger on every repository the user opens.
  - Idempotent overwrite; print the installed path, the skill version (the pactum binary version), and a note to reload/restart the agent if the skill does not appear.
- Add a discovery check (`pactum skill doctor`, or `skill install --check`): verify the skill files exist at the expected path for the selected agent/scope and that SKILL.md frontmatter parses.
- JSON output (`--json`) consistent with the rest of the CLI (carry `next` and the error envelope where applicable).

C. Update docs/skill-install.md to lead with `pactum skill install` as the one-command path (per-agent, correct paths), keeping manual copy as a documented fallback.

Out of scope: any marketplace/plugin distribution; changing the staged workflow commands themselves; the binary release workflow (separate); Gemini/Antigravity-specific skill paths beyond a noted comment (alpha targets claude + codex).

Tests (helper/temp-dir; do not invoke real agents): embedded skill is byte-identical to the on-disk package; `skill install` writes the package to the correct directory for each --agent and --scope into a temp HOME/repo; idempotent re-install; doctor/check reports present vs absent correctly; SKILL.md frontmatter still parses and the existing skill sync test passes.

Validation: go test ./internal/..., go test ./..., go build ./..., make check.

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

Generated: 2026-06-20T06:32:31Z

Map run: map_20260619_160233
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-19T16:02:33Z

Repository root: `.`

## Summary

- Indexed files: 164
- Ignored files/directories: 5372
- Detected languages: 6
- Code items (best-effort hints): 1849

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

- Go: 129 file(s)
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
  "query": "Make the pactum agent skill self-sufficient and one-command installable, so a stranger driving pactum through their coding agent (Claude Code or Codex) reliably follows the safe workflow. Two parts: (A) rewrite the skill content so an agent does not need to read the reference files first, and (B) add a `pactum skill install` command that go:embeds the skill and writes it to the correct per-agent directory.\n\nBackground (from a Claude+Codex+Gemini design review): agents reliably SKIP \"read references/workflow.md before acting\" — they see an inline skeleton and start running commands, or skip context entirely (the same way our executor ignored the repo map). So the skill's entry file must be self-contained for the safe path, with the reference files demoted to optional enrichment. Pactum's real strength is its machine affordances: every --json command carries a `next` array of legal next commands and failures carry error.code/error.fix. The safety boundary is already machine-enforced: `pactum execute run` is never advertised in any `next` array, so an agent that only runs commands from `next` cannot auto-cross into unsandboxed execution.\n\nIn scope:\n\nA. Rewrite assets/agent-skills/pactum/SKILL.md into a self-sufficient, next-driven \"agent card\" for the safe happy path:\n- Keep YAML frontmatter `name: pactum` + `description` only (stay on the Agent Skills common subset that BOTH Claude Code and Codex accept; do NOT add Claude-only fields like disable-model-invocation).\n- Inline the COMPLETE safe command sequence with `--json` on every command (init/status, map refresh if stale, task new, search + read, clarify, contract show/revise/approve, prompt build, execute plan — the safe stop).\n- State the core loop rule explicitly: after each command, read the `next` array and run ONLY commands it lists; if `next` is empty, STOP and report to the user; on failure, use error.fix.\n- Keep the dangerous rules inline and repeat them at the boundary: never run `pactum execute run` or `pactum review run` by default; `pactum execute plan` is the stop point; and add: do NOT implement source changes by hand after `execute plan` — pactum stops there unless the user explicitly exits pactum or approves unsandboxed execution (otherwise the agent runs the plan then just edits code itself).\n- Do NOT hardcode `--agent codex`; use a placeholder/`\u003cagent\u003e` and tell the agent to use the configured executor (detect codex vs claude or ask), since claude users must not be steered to codex.\n- Make map/search non-optional where it is pactum's value: instruct the agent to use `pactum search`/read for required context discovery and not substitute raw rg/cat unless pactum directs otherwise.\n- Fix the `.heurema` rule (the current \"do not commit .heurema/\" conflicts with pactum's own durable run-record story): say do not include `.heurema/pactum` churn in feature commits, never delete or revert it, and report it separately unless the user asks for an audit-record commit.\n- Specify the exact final report format: run id, contract status (approved or not), the plan command, files likely touched, and an explicit \"stopped at execute plan\" statement.\n- Update the SKILL.md pointers and the reference files so references/workflow.md, install.md, safety.md are clearly OPTIONAL enrichment/detail, not required reading before acting.\n\nB. Add a `pactum skill install` command:\n- go:embed the assets/agent-skills/pactum/ package into the binary (a new embed source) so the installed binary can write the skill out without the repo present. Add a test that the embedded copy is byte-identical to assets/agent-skills/pactum/ (anti-drift), extending the existing internal/docs skill sync test rather than duplicating it.\n- `pactum skill install --agent \u003cclaude|codex|auto|all\u003e --scope \u003cuser|repo\u003e`:\n  - claude → user: ~/.claude/skills/pactum/ ; repo: .claude/skills/pactum/\n  - codex  → user: ~/.agents/skills/pactum/ ; repo: .agents/skills/pactum/\n  - auto detects which agent skill dirs / CLIs are present; all installs to every known target.\n  - DEFAULT scope is repo (.\u003cagent\u003e/skills) for alpha, so a global skill does not trigger on every repository the user opens.\n  - Idempotent overwrite; print the installed path, the skill version (the pactum binary version), and a note to reload/restart the agent if the skill does not appear.\n- Add a discovery check (`pactum skill doctor`, or `skill install --check`): verify the skill files exist at the expected path for the selected agent/scope and that SKILL.md frontmatter parses.\n- JSON output (`--json`) consistent with the rest of the CLI (carry `next` and the error envelope where applicable).\n\nC. Update docs/skill-install.md to lead with `pactum skill install` as the one-command path (per-agent, correct paths), keeping manual copy as a documented fallback.\n\nOut of scope: any marketplace/plugin distribution; changing the staged workflow commands themselves; the binary release workflow (separate); Gemini/Antigravity-specific skill paths beyond a noted comment (alpha targets claude + codex).\n\nTests (helper/temp-dir; do not invoke real agents): embedded skill is byte-identical to the on-disk package; `skill install` writes the package to the correct directory for each --agent and --scope into a temp HOME/repo; idempotent re-install; doctor/check reports present vs absent correctly; SKILL.md frontmatter still parses and the existing skill sync test passes.\n\nValidation: go test ./internal/..., go test ./..., go build ./..., make check.",
  "queries": [
    "references/workflow.md",
    "error.code/error.fix",
    "assets/agent-skills/pactum/SKILL.md",
    "init/status",
    "show/revise/approve",
    "error.fix",
    "placeholder/`\u003cagent\u003e",
    "map/search"
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
