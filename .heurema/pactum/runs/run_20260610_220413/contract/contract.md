# Contract Draft

## Goal
Add an export command that dumps a run's full record as a single archive

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260610_211052
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — What should 'full record' include for export: only `.heurema/pactum/runs/<run_id>/`, that run directory plus filtered workspace-level ledger events, generated map/context artifacts, raw `.log` transcripts, or accepted project memory outside the run?
  Rationale: The repo has run-local paths in `contractRunPaths`, but lifecycle events are written to `.heurema/pactum/ledger/events.jsonl`. The actual workspace `.gitignore` ignores generated `runs/*/context/` and `*.log`, while structured execute/review/ledger artifacts are versionable. This changes archive contents, size, and privacy.
  Answer: pending
- q_002 [blocking] — Where should the export command live and what CLI shape should it expose: a top-level `pactum export`, a `pactum task export`, or another command group?
  Rationale: The existing CLI has top-level groups for lifecycle stages and no `run` group. The goal says 'Add an export command' but does not name the command path, arguments, or output flag.
  Answer: pending
- q_003 [blocking] — What archive format and internal path layout should 'single archive' mean: `.zip`, `.tar.gz`, or another format, and should paths be rooted at the run id or copied exactly relative to `.heurema/pactum/`?
  Rationale: The repository currently ignores `.zip` and `.tar` in project-map scans but has no archive implementation. Format and path layout affect cross-platform behavior and tests.
  Answer: pending
- q_004 [blocking] — Should exporting mutate Pactum state, such as appending an `export_*` event to the ledger or updating `run.json`, or should it be read-only apart from writing the archive file?
  Rationale: Existing read-only commands do not append ledger events, but export creates an external artifact. Mutating state would make export part of the audited run record; read-only export keeps repeated exports from changing the record being exported.
  Answer: pending
- q_005 [blocking] — For the concrete scenario where the requested output archive already exists, should `pactum export` overwrite it, fail, or require a force flag?
  Rationale: Archive creation is a filesystem write outside normal Pactum state. Overwriting could destroy a prior export, while adding `--force` expands scope.
  Answer: pending
- q_006 [blocking] — For the concrete scenario where the output path is inside `.heurema/pactum/runs/<run_id>/`, should export reject it to avoid including the archive inside itself?
  Rationale: If the archive is created inside the directory being walked, the export can recursively include itself or produce nondeterministic contents.
  Answer: pending
- q_007 [blocking] — For the concrete scenario where a run is still in progress or a file disappears while export is reading it, should export allow a partial snapshot, wait/lock, or fail?
  Rationale: Runs can be active and agent/review attempts may write files over time. The repo has lock directories but no current export semantics.
  Answer: pending
- q_008 — What acceptance checks should define completion for this export feature?
  Rationale: The contract draft has no acceptance criteria or validation commands. The repo standard requires `make check` before reporting changes.
  Answer: pending
- q_009 [blocking] — For the concrete scenario where `.heurema/pactum/runs/<run_id>/` contains a symlink or non-regular filesystem entry such as a FIFO, socket, or device file, should `pactum export` follow it, preserve it, skip it, or fail?
  Rationale: The current questions define archive contents broadly, but the repo does not define archive behavior for symlinks or special files. Following a symlink could leak files outside the run record, while preserving such entries in ZIP is platform-sensitive.
  Answer: Export only regular files and directories. Do not follow symlinks, and fail the export if any symlink or non-regular filesystem entry is encountered under the selected run record.
- q_010 — How should `--output <path>` be interpreted when the user passes a relative path from a subdirectory: relative to the invocation working directory, relative to the repository root, or relative to `.heurema/pactum/`?
  Rationale: The existing command-shape question names `--output <path>` but does not specify path resolution. Pactum has repo-relative path globs for contract scope, while normal file output flags usually resolve relative to the process working directory.
  Answer: Accept absolute output paths. Resolve relative output paths against the process working directory where `pactum export` was invoked, and allow output outside the repository except for the explicit rejection of paths inside the exported run directory.
- q_011 — For the concrete scenario where `--output reports/run.zip` is requested but the `reports/` parent directory does not exist, should export create the parent directories or fail?
  Rationale: Pactum's internal store creates parent directories for workspace artifacts, but this command writes a user-requested external artifact. Auto-creating arbitrary external directories could hide path typos.
  Answer: pending
- q_012 — For the concrete scenario where the same run is exported twice with identical file contents but different filesystem mtimes or permissions, should the archive be byte-for-byte deterministic or preserve source file metadata?
  Rationale: The existing archive-format question recommends a deterministic ZIP with sorted entries, but the repo does not define whether ZIP entry metadata should be normalized. Pactum generally favors deterministic artifacts, and preserving mtimes or host-specific modes would make repeated exports differ even when the run record contents are unchanged.
  Answer: pending
- q_013 — For the concrete scenario where `.heurema/pactum/ledger/events.jsonl` is missing, unreadable, or contains a malformed JSONL line while building the filtered run events sidecar, should export fail or produce the archive without those events?
  Rationale: The current questions cover unreadable selected files, but filtering workspace ledger events requires reading and parsing a file outside the run directory. The event ledger is part of Pactum's audit trail, so silently omitting it would make a 'full record' export incomplete.
  Answer: pending
- q_014 — What exact success payload should `pactum export --json` emit after creating an archive?
  Rationale: The existing acceptance question says `--json` must be supported, but the repo's JSON commands usually expose a stable schema field and predictable command-specific fields. Without this, tests may only prove that some JSON exists rather than the contract users can rely on.
  Answer: Return a `pactum.export.v1` JSON object containing at least `schema`, `run_id`, `output`, `archive_format`, `archive_root`, `entries`, `bytes`, and `filtered_events`. The human output should report the same essentials in text form without printing archive contents.

## In scope
TBD

## Out of scope
TBD

## Acceptance criteria
TBD

## Validation commands
TBD

## Assumptions
TBD

## Open questions
- What should 'full record' include for export: only `.heurema/pactum/runs/<run_id>/`, that run directory plus filtered workspace-level ledger events, generated map/context artifacts, raw `.log` transcripts, or accepted project memory outside the run?
- Where should the export command live and what CLI shape should it expose: a top-level `pactum export`, a `pactum task export`, or another command group?
- What archive format and internal path layout should 'single archive' mean: `.zip`, `.tar.gz`, or another format, and should paths be rooted at the run id or copied exactly relative to `.heurema/pactum/`?
- Should exporting mutate Pactum state, such as appending an `export_*` event to the ledger or updating `run.json`, or should it be read-only apart from writing the archive file?
- For the concrete scenario where the requested output archive already exists, should `pactum export` overwrite it, fail, or require a force flag?
- For the concrete scenario where the output path is inside `.heurema/pactum/runs/<run_id>/`, should export reject it to avoid including the archive inside itself?
- For the concrete scenario where a run is still in progress or a file disappears while export is reading it, should export allow a partial snapshot, wait/lock, or fail?
- What acceptance checks should define completion for this export feature?
- For the concrete scenario where `--output reports/run.zip` is requested but the `reports/` parent directory does not exist, should export create the parent directories or fail?
- For the concrete scenario where the same run is exported twice with identical file contents but different filesystem mtimes or permissions, should the archive be byte-for-byte deterministic or preserve source file metadata?
- For the concrete scenario where `.heurema/pactum/ledger/events.jsonl` is missing, unreadable, or contains a malformed JSONL line while building the filtered run events sidecar, should export fail or produce the archive without those events?
