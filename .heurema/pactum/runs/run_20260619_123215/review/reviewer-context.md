# Reviewer Context

## Run
- Run id: run_20260619_123215
- Run status: contract_approved

## Contract
- Goal: Make the review loop (both contract_review and code_review, which share internal/app/review_loop.go) never silently terminate with an OPEN BLOCKING finding as if the run were approvable, and stop the fixer from burning all rounds when it makes no progress on a blocker.

Observed problem (live): on a large contract the contract-review loop ran the full max_rounds, hit a stalemate/max_rounds terminal, and HANDED BACK a contract with a real blocking finding still open — appearing as a normal stop, so an operator could approve a contract that still has an unresolved blocker. Separately, the fixer kept failing to land the one blocking finding round after round, burning all rounds rewording around it.

In scope:
1. Loud, non-approvable terminal on open blockers: when the loop terminates via stalemate or max_rounds while there is at least one OPEN BLOCKING finding, the terminal_reason / status must be a DISTINCT, clearly non-approvable terminal (e.g. blockers_open) rather than the generic stalemate/max_rounds that reads as a clean stop. The summary/output must make the open blocking count prominent.
2. Approve guard holds: contract approve and review approve must REFUSE while any blocking finding is open (verify the existing guard; if it already refuses, ensure the loop's terminal state and human/JSON output surface the open blocker loudly and point the operator at it via next-commands, instead of looking like a successful convergence). The two-layer point: the loop must not pretend to have resolved a blocker it did not.
3. Fixer-no-progress escalation: if the fixer fails to change the set/status of OPEN BLOCKING findings for K consecutive rounds (K small, e.g. 2), stop early with a distinct escalation terminal rather than running the remaining rounds rewording around the same blocker. Record the escalation reason.
4. Advisory findings must not drive non-convergence: the loop converges once all BLOCKING findings are resolved; advisory (non-blocking) findings are recorded but do not extend the loop or count toward non-convergence. If this is already the behavior, add a test that locks it.

Keep max_rounds default at 10 (the round count is not the lever). Do not change the reviewer panel composition, the reviewer-findings capture/parse (that is the shipped #196 behavior), or add reviewer-attempt timeouts (separate slice). Do not reintroduce any plan-DAG concept (removed).

Tests: a loop run that ends at stalemate/max_rounds with an open blocking finding produces the distinct non-approvable terminal and approve refuses; a fixer that makes no progress on a blocker for K rounds escalates early instead of running all rounds; advisory-only remaining findings converge clean; existing convergence (blockers cleared) still reaches the resolved/clean terminal. Cover both contract_review and the shared review_loop paths with helper-process fixtures; do not invoke real agents.

Validation: go test ./internal/app -run 'Review|Loop|Contract', go test ./..., go build ./..., make check.
- In scope:
  - Update contract_review and code_review loop termination so stalemate or max_rounds with at least one open blocking finding is reported as a distinct non-approvable terminal_reason, using blockers_open.
  - Add fixer no-progress detection for open blocking findings with K=2 consecutive no-change fixer rounds, ending early with terminal_reason fixer_no_progress and recording the reason/streak in loop output.
  - Surface open blocking finding counts in JSON loop responses as an integer field named open_blocking_count; surface the fixer no-progress streak count and reason in JSON loop output as integer field no_progress_streak and string field no_progress_reason; surface the open blocking count in durable summary artifacts where present and in human-readable output as a decimal count immediately preceding the word 'blocking' (e.g., '3 open blocking findings remain') for blockers_open and fixer_no_progress terminals.
  - Persist contract-review findings from the final loop round to a durable run artifact (findings.jsonl, one JSON object per line containing at minimum each finding's category, message, and blocking fields) in the contract-review run directory at loop end, so that contract approve and next-command logic can enforce the approval guard across CLI invocations without relying on in-memory state.
  - Ensure contract approve reads findings.jsonl from the latest contract-review run and refuses while any entry has blocking=true; ensure review approve reads the latest review run's stored output or summary artifact and refuses while any finding has blocking=true; both commands fail-closed: if the relevant artifact is absent, unreadable, or malformed they exit nonzero and refuse approval.
  - Add helper-process tests for both contract_review and code_review/review_loop paths; tests must not invoke real agents.
- Out of scope:
  - Changing the default max_rounds value from 10.
  - Changing reviewer panel composition or lens selection.
  - Changing reviewer finding capture/parsing behavior.
  - Adding reviewer-attempt timeout behavior.
  - Reintroducing plan-DAG concepts.
  - Running real agent execution in tests.
- Acceptance criteria:
  - A contract_review loop that reaches stalemate or max rounds while the final round contains at least one finding with blocking=true returns terminal_reason blockers_open, not stalemate or max_rounds; its JSON loop response includes an integer field open_blocking_count greater than zero equal to the count of such findings, and human output includes that decimal count immediately before the word 'blocking' (for example, '2 open blocking findings remain').
  - A code_review review loop that reaches stalemate or max rounds while the count of findings with blocking=true in the latest round is greater than zero returns terminal_reason blockers_open, not stalemate or max_rounds; its JSON response includes an integer field open_blocking_count greater than zero equal to that count, its summary artifact includes the same open_blocking_count, and human output includes that decimal count immediately before the word 'blocking'.
  - When the fixer leaves the set of canonical keys (category, normalized-message) of findings with blocking=true unchanged for 2 consecutive fixer rounds, the loop stops before consuming remaining max_rounds and returns terminal_reason fixer_no_progress; the JSON loop output includes an integer field no_progress_streak equal to 2 and a string field no_progress_reason describing the stalled canonical key(s).
  - Advisory-only findings do not prevent convergence: contract_review resolves when all findings with blocking=true are gone (findings with blocking=false may remain recorded without blocking convergence), and code_review reaches a clean round when zero findings with blocking=true remain in that round (findings with blocking=false in the round do not count toward the clean-round threshold and do not extend the loop). A test must lock the clean-round-counts-only-blocking-findings behavior, covering the case where advisory findings (blocking=false) are present in the round but no findings with blocking=true remain.
  - Existing successful convergence remains unchanged: loops still return resolved or clean_round when all findings with blocking=true are cleared.
  - review approve exits nonzero and does not mark the review approved while any finding with blocking=true is present in the latest review run's stored output or summary artifact; if the artifact is absent, unreadable, or malformed, review approve exits nonzero and refuses approval (fail-closed).
  - contract approve exits nonzero and does not mark the contract approved when the latest contract-review run's findings.jsonl artifact contains any entry with blocking=true; if findings.jsonl is absent, unreadable, or malformed, contract approve exits nonzero and refuses approval (fail-closed).
  - Next-command output for blockers_open and fixer_no_progress terminals must include at least one read-only inspection command and must not include any command that would approve the contract or review; tests verify: (a) no string in the next_commands list contains the substring 'approve', and (b) at least one string in the next_commands list matches the regular expression /show|list|inspect/.
- Validation commands:
  - go test ./internal/app
  - go test ./...
  - go build ./...
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 5
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./... (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: go build ./... (exit 0, timed out: false, result: gate/validation/command_003/result.json)
  - command_004: make check (exit 0, timed out: false, result: gate/validation/command_004/result.json)
- Change summary:
  - changed files:
    - internal/app/contract.go
    - internal/app/contract_review.go
    - internal/app/contract_review_test.go
    - internal/app/review.go
    - internal/app/review_loop.go
    - internal/app/review_loop_test.go
    - internal/app/review_test.go
    - internal/app/run.go
  - new files:
    - none
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=13 open=2 resolved=11 blocking_open=0
- Existing findings:
  - f_001 severity=high category=correctness blocking=true status=resolved: The code-review loop can still terminate as clean_round while a durable open blocking finding remains. In the no-proposals/no-warnings path, the loop returns Clean=true without checking reviewSummaryAfterAccept.BlockingOpen, and the settled terminal is mapped directly to clean_round. A concrete fixture path already exists: round 1 records a blocking finding, the fixer does not resolve it, round 2 emits no proposals, and the test still expects clean_round with the finding left open.
  - f_002 severity=high category=correctness blocking=true status=resolved: contract approve only enforces the contract-review findings guard when the current config still has contract reviewers configured. If a run already has contract/reviewer/findings.jsonl with blocking=true but pipeline.contract_review.by is later empty, this branch skips checkContractReviewFindingsApprovalGuard entirely and approval can proceed despite the stored blocker.
  - f_003 severity=high category=correctness blocking=true status=resolved: review approve still creates missing review artifacts and can approve instead of failing closed when the findings artifact is absent.
  - f_004 severity=high category=correctness blocking=true status=resolved: contract approve can bypass the durable contract-review blocker guard when reviewers are not currently configured.
  - f_005 severity=medium category=correctness blocking=true status=resolved: contract-review findings lose the reviewer category and use the lens as category for persistence and no-progress keys.
  - f_006 severity=medium category=scope blocking=true status=resolved: blockers_open and fixer_no_progress human output does not put the count immediately before the word blocking.
  - f_007 severity=medium category=quality blocking=true status=resolved: The code-review fixer_no_progress next-command test computes hasInspect but discards it, so the test would pass even if resp.Next had no show/list/inspect command. The adjacent blockers_open test also only rejects approve and never positively asserts an inspection command.
  - f_008 severity=medium category=quality blocking=true status=resolved: Contract approve fail-closed coverage does not test malformed contract-review findings.jsonl, leaving the new JSON parse error path untested.
  - f_009 severity=medium category=quality blocking=true status=resolved: Human-readable open-blocking-count output is not tested for the new blockers_open/fixer_no_progress terminals.
  - f_010 severity=medium category=quality blocking=true status=resolved: The no-progress JSON assertions are too weak to lock the contract: they allow any streak >= 2 and never assert no_progress_reason.
  - f_011 severity=medium category=correctness blocking=true status=resolved: The blockers_open terminal check silently ignores errors from reviewLoopReviewSummary and falls back to the old stalemate/max_rounds terminal names.
  - f_012 severity=medium category=quality blocking=false status=open: The workflow docs for `pactum contract approve` were not updated for the new contract-review approval guard. `ContractApprove` now checks whether contract reviewers are configured and refuses approval if `contract/reviewer/findings.jsonl` is absent, unreadable, malformed, or contains blocking entries, but the contract workflow section still describes approval only as pinning the contract hash.
  - f_013 severity=medium category=quality blocking=false status=open: The review-run terminal reason documentation was not updated for the new non-approvable terminals and JSON fields. The docs still list only the old `stalemate` and `max_rounds` meanings, so operators reading the documented terminal list will not know how to interpret `blockers_open`, `fixer_no_progress`, `open_blocking_count`, `no_progress_streak`, or `no_progress_reason`.
- Existing resolutions:
  - r_001 finding=f_001 source=review_fix note=review_loop.go path (4): wrapped the clean_round return in a `reviewSummaryAfterAccept.BlockingOpen == 0` check. When blocking findings are open, the round falls through to the fixer instead of returning Clean:true.
  - r_002 finding=f_002 source=review_fix note=contract.go ContractApprove: guard now also fires when `isRegularFile(context.RunPaths.ContractReviewFindingsJSONL)` is true, regardless of current reviewer config. Removing reviewers from config after a blocking run can no longer bypass the guard.
  - r_003 finding=f_003 source=review_fix note=review.go ReviewApprove: added explicit gate-report-first check (preserving gate_report_missing code), then a fail-closed check requiring ReviewFindingsJSONL to already exist before calling ensureReviewRecord. ensureReviewRecord can no longer scaffold an empty findings file that zero-blocking approval proceeds through.
  - r_004 finding=f_004 source=review_fix note=Same change as f_002 (co-located in contract.go). The `isRegularFile` check covers the configuration-removal bypass path described by f_004.
  - r_005 finding=f_005 source=review_fix note=contract_review.go: added `Category string` field to contractReviewFinding, populated from input.Category in contractFindingFromInput. writeContractReviewFindingsJSONL and contractBlockingFindingKeySet both fall back to f.Lens when Category is empty.
  - r_006 finding=f_006 source=review_fix note=review_loop.go human output: changed format string from `"  open blocking findings: %d\n"` to `"  %d open blocking findings remain\n"` so tests and human readers see the count inline.
  - r_007 finding=f_007 source=review_fix note=review_loop_test.go TestReviewLoopBlockersOpenSetsOpenBlockingCount: added hasInspect assertion (Next commands must include show/list/inspect) and anti-approve assertion (Next must not include approve).
  - r_008 finding=f_008 source=review_fix note=contract_review_test.go TestContractApproveGuardFailClosed: converted to subtests, added malformed subtest that writes `not-json\n` to findings.jsonl and expects exit-1 with stderr containing "malformed".
  - r_009 finding=f_009 source=review_fix note=review_loop_test.go blockers_open human output test: added `strings.Contains(got, "blocking findings remain")` assertion so the round count and human count line are both verified.
  - r_010 finding=f_010 source=review_fix note=review_loop_test.go fixer_no_progress assertions: tightened to exact NoProgressStreak == 2 (K=2 constant), added NoProgressReason non-empty assertion, fixed `_ = hasInspect` dead-assignment bug, and verified artifact fields.
  - r_011 finding=f_011 source=review_fix note=review_loop.go stalemate/max branch: propagates error from reviewLoopReviewSummary instead of silently falling back. When BlockingOpen > 0 at stalemate or max_rounds, sets TerminalReason to blockers_open with OpenBlockingCount populated.
- Proposal summary: pending=0 accepted=13 rejected=0
- Existing proposals:
  - p_001 severity=high category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_001: The code-review loop can still terminate as clean_round while a durable open blocking finding remains. In the no-proposals/no-warnings path, the loop returns Clean=true without checking reviewSummaryAfterAccept.BlockingOpen, and the settled terminal is mapped directly to clean_round. A concrete fixture path already exists: round 1 records a blocking finding, the fixer does not resolve it, round 2 emits no proposals, and the test still expects clean_round with the finding left open.
    location: internal/app/review_loop.go:310
  - p_002 severity=high category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_001: contract approve only enforces the contract-review findings guard when the current config still has contract reviewers configured. If a run already has contract/reviewer/findings.jsonl with blocking=true but pipeline.contract_review.by is later empty, this branch skips checkContractReviewFindingsApprovalGuard entirely and approval can proceed despite the stored blocker.
    location: internal/app/contract.go:312
  - p_003 severity=high category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: review approve still creates missing review artifacts and can approve instead of failing closed when the findings artifact is absent.
    location: internal/app/review.go:533
  - p_004 severity=high category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: contract approve can bypass the durable contract-review blocker guard when reviewers are not currently configured.
    location: internal/app/contract.go:312
  - p_005 severity=medium category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: contract-review findings lose the reviewer category and use the lens as category for persistence and no-progress keys.
    location: internal/app/contract_review.go:905
  - p_006 severity=medium category=scope blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: blockers_open and fixer_no_progress human output does not put the count immediately before the word blocking.
    location: internal/app/review_loop.go:1319
  - p_007 severity=medium category=quality blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: The code-review fixer_no_progress next-command test computes hasInspect but discards it, so the test would pass even if resp.Next had no show/list/inspect command. The adjacent blockers_open test also only rejects approve and never positively asserts an inspection command.
    location: internal/app/review_loop_test.go:2007
  - p_008 severity=medium category=quality blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: Contract approve fail-closed coverage does not test malformed contract-review findings.jsonl, leaving the new JSON parse error path untested.
    location: internal/app/contract_review_test.go:999
  - p_009 severity=medium category=quality blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: Human-readable open-blocking-count output is not tested for the new blockers_open/fixer_no_progress terminals.
    location: internal/app/review_loop_test.go:277
  - p_010 severity=medium category=quality blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: The no-progress JSON assertions are too weak to lock the contract: they allow any streak >= 2 and never assert no_progress_reason.
    location: internal/app/review_loop_test.go:1994
  - p_011 severity=medium category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_005: The blockers_open terminal check silently ignores errors from reviewLoopReviewSummary and falls back to the old stalemate/max_rounds terminal names.
    location: internal/app/review_loop.go:498
  - p_012 severity=medium category=quality blocking=false status=accepted source=reviewer_attempt attempt=reviewer_attempt_002: The workflow docs for `pactum contract approve` were not updated for the new contract-review approval guard. `ContractApprove` now checks whether contract reviewers are configured and refuses approval if `contract/reviewer/findings.jsonl` is absent, unreadable, malformed, or contains blocking entries, but the contract workflow section still describes approval only as pinning the contract hash.
    location: docs/flow.md:154
  - p_013 severity=medium category=quality blocking=false status=accepted source=reviewer_attempt attempt=reviewer_attempt_002: The review-run terminal reason documentation was not updated for the new non-approvable terminals and JSON fields. The docs still list only the old `stalemate` and `max_rounds` meanings, so operators reading the documented terminal list will not know how to interpret `blockers_open`, `fixer_no_progress`, `open_blocking_count`, `no_progress_streak`, or `no_progress_reason`.
    location: docs/flow.md:277

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
