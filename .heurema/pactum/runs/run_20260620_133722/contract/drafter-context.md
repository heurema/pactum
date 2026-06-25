# Contract Drafter Context

## Run
- Run id: run_20260620_133722
- Run status: contract_draft

## Contract goal
Add pactum's own, self-contained npm distribution launcher so users can install the pactum CLI via `npm i -g @heurema/pactum` / `npx @heurema/pactum`, WITHOUT coupling to any external/forked toolchain. This slice is the distribution MECHANISM only (the launcher package + hermetic tests + docs); the GitHub-Actions release wiring that actually publishes binaries + the npm package is a SEPARATE follow-up slice and is OUT OF SCOPE here.

Design (decided by a Claude+Codex+Gemini council): a SINGLE npm package whose `bin` is a thin Node launcher that, on first run, lazily downloads the matching prebuilt pactum binary from the project's existing GitHub Release, verifies it against a checksum manifest BAKED INTO the npm package (so the npm registry — not the GitHub release — is the root of trust), caches it per-version, and execs it. No `optionalDependencies`, no platform sub-packages, no postinstall script (lazy first-run download survives `--ignore-scripts` and is immune to npm's optional-deps lockfile-pruning bug).

In scope — create a new top-level `npm/` package directory in the pactum repo:

1. `npm/package.json`:
   - `name`: `@heurema/pactum`, `type`: `module`, `bin`: `{ "pactum": "bin/pactum.mjs" }`.
   - `version`: a placeholder (e.g. `0.0.0`) — the real version is stamped from the release tag by the future release job; do not hardcode a real version.
   - `files`: `["bin", "checksums.json"]`. `engines.node`: a sane floor (e.g. `>=18`).
   - NO runtime dependencies (pure Node stdlib only: `node:os`, `node:fs`, `node:path`, `node:https`, `node:crypto`, `node:child_process`, `node:zlib` if needed). `description`, `license`, `repository`, `homepage` pointing at github.com/heurema/pactum.

2. `npm/bin/pactum.mjs` — the launcher (Node ESM, `#!/usr/bin/env node`):
   - PLATFORM MAP: map `process.platform`/`process.arch` to the release asset name using GO naming for the arch (Node `x64` -> Go `amd64`, Node `arm64` -> `arm64`). Supported targets ONLY: `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`. Asset name pattern: `pactum-<goos>-<goarch>` (a BARE binary, not the .tar.gz — so the launcher never needs a cross-platform archive extractor).
   - PLATFORM GATE (run BEFORE any network or cache work; fail loud, early, nonzero, actionable):
     * `win32` (any arch): print that Windows binaries are not published yet and exit nonzero (do NOT fall through to a cryptic ENOENT). (Windows is a deliberate future target.)
     * Linux on musl/Alpine: detect (e.g. existence of `/etc/alpine-release`, or `process.report.getReport().header.glibcVersion` being absent) and print that pactum currently ships glibc-only Linux binaries — use a glibc image (Ubuntu/Debian) — then exit nonzero.
     * Any other unsupported platform/arch: a clear "unsupported platform: <platform>/<arch>" error, nonzero.
   - CACHE: resolve a per-version cache path. Base dir = `process.env.PACTUM_NPM_CACHE` || (`$XDG_CACHE_HOME`/pactum || `~/.cache/pactum`); cached binary at `<base>/<version>/<assetName>` (read `version` from the package's own package.json so cache is version-scoped). If a verified binary already exists there, use it (NEVER re-download).
   - DOWNLOAD (only when cache miss): fetch the bare binary from a PINNED, versioned URL: `https://github.com/heurema/pactum/releases/download/v<version>/<assetName>`. Follow GitHub's release-asset redirect to its asset CDN (`*.githubusercontent.com`) but REJECT redirects to any other host. Stream to a temp file, then verify sha256 against the expected value from the baked `checksums.json`; on mismatch, delete the temp file and fail loud (do not exec an unverified binary). On success, `chmod 0o755` and ATOMICALLY rename into the cache path.
   - CHECKSUM SOURCE: read expected sha256 from `npm/checksums.json` shipped inside the package (keyed by asset name, e.g. `{ "pactum-darwin-arm64": "<sha256>", ... }`). For THIS slice (no release wiring yet) create a `npm/checksums.json` with the four asset keys and placeholder/empty values plus a clear comment-doc that the release job overwrites it; the launcher must handle a missing/empty checksum by failing loud ("no published checksum for <asset>; this build was not released") rather than skipping verification.
   - EXEC: `spawnSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' })`; exit with the child's status (or a clear error if spawn fails).
   - Errors must be human-readable single-line messages on stderr with a nonzero exit; no stack-trace dumps for expected failure modes (unsupported platform, checksum mismatch, network failure with the URL shown).

3. `npm/bin/pactum.test.mjs` (node:test, hermetic — NO network, NO real downloads):
   - platform/arch -> asset-name mapping incl. the `x64`->`amd64` translation and all four supported targets.
   - the gate rejects `win32` and a simulated musl/Alpine and an unsupported arch, each nonzero with a message (inject the detection via a testable helper / env override rather than mutating the real OS).
   - sha256 verification: a known buffer verifies against its correct digest and is rejected for a wrong digest.
   - cache-path construction is version-scoped and honors `PACTUM_NPM_CACHE`.
   - missing/empty checksum entry -> loud failure (no silent skip).
   Structure the launcher so these are unit-testable (export pure helpers from a `npm/bin/lib.mjs` or similar that both `pactum.mjs` and the test import), keeping `pactum.mjs` as a thin entry.

4. Docs: add a concise `docs/install-npm.md` (and a pointer from the existing install docs): the one-command path `npm i -g @heurema/pactum` (then `pactum ...`) and `npx @heurema/pactum ...`; the supported matrix (macOS arm64/x64, Linux amd64/arm64 glibc); explicitly note Windows and Alpine/musl are NOT yet supported and how it fails; note the binary is cached under `~/.cache/pactum/<version>/` and the `PACTUM_NPM_CACHE` override; note the GitHub-Release tarball remains the manual/alternative channel. English only; no references to the codex-acp fork or any external project.

Out of scope (do NOT do here): the release.yml changes that publish bare-binary assets + generate/​bake the real `checksums.json` + the `publish-npm` job (separate slice; this slice only DEFINES the asset-name + URL + checksums.json format the release job must satisfy); `optionalDependencies`/platform sub-packages (the future "variant A" upgrade); Windows or musl/Alpine binaries; signed provenance/SLSA; Homebrew; changing the existing `.tar.gz` GitHub-Release packaging; any Go source changes (pactum's binary, including its go:embedded skill, is unchanged).

Tests / validation (all must pass in the gate; node is available): `node --check npm/bin/pactum.mjs`; `node --test npm/` (the hermetic launcher tests); a JSON-validity check of `npm/package.json` and `npm/checksums.json` (e.g. `node -e "JSON.parse(require('fs').readFileSync('npm/package.json'))"`); `make check` (the Go suite must remain green — no Go files change). Note for the contract: the launcher's REAL end-to-end download is intentionally NOT gate-tested (it requires a published release); it is verified by a later live smoke-test, and the gate covers the logic hermetically.

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

Generated: 2026-06-20T13:37:22Z

Map run: map_20260620_115144
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

# Pactum Project Map

Generated: 2026-06-20T11:51:44Z

Repository root: `.`

## Summary

- Indexed files: 171
- Ignored files/directories: 5827
- Detected languages: 6
- Code items (best-effort hints): 1941

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

- Go: 135 file(s)
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
- `assets/embed.go`
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
- `internal/app/skill.go`
- `internal/app/skill_test.go`
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
  "query": "Add pactum's own, self-contained npm distribution launcher so users can install the pactum CLI via `npm i -g @heurema/pactum` / `npx @heurema/pactum`, WITHOUT coupling to any external/forked toolchain. This slice is the distribution MECHANISM only (the launcher package + hermetic tests + docs); the GitHub-Actions release wiring that actually publishes binaries + the npm package is a SEPARATE follow-up slice and is OUT OF SCOPE here.\n\nDesign (decided by a Claude+Codex+Gemini council): a SINGLE npm package whose `bin` is a thin Node launcher that, on first run, lazily downloads the matching prebuilt pactum binary from the project's existing GitHub Release, verifies it against a checksum manifest BAKED INTO the npm package (so the npm registry — not the GitHub release — is the root of trust), caches it per-version, and execs it. No `optionalDependencies`, no platform sub-packages, no postinstall script (lazy first-run download survives `--ignore-scripts` and is immune to npm's optional-deps lockfile-pruning bug).\n\nIn scope — create a new top-level `npm/` package directory in the pactum repo:\n\n1. `npm/package.json`:\n   - `name`: `@heurema/pactum`, `type`: `module`, `bin`: `{ \"pactum\": \"bin/pactum.mjs\" }`.\n   - `version`: a placeholder (e.g. `0.0.0`) — the real version is stamped from the release tag by the future release job; do not hardcode a real version.\n   - `files`: `[\"bin\", \"checksums.json\"]`. `engines.node`: a sane floor (e.g. `\u003e=18`).\n   - NO runtime dependencies (pure Node stdlib only: `node:os`, `node:fs`, `node:path`, `node:https`, `node:crypto`, `node:child_process`, `node:zlib` if needed). `description`, `license`, `repository`, `homepage` pointing at github.com/heurema/pactum.\n\n2. `npm/bin/pactum.mjs` — the launcher (Node ESM, `#!/usr/bin/env node`):\n   - PLATFORM MAP: map `process.platform`/`process.arch` to the release asset name using GO naming for the arch (Node `x64` -\u003e Go `amd64`, Node `arm64` -\u003e `arm64`). Supported targets ONLY: `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`. Asset name pattern: `pactum-\u003cgoos\u003e-\u003cgoarch\u003e` (a BARE binary, not the .tar.gz — so the launcher never needs a cross-platform archive extractor).\n   - PLATFORM GATE (run BEFORE any network or cache work; fail loud, early, nonzero, actionable):\n     * `win32` (any arch): print that Windows binaries are not published yet and exit nonzero (do NOT fall through to a cryptic ENOENT). (Windows is a deliberate future target.)\n     * Linux on musl/Alpine: detect (e.g. existence of `/etc/alpine-release`, or `process.report.getReport().header.glibcVersion` being absent) and print that pactum currently ships glibc-only Linux binaries — use a glibc image (Ubuntu/Debian) — then exit nonzero.\n     * Any other unsupported platform/arch: a clear \"unsupported platform: \u003cplatform\u003e/\u003carch\u003e\" error, nonzero.\n   - CACHE: resolve a per-version cache path. Base dir = `process.env.PACTUM_NPM_CACHE` || (`$XDG_CACHE_HOME`/pactum || `~/.cache/pactum`); cached binary at `\u003cbase\u003e/\u003cversion\u003e/\u003cassetName\u003e` (read `version` from the package's own package.json so cache is version-scoped). If a verified binary already exists there, use it (NEVER re-download).\n   - DOWNLOAD (only when cache miss): fetch the bare binary from a PINNED, versioned URL: `https://github.com/heurema/pactum/releases/download/v\u003cversion\u003e/\u003cassetName\u003e`. Follow GitHub's release-asset redirect to its asset CDN (`*.githubusercontent.com`) but REJECT redirects to any other host. Stream to a temp file, then verify sha256 against the expected value from the baked `checksums.json`; on mismatch, delete the temp file and fail loud (do not exec an unverified binary). On success, `chmod 0o755` and ATOMICALLY rename into the cache path.\n   - CHECKSUM SOURCE: read expected sha256 from `npm/checksums.json` shipped inside the package (keyed by asset name, e.g. `{ \"pactum-darwin-arm64\": \"\u003csha256\u003e\", ... }`). For THIS slice (no release wiring yet) create a `npm/checksums.json` with the four asset keys and placeholder/empty values plus a clear comment-doc that the release job overwrites it; the launcher must handle a missing/empty checksum by failing loud (\"no published checksum for \u003casset\u003e; this build was not released\") rather than skipping verification.\n   - EXEC: `spawnSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' })`; exit with the child's status (or a clear error if spawn fails).\n   - Errors must be human-readable single-line messages on stderr with a nonzero exit; no stack-trace dumps for expected failure modes (unsupported platform, checksum mismatch, network failure with the URL shown).\n\n3. `npm/bin/pactum.test.mjs` (node:test, hermetic — NO network, NO real downloads):\n   - platform/arch -\u003e asset-name mapping incl. the `x64`-\u003e`amd64` translation and all four supported targets.\n   - the gate rejects `win32` and a simulated musl/Alpine and an unsupported arch, each nonzero with a message (inject the detection via a testable helper / env override rather than mutating the real OS).\n   - sha256 verification: a known buffer verifies against its correct digest and is rejected for a wrong digest.\n   - cache-path construction is version-scoped and honors `PACTUM_NPM_CACHE`.\n   - missing/empty checksum entry -\u003e loud failure (no silent skip).\n   Structure the launcher so these are unit-testable (export pure helpers from a `npm/bin/lib.mjs` or similar that both `pactum.mjs` and the test import), keeping `pactum.mjs` as a thin entry.\n\n4. Docs: add a concise `docs/install-npm.md` (and a pointer from the existing install docs): the one-command path `npm i -g @heurema/pactum` (then `pactum ...`) and `npx @heurema/pactum ...`; the supported matrix (macOS arm64/x64, Linux amd64/arm64 glibc); explicitly note Windows and Alpine/musl are NOT yet supported and how it fails; note the binary is cached under `~/.cache/pactum/\u003cversion\u003e/` and the `PACTUM_NPM_CACHE` override; note the GitHub-Release tarball remains the manual/alternative channel. English only; no references to the codex-acp fork or any external project.\n\nOut of scope (do NOT do here): the release.yml changes that publish bare-binary assets + generate/​bake the real `checksums.json` + the `publish-npm` job (separate slice; this slice only DEFINES the asset-name + URL + checksums.json format the release job must satisfy); `optionalDependencies`/platform sub-packages (the future \"variant A\" upgrade); Windows or musl/Alpine binaries; signed provenance/SLSA; Homebrew; changing the existing `.tar.gz` GitHub-Release packaging; any Go source changes (pactum's binary, including its go:embedded skill, is unchanged).\n\nTests / validation (all must pass in the gate; node is available): `node --check npm/bin/pactum.mjs`; `node --test npm/` (the hermetic launcher tests); a JSON-validity check of `npm/package.json` and `npm/checksums.json` (e.g. `node -e \"JSON.parse(require('fs').readFileSync('npm/package.json'))\"`); `make check` (the Go suite must remain green — no Go files change). Note for the contract: the launcher's REAL end-to-end download is intentionally NOT gate-tested (it requires a published release); it is verified by a later live smoke-test, and the gate covers the logic hermetically.",
  "queries": [
    "@heurema/pactum",
    "/",
    "external/forked",
    "npm/",
    "npm/package.json",
    "bin/pactum.mjs",
    "e.g",
    "checksums.json"
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
