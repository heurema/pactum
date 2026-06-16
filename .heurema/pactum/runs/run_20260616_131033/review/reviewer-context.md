# Reviewer Context

## Run
- Run id: run_20260616_131033
- Run status: contract_approved

## Contract
- Goal: Implement slice 2 of contract cross-review, per docs/contract-review-design.md: the auto-fixer and convergence loop. Today 'contract review' (slice 1) only emits findings. Add: when the contract reviewers produce accepted findings, a fixer applies them to the contract via the declarative 'contract revise --from -' primitive (partial-replace, version-guarded — read the current contract version, apply the fix, never reset approval since this runs pre-approve), then the panel re-reviews, converging until a clean round or max rounds — mirroring the code review loop. Reuse the convergence machinery in internal/app/review_loop.go (rounds / patience / clean_rounds) and the graceful skip-on-failed-lens behavior; the fixer is the drafter-role agent resolved like elsewhere. When contract.reviewers is empty/absent the whole step stays a no-op (slice-1 behavior). The loop must surface per-round results (findings, accepted fixes, skipped lenses) and a terminal reason, in human-readable and --json form. Add tests for: a finding is fixed via revise and the re-review converges clean; convergence stops at max rounds with findings still open; the version guard is used by the fixer so a concurrent change is not clobbered; empty reviewers stays a no-op. Do not change the code review loop. Follow docs/contract-review-design.md.
- In scope:
  - Implement slice 2 for `pactum contract review`: configured contract reviewers produce accepted contract findings, a fixer applies valid fixes, and the panel re-reviews until convergence or a configured stop.
  - Add contract-review fixer prompt/context/result handling that uses `pactum contract show --json` and applies edits only through `pactum contract revise <run> --from -` with `base_version`.
  - Reuse the existing review loop concepts for max rounds, patience/stalemate, clean rounds, skipped lenses, per-round summaries, and terminal reasons without changing code-review loop behavior.
  - Surface contract review loop results in both human-readable output and `--json`, including per-round findings, accepted fixes, skipped lenses, and terminal reason.
  - Add focused tests for clean convergence after a fixer revise, max-round termination with findings still open, stale version protection, and empty `contract.reviewers` no-op behavior.
- Out of scope:
  - Changing the contract goal.
  - Changing code review loop behavior or code review artifacts except for narrowly shared/extracted reusable helpers.
  - Running real agent subprocesses in tests; tests should use helper processes or fakes.
  - Changing `contract revise --from` partial-replace or version-guard semantics except as needed to consume the existing primitive.
  - Supporting approval-resetting contract review fixes; this slice runs before contract approval.
- Acceptance criteria:
  - With non-empty `contract.reviewers`, `pactum contract review <run> --json` returns a contract-review loop response containing `rounds`, per-round findings/fix data, skipped lenses, and `terminal_reason`.
  - Human-readable `pactum contract review <run>` output shows each round's findings/fixes/skipped lenses and the terminal reason.
  - When a blocking contract finding is emitted, the fixer is invoked, reads the current contract version, calls `contract revise --from -` with `base_version`, and the next reviewer round runs against the revised contract.
  - A successful fixer revise can lead to a subsequent clean round and a clean terminal reason without requiring human approval during the loop.
  - When findings remain through the configured round cap, the loop stops with `terminal_reason` of `max_rounds` and reports the remaining open findings.
  - A stale `base_version` from the fixer path does not overwrite a concurrent contract change; the failure is surfaced and the contract remains unchanged.
  - When `contract.reviewers` is empty or absent, `pactum contract review` remains a no-op: no reviewer or fixer attempts are created and existing slice-1 behavior is preserved.
  - Existing code review loop tests continue to pass unchanged.
  - Fixer-failure path is explicit: a stale base_version (concurrent contract change) or a fixer agent error/timeout/unparseable-revise causes that finding to be skipped for the round, recorded in the round result, and the loop refreshes the version and continues — it does not abort or overwrite the concurrent change.
  - Terminal reasons are named and asserted: 'resolved' (no open blocking findings), 'max_rounds' (rounds exhausted with open blocking findings), and 'stalemate' (patience exhausted with no progress).
  - The fixer is invoked for blocking findings only; advisory (non-blocking) findings are surfaced but do not drive a revise.
  - The fixer never modifies the contract goal field (scope.out); a revise that would change goal is rejected.
  - A test exercises a failed/skipped reviewer lens so the skipped_lenses output element is populated and asserted.
- Validation commands:
  - go test ./internal/app -run TestContractReview
  - go test ./internal/app -run TestReviewLoop
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
  - command_001: go test ./internal/app -run TestContractReview (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./internal/app -run TestReviewLoop (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: make check (exit 0, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - docs/backlog.md
    - internal/app/attempt_paths_test.go
    - internal/app/contract_review.go
    - internal/app/contract_review_test.go
    - internal/app/run.go
  - new files:
    - none
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
