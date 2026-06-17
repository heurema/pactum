# Contract Drafter Context

## Run
- Run id: run_20260617_090147
- Run status: contract_draft

## Contract goal
Rework the pactum config to the new pipeline shape and wire it through the existing code; behaviour-preserving (no new runtime capability), config FORMAT change only. No users, so breaking the config shape is free.

NEW top-level config keys: version (string, value v1alpha1 — REPLACES the old schema: pactum.config.v1alpha1 field; the config file is standalone so renaming its version field is safe; do NOT touch the schema discriminator on any other artifact/record type); agents (unchanged: [{name, model, effort?}]); map (unchanged); out_of_scope (string, block|warn — REPLACES gate.scope_enforcement; drop the gate: wrapper); pipeline (a map of stage -> {by, loop?}). Remove the old top-level keys schema, gate, review, contract, clarify, timeouts.

pipeline stages and their value shape: clarify, contract_draft, contract_review, execute, code_review, memory. Each stage is an object. by: is the performer(s) — a scalar agent name OR a list, normalized to []string. loop: is {max, patience, settle} (optional).

VALIDATION (load-time): loop: is valid ONLY on clarify, contract_review, code_review (the loop stages); loop on contract_draft/execute/memory is a load error. A by: LIST (len>1) is valid ONLY on contract_review and code_review (the existing panels); len>1 on any other stage is a load error. Every by: agent name must resolve in agents (else load error). out_of_scope must be block or warn. Reject unknown top-level keys and unknown stage names.

MAPPING (the resolver must reproduce today's behaviour exactly): review.panel -> pipeline.code_review.by; review.{max_rounds,patience,clean_rounds} -> pipeline.code_review.loop.{max,patience,settle}; contract.reviewers -> pipeline.contract_review.by; contract_review now ALSO gets loop knobs via pipeline.contract_review.loop.{max,patience,settle} (today it reuses the review limits — keep that resolution, just sourced from pipeline.contract_review.loop, falling back to the same defaults); clarify.max_rounds -> pipeline.clarify.loop.max; gate.scope_enforcement -> out_of_scope. Single-agent stages contract_draft/execute/memory and clarify each name their agent via by:; these must reproduce today's resolved per-stage agent assignment (verify against run.go role-resolution before hardcoding the default config's by: values).

Update the default-config writer to emit the new shape. Update all config call sites (config.Schema, config.Review.*, config.Contract.*, config.Gate.*, config.Clarify.*, config.Timeouts.*) to read from the new structs. Update config tests.

Constraints: behaviour-preserving (same agents/limits resolve as today); do NOT change internal/loop, the loop bodies (review_loop.go/contract_review.go/clarify_loop.go logic), or add multi-model/best-of-N. Validation: go build ./..., go test ./..., make check, and go test ./internal/app -run TestReadConfig.

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

Generated: 2026-06-17T09:01:47Z

Map run: map_20260617_090146
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-17T09:01:46Z

Repository root: `.`

## Summary

- Indexed files: 162
- Ignored files/directories: 3506
- Detected languages: 6
- Code items (best-effort hints): 1822

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
- Markdown: 27 file(s)
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
  "query": "Rework the pactum config to the new pipeline shape and wire it through the existing code; behaviour-preserving (no new runtime capability), config FORMAT change only. No users, so breaking the config shape is free.\n\nNEW top-level config keys: version (string, value v1alpha1 — REPLACES the old schema: pactum.config.v1alpha1 field; the config file is standalone so renaming its version field is safe; do NOT touch the schema discriminator on any other artifact/record type); agents (unchanged: [{name, model, effort?}]); map (unchanged); out_of_scope (string, block|warn — REPLACES gate.scope_enforcement; drop the gate: wrapper); pipeline (a map of stage -\u003e {by, loop?}). Remove the old top-level keys schema, gate, review, contract, clarify, timeouts.\n\npipeline stages and their value shape: clarify, contract_draft, contract_review, execute, code_review, memory. Each stage is an object. by: is the performer(s) — a scalar agent name OR a list, normalized to []string. loop: is {max, patience, settle} (optional).\n\nVALIDATION (load-time): loop: is valid ONLY on clarify, contract_review, code_review (the loop stages); loop on contract_draft/execute/memory is a load error. A by: LIST (len\u003e1) is valid ONLY on contract_review and code_review (the existing panels); len\u003e1 on any other stage is a load error. Every by: agent name must resolve in agents (else load error). out_of_scope must be block or warn. Reject unknown top-level keys and unknown stage names.\n\nMAPPING (the resolver must reproduce today's behaviour exactly): review.panel -\u003e pipeline.code_review.by; review.{max_rounds,patience,clean_rounds} -\u003e pipeline.code_review.loop.{max,patience,settle}; contract.reviewers -\u003e pipeline.contract_review.by; contract_review now ALSO gets loop knobs via pipeline.contract_review.loop.{max,patience,settle} (today it reuses the review limits — keep that resolution, just sourced from pipeline.contract_review.loop, falling back to the same defaults); clarify.max_rounds -\u003e pipeline.clarify.loop.max; gate.scope_enforcement -\u003e out_of_scope. Single-agent stages contract_draft/execute/memory and clarify each name their agent via by:; these must reproduce today's resolved per-stage agent assignment (verify against run.go role-resolution before hardcoding the default config's by: values).\n\nUpdate the default-config writer to emit the new shape. Update all config call sites (config.Schema, config.Review.*, config.Contract.*, config.Gate.*, config.Clarify.*, config.Timeouts.*) to read from the new structs. Update config tests.\n\nConstraints: behaviour-preserving (same agents/limits resolve as today); do NOT change internal/loop, the loop bodies (review_loop.go/contract_review.go/clarify_loop.go logic), or add multi-model/best-of-N. Validation: go build ./..., go test ./..., make check, and go test ./internal/app -run TestReadConfig.",
  "queries": [
    "artifact/record",
    "contract_draft/execute/memory",
    "review.panel",
    "pipeline.code_review.by",
    "pipeline.contract_review.by",
    "pipeline.contract_review.loop",
    "pipeline.clarify.loop.max",
    "run.go"
  ],
  "query_source": "task",
  "results": [
    {
      "rank": 1,
      "id": "wiki:tests.md",
      "kind": "wiki",
      "path": "map/wiki/tests.md",
      "title": "Tests",
      "language": "",
      "code_kind": "",
      "score": -2.817736804722567,
      "snippet": "...app/memory_freshness_test.go`\n- `internal/app/memory_prompt_boundary_test.go`\n- `internal/app/memory_selection...",
      "source_query": "contract_draft/execute/memory"
    },
    {
      "rank": 2,
      "id": "file:internal/app/run.go",
      "kind": "file",
      "path": "internal/app/run.go",
      "title": "run.go",
      "language": "Go",
      "code_kind": "source",
      "score": -6.31833408036134,
      "snippet": "path: internal/app/run.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "run.go"
    },
    {
      "rank": 3,
      "id": "wiki:areas/internal.md",
      "kind": "wiki",
      "path": "map/wiki/areas/internal.md",
      "title": "Area: internal",
      "language": "",
      "code_kind": "",
      "score": -2.3474021008144548,
      "snippet": "...execute.go`\n- `internal/app/execute_report.go`\n- `internal/app/execute_report_test.go`\n- `internal/app/execute...",
      "source_query": "contract_draft/execute/memory"
    },
    {
      "rank": 4,
      "id": "code_item:internal/app/app.go:go_func:Run:45",
      "kind": "code_item",
      "path": "internal/app/app.go",
      "title": "Run",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 45,
      "end_line": 52,
      "signature": "func Run(args []string, stdout, stderr io.Writer) int",
      "score": -6.331468495651921,
      "snippet": "...func Run(args []string, stdout, stderr io.Writer) int\npath: internal/app/app.go",
      "source_query": "run.go"
    },
    {
      "rank": 5,
      "id": "repo-map.md",
      "kind": "repo_map",
      "path": "map/repo-map.md",
      "title": "Repository map",
      "language": "",
      "code_kind": "",
      "score": -1.2403950981210956,
      "snippet": "...execute.go`\n- `internal/app/execute_report.go`\n- `internal/app/execute_report_test.go`\n- `internal/app/execute...",
      "source_query": "contract_draft/execute/memory"
    },
    {
      "rank": 6,
      "id": "code_item:internal/agents/acp_transport.go:go_method:Run:25",
      "kind": "code_item",
      "path": "internal/agents/acp_transport.go",
      "title": "Run",
      "language": "go",
      "code_kind": "go_method",
      "start_line": 25,
      "end_line": 142,
      "signature": "func (ACPTransport) Run(request RunRequest) (RunResult, error)",
      "score": -6.278448040729596,
      "snippet": "...func (ACPTransport) Run(request RunRequest) (RunResult, error)\npath: internal/agents/acp_transport.go",
      "source_query": "run.go"
    },
    {
      "rank": 7,
      "id": "code_item:internal/loop/loop.go:go_func:Run:68",
      "kind": "code_item",
      "path": "internal/loop/loop.go",
      "title": "Run",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 68,
      "end_line": 114,
      "signature": "func Run(ctx context.Context, limits Limits, step Step) (Outcome, error)",
      "score": -6.226308210290116,
      "snippet": "...func Run(ctx context.Context, limits Limits, step Step) (Outcome, error)\npath: internal/loop/loop.go",
      "source_query": "run.go"
    },
    {
      "rank": 8,
      "id": "code_item:internal/app/app.go:go_method:Run:60",
      "kind": "code_item",
      "path": "internal/app/app.go",
      "title": "Run",
      "language": "go",
      "code_kind": "go_method",
      "start_line": 60,
      "end_line": 147,
      "signature": "func (a App) Run(args []string, stdout, stderr io.Writer) (code int)",
      "score": -6.074958402668605,
      "snippet": "...Run\nkind: go_method\nlanguage: go\npackage: app\nparent: App\nsignature: func (a App) Run(args...",
      "source_query": "run.go"
    },
    {
      "rank": 9,
      "id": "file:internal/app/run_context_test.go",
      "kind": "file",
      "path": "internal/app/run_context_test.go",
      "title": "run_context_test.go",
      "language": "Go",
      "code_kind": "source",
      "score": -6.014231151216428,
      "snippet": "path: internal/app/run_context_test.go\nlanguage: Go\nkind: source\ntop_level: internal",
      "source_query": "run.go"
    },
    {
      "rank": 10,
      "id": "code_item:internal/app/run_context_test.go:go_func:TestExtractRunContextQueries:21",
      "kind": "code_item",
      "path": "internal/app/run_context_test.go",
      "title": "TestExtractRunContextQueries",
      "language": "go",
      "code_kind": "go_func",
      "start_line": 21,
      "end_line": 45,
      "signature": "func TestExtractRunContextQueries(t *testing.T)",
      "score": -4.179173095699973,
      "snippet": "...internal/app/run_context_test.go",
      "source_query": "run.go"
    }
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
