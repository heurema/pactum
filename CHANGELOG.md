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
- Map quality fixture coverage (Go CLI, TS/Vue/Vite, Python, config-heavy) and
  wiki overclaiming checks.
- `pactum prompt build` now prints a `Next:` hint pointing at `pactum execute
  dry-run`.
- Built-in `codex` and `claude` agent execution boundaries.
- Gate, review, reviewer proposal, and memory workflows.
- Local build/install/smoke story (`Makefile`, `scripts/smoke.sh`).
- CLI v0.2 cleanup: current run, JSON error envelopes, `pactum version`.

### Changed
- Replaced the old top-level `run` command with `task new`.
- `execute run` and `review run` require confirmation or `--yes`.
- Made the project map wiki-first in place: `repo-map.md` and `llms.txt` now
  route to the wiki before the code surface, and code items are framed as
  best-effort symbol hints (incomplete by design) rather than semantic truth.

### Not yet included
- Release publishing automation.
- Packaged binaries.
- Docker image.
- Web UI.
