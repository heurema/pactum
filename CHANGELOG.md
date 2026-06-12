# Changelog

All notable changes to Pactum are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/). There is no released version
yet — everything lives under **Unreleased**.

## Unreleased

### Added
- Contract-first task workflow with `pactum task new`.
- Deterministic, wiki-first project map and lexical search.
- Deterministic map wiki under `map/wiki/` (overview, structure, commands,
  entrypoints, config, tests, and per-area pages), generated from file
  inventory and manifests with conservative, evidence-backed language.
- Search now indexes the map wiki (`--kind wiki`) and import-like entries
  (`--kind import`); `--kind code_item` no longer returns import-like entries.
- Deterministic search ranking polish: import-like entries are penalized and
  the entrypoints/commands/config wiki pages are boosted in unfiltered
  searches, with a small exact title/path-match boost.
- Map quality fixture coverage (Go CLI, TS/Vue/Vite, Python, .NET, Java/Maven,
  Gradle, Rust, config-heavy) and wiki overclaiming checks.
- .NET project detection in the map wiki: `.csproj`/`.fsproj`/`.vbproj`/`.sln`/
  `global.json`/`nuget.config` are recognized as a "C# / .NET" ecosystem and
  surfaced in config, with `dotnet build`/`dotnet test` command hints and
  `Program.cs` as a candidate entrypoint.
- Java/Maven and JVM/Gradle detection: `pom.xml`/`build.gradle(.kts)`/`gradlew`
  are recognized as ecosystems and config, with `mvn test`/`mvn package` and
  `gradle`/`./gradlew` command hints (wrapper preferred when present).
- Rust entrypoint conventions: `src/main.rs`/`main.rs`/`src/bin/*.rs` are listed
  as candidate Rust binary entrypoints when a `Cargo.toml` is present.
- Best-effort CommonJS symbol hints for JavaScript/TypeScript: top-level
  `require(...)` bindings index as import-like hints, and `module.exports` /
  `exports.foo` / `module.exports.foo` (including `exports = module.exports = x`
  chains and object-literal exports) index as code-item hints. No require-path
  resolution, dependency graph, or route detection.
- Repo-local, cross-agent Pactum agent skill package
  (`assets/agent-skills/pactum/`): a portable `SKILL.md` plus
  `references/{workflow,install,safety}.md`, loadable by Codex (`.agents/skills`)
  and Claude Code (`.claude/skills`), with a committed `AGENTS.md` pointer and
  `docs/agent-skill.md` / `docs/skill-install.md`. Marketplace/plugin packaging
  is deferred.
- Monorepo entrypoint conventions: `apps/*`/`services/*` `src/main.*` and
  `src/index.*`, `packages/*`/`libs/*` `index.*` (as candidate package/library
  roots), and `crates/*` `src/main.rs`/`lib.rs` are detected as candidate
  entrypoints, with shallow workspace evidence (`package.json` workspaces,
  `pnpm-workspace.yaml`, `turbo.json`/`nx.json`/`lerna.json`, Cargo
  `[workspace]`) surfaced in the wiki. No package graph.
- `pactum prompt build` now prints a `Next:` hint pointing at `pactum execute
  dry-run`.
- Built-in `codex` and `claude` agent execution boundaries.
- Gate, review, reviewer proposal, and memory workflows.
- Local build/install/smoke story (`Makefile`, `scripts/smoke.sh`).
- CLI v0.2 cleanup: current run, JSON error envelopes, `pactum version`.
- CI hardening: `make vuln` runs `govulncheck` (pinned via a go.mod tool
  directive) as a separate blocking CI job, and `make heurema-hygiene`
  deterministically scans tracked and staged-added `.heurema` run records for
  absolute home-directory paths and credential-shaped strings, failing with
  redacted `file:line` findings (also run as a CI step).

### Changed
- Improved run-context search retrieval: instead of running the whole task
  sentence as one (all-tokens-must-match) FTS query — which returned nothing for
  natural-language tasks — the run now extracts targeted queries (paths, code
  identifiers, domain terms) and combines their results, and refreshes this
  context from the approved contract at `prompt build`. The executor context
  shows the query source, targeted queries, and top results. Combined results
  are merged round-robin across the targeted queries (so each query is
  represented rather than the most specific one draining every slot) and
  import-like hits are de-prioritized below definitions/files/wiki.
- Replaced the old top-level `run` command with `task new`.
- Removed the interactive confirmation layer: commands never prompt, `--yes`
  and `gate run --allow-commands` are gone, and every decision verb carries an
  optional `--by <principal>` (default `manual`) recorded in the decision
  artifact.
- Smoothed the pipeline (hard removals, no aliases): `pactum review run` now
  drives the reviewer/fixer rounds (the separate loop spelling is gone, and
  `--no-fix` keeps it a reviewer-only pass that stops on open blocking
  findings as `findings_open`); the review record self-scaffolds once a gate
  report exists (the preparation spelling and the `review_not_prepared` error
  are gone, and `review status`/`show` derive the empty pending state);
  `prompt build` self-heals a stale project map and records the refresh as an
  additive `map_refresh` object; the standalone clarifier-suggestion spelling
  folded into `pactum clarify run --no-auto --max-rounds 1`; and
  `pactum clarify answer` gained the recommended-answer decision verbs
  (`--recommended`, `--all-recommended`) with their own recorded provenance.
- Made the project map wiki-first in place: `repo-map.md` and `llms.txt` now
  route to the wiki before the code surface, and code items are framed as
  best-effort symbol hints (incomplete by design) rather than semantic truth.
- Friendlier map-wiki area roles for non-code directories (configuration,
  documentation, scripts/tooling, source area) instead of "likely JSON code".
- Tighter frontend detection: a repo is only labeled "frontend" with app-level
  evidence (`.vue`/`.svelte`, an app entrypoint, Vite config plus an entrypoint,
  or a framework dependency plus app-like structure) — Vite as a devDependency
  alone no longer qualifies.

### Not yet included
- Release publishing automation.
- Packaged binaries.
- Docker image.
- Web UI.
