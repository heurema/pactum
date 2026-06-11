# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260610_220413/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260610_220413/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260610_220413/review/review.json, .heurema/pactum/runs/run_20260610_220413/review/findings.jsonl, .heurema/pactum/runs/run_20260610_220413/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=8 open=8 resolved=0 blocking_open=3
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_004 severity=high category=correctness blocking=true status=open: Output-inside-run rejection can be bypassed with a symlinked output parent, causing export to write the archive inside the run record.
    location: internal/app/export.go:114
  - f_005 severity=high category=correctness blocking=true status=open: Explicit run IDs can traverse outside the runs directory and export non-run workspace directories.
    location: internal/app/export.go:57
  - f_006 severity=medium category=correctness blocking=true status=open: A run record containing a regular ledger/events.jsonl file cannot be exported because the filtered sidecar path collides with it.
    location: internal/app/export.go:212
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=low category=quality blocking=false status=open: TestExportArchivesRunRecord claims to prove that relative output paths resolve against the invocation working directory, but testApp sets WorkingDir equal to the workspace root, so resolution against the working directory is indistinguishable from resolution against the repo root. Workspace discovery walks up from WorkingDir (resolveStatusRoot), so subdirectory invocation is a real supported scenario; a regression in resolveExportOutput from a.WorkingDir to the discovered root would pass all export tests. Add a test that runs export with WorkingDir set to a subdirectory of the workspace and asserts the archive lands relative to that subdirectory.
    location: internal/app/export_test.go:44
  - f_002 severity=low category=quality blocking=false status=open: The new error path in appendEventsSidecar — failing the export when the run record already contains ledger/events.jsonl (collision with the generated sidecar) — has no test. Every other new error path in export.go (missing run, existing output, missing parent, output inside run dir, symlink, unreadable file, missing/malformed ledger) is tested; this one is not, and it is trivially testable by writing runs/<run_id>/ledger/events.jsonl before exporting.
    location: internal/app/export.go:213
  - f_003 severity=medium category=quality blocking=false status=open: New top-level user-visible CLI command `pactum export [run_id] --output <path> [--json]` has no documentation update. docs/flow.md enumerates every CLI command in the stages table (lines 12-24) and lists the read-only commands that never append ledger events (lines 27-29); export is read-only and missing from both. The README command tour (lines 118-184) also enumerates the CLI surface and lacks export. Only internal/app/ files changed per the gate report.
    location: docs/flow.md:12
  - f_007 severity=low category=quality blocking=false status=open: The new sidecar-collision error path is untested.
    location: internal/app/export.go:213
  - f_008 severity=low category=quality blocking=false status=open: The new top-level `pactum export [run_id] --output <path> [--json]` command is not documented in the durable workflow docs.
    location: docs/flow.md:12

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review resolve`, or any review loop command.

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
