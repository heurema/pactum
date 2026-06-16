# Contract Draft Proposal

## Status
- Run id: run_20260616_101840
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-16T10:22:18Z

## In scope
- Expose a top-level version token from pactum contract show <run> --json for the editable contract content, using the same normalized contract hash used by contract approve.
- Replace contract revise add-style flags with pactum contract revise <run> --from -|<file> reading a JSON wrapper {base_version, contract:{...}}.
- Implement partial-replace semantics for editable contract fields: present fields replace wholesale, absent fields are untouched, and list fields preserve submitted order.
- Apply stable list deduplication with keep-first behavior and report dropped duplicates in the revise result.
- Require base_version by default and reject missing or stale versions atomically without writing contract, approval, run, ledger, or rendered contract artifacts.
- Return single structured JSON failure objects for revise validation errors, aggregating all issues with field path, machine code, and message.
- Gate content-changing revisions to approved contracts behind --allow-approval-reset and report approval_reset, previous_approval_hash, and attempts_orphaned on success.
- Switch contract accept so draft proposals are applied through the new partial-replace mechanism while preserving existing accept behavior and ledger events.
- Remove --goal and all --add-* contract revise flags from CLI grammar, help output, tests, and in-repo Pactum agent/skill instructions.

## Out of scope
- Do not implement --dry-run for contract revise in this slice.
- Do not implement field-level diff output in this slice.
- Do not implement a --force or unconditional version-bypass path in this slice.
- Do not add whole-document replace, JSON Patch, or editor-driven revise flows.
- Do not keep deprecated compatibility aliases for the removed --goal or --add-* flags.

## Acceptance criteria
- contract show <run> --json returns a parseable JSON object containing top-level version and contract fields; version equals the normalized contract content hash that contract approve records for the same content.
- contract revise <run> --from - and contract revise <run> --from <file> both accept a valid partial JSON document with base_version and contract fields and apply the same behavior.
- For supported editable fields goal, scope.in, scope.out, paths_in_scope, paths_out_of_scope, acceptance_criteria, validation.commands, and assumptions, every present field replaces the stored value wholesale; absent fields remain byte-for-byte equivalent in the resulting contract content.
- Submitting [] for a supported list field clears that list; duplicate submitted list entries are stable-deduped with the first occurrence kept, submitted order preserved, and dropped duplicates reported in the success JSON.
- A missing or stale base_version exits non-zero with one structured JSON error object including ok:false, contract_unchanged:true, and issues[], and leaves all run artifacts unchanged.
- Invalid revise input reports all detected issues in one structured JSON object, including unknown fields with a did-you-mean message and null goal rejected as a non-nullable scalar.
- A successful content-changing revise returns JSON with ok, contract, base_version, new_version, changed:true, and deduped fields, updates rendered contract artifacts consistently, and records the expected contract_revised ledger event.
- Submitting a partial update that normalizes to the existing content exits 0 with changed:false, new_version equal to the current version, no approval reset, and no changed approval hash.
- On an approved contract, a content-changing revise without --allow-approval-reset is rejected atomically; with --allow-approval-reset it succeeds, resets approval to pending, reports previous_approval_hash and attempts_orphaned, and does not delete old attempt artifacts.
- contract revise --help no longer advertises --goal or any --add-* option, and attempts to use those removed flags fail without mutating contract state.
- contract accept still accepts a pending drafter proposal onto an empty skeleton contract and produces the same accepted proposal, revised contract fields, next commands, and ledger events expected by existing accept workflows.

## Validation commands
- go test ./internal/app -run TestContract
- go test ./internal/app
- make check

## Assumptions
- Slice 1 editable fields are limited to goal, scope, paths_in_scope, paths_out_of_scope, acceptance_criteria, validation, and assumptions; generated identity/status/clarification/memory fields remain managed by existing flows.
- The version string should use the same canonical content and string format as approval.contract_sha256 rather than introducing a separate revision counter.
- Deduplication uses exact string equality after existing trimming/sanitization behavior, not fuzzy or semantic matching.
- Orphaned attempts are counted and reported when approval is reset, but their artifacts remain queryable and are not deleted.

