# Contract Draft

## Goal
Fix three valid external review findings. (1) pactum export must preserve its no-overwrite guarantee at rename time: today two concurrent exports targeting the same --output can both pass the early existence check and the final os.Rename silently replaces the file that appeared in the window; finalize through a no-replace path (e.g. os.Link from the staged temp file to the output then remove the temp, treating link EEXIST as the existing 'output path already exists' error; keep behavior correct on Windows where os.Rename already refuses to replace) so a concurrent winner is never clobbered. (2) pactum export must reject run-record relative paths containing backslashes before writing ZIP entry names: on Unix a backslash is a valid filename character and filepath.ToSlash does not rewrite it, which would emit a non-portable entry name that some Windows extractors treat as a separator; fail the export with a clear error naming the offending path. (3) memory accept must not accept a stale candidate: memory propose records a freshness pin of the review state (for example the review document's updated_at or a content hash) inside memory-candidate.json; memory accept verifies the pin against the current review document and fails with a clear error plus error.fix pactum memory propose <run_id> when the review changed after the candidate was generated; the next-affordance selection that advertises memory accept applies the same staleness check and advertises memory propose instead when stale. Tests cover: concurrent-window overwrite refusal (simulate by creating the output file between staging and finalize), backslash rejection, stale-candidate refusal with the fix affordance, and the next selector switching to propose on staleness.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260611_173351
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — For the memory-staleness finding, should "review state" mean only `review/review.json` and its `updated_at`, or the full review state assembled by `buildReviewStateWithProposals` from `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl`, `review/proposals.jsonl`, and `review/proposal-decisions.jsonl`?
  Rationale: `memory propose` currently builds the candidate from the full review state, including findings, resolutions, proposals, and proposal decisions. Some relevant review changes, such as proposal collection or rejection, are stored in JSONL artifacts and may not be represented by `review/review.json` alone, so pinning only `updated_at` could miss stale candidates.
  Answer: Define the freshness pin as a deterministic content hash over the full review state used to build the memory candidate: `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl`, `review/proposals.jsonl`, and `review/proposal-decisions.jsonl`; `memory accept` and next-affordance selection compare that pin to the current full-state hash.
- q_002 [blocking] — How should `pactum memory accept` handle an existing `memory/memory-candidate.json` that was generated before this change and therefore has no freshness pin?
  Rationale: Existing candidates in the current schema have no field that proves which review state they were generated from. Accepting such a candidate would bypass the new guarantee; rejecting it may affect backwards compatibility for already-proposed but unaccepted runs.
  Answer: Treat a missing or unrecognized freshness pin as stale/unverifiable: do not append a memory item, fail with a clear stale-candidate error, and direct the user to regenerate with `pactum memory propose <run_id>`.
- q_003 [blocking] — For export path validation, should "run-record relative paths containing backslashes" reject only literal `\` characters that remain in the ZIP entry name after platform separator conversion, rather than rejecting normal Windows path separators?
  Rationale: On Unix, a backslash can be part of a filename and `filepath.ToSlash` leaves it unchanged, creating a non-portable ZIP entry. On Windows, backslash is the normal path separator and `filepath.ToSlash` should convert it to `/`; rejecting all relative paths containing `\` before conversion would break normal Windows exports.
  Answer: Validate the final archive entry name after `filepath.ToSlash`; reject any entry name that still contains `\`, and report the offending run-relative path. This rejects Unix filenames with literal backslashes while preserving normal Windows separator conversion.
- q_004 [blocking] — If a memory candidate is stale and the current review state also makes `pactum memory propose <run_id>` illegal, for example a new blocking finding reset review approval or a pending proposal was added, should `memory accept` still return `error.fix: pactum memory propose <run_id>` or should it point to review inspection first?
  Rationale: The finding asks for `error.fix pactum memory propose <run_id>` when the review changed. The existing precondition model uses `fix` only when one exact command remedies the failure; when review approval is reset or proposals are pending, `memory propose` itself will fail and existing lifecycle affordances point at `pactum review show <run_id>`.
  Answer: Use a context-sensitive affordance: if the current review state is still approved and proposal-clean, stale `memory accept` fails with `error.fix: pactum memory propose <run_id>`; if review approval or proposal preconditions are no longer satisfied, fail without accepting and expose `next: ["pactum review show <run_id>"]` so the user can repair the review state first.

## In scope
- Finalize `pactum export` archives through a no-replace path so an output file created after staging but before finalization is never overwritten.
- Validate final ZIP entry names after `filepath.ToSlash` and reject any entry name that still contains a literal backslash, reporting the offending run-relative path.
- Add a versioned freshness pin to `memory-candidate.json` generated by `pactum memory propose` using a deterministic content hash over `review/review.json`, `review/findings.jsonl`, `review/resolutions.jsonl`, `review/proposals.jsonl`, and `review/proposal-decisions.jsonl`.
- Make `pactum memory accept` verify the candidate freshness pin before appending memory, updating acceptance, updating project memory, or writing ledger events.
- Update next-affordance selection so stale memory candidates do not advertise `pactum memory accept`.

## Out of scope
- Changing the contract goal or clarification answers.
- Changing the successful export archive layout, deterministic metadata, or filtered-events sidecar behavior except where needed for backslash validation.
- Accepting legacy memory candidates that lack a recognized freshness pin.
- Running real agent execution commands such as `pactum execute run` or `pactum review run`.

## Acceptance criteria
- If an export output path appears after ZIP staging and before finalization, export fails with the existing clear `output path already exists` style error, preserves the existing file contents, and removes the staged temporary archive.
- On Unix-like platforms, export finalization uses a no-replace mechanism such as linking the staged archive to the output path and treating `EEXIST` as an existing-output failure; on Windows, behavior remains no-clobber where `os.Rename` already refuses to replace an existing destination.
- Export rejects a run-record path whose final archive entry name contains `\` after `filepath.ToSlash`, fails before producing the requested archive, names the offending run-relative path in the error, and does not reject normal Windows separators that are converted to `/`.
- `pactum memory propose` writes a freshness pin into `memory-candidate.json`; the pin is stable for unchanged full review state and changes when any of the five pinned review-state artifacts changes.
- `pactum memory accept` treats a missing, unrecognized, or mismatched freshness pin as stale or unverifiable, exits without appending a memory item or mutating acceptance/project-memory/ledger state, and emits a clear stale-candidate error.
- When a stale candidate's current review state is still approved and proposal-clean, `memory accept` exposes `error.fix: pactum memory propose <run_id>`.
- When a stale candidate's current review state is no longer approved or proposal-clean, `memory accept` refuses acceptance and exposes `next: ["pactum review show <run_id>"]` rather than pointing at an illegal memory propose command.
- For a stale `memory_proposed` run, lifecycle next-affordance selection advertises `pactum memory propose <run_id>` when reproposal is legal, or `pactum review show <run_id>` when review preconditions must be repaired; it does not advertise `pactum memory accept <run_id>`.
- Fresh memory candidates generated from the current full review state still accept successfully and preserve existing memory acceptance behavior.

## Validation commands
- go test ./internal/app -run 'Test(Export|Memory|Lifecycle)'
- make check

## Assumptions
- The freshness pin may be introduced as a new versioned field/object in `memory-candidate.json` as long as old or unknown formats are rejected as unverifiable.
- The staged export archive remains a sibling of the requested output path, so a hard-link based no-replace finalize path does not need to support cross-filesystem linking.

## Open questions
- None
