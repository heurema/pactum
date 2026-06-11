# Reviewer Context

## Run
- Run id: run_20260610_220413
- Run status: contract_approved

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

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 0
- Fresh: 0
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: make check (exit 0, timed out: false, result: gate/validation/command_001/result.json)
- Change summary:
  - changed files:
    - internal/app/cli.go
    - internal/app/commands.go
  - new files:
    - internal/app/export.go
    - internal/app/export_test.go
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If you are not certain an issue is real after verification, do not flag it.
