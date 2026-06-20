# Contract Draft Proposal

## Status
- Run id: run_20260620_072128
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-20T07:23:48Z

## In scope
- Persist contract-review findings under contract/reviewer/findings.jsonl with stable non-empty id and fingerprint fields while preserving category, message, and blocking.
- Derive each contract-review finding fingerprint from the same normalized blocker key used by contract-review no-progress detection.
- Add append-only contract/reviewer/resolutions.jsonl records for manual contract-review finding resolutions, including finding_id, fingerprint, current contract hash/version, reason, by, timestamp, and source="manual".
- Add pactum contract review finding resolve [run_id] <id> --reason "..." --by <who> with --json support, standard next affordances, and standard error-envelope behavior.
- Make contract approve subtract active manual contract-review resolutions from current blocking findings, where active means matching current contract hash/version.
- Print a loud waiver summary during contract approval when operator-resolved blocking findings are accepted, including count plus each waived finding id, reason, and by.
- Reuse or extract existing code-review resolution, deduplication, and summary helpers where practical while keeping contract-review artifacts separate under contract/reviewer/.

## Out of scope
- Changing the contract goal.
- Reviewer-prompt calibration such as no-nitpicks behavior.
- Adding a disposition enum, multi-value disposition taxonomy, or force/bypass approval flag.
- Changing the shared pactum.reviewer_findings.v1alpha1 capture schema emitted by reviewers.
- Implementing changed-in-that-area or partial invalidation logic for resolutions.
- Changing code-review command behavior, code-review approval gates, or code-review artifact locations.
- Running real contract reviewers, fixers, or other unsandboxed agents in tests.

## Acceptance criteria
- A contract-review loop that writes findings produces contract/reviewer/findings.jsonl entries with non-empty id and fingerprint fields in addition to category, message, and blocking.
- Resolving an existing blocking contract-review finding with pactum contract review finding resolve <run_id> <id> --reason "operator accepted" --by alice --json appends exactly one manual resolution record to contract/reviewer/resolutions.jsonl and returns JSON containing the resolution and next commands.
- The resolution command rejects missing --reason, missing --by, unknown finding ids, and non-blocking finding ids without appending a resolution.
- A successful manual resolution appends a ledger event that indexes the resolution action without storing the detailed reason or by value in the ledger payload.
- contract approve succeeds when every current blocking contract-review finding is either absent from current findings or has an active manual resolution for the current contract hash/version.
- contract approve fails when any unresolved blocking contract-review finding remains, including when reviewers have been removed from config after findings were written.
- Changing the contract after a manual resolution invalidates the previous resolution because the current contract hash/version no longer matches, so approval fails until the finding is resolved again.
- contract approve remains fail-closed for absent, unreadable, or malformed contract-review findings or resolutions artifacts.
- Approval output that relies on one or more manual resolutions includes a loud waiver summary with the waived finding count and each waived finding id, reason, and by.
- Helper-process or temp-directory tests cover resolution success, unresolved blocker failure, hash invalidation, required flags, resolution artifact writing, ledger event writing, and malformed or missing artifact failure.

## Validation commands
- go test ./internal/app
- go test ./...
- go build ./...
- make check

## Assumptions
- contract/reviewer/findings.jsonl is the durable source of current contract-review findings for approval gating.
- Historical contract-review findings that lack id or fingerprint do not need migration; they may require rerunning contract review or continue to fail closed.
- The current contract hash/version can be computed at both resolution time and approval time from the existing contract artifact state.
- Existing code-review helper extraction can be done without changing code-review CLI flags, artifact shape, or approval semantics.

