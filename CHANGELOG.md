# Changelog

All notable changes to Pactum are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/). There is no released version
yet — everything lives under **Unreleased**.

## Unreleased

### Added
- Contract-first task workflow with `pactum task new`.
- Deterministic project map and lexical search.
- Built-in `codex` and `claude` agent execution boundaries.
- Gate, review, reviewer proposal, and memory workflows.
- Local build/install/smoke story (`Makefile`, `scripts/smoke.sh`).
- CLI v0.2 cleanup: current run, JSON error envelopes, `pactum version`.

### Changed
- Replaced the old top-level `run` command with `task new`.
- `execute run` and `review run` require confirmation or `--yes`.

### Not yet included
- Release publishing automation.
- Packaged binaries.
- Docker image.
- Web UI.
