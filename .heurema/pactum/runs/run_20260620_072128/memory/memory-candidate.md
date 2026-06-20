# Memory Candidate

## Run
- Run id: run_20260620_072128
- Source: deterministic

## Contract
- Goal: Give the CONTRACT-REVIEW loop the same operator finding-resolution that CODE-REVIEW already has, so a contract that ends `blockers_open` but is sound on operator judgment can be unblocked by resolving specific findings with a recorded, auditable reason — mirroring code-review's existing machinery, NOT a parallel system and NOT a force/bypass flag.

Background: contract review can terminate `blockers_open` with open blocking findings. Today `contract approve` fail-closes on those and there is NO way to resolve a contract-review finding (the only action is re-running `contract review`, which re-stalemates). Code-review already solved this: `review finding resolve`, a resolutions store, fingerprint dedup, and an approval gate of `BlockingOpen == 0` (current blocking findings minus active resolutions). This slice brings the same to contract-review. The reviewer-prompt calibration ("no nitpicks") is a SEPARATE prompt-only slice and is out of scope here.

In scope:
1. Persist contract-review findings with a stable ID and a fingerprint. The current `contract/reviewer/findings.jsonl` entry carries only category/message/blocking; add an `id` and a `fingerprint` (the normalized blocker key already used for no-progress detection, e.g. normalized lens/category/message) to each persisted finding.
2. Add an append-only `contract/reviewer/resolutions.jsonl`, mirroring code-review's `review/resolutions.jsonl` record shape. A manual resolution record carries: finding id, fingerprint, the contract hash/version at resolution time, reason, by (principal), timestamp, source="manual".
3. Add `pactum contract review finding resolve [run_id] <id> --reason "..." --by <who>`: append a manual resolution for a blocking contract-review finding. `--reason` and `--by` are REQUIRED. Mirror the command family and flag shape of code-review's `review finding resolve`. Emit JSON output (`--json`) consistent with the rest of the CLI (carry `next`; use the error envelope on failure). Append a ledger event as an index; the detailed reason/by live in the resolutions JSONL, not the ledger event payload.
4. Make `contract approve` resolution-aware: a blocking finding is cleared if it is fixed (absent from the current findings) OR has an ACTIVE manual resolution. A resolution is active only if its recorded contract hash/version matches the CURRENT contract hash — any change to the contract invalidates prior resolutions (conservative; do not implement "changed in that area" detection). The gate mirrors code-review's "no open blocking findings remain". Keep the existing fail-closed behavior on absent/unreadable/malformed artifacts. When approving while one or more blockers are operator-resolved (waived), print a LOUD waiver summary: a count and the list of waived findings (id + reason + by).
5. Reuse, do not duplicate: extract or share the resolution-record / dedup / summary helpers that code-review already has (in review.go / review_loop.go) rather than building a parallel mechanism for contract-review. Keep the STORES separate — contract-review findings/resolutions live under contract/reviewer/, NOT under review/ (do not conflate pre-approval contract review with post-execution code review).

Out of scope (do NOT do here): the reviewer-prompt "no nitpicks" calibration (separate prompt-only slice); any `--disposition` enum or multi-value disposition taxonomy; any change to the shared `pactum.reviewer_findings.v1alpha1` capture schema; "changed in that area" partial resolution invalidation; any change to code-review's own commands or gate; a `contract approve --force` flag.

Tests (helper-process/temp; no real agents): resolving a blocking contract-review finding lets `contract approve` succeed and prints the waiver summary; approve still fails-closed while any unresolved blocking finding remains; after a resolution exists, changing the contract (new hash) invalidates it so approve refuses again until re-resolved; `--reason`/`--by` are required; the resolution is written to contract/reviewer/resolutions.jsonl and a ledger event is appended; malformed/missing artifacts still fail closed.

Validation: go test ./internal/app, go test ./..., go build ./..., make check.
- In scope:
  - Persist contract-review findings under contract/reviewer/findings.jsonl with stable non-empty id and fingerprint fields while preserving category, message, and blocking.
  - Derive each contract-review finding fingerprint from the same normalized blocker key used by contract-review no-progress detection.
  - Add append-only contract/reviewer/resolutions.jsonl records for manual contract-review finding resolutions, including finding_id, fingerprint, current contract hash/version, reason, by, timestamp, and source="manual".
  - Add pactum contract review finding resolve [run_id] <id> --reason "..." --by <who> with --json support, standard next affordances, and standard error-envelope behavior.
  - Make contract approve subtract active manual contract-review resolutions from current blocking findings, where active means matching current contract hash/version.
  - Print a loud waiver summary during contract approval when operator-resolved blocking findings are accepted, including count plus each waived finding id, reason, and by.
  - Reuse or extract existing code-review resolution, deduplication, and summary helpers where practical while keeping contract-review artifacts separate under contract/reviewer/.
- Out of scope:
  - Changing the contract goal.
  - Reviewer-prompt calibration such as no-nitpicks behavior.
  - Adding a disposition enum, multi-value disposition taxonomy, or force/bypass approval flag.
  - Changing the shared pactum.reviewer_findings.v1alpha1 capture schema emitted by reviewers.
  - Implementing changed-in-that-area or partial invalidation logic for resolutions.
  - Changing code-review command behavior, code-review approval gates, or code-review artifact locations.
  - Running real contract reviewers, fixers, or other unsandboxed agents in tests.
- Acceptance criteria:
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
  - A blocking finding re-emitted by a subsequent contract review (same fingerprint, possibly different id) remains cleared by its existing active manual resolution — matched by fingerprint, not id — so contract approve still succeeds without re-resolving, provided the contract hash is unchanged.
  - When the same finding fingerprint is resolved more than once, all resolution records are retained in contract/reviewer/resolutions.jsonl; the approval gate treats any active resolution for a fingerprint as sufficient to clear that fingerprint, and the waiver summary deduplicates by fingerprint so each waived fingerprint appears at most once in the output regardless of how many resolution records exist for it.
  - Multiple current blocking findings that share the same fingerprint are collectively cleared by a single active manual resolution for that fingerprint; the waiver summary treats them as one waived entry rather than one entry per matching finding instance.
- Validation commands:
  - go test ./internal/app
  - go test ./...
  - go build ./...
  - make check

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - docs/contract-review-design.md
  - internal/app/cli.go
  - internal/app/cli_grammar_test.go
  - internal/app/commands.go
  - internal/app/contract.go
  - internal/app/contract_review.go
  - internal/app/contract_review_test.go
  - internal/app/resolve.go
  - internal/app/run.go
- New files:
  - internal/app/contract_review_resolve_test.go
- Missing files: none

## Clarifications
- None

## Review Decisions
- f_001 [medium] resolved internal/app/resolve.go:268: The next affordance still emits the old `pactum contract review <run_id>` spelling after the CLI was changed to require `pactum contract review run <run_id>`, so JSON/status guidance can direct operators to an invalid command.
  Resolution: Changed `"pactum contract review " + runID` to `"pactum contract review run " + runID` at internal/app/resolve.go:268 so the next affordance matches the current CLI grammar.
- f_002 [medium] resolved internal/app/contract_review.go:1057: The waiver summary is not deduplicated by fingerprint: duplicate current findings with the same fingerprint are cleared by one resolution but still produce multiple waiver entries and an inflated warning count.
  Resolution: Added `seenWaiver := map[string]bool{}` guard in `checkContractReviewFindingsApprovalGuard` (internal/app/contract_review.go) so that multiple current findings sharing the same fingerprint produce exactly one waiver entry rather than one per matching finding instance.
- f_003 [medium] resolved internal/app/contract_review.go:1015: contract approve does not fail closed when the contract-review resolutions artifact is absent.
  Resolution: False positive. When resolutions.jsonl is absent, `readJSONLines` returns an empty slice with no error, making `activeByFingerprint` empty. Any blocking finding then has no active resolution, so `blockingCount > 0` and `checkContractReviewFindingsApprovalGuard` returns an error — approval is refused. The behavior IS fail-closed for the only case that matters (blocking findings present). When no blocking findings exist, absent resolutions are irrelevant and approval succeeds correctly.
- f_004 [medium] resolved internal/app/contract_review.go:1057: Waiver summaries are not deduplicated by fingerprint.
  Resolution: Same root cause and fix as f_002: the `seenWaiver` fingerprint guard in `checkContractReviewFindingsApprovalGuard` prevents duplicate waiver entries and the inflated warning count.
- f_005 [medium] resolved internal/app/resolve.go:268: The lifecycle next affordance still points to the old contract review command spelling.
  Resolution: Same fix as f_001: the single change at internal/app/resolve.go:268 corrects both affordance spellings (f_001 and f_005 point to the same line).
- f_006 [medium] resolved internal/app/contract_review_resolve_test.go:395: The waiver-summary deduplication test does not actually assert deduplication.
  Resolution: Replaced the weak `strings.Contains(out, "WARNING")` check with `strings.Contains(out, "WARNING: 1 ")` in TestContractReviewApproveWaiverSummaryDedupesByFingerprint (internal/app/contract_review_resolve_test.go:395) so the test actually asserts that two findings sharing a fingerprint collapse to exactly one waiver entry.
- f_007 [medium] resolved internal/app/contract_review.go:1015: The contract-review approval path silently treats a missing contract/reviewer/resolutions.jsonl as an empty resolution set, so missing durable resolution state is not fail-closed.
  Resolution: Same false positive as f_003. Absent resolutions are treated as an empty set; when blocking findings exist the approval gate still refuses because no fingerprint is active. There is no code path where absent resolutions allows a false pass.
- f_008 [medium] resolved docs/contract-review-design.md:76: The contract-review documentation still advertises the old `contract review <run>` command and does not document the new resolution workflow, even though the CLI now exposes `pactum contract review run` plus `pactum contract review finding resolve`.
  Resolution: Updated docs/contract-review-design.md: replaced `contract review <run>` with `pactum contract review run <run>` in the First slice section and added a new 'Operator finding resolution' section documenting `pactum contract review finding resolve`, the resolutions artifact, hash-based invalidation, and the waiver summary printed during approval.
- Proposal summary: pending=0 accepted=8 rejected=0

## Reusable Project Knowledge
- scope: in scope: Persist contract-review findings under contract/reviewer/findings.jsonl with stable non-empty id and fingerprint fields while preserving category, message, and blocking.
- scope: in scope: Derive each contract-review finding fingerprint from the same normalized blocker key used by contract-review no-progress detection.
- scope: in scope: Add append-only contract/reviewer/resolutions.jsonl records for manual contract-review finding resolutions, including finding_id, fingerprint, current contract hash/version, reason, by, timestamp, and source="manual".
- scope: in scope: Add pactum contract review finding resolve [run_id] <id> --reason "..." --by <who> with --json support, standard next affordances, and standard error-envelope behavior.
- scope: in scope: Make contract approve subtract active manual contract-review resolutions from current blocking findings, where active means matching current contract hash/version.
- scope: in scope: Print a loud waiver summary during contract approval when operator-resolved blocking findings are accepted, including count plus each waived finding id, reason, and by.
- scope: in scope: Reuse or extract existing code-review resolution, deduplication, and summary helpers where practical while keeping contract-review artifacts separate under contract/reviewer/.
- scope: out of scope: Changing the contract goal.
- scope: out of scope: Reviewer-prompt calibration such as no-nitpicks behavior.
- scope: out of scope: Adding a disposition enum, multi-value disposition taxonomy, or force/bypass approval flag.
- scope: out of scope: Changing the shared pactum.reviewer_findings.v1alpha1 capture schema emitted by reviewers.
- scope: out of scope: Implementing changed-in-that-area or partial invalidation logic for resolutions.
- scope: out of scope: Changing code-review command behavior, code-review approval gates, or code-review artifact locations.
- scope: out of scope: Running real contract reviewers, fixers, or other unsandboxed agents in tests.
- review_resolution: f_001 resolved: The next affordance still emits the old `pactum contract review <run_id>` spelling after the CLI was changed to require `pactum contract review run <run_id>`, so JSON/status guidance can direct operators to an invalid command.; resolution: Changed `"pactum contract review " + runID` to `"pactum contract review run " + runID` at internal/app/resolve.go:268 so the next affordance matches the current CLI grammar.
- review_resolution: f_002 resolved: The waiver summary is not deduplicated by fingerprint: duplicate current findings with the same fingerprint are cleared by one resolution but still produce multiple waiver entries and an inflated warning count.; resolution: Added `seenWaiver := map[string]bool{}` guard in `checkContractReviewFindingsApprovalGuard` (internal/app/contract_review.go) so that multiple current findings sharing the same fingerprint produce exactly one waiver entry rather than one per matching finding instance.
- review_resolution: f_003 resolved: contract approve does not fail closed when the contract-review resolutions artifact is absent.; resolution: False positive. When resolutions.jsonl is absent, `readJSONLines` returns an empty slice with no error, making `activeByFingerprint` empty. Any blocking finding then has no active resolution, so `blockingCount > 0` and `checkContractReviewFindingsApprovalGuard` returns an error — approval is refused. The behavior IS fail-closed for the only case that matters (blocking findings present). When no blocking findings exist, absent resolutions are irrelevant and approval succeeds correctly.
- review_resolution: f_004 resolved: Waiver summaries are not deduplicated by fingerprint.; resolution: Same root cause and fix as f_002: the `seenWaiver` fingerprint guard in `checkContractReviewFindingsApprovalGuard` prevents duplicate waiver entries and the inflated warning count.
- review_resolution: f_005 resolved: The lifecycle next affordance still points to the old contract review command spelling.; resolution: Same fix as f_001: the single change at internal/app/resolve.go:268 corrects both affordance spellings (f_001 and f_005 point to the same line).
- review_resolution: f_006 resolved: The waiver-summary deduplication test does not actually assert deduplication.; resolution: Replaced the weak `strings.Contains(out, "WARNING")` check with `strings.Contains(out, "WARNING: 1 ")` in TestContractReviewApproveWaiverSummaryDedupesByFingerprint (internal/app/contract_review_resolve_test.go:395) so the test actually asserts that two findings sharing a fingerprint collapse to exactly one waiver entry.
- review_resolution: f_007 resolved: The contract-review approval path silently treats a missing contract/reviewer/resolutions.jsonl as an empty resolution set, so missing durable resolution state is not fail-closed.; resolution: Same false positive as f_003. Absent resolutions are treated as an empty set; when blocking findings exist the approval gate still refuses because no fingerprint is active. There is no code path where absent resolutions allows a false pass.
- review_resolution: f_008 resolved: The contract-review documentation still advertises the old `contract review <run>` command and does not document the new resolution workflow, even though the CLI now exposes `pactum contract review run` plus `pactum contract review finding resolve`.; resolution: Updated docs/contract-review-design.md: replaced `contract review <run>` with `pactum contract review run <run>` in the First slice section and added a new 'Operator finding resolution' section documenting `pactum contract review finding resolve`, the resolutions artifact, hash-based invalidation, and the waiver summary printed during approval.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- review_resolution: proposal p_007 accepted as f_007
- review_resolution: proposal p_008 accepted as f_008
- validation: go test ./internal/app passed
- validation: go test ./... passed
- validation: go build ./... passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
