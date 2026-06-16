# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260616_131033/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260616_131033/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260616_131033/review/review.json, .heurema/pactum/runs/run_20260616_131033/review/findings.jsonl, .heurema/pactum/runs/run_20260616_131033/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=9 open=9 resolved=0 blocking_open=5
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: No-op fixer revises are reported as applied fixes.
    location: internal/app/contract_review.go:578
  - f_002 severity=high category=correctness blocking=true status=open: Contract-review fixer attempts are launched write-enabled, so edits are not constrained to the version-guarded contract revise path.
    location: internal/app/contract_review.go:599
  - f_003 severity=medium category=scope blocking=true status=open: The contract-review fixer is resolved through the executor role instead of the drafter/reviewer role used by contract drafting.
    location: internal/app/contract_review.go:493
  - f_004 severity=medium category=correctness blocking=true status=open: The empty contract.reviewers JSON path changes the slice-1 no-op response shape instead of preserving existing behavior.
    location: internal/app/contract_review.go:233
  - f_008 severity=high category=quality blocking=true status=open: SECURITY.md still lists pactum contract review as a read-only stage, but the new convergence loop can launch a write-enabled contract fixer.
    location: SECURITY.md:27
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_005 severity=medium category=quality blocking=false status=open: The contract-review fixer has an explicit branch rejecting a revise payload that changes contract.goal, but no test emits a goal-changing fixer revise or asserts that this is skipped and warned.
    location: internal/app/contract_review_test.go:81
  - f_006 severity=medium category=quality blocking=false status=open: The fixer agent failure path is untested; current helper modes always exit 0, so the branch that records runErr as a skipped fix is not covered.
    location: internal/app/contract_review_test.go:97
  - f_007 severity=low category=quality blocking=false status=open: Unused test helper wrapper `contractReviewFixerAttemptPaths` adds no behavior and has no callers.
    location: internal/app/attempt_paths_test.go:25
  - f_009 severity=medium category=quality blocking=false status=open: docs/contract-review-design.md still documents contract review as the slice-1 behavior with no auto-fixer and says the convergence loop is deferred to slice 2, but this change implements that slice.
    location: docs/contract-review-design.md:76

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

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
