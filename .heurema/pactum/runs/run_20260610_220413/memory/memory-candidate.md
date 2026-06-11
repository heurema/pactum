# Memory Candidate

## Run
- Run id: run_20260610_220413
- Source: deterministic

## Contract
- Goal: Add an export command that dumps a run's full record as a single archive
- In scope:
  - Add a top-level read-oriented CLI command `pactum export [run_id] --output <path> [--json]`.
  - Resolve an omitted `run_id` using the same current-run or sole-active-run behavior as existing read-only run commands.
  - Export all regular files currently under `.heurema/pactum/runs/<run_id>/`, including `.log` files when present.
  - Add an export-only filtered copy of `.heurema/pactum/ledger/events.jsonl` containing only events for the exported run.
  - Produce a deterministic ZIP archive with slash-separated entries rooted at `pactum-run-<run_id>/` and no absolute local paths.
  - Normalize ZIP metadata: stable sorted entry order, fixed timestamps, regular file mode `0644`, directory mode `0755`, and no owner/group metadata.
  - Support human output and `--json` success output using schema `pactum.export.v1` with at least `schema`, `run_id`, `output`, `archive_format`, `archive_root`, `entries`, `bytes`, and `filtered_events`.
  - Write the archive through a temporary sibling file and atomically rename it on success when possible.
- Out of scope:
  - Do not export project map, cache, tmp, locks, or global accepted memory unless those files already exist inside the run directory.
  - Do not add overwrite or `--force` behavior.
  - Do not mutate Pactum state during export: no `run.json`, contract, approval, ledger, or memory updates.
  - Do not follow or preserve symlinks, FIFOs, sockets, device files, or other non-regular filesystem entries.
- Acceptance criteria:
  - `pactum export <run_id> --output <path>` creates a ZIP archive containing the expected run files under `pactum-run-<run_id>/`.
  - The archive includes a filtered ledger events sidecar containing only events for the exported run; if there are no events for that run, the sidecar is present and empty.
  - The archive entries are slash-separated, sorted deterministically, rooted at `pactum-run-<run_id>/`, and contain no absolute local paths.
  - Repeated exports of unchanged inputs are byte-for-byte stable even when source mtimes or permissions differ.
  - `pactum export --output <path>` resolves the run using existing current-run or sole-active behavior.
  - `--json` emits a valid `pactum.export.v1` object with at least `schema`, `run_id`, `output`, `archive_format`, `archive_root`, `entries`, `bytes`, and `filtered_events`.
  - Human output reports the export essentials without printing the archive contents.
  - The command fails for a missing run.
  - The command fails when the output path already exists.
  - The command fails when the output parent directory does not exist.
  - Relative output paths are resolved against the invocation working directory, while absolute output paths are accepted.
  - The command rejects output paths inside the exported run directory.
  - The command fails and removes any partial archive if a selected run file cannot be read during archive creation.
  - The command fails and removes any partial archive if `.heurema/pactum/ledger/events.jsonl` is missing, unreadable, or contains malformed JSONL.
  - The command fails if any symlink or non-regular filesystem entry is encountered under the selected run record.
  - Tests prove that exporting leaves Pactum run files and ledger state unchanged.
- Validation commands:
  - make check

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - internal/app/cli.go
  - internal/app/commands.go
- New files:
  - internal/app/export.go
  - internal/app/export_test.go
- Missing files: none

## Clarifications
- q_001: What should 'full record' include for export: only `.heurema/pactum/runs/<run_id>/`, that run directory plus filtered workspace-level ledger events, generated map/context artifacts, raw `.log` transcripts, or accepted project memory outside the run?
  Answer: Define 'full record' as all files currently under `.heurema/pactum/runs/<run_id>/`, including raw `.log` files if present, plus a generated export-only filtered copy of `.heurema/pactum/ledger/events.jsonl` containing only events for that run. Exclude project map, cache, tmp, locks, and global accepted memory unless the file already exists inside the run directory.
- q_002: Where should the export command live and what CLI shape should it expose: a top-level `pactum export`, a `pactum task export`, or another command group?
  Answer: Add a top-level read-oriented command `pactum export [run_id] --output <path> [--json]`. Resolve omitted `run_id` using the same current-run/sole-active behavior as other read-only run commands.
- q_003: What archive format and internal path layout should 'single archive' mean: `.zip`, `.tar.gz`, or another format, and should paths be rooted at the run id or copied exactly relative to `.heurema/pactum/`?
  Answer: Produce a ZIP archive with deterministic, slash-separated relative entries rooted at `pactum-run-<run_id>/`, with stable sorted file order and no absolute local paths introduced by the exporter.
- q_004: Should exporting mutate Pactum state, such as appending an `export_*` event to the ledger or updating `run.json`, or should it be read-only apart from writing the archive file?
  Answer: Treat export as read-only Pactum state: do not update `run.json`, contract files, approval, or ledgers. The only write is the requested archive output path, and JSON/human output reports what was exported.
- q_005: For the concrete scenario where the requested output archive already exists, should `pactum export` overwrite it, fail, or require a force flag?
  Answer: Fail if the output path already exists. Do not add `--force` in this change unless explicitly requested later.
- q_006: For the concrete scenario where the output path is inside `.heurema/pactum/runs/<run_id>/`, should export reject it to avoid including the archive inside itself?
  Answer: Reject output paths inside the exported run directory. Write via a temporary sibling file and atomically rename on success when possible.
- q_007: For the concrete scenario where a run is still in progress or a file disappears while export is reading it, should export allow a partial snapshot, wait/lock, or fail?
  Answer: Allow exporting any existing run status as a best-effort current on-disk snapshot, but fail the command if any file selected for export cannot be read during archive creation; remove the partial archive on failure.
- q_008: What acceptance checks should define completion for this export feature?
  Answer: Acceptance requires tests proving `pactum export [run_id] --output <path>` creates a ZIP containing the expected run files and filtered run events, supports `--json`, fails for missing runs, fails when output exists, rejects output inside the exported run directory, and leaves Pactum run/ledger state unchanged. Validation command: `make check`.
- q_009: For the concrete scenario where `.heurema/pactum/runs/<run_id>/` contains a symlink or non-regular filesystem entry such as a FIFO, socket, or device file, should `pactum export` follow it, preserve it, skip it, or fail?
  Answer: Export only regular files and directories. Do not follow symlinks, and fail the export if any symlink or non-regular filesystem entry is encountered under the selected run record.
- q_010: How should `--output <path>` be interpreted when the user passes a relative path from a subdirectory: relative to the invocation working directory, relative to the repository root, or relative to `.heurema/pactum/`?
  Answer: Accept absolute output paths. Resolve relative output paths against the process working directory where `pactum export` was invoked, and allow output outside the repository except for the explicit rejection of paths inside the exported run directory.
- q_011: For the concrete scenario where `--output reports/run.zip` is requested but the `reports/` parent directory does not exist, should export create the parent directories or fail?
  Answer: Fail if the output path's parent directory does not already exist. The command may create only its temporary archive file in that existing directory before atomically renaming it to the requested output path.
- q_012: For the concrete scenario where the same run is exported twice with identical file contents but different filesystem mtimes or permissions, should the archive be byte-for-byte deterministic or preserve source file metadata?
  Answer: Normalize archive entry metadata instead of preserving host-specific source metadata: use stable slash-separated names, fixed ZIP timestamps, regular file mode 0644, directory mode 0755, and no owner/group or absolute path metadata. Acceptance may check stable entry order and byte-stable output for unchanged inputs.
- q_013: For the concrete scenario where `.heurema/pactum/ledger/events.jsonl` is missing, unreadable, or contains a malformed JSONL line while building the filtered run events sidecar, should export fail or produce the archive without those events?
  Answer: If filtered workspace events are part of the export, fail the export when `events.jsonl` is missing, unreadable, or malformed, and remove any partial archive. If the ledger is readable but has no events for the run, include an empty filtered `events.jsonl` sidecar.
- q_014: What exact success payload should `pactum export --json` emit after creating an archive?
  Answer: Return a `pactum.export.v1` JSON object containing at least `schema`, `run_id`, `output`, `archive_format`, `archive_root`, `entries`, `bytes`, and `filtered_events`. The human output should report the same essentials in text form without printing archive contents.

## Review Decisions
- f_001 [low] open internal/app/export_test.go:44: TestExportArchivesRunRecord claims to prove that relative output paths resolve against the invocation working directory, but testApp sets WorkingDir equal to the workspace root, so resolution against the working directory is indistinguishable from resolution against the repo root. Workspace discovery walks up from WorkingDir (resolveStatusRoot), so subdirectory invocation is a real supported scenario; a regression in resolveExportOutput from a.WorkingDir to the discovered root would pass all export tests. Add a test that runs export with WorkingDir set to a subdirectory of the workspace and asserts the archive lands relative to that subdirectory.
- f_002 [low] open internal/app/export.go:213: The new error path in appendEventsSidecar — failing the export when the run record already contains ledger/events.jsonl (collision with the generated sidecar) — has no test. Every other new error path in export.go (missing run, existing output, missing parent, output inside run dir, symlink, unreadable file, missing/malformed ledger) is tested; this one is not, and it is trivially testable by writing runs/<run_id>/ledger/events.jsonl before exporting.
- f_003 [medium] resolved docs/flow.md:12: New top-level user-visible CLI command `pactum export [run_id] --output <path> [--json]` has no documentation update. docs/flow.md enumerates every CLI command in the stages table (lines 12-24) and lists the read-only commands that never append ledger events (lines 27-29); export is read-only and missing from both. The README command tour (lines 118-184) also enumerates the CLI surface and lacks export. Only internal/app/ files changed per the gate report.
- f_004 [high] resolved internal/app/export.go:114: Output-inside-run rejection can be bypassed with a symlinked output parent, causing export to write the archive inside the run record.
  Resolution: resolveExportOutput (internal/app/export.go) now checks parent existence first, then resolves both the output parent and the run directory with filepath.EvalSymlinks and applies the inside-run rejection to the physical paths, so a symlinked output parent can no longer smuggle the archive into the run record. Regression test TestExportRejectsSymlinkedOutputParentIntoRunDirectory symlinks an output parent at the run dir, asserts the 'output path is inside the exported run directory' error, and verifies the run record was not written to.
- f_005 [high] resolved internal/app/export.go:57: Explicit run IDs can traverse outside the runs directory and export non-run workspace directories.
  Resolution: Export (internal/app/export.go) now validates the resolved run id before use: ids containing path separators or lacking the run_ prefix (runID != filepath.Base(runID) || !looksLikeRunID(runID)) fail with 'invalid run id', so explicit ids can no longer traverse outside paths.RunsDir and export non-run directories. Regression test TestExportRejectsTraversalRunID covers '..', '../..', and 'run_…/../..'.
- f_006 [medium] resolved internal/app/export.go:212: A run record containing a regular ledger/events.jsonl file cannot be exported because the filtered sidecar path collides with it.
  Resolution: The generated sidecar was renamed from ledger/events.jsonl to ledger/events.filtered.jsonl in appendEventsSidecar (internal/app/export.go), so a run record carrying its own ledger/events.jsonl exports verbatim at its real path with the filtered workspace sidecar alongside it; the refuse-to-shadow guard now applies only to the export-reserved filtered name. New test TestExportIncludesRunRecordEventsLedgerAlongsideSidecar proves the formerly-failing scenario succeeds with both files' contents correct; existing sidecar tests updated to the new name.
- f_007 [low] open internal/app/export.go:213: The new sidecar-collision error path is untested.
- f_008 [low] resolved docs/flow.md:12: The new top-level `pactum export [run_id] --output <path> [--json]` command is not documented in the durable workflow docs.
- Proposal summary: pending=0 accepted=8 rejected=0

## Reusable Project Knowledge
- scope: in scope: Add a top-level read-oriented CLI command `pactum export [run_id] --output <path> [--json]`.
- scope: in scope: Resolve an omitted `run_id` using the same current-run or sole-active-run behavior as existing read-only run commands.
- scope: in scope: Export all regular files currently under `.heurema/pactum/runs/<run_id>/`, including `.log` files when present.
- scope: in scope: Add an export-only filtered copy of `.heurema/pactum/ledger/events.jsonl` containing only events for the exported run.
- scope: in scope: Produce a deterministic ZIP archive with slash-separated entries rooted at `pactum-run-<run_id>/` and no absolute local paths.
- scope: in scope: Normalize ZIP metadata: stable sorted entry order, fixed timestamps, regular file mode `0644`, directory mode `0755`, and no owner/group metadata.
- scope: in scope: Support human output and `--json` success output using schema `pactum.export.v1` with at least `schema`, `run_id`, `output`, `archive_format`, `archive_root`, `entries`, `bytes`, and `filtered_events`.
- scope: in scope: Write the archive through a temporary sibling file and atomically rename it on success when possible.
- scope: out of scope: Do not export project map, cache, tmp, locks, or global accepted memory unless those files already exist inside the run directory.
- scope: out of scope: Do not add overwrite or `--force` behavior.
- scope: out of scope: Do not mutate Pactum state during export: no `run.json`, contract, approval, ledger, or memory updates.
- scope: out of scope: Do not follow or preserve symlinks, FIFOs, sockets, device files, or other non-regular filesystem entries.
- clarification: q_001: What should 'full record' include for export: only `.heurema/pactum/runs/<run_id>/`, that run directory plus filtered workspace-level ledger events, generated map/context artifacts, raw `.log` transcripts, or accepted project memory outside the run? Answer: Define 'full record' as all files currently under `.heurema/pactum/runs/<run_id>/`, including raw `.log` files if present, plus a generated export-only filtered copy of `.heurema/pactum/ledger/events.jsonl` containing only events for that run. Exclude project map, cache, tmp, locks, and global accepted memory unless the file already exists inside the run directory.
- clarification: q_002: Where should the export command live and what CLI shape should it expose: a top-level `pactum export`, a `pactum task export`, or another command group? Answer: Add a top-level read-oriented command `pactum export [run_id] --output <path> [--json]`. Resolve omitted `run_id` using the same current-run/sole-active behavior as other read-only run commands.
- clarification: q_003: What archive format and internal path layout should 'single archive' mean: `.zip`, `.tar.gz`, or another format, and should paths be rooted at the run id or copied exactly relative to `.heurema/pactum/`? Answer: Produce a ZIP archive with deterministic, slash-separated relative entries rooted at `pactum-run-<run_id>/`, with stable sorted file order and no absolute local paths introduced by the exporter.
- clarification: q_004: Should exporting mutate Pactum state, such as appending an `export_*` event to the ledger or updating `run.json`, or should it be read-only apart from writing the archive file? Answer: Treat export as read-only Pactum state: do not update `run.json`, contract files, approval, or ledgers. The only write is the requested archive output path, and JSON/human output reports what was exported.
- clarification: q_005: For the concrete scenario where the requested output archive already exists, should `pactum export` overwrite it, fail, or require a force flag? Answer: Fail if the output path already exists. Do not add `--force` in this change unless explicitly requested later.
- clarification: q_006: For the concrete scenario where the output path is inside `.heurema/pactum/runs/<run_id>/`, should export reject it to avoid including the archive inside itself? Answer: Reject output paths inside the exported run directory. Write via a temporary sibling file and atomically rename on success when possible.
- clarification: q_007: For the concrete scenario where a run is still in progress or a file disappears while export is reading it, should export allow a partial snapshot, wait/lock, or fail? Answer: Allow exporting any existing run status as a best-effort current on-disk snapshot, but fail the command if any file selected for export cannot be read during archive creation; remove the partial archive on failure.
- clarification: q_008: What acceptance checks should define completion for this export feature? Answer: Acceptance requires tests proving `pactum export [run_id] --output <path>` creates a ZIP containing the expected run files and filtered run events, supports `--json`, fails for missing runs, fails when output exists, rejects output inside the exported run directory, and leaves Pactum run/ledger state unchanged. Validation command: `make check`.
- clarification: q_009: For the concrete scenario where `.heurema/pactum/runs/<run_id>/` contains a symlink or non-regular filesystem entry such as a FIFO, socket, or device file, should `pactum export` follow it, preserve it, skip it, or fail? Answer: Export only regular files and directories. Do not follow symlinks, and fail the export if any symlink or non-regular filesystem entry is encountered under the selected run record.
- clarification: q_010: How should `--output <path>` be interpreted when the user passes a relative path from a subdirectory: relative to the invocation working directory, relative to the repository root, or relative to `.heurema/pactum/`? Answer: Accept absolute output paths. Resolve relative output paths against the process working directory where `pactum export` was invoked, and allow output outside the repository except for the explicit rejection of paths inside the exported run directory.
- clarification: q_011: For the concrete scenario where `--output reports/run.zip` is requested but the `reports/` parent directory does not exist, should export create the parent directories or fail? Answer: Fail if the output path's parent directory does not already exist. The command may create only its temporary archive file in that existing directory before atomically renaming it to the requested output path.
- clarification: q_012: For the concrete scenario where the same run is exported twice with identical file contents but different filesystem mtimes or permissions, should the archive be byte-for-byte deterministic or preserve source file metadata? Answer: Normalize archive entry metadata instead of preserving host-specific source metadata: use stable slash-separated names, fixed ZIP timestamps, regular file mode 0644, directory mode 0755, and no owner/group or absolute path metadata. Acceptance may check stable entry order and byte-stable output for unchanged inputs.
- clarification: q_013: For the concrete scenario where `.heurema/pactum/ledger/events.jsonl` is missing, unreadable, or contains a malformed JSONL line while building the filtered run events sidecar, should export fail or produce the archive without those events? Answer: If filtered workspace events are part of the export, fail the export when `events.jsonl` is missing, unreadable, or malformed, and remove any partial archive. If the ledger is readable but has no events for the run, include an empty filtered `events.jsonl` sidecar.
- clarification: q_014: What exact success payload should `pactum export --json` emit after creating an archive? Answer: Return a `pactum.export.v1` JSON object containing at least `schema`, `run_id`, `output`, `archive_format`, `archive_root`, `entries`, `bytes`, and `filtered_events`. The human output should report the same essentials in text form without printing archive contents.
- review_resolution: f_003 resolved: New top-level user-visible CLI command `pactum export [run_id] --output <path> [--json]` has no documentation update. docs/flow.md enumerates every CLI command in the stages table (lines 12-24) and lists the read-only commands that never append ledger events (lines 27-29); export is read-only and missing from both. The README command tour (lines 118-184) also enumerates the CLI surface and lacks export. Only internal/app/ files changed per the gate report.
- review_resolution: f_004 resolved: Output-inside-run rejection can be bypassed with a symlinked output parent, causing export to write the archive inside the run record.; resolution: resolveExportOutput (internal/app/export.go) now checks parent existence first, then resolves both the output parent and the run directory with filepath.EvalSymlinks and applies the inside-run rejection to the physical paths, so a symlinked output parent can no longer smuggle the archive into the run record. Regression test TestExportRejectsSymlinkedOutputParentIntoRunDirectory symlinks an output parent at the run dir, asserts the 'output path is inside the exported run directory' error, and verifies the run record was not written to.
- review_resolution: f_005 resolved: Explicit run IDs can traverse outside the runs directory and export non-run workspace directories.; resolution: Export (internal/app/export.go) now validates the resolved run id before use: ids containing path separators or lacking the run_ prefix (runID != filepath.Base(runID) || !looksLikeRunID(runID)) fail with 'invalid run id', so explicit ids can no longer traverse outside paths.RunsDir and export non-run directories. Regression test TestExportRejectsTraversalRunID covers '..', '../..', and 'run_…/../..'.
- review_resolution: f_006 resolved: A run record containing a regular ledger/events.jsonl file cannot be exported because the filtered sidecar path collides with it.; resolution: The generated sidecar was renamed from ledger/events.jsonl to ledger/events.filtered.jsonl in appendEventsSidecar (internal/app/export.go), so a run record carrying its own ledger/events.jsonl exports verbatim at its real path with the filtered workspace sidecar alongside it; the refuse-to-shadow guard now applies only to the export-reserved filtered name. New test TestExportIncludesRunRecordEventsLedgerAlongsideSidecar proves the formerly-failing scenario succeeds with both files' contents correct; existing sidecar tests updated to the new name.
- review_resolution: f_008 resolved: The new top-level `pactum export [run_id] --output <path> [--json]` command is not documented in the durable workflow docs.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- review_resolution: proposal p_007 accepted as f_007
- review_resolution: proposal p_008 accepted as f_008
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
