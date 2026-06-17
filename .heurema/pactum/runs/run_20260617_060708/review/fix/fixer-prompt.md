# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260617_060708/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260617_060708/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260617_060708/review/review.json, .heurema/pactum/runs/run_20260617_060708/review/findings.jsonl, .heurema/pactum/runs/run_20260617_060708/review/resolutions.jsonl

## Approved contract
- Goal: Port the code-review loop (internal/app/review_loop.go) onto the existing internal/loop engine, behaviour-preserving. The engine internal/loop.Run(ctx, Limits{Max,Patience,Settle}, Step) (Outcome, error) ALREADY EXISTS and is already used by contract_review.go — reuse it, do NOT modify or recreate it. Replace review_loop.go's hand-rolled 'for round := 1; round <= maxRounds; round++' loop with a loop.Run call whose Step closure runs the existing per-round work unchanged (reviewer fan-out across lenses, finding proposal/dedup/accept, fixer apply, gate run, stop-signal computation). Stop semantics must match today EXACTLY: max rounds -> existing max_rounds terminal, stalemate/patience -> existing terminal, clean_rounds convergence -> existing resolved terminal. Map review_loop's streak fields to the engine's signals exactly as contract_review did: RoundResult.Clean = (blocking findings this round == 0); RoundResult.Progress = the fixer ran AND the working tree changed (mirror contract_review's content-hash progress signal, adapted to the code-review fixer/working-tree). A clean round must leave the stale streak UNCHANGED (the engine already enforces this). Preserve ALL existing review_loop behaviour, the public JSON response shape and field names, ledger events, and existing tests. Constraints: do NOT change internal/loop/*, do NOT change contract_review.go or clarify_loop.go, do NOT change any config or config schema, no multi-model/best-of-N. Validation: go test ./internal/loop, go test ./internal/app -run TestReviewLoop, go test ./internal/app -run TestContractReview, go build ./..., go test ./..., make check.
- In scope:
  - Refactor internal/app/review_loop.go so ReviewRun uses internal/loop.Run with loop.Limits populated from the existing reviewLimits values.
  - Move the existing per-round code-review work into the loop.Run Step closure: reviewer fan-out, proposal collection, deduplication, acceptance, fixer invocation, fix outcome application, gate run, summary updates, and stop-signal computation.
  - Map loop outcomes back to the existing code-review terminal_reason strings and summary fields without changing the public JSON response or summary artifact shape.
  - Measure RoundResult.Progress from whether the code-review fixer ran and changed the working tree fingerprint, not from whether proposals were created, accepted, skipped, or whether the gate status changed.
  - Keep code-review-specific early terminals such as findings_open, reviewer_findings_unparsed, gate_failed, and error observable exactly as they are today.
- Out of scope:
  - Changing internal/loop/* or the loop engine semantics.
  - Changing internal/app/contract_review.go or internal/app/clarify_loop.go.
  - Changing review configuration keys, defaults, command-line flags, or config schema.
  - Changing reviewer panel selection, lens fan-out, stagger behavior, proposal parsing, deduplication, finding IDs, fixer output parsing, or gate execution behavior.
  - Adding multi-model selection, best-of-N behavior, or new agent execution strategies.
  - Changing ledger event names, public JSON field names, artifact paths, or next-command affordances.
- Acceptance criteria:
  - RoundResult.Clean semantics (authoritative, supersedes any contradictory language in the Goal): The Step closure must set RoundResult.Clean = true if and only if proposals.Created == 0 AND warnings == 0 in that round. This is the governing predicate for the clean signal — a round is clean only when the reviewer produced no proposals of any kind and no parse warnings. The per-round terminal conditions are evaluated in the following explicit precedence order, which matches today's implementation where the warnings-only branch is nested inside the proposals.Created == 0 condition (accepted == 0 && duplicates == 0): (1) when proposals.Created > 0 and blockingOpen > 0, the fixer path executes without setting Clean=true; (2) when proposals.Created > 0 and blockingOpen == 0, the Step closure returns errResolved without setting Clean=true; (3) when proposals.Created == 0 and warnings > 0, the Step closure returns errReviewerUnparsed without setting Clean=true; (4) when proposals.Created == 0 and warnings == 0, the Step closure sets Clean=true and returns nil. A round where both proposals.Created > 0 and warnings > 0 co-occur follows path (1) or (2) based solely on blockingOpen — the errReviewerUnparsed path (3) is unreachable when proposals.Created > 0. proposals.Created == 0 is equivalent to accepted == 0 AND duplicates == 0 because every created proposal is counted as either accepted or duplicate. The Goal section's phrase 'blocking findings this round == 0' is informal shorthand describing the convergence condition the loop works toward, not the literal Clean predicate; the authoritative definition in this criterion governs and implementers must follow this criterion rather than the Goal's shorthand.
  - internal/app/review_loop.go contains a call to internal/loop.Run for the code-review loop and no longer drives normal review rounds with the previous hand-written `for round := 1; round <= maxRounds; round++` loop.
  - A clean code-review round — one where the reviewer produced no new proposals of any kind (proposals.Created == 0, which is equivalent to accepted == 0 AND duplicates == 0 because every created proposal is counted as either accepted or duplicate) and no parse-warning proposals (warnings == 0) — increments clean_streak, leaves unchanged_fingerprint_streak unchanged, does not invoke the fixer, and returns RoundResult.Clean=true with a nil Step error. When loop.Run returns Converged (Settle consecutive clean rounds), the calling code emits terminal_reason "clean_round". Rounds where warnings > 0 exit via the errReviewerUnparsed sentinel before any clean_streak update and do not return Clean=true. Rounds where proposals.Created > 0 (whether blocking > 0 or blocking == 0) do not return Clean=true. Note: the Goal section's phrase 'clean_rounds convergence -> existing resolved terminal' is a misstatement; terminal_reason "clean_round" as specified in this criterion is authoritative and governs over the Goal's phrasing.
  - A round where proposals.Created > 0 AND blocking findings == 0 (all accepted proposals are non-blocking) terminates immediately without invoking the fixer. The Step closure returns a sentinel error errResolved; loop.Run aborts after this round. The calling code detects errResolved and emits terminal_reason "resolved" without waiting for additional Settle-count rounds. Accepted non-blocking findings are still recorded and open_blocking_findings remains 0.
  - A round with open blocking findings still invokes the fixer unless --no-fix is set, applies fixer outcomes, runs the gate, updates open_findings and open_blocking_findings, and records fixer_attempt_id, fix outcome counts, gate_status, and gate_report_artifact as before. If gate passes and blocking findings == 0 after applying fixer outcomes, the Step closure returns the sentinel error errResolved; the calling code emits terminal_reason "resolved" immediately. If gate fails (gate process error), the Step closure returns the sentinel error errGateFailed; the calling code emits terminal_reason "gate_failed" and records the gate status/report fields.
  - Stalemate detection still uses unchanged_fingerprint_streak and terminal_reason "stalemate" when consecutive dirty no-progress rounds reach patience; clean rounds do not reset that stale streak.
  - A fixer round that changes the working tree fingerprint resets unchanged_fingerprint_streak to 0 and prevents premature stalemate.
  - Max-round termination still reports terminal_reason "max_rounds" and executes no reviewer or fixer attempts beyond max_rounds.
  - --no-fix still stops after the first round with open blocking findings and creates no fixer attempt. The Step closure signals this condition by returning a sentinel error errFindingsOpen; the calling code maps this sentinel to terminal_reason "findings_open".
  - Reviewer findings that cannot be parsed still trigger the existing warnings. The Step closure signals this condition by returning a sentinel error errReviewerUnparsed; the calling code maps this sentinel to terminal_reason "reviewer_findings_unparsed" with the same warnings as today. Rounds with parse warnings do not increment clean_streak.
  - Gate process failure still records the gate status/report fields already present today. The Step closure signals gate failure by returning a sentinel error errGateFailed; the calling code maps this sentinel to terminal_reason "gate_failed".
  - All five early-exit conditions — findings_open, reviewer_findings_unparsed, gate_failed, and the two resolved paths (pre-fixer and post-fixer) — are implemented by returning a named sentinel error from the Step closure, using loop.Run's error-abort as the single early-exit mechanism. This mechanism is viable without engine modification because internal/loop.Run propagates the Step closure's error verbatim: when Step returns a non-nil error, Run immediately executes `return Outcome{}, err` (internal/loop/loop.go:78-79), performing no wrapping, so named sentinel errors are detectable at the call site via errors.Is or direct pointer equality. The calling code checks the error returned by loop.Run against each sentinel before falling back to the generic error terminal: errFindingsOpen → "findings_open", errReviewerUnparsed → "reviewer_findings_unparsed", errGateFailed → "gate_failed", errResolved → "resolved". Only errors that match none of the sentinels map to terminal_reason "error" with the same error message and warning as today, creating no additional reviewer or fixer attempt for that round.
  - The stdout JSON response and review/loop-summary.json artifact retain the existing schema, field names, field omission behavior, round ordering, reviewer attempt references, timestamps, artifacts.summary path, and next-command behavior. The authoritative field lists are: stdout JSON response (reviewLoopResponse) always-present top-level fields: schema, run_id, started_at, finished_at, max_rounds, stalemate_patience, clean_rounds_required, terminal_reason, rounds, artifacts, next; optional omitempty top-level fields present only when non-empty: reviewer (single-reviewer name), reviewers (multi-reviewer list), agent (fixer agent name). The review/loop-summary.json artifact contains the same fields except next. Per-round entries in the rounds array always-present fields: round, reviewer_attempt_id, proposals_created, proposals_accepted, open_findings, open_blocking_findings, clean_streak, unchanged_fingerprint_streak; omitempty per-round fields present only when non-zero or non-nil: reviewer_attempt_ids, reviewer_attempts, warnings, skipped_lenses, working_tree_fingerprint, fixer_attempt_id, fix_outcomes_resolved, fix_outcomes_rebutted, fix_outcomes_blocked, gate_status, gate_report_artifact. No fields may be added to or removed from either document relative to these lists. Sub-test (k) must verify this preservation explicitly.
  - Ledger events emitted by review run, reviewer attempts, fixer attempts, proposal decisions, finding changes, gate runs, and review_loop_started/review_loop_finished remain present with the existing event names. The authoritative event-name baseline is: review-loop lifecycle: review_loop_started, review_loop_finished; reviewer attempt lifecycle: reviewer_attempt_started, reviewer_attempt_finished; fixer attempt lifecycle: review_fix_attempt_started, review_fix_attempt_finished; gate run lifecycle: gate_run_started, gate_run_finished; proposal pipeline (accept path): review_findings_proposed, review_proposal_accepted, review_finding_added; proposal pipeline (duplicate path): review_proposal_duplicate; finding severity upgrade: review_finding_severity_upgraded; fix outcome application: review_fix_outcomes_applied. Sub-test (l) must verify this preservation explicitly.
  - The working-tree fingerprint used for RoundResult.Progress must exclude review run-records (.heurema/ directory), ledger files, gate reports, summary artifacts, and other files written by the review runner itself, so that runner-side writes do not falsely signal progress and do not prevent stalemate. If the existing code-review fingerprint helper already implements these exclusions, it must be used as-is; if it does not, extending the helper to add the required exclusions is explicitly in scope for this port, and any such extension must not alter the helper's behavior for paths it already covers correctly. Test sub-test (h) must empirically verify these exclusions are in effect regardless of whether they were pre-existing or newly added.
  - TestReviewLoop must include named sub-tests implemented via t.Run calls within the TestReviewLoop test function (not as separately-named top-level test functions), so that each sub-test is addressable via -run TestReviewLoop/<name> and appears as '=== RUN   TestReviewLoop/<name>' in verbose test output. There must be at least fifteen such sub-tests, individually asserting the following behaviors: (a) clean_round terminal fires after Settle consecutive fully-clean rounds (proposals.Created == 0, warnings == 0); (b) resolved terminal fires after exactly one round with proposals.Created > 0 and blocking == 0, without waiting for additional Settle-count rounds, and RoundResult.Clean is false for that round; (c) resolved terminal fires after a fixer round where gate passes and blocking == 0 following fixer outcome application; (d) stalemate terminal fires after patience consecutive dirty no-progress rounds, driven by unchanged_fingerprint_streak reaching the patience limit; (e) findings_open terminal fires on the first blocking-findings round when --no-fix is set; (f) gate_failed terminal fires on gate process failure; (g) reviewer_findings_unparsed terminal fires on a round with unparseable reviewer output and does not increment clean_streak; (h) working-tree fingerprint computation excludes .heurema/ directory contents, ledger files, gate reports, and review summary artifacts; (i) for each named-terminal scenario the stdout JSON response includes the fields terminal_reason, open_blocking_findings, open_findings, and round with values matching the scenario outcome; (j) for any run that completes normally or via a named sentinel, ledger events with names review_loop_started and review_loop_finished are both emitted; (k) the stdout JSON response for a completed run includes exactly the always-present top-level fields enumerated in the schema-preservation criterion (schema, run_id, started_at, finished_at, max_rounds, stalemate_patience, clean_rounds_required, terminal_reason, rounds, artifacts, next) and the review/loop-summary.json artifact includes the same fields except next, with each round entry containing the always-present per-round fields (round, reviewer_attempt_id, proposals_created, proposals_accepted, open_findings, open_blocking_findings, clean_streak, unchanged_fingerprint_streak) plus optional fields only when non-zero or non-nil, and no fields are added to or removed from either document relative to the enumeration in the schema-preservation criterion; (l) for a run that involves at least one reviewer execution, one fixer execution, one gate execution, and one proposal decision (accept path), the ledger must contain events with the exact event-type names enumerated in the ledger-events criterion; at minimum the sub-test must assert the presence of: review_loop_started, review_loop_finished, reviewer_attempt_started, reviewer_attempt_finished, review_fix_attempt_started, review_fix_attempt_finished, gate_run_started, gate_run_finished, review_findings_proposed, review_proposal_accepted, review_finding_added; (m) a round where proposals.Created > 0 and warnings > 0 co-occur routes to path (1) or (2) based solely on blockingOpen, not to path (3) (errReviewerUnparsed) — specifically, when proposals.Created > 0 and blocking > 0 and warnings > 0, the fixer executes and clean_streak is not incremented, confirming that errReviewerUnparsed is unreachable when proposals.Created > 0; (n) max-rounds termination fires after exactly max_rounds reviewer executions and creates no additional reviewer or fixer attempt beyond round max_rounds, verified by asserting that the rounds array has exactly max_rounds entries (len(response.rounds) == max_rounds) and no round summary has round > max_rounds — reviewer attempt events are NOT counted directly, since each round fans out across len(reviewLenses) lenses per panel member, so they number max_rounds × len(reviewLenses) × panelSize, not max_rounds; (o) a fixer round that changes the working tree fingerprint resets unchanged_fingerprint_streak to 0, so that a subsequent no-progress dirty round begins that streak from 0 — assert that stalemate requires patience additional consecutive no-progress rounds after the fingerprint-changing fixer round rather than firing sooner based on pre-fixer stale history.
- Validation commands:
  - go test ./internal/loop
  - go test ./internal/app -run TestReviewLoop
  - go test ./internal/app -run TestContractReview
  - go build ./...
  - go test ./...
  - make check

## Current review findings
- Summary: findings=5 open=5 resolved=0 blocking_open=4
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=validation blocking=true status=open: TestReviewLoop is missing several contract-required sub-tests: fingerprint exclusions for runner artifacts, named-terminal stdout field assertions, exact JSON schema preservation, and the full ledger event baseline.
    location: internal/app/review_loop_test.go:1028
  - f_002 severity=medium category=quality blocking=true status=open: TestReviewLoop does not verify the exact stdout/summary JSON schema required by the contract; the current summary_artifact_matches_stdout_json subtest unmarshals into structs and compares only a few fields, so added or removed JSON keys would still pass.
    location: internal/app/review_loop_test.go:1427
  - f_003 severity=medium category=quality blocking=true status=open: TestReviewLoop has no subtest asserting the required review-loop ledger event-name baseline.
    location: internal/app/review_loop_test.go:1028
  - f_004 severity=medium category=quality blocking=true status=open: TestReviewLoop has no empirical test that review-loop working-tree fingerprints exclude runner-written artifacts such as .heurema/, ledger files, gate reports, and review summaries.
    location: internal/app/review_loop_test.go:1028
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_005 severity=low category=quality blocking=false status=open: reviewLoopTerminalFindingsOpen is now dead code after the loop.Run refactor replaced its only use with errFindingsOpen and a hard-coded terminal string.
    location: internal/app/review_loop.go:27

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
  "schema": "pactum.review_fix_outcomes.v1alpha1",
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
