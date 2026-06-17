# Contract Drafter Context

## Run
- Run id: run_20260617_210449
- Run status: contract_draft

## Contract goal
Make pactum's code-review loop never silently drop reviewer findings, and recover automatically when a reviewer omits the structured findings block before failing loud.

Problem: the reviewer prompt (renderReviewerPrompt in internal/app/review.go) permits findings either in prose OR in a fenced JSON block (schema pactum.reviewer_findings.v1alpha1), but the parser (parseReviewerFindingBlocks in internal/app/review_proposals.go) reads ONLY the JSON block. A reviewer (codex over ACP) that writes findings in prose has them silently dropped. The loop's clean/unparsed discriminator (internal/app/review_loop.go) only treats zero proposals as a parse-miss when a warning fired, and that warning only fires when the schema string is literally present in the text. So prose-only findings -- or findings from one lens while other lenses produced proposals -- vanish with no signal, and the review can look approvable while real findings are missing. This actually happened: two lenses found a validation-gating bug and a renderer bug, wrote them in prose, and they were silently absent from findings.jsonl.

Fix -- force the format + structural discriminator + per-lens corrective retry with escalation:

1. Prompt: make the fenced pactum.reviewer_findings.v1alpha1 JSON block MANDATORY and ALWAYS emitted -- emit "findings": [] when there are no findings. Drop "prose OR json" as equal reporting channels; prose becomes a human-readable supplement only and is never parsed. Include a worked clean example showing "findings": [].

2. Struct: change reviewerFindingBlock.Findings from []json.RawMessage to *[]json.RawMessage (or otherwise track key presence) so a block {"schema": ...} with NO findings key (nil) is distinguishable from an explicit empty array. Absent/nil => malformed parse-miss; non-nil (including empty []) => valid block, clean when empty.

3. Parser: warn whenever a reviewer attempt yields no VALID findings block (drop the guard that only warns when the schema string is present). A valid block with "findings": [] parses to one block, zero findings, zero warnings (clean). A block missing the findings key, or no block at all, is a parse-miss => warning.

4. Per-lens enforcement in the discriminator: a missing or invalid block on ANY lens's attempt must surface loudly and prevent the round from looking clean/approvable -- even when OTHER lenses produced proposals. A lens that emitted no valid block makes the round unparsed (or otherwise non-approvable), never silently partial.

5. Per-lens corrective retry with escalation (the PRIMARY recovery path, not just loud-fail): when a lens attempt yields no valid block, give the reviewer a corrective signal and let it retry, bounded by a small cap (1-2). Prefer a same-session follow-up turn if the ACP reviewer session supports a second turn ("your previous response did not include the required block; emit exactly one pactum.reviewer_findings.v1alpha1 block now, findings: [] if none; prose is ignored"); otherwise re-run the attempt with the hardened format instruction. Only after the bounded retries still yield no valid block does the round escalate to a loud reviewer_findings_unparsed terminal stop. The retry trigger is STRUCTURAL (no valid block parsed), never a prose heuristic, so a genuinely-clean reviewer -- which emits "findings": [] -- is never re-prompted (this is why forcing the format matters: it removes any need to inspect prose). The retry lives at the reviewer-attempt layer, below the outer loop's Max/Patience, so loop round accounting is unaffected.

Invariant: a round counts as clean if and only if every reviewer lens emitted a valid block whose findings array is empty -- never because zero proposals were extracted from the output.

In scope: the prompt change, the struct/presence change, the parser warning change, the per-lens discriminator change, the bounded corrective-retry-then-escalate mechanism, and focused Go tests.

Tests must cover the exact bug: a valid "findings": [] block => clean round, no warning; a clean reviewer that writes residual-risk prose AND emits "findings": [] => still clean, no false stop; a prose-only attempt with no block => corrective retry, then on persistent miss => reviewer_findings_unparsed loud stop; a retry that succeeds on the second attempt => findings captured, no stop; a mixed round (one lens emits a valid block with findings, another lens emits no block) => the missing-block lens is surfaced loudly and the round is not silently partial; a block carrying the schema but no findings key => parse-miss, not clean.

Out of scope: routing or defaulting the reviewer role to a more reliable emitter (Claude) where a cross-model reviewer exists -- that is a separate later slice. Parsing prose findings into proposals (rejected by design as unsafe given proposals auto-accept to the fixer). Changing the fixer or the proposal auto-accept path.

Validation: go test ./internal/app -run Review, go test ./..., go build ./..., make check.

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

Generated: 2026-06-17T21:04:49Z

Map run: map_20260617_182427
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-17T18:24:27Z

Repository root: `.`

## Summary

- Indexed files: 164
- Ignored files/directories: 3941
- Detected languages: 6
- Code items (best-effort hints): 1831

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
  "query": "Make pactum's code-review loop never silently drop reviewer findings, and recover automatically when a reviewer omits the structured findings block before failing loud.\n\nProblem: the reviewer prompt (renderReviewerPrompt in internal/app/review.go) permits findings either in prose OR in a fenced JSON block (schema pactum.reviewer_findings.v1alpha1), but the parser (parseReviewerFindingBlocks in internal/app/review_proposals.go) reads ONLY the JSON block. A reviewer (codex over ACP) that writes findings in prose has them silently dropped. The loop's clean/unparsed discriminator (internal/app/review_loop.go) only treats zero proposals as a parse-miss when a warning fired, and that warning only fires when the schema string is literally present in the text. So prose-only findings -- or findings from one lens while other lenses produced proposals -- vanish with no signal, and the review can look approvable while real findings are missing. This actually happened: two lenses found a validation-gating bug and a renderer bug, wrote them in prose, and they were silently absent from findings.jsonl.\n\nFix -- force the format + structural discriminator + per-lens corrective retry with escalation:\n\n1. Prompt: make the fenced pactum.reviewer_findings.v1alpha1 JSON block MANDATORY and ALWAYS emitted -- emit \"findings\": [] when there are no findings. Drop \"prose OR json\" as equal reporting channels; prose becomes a human-readable supplement only and is never parsed. Include a worked clean example showing \"findings\": [].\n\n2. Struct: change reviewerFindingBlock.Findings from []json.RawMessage to *[]json.RawMessage (or otherwise track key presence) so a block {\"schema\": ...} with NO findings key (nil) is distinguishable from an explicit empty array. Absent/nil =\u003e malformed parse-miss; non-nil (including empty []) =\u003e valid block, clean when empty.\n\n3. Parser: warn whenever a reviewer attempt yields no VALID findings block (drop the guard that only warns when the schema string is present). A valid block with \"findings\": [] parses to one block, zero findings, zero warnings (clean). A block missing the findings key, or no block at all, is a parse-miss =\u003e warning.\n\n4. Per-lens enforcement in the discriminator: a missing or invalid block on ANY lens's attempt must surface loudly and prevent the round from looking clean/approvable -- even when OTHER lenses produced proposals. A lens that emitted no valid block makes the round unparsed (or otherwise non-approvable), never silently partial.\n\n5. Per-lens corrective retry with escalation (the PRIMARY recovery path, not just loud-fail): when a lens attempt yields no valid block, give the reviewer a corrective signal and let it retry, bounded by a small cap (1-2). Prefer a same-session follow-up turn if the ACP reviewer session supports a second turn (\"your previous response did not include the required block; emit exactly one pactum.reviewer_findings.v1alpha1 block now, findings: [] if none; prose is ignored\"); otherwise re-run the attempt with the hardened format instruction. Only after the bounded retries still yield no valid block does the round escalate to a loud reviewer_findings_unparsed terminal stop. The retry trigger is STRUCTURAL (no valid block parsed), never a prose heuristic, so a genuinely-clean reviewer -- which emits \"findings\": [] -- is never re-prompted (this is why forcing the format matters: it removes any need to inspect prose). The retry lives at the reviewer-attempt layer, below the outer loop's Max/Patience, so loop round accounting is unaffected.\n\nInvariant: a round counts as clean if and only if every reviewer lens emitted a valid block whose findings array is empty -- never because zero proposals were extracted from the output.\n\nIn scope: the prompt change, the struct/presence change, the parser warning change, the per-lens discriminator change, the bounded corrective-retry-then-escalate mechanism, and focused Go tests.\n\nTests must cover the exact bug: a valid \"findings\": [] block =\u003e clean round, no warning; a clean reviewer that writes residual-risk prose AND emits \"findings\": [] =\u003e still clean, no false stop; a prose-only attempt with no block =\u003e corrective retry, then on persistent miss =\u003e reviewer_findings_unparsed loud stop; a retry that succeeds on the second attempt =\u003e findings captured, no stop; a mixed round (one lens emits a valid block with findings, another lens emits no block) =\u003e the missing-block lens is surfaced loudly and the round is not silently partial; a block carrying the schema but no findings key =\u003e parse-miss, not clean.\n\nOut of scope: routing or defaulting the reviewer role to a more reliable emitter (Claude) where a cross-model reviewer exists -- that is a separate later slice. Parsing prose findings into proposals (rejected by design as unsafe given proposals auto-accept to the fixer). Changing the fixer or the proposal auto-accept path.\n\nValidation: go test ./internal/app -run Review, go test ./..., go build ./..., make check.",
  "queries": [
    "internal/app/review.go",
    "internal/app/review_proposals.go",
    "clean/unparsed",
    "internal/app/review_loop.go",
    "findings.jsonl",
    "Absent/nil",
    "clean/approvable",
    "Max/Patience"
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
