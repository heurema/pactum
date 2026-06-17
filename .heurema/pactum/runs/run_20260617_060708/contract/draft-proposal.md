# Contract Draft Proposal

## Status
- Run id: run_20260617_060708
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-17T06:09:40Z

## In scope
- Refactor internal/app/review_loop.go so ReviewRun uses internal/loop.Run with loop.Limits populated from the existing reviewLimits values.
- Move the existing per-round code-review work into the loop.Run Step closure: reviewer fan-out, proposal collection, deduplication, acceptance, fixer invocation, fix outcome application, gate run, summary updates, and stop-signal computation.
- Map loop outcomes back to the existing code-review terminal_reason strings and summary fields without changing the public JSON response or summary artifact shape.
- Measure RoundResult.Progress from whether the code-review fixer ran and changed the working tree fingerprint, not from whether proposals were created, accepted, skipped, or whether the gate status changed.
- Keep code-review-specific early terminals such as findings_open, reviewer_findings_unparsed, gate_failed, and error observable exactly as they are today.

## Out of scope
- Changing internal/loop/* or the loop engine semantics.
- Changing internal/app/contract_review.go or internal/app/clarify_loop.go.
- Changing review configuration keys, defaults, command-line flags, or config schema.
- Changing reviewer panel selection, lens fan-out, stagger behavior, proposal parsing, deduplication, finding IDs, fixer output parsing, or gate execution behavior.
- Adding multi-model selection, best-of-N behavior, or new agent execution strategies.
- Changing ledger event names, public JSON field names, artifact paths, or next-command affordances.

## Acceptance criteria
- internal/app/review_loop.go contains a call to internal/loop.Run for the code-review loop and no longer drives normal review rounds with the previous hand-written for round := 1; round <= maxRounds; round++ loop.
- A clean code-review round still increments clean_streak, leaves unchanged_fingerprint_streak unchanged, does not invoke the fixer, and terminates with terminal_reason "clean_round" once clean_rounds_required is reached.
- A round with accepted non-blocking findings still records those findings, leaves open_blocking_findings at 0, does not invoke the fixer, and terminates with terminal_reason "resolved".
- A round with open blocking findings still invokes the fixer unless --no-fix is set, applies fixer outcomes, runs the gate, updates open_findings and open_blocking_findings, and records fixer_attempt_id, fix outcome counts, gate_status, and gate_report_artifact as before.
- Stalemate detection still uses unchanged_fingerprint_streak and terminal_reason "stalemate" when consecutive dirty no-progress rounds reach patience; clean rounds do not reset that stale streak.
- A fixer round that changes the working tree fingerprint resets unchanged_fingerprint_streak to 0 and prevents premature stalemate.
- Max-round termination still reports terminal_reason "max_rounds" and executes no reviewer or fixer attempts beyond max_rounds.
- --no-fix still stops after the first round with open blocking findings, reports terminal_reason "findings_open", and creates no fixer attempt.
- Reviewer findings that cannot be parsed still preserve the existing reviewer_findings_unparsed terminal behavior and warnings.
- Gate process failure still preserves terminal_reason "gate_failed" and records the gate status/report fields already present today.
- The stdout JSON response and review/loop-summary.json artifact retain the existing schema, field names, field omission behavior, round ordering, reviewer attempt references, timestamps, artifacts.summary path, and next-command behavior.
- Ledger events emitted by review run, reviewer attempts, fixer attempts, proposal decisions, finding changes, gate runs, and review_loop_started/review_loop_finished remain present with the existing event names.

## Validation commands
- go test ./internal/loop
- go test ./internal/app -run TestReviewLoop
- go test ./internal/app -run TestContractReview
- go build ./...
- go test ./...
- make check

## Assumptions
- The existing internal/loop.Run implementation and tests define the loop engine semantics and are not to be changed for this port.
- The existing TestReviewLoop, TestReviewRun, and TestContractReview coverage is intended to remain behavior-preserving; tests may be added but should not be weakened or removed to make the refactor pass.
- Working tree fingerprint comparison via the existing code-review helper is the appropriate code-review analogue of contract_review.go's content-hash progress signal.

