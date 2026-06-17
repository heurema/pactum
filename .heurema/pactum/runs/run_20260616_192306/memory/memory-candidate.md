# Memory Candidate

## Run
- Run id: run_20260616_192306
- Source: deterministic

## Contract
- Goal: Extract a shared bounded-loop engine into a new internal/loop package and port the contract_review loop onto it, behaviour-preserving, plus add the ledger events contract_review currently lacks.

internal/loop provides Run(ctx, Limits, Step) (Outcome, error). Types: Limits{Max,Patience,Settle int}; RoundContext{Round int}; HumanExit{Reason string}; RoundResult{Clean bool, Progress bool, Human *HumanExit, Summary string}; Outcome{Reason string, Rounds int, Last RoundResult}; Step func(ctx, RoundContext)(RoundResult,error). Run owns ONLY the round counter and the stop machine, blind to performers and domain: per round call step; a Clean round grows a clean-streak (reset on non-clean) and Settle>0 && streak>=Settle => Reason 'settled'; a non-Clean non-Progress round grows a stale-streak (reset otherwise) and Patience>0 && streak>=Patience => 'stalemate'; Human!=nil => immediate 'human'; reaching Max => 'max'. Table tests for settled, stalemate, max, human, and error propagation.

Then replace contract_review.go's hand-rolled 'for round := 1; round <= maxRounds; round++' loop with a loop.Run call whose Step closure runs the existing per-round work (reviewer fan-out, finding merge, fixer apply, stop-signal computation) unchanged; stop semantics must match today exactly. Also emit the round/finished ledger events contract_review lacks today, mirroring what review_loop.go emits, adapted to the contract-review schema.

Constraints: do NOT change any config or config schema; do NOT touch review_loop.go or clarify_loop.go; do NOT add multi-model/best-of-N/judge behaviour. Validation: go build ./..., go test ./..., make check must pass.
- In scope:
  - Create internal/loop with the specified Run, Limits, RoundContext, HumanExit, RoundResult, Outcome, and Step API.
  - Add table-driven tests for internal/loop covering settled, stalemate, max, human exit, error propagation, and streak reset behavior.
  - Refactor internal/app/contract_review.go so the configured reviewer/fixer convergence loop delegates round counting and stop decisions to loop.Run.
  - Preserve contract review per-round behavior: reviewer fan-out, skipped lens handling, finding parsing, blocking/advisory distinction, fixer invocation, contract revise application, warnings, and JSON/human output shape.
  - Emit contract review loop ledger events using the existing ledger.Event shape and contract-review-specific event names.
  - Update focused contract review tests to assert preserved stop semantics and the new ledger events.
- Out of scope:
  - Changing the contract goal.
  - Changing config files, config defaults, CLI flags, or config schema.
  - Changing internal/app/review_loop.go or internal/app/clarify_loop.go.
  - Adding multi-model, best-of-N, judge, or new agent-selection behavior.
  - Changing reviewer, fixer, gate, or attempt artifact schemas outside what is required to use the shared loop.
  - Adding a new contract-review loop summary artifact unless already required by existing code paths.
- Acceptance criteria:
  - internal/loop.Run calls Step with 1-based consecutive RoundContext.Round values and stops without domain knowledge of reviewers, fixers, contracts, files, or ledger events; the internal/loop package must import only standard-library packages (including "context") and must not import any sibling package within the same module; a TestDepsIsolation test in the internal/loop package must enforce this constraint by running go list -deps on the package and (a) asserting that no output line contains the module's internal path prefix (e.g. "github.com/heurema/pactum/internal/") except for the internal/loop package path itself—this catches all current and future sibling packages without enumerating them—and (b) cross-referencing the full dependency list against go list std and asserting that every dependency that is not the internal/loop package itself is present in the standard-library set, so that external non-stdlib dependencies are also rejected. internal/loop.Run checks ctx.Err() before invoking Step each round; if the context is already done when a round is about to start, Run returns the context error without calling Step.
  - internal/loop.Run returns Outcome.Reason "settled" after Settle consecutive Clean rounds when Settle > 0, resets the clean streak on a non-clean round, and records the stopping round count and last result.
  - internal/loop.Run returns Outcome.Reason "stalemate" after Patience consecutive rounds where Clean is false and Progress is false when Patience > 0; the stale streak resets when Progress is true; the stale streak is left unchanged (neither incremented nor reset) when Clean is true, regardless of the Progress value; the stale streak increments only when both Clean is false and Progress is false.
  - internal/loop.Run returns Outcome.Reason "human" immediately when Step returns a RoundResult with Human != nil, returns Outcome.Reason "max" after exactly Max rounds when no earlier stop applies, and returns Step errors without calling Step again. When a single Step result simultaneously satisfies multiple stop conditions, precedence is evaluated in this fixed order within the round: (1) Human != nil → "human", checked before streak updates; (2) Settle > 0 and cleanStreak >= Settle after updating streaks → "settled"; (3) Patience > 0 and staleStreak >= Patience after updating streaks → "stalemate"; (4) round == Max → "max". Only the first matching condition determines Outcome.Reason. Because "settled" requires Clean == true and "stalemate" requires Clean == false, conditions (2) and (3) are mutually exclusive by definition; the explicit ordering covers human-vs-settled, human-vs-stalemate, human-vs-max, settled-vs-max, and stalemate-vs-max without ambiguity.
  - contract review still reports the existing public terminal_reason values: loop settled maps to "resolved", loop stalemate maps to "stalemate", loop max maps to "max_rounds", and loop human maps to "human".
  - Contract review clean and stale streak fields in each round summary match existing semantics: blocking_findings == 0 increments clean_streak; blocking findings reset clean_streak; unchanged contract-file content hash after a fixer round increments unchanged_version_streak; changed content hash resets unchanged_version_streak; a clean round (no blocking findings) leaves unchanged_version_streak unmodified. In the Step closure supplied by contract_review, RoundResult.Progress is true if and only if the contract fixer was invoked and the SHA-256 hash of ContractJSON (as computed by the existing storeFileSHA256 function) changed from before to after the fixer call; Progress is false on clean rounds (fixer not invoked) and when the fixer was invoked but the content hash is unchanged; when the fixer returns an error, the Step closure returns that error rather than a RoundResult, loop.Run propagates it, and the loop terminates—fixer errors are not converted to non-progress rounds and do not cause the loop to continue; skipped lenses and partial reviewer failures do not independently determine Progress.
  - Blocking findings still invoke the contract fixer; advisory-only findings do not invoke the fixer and allow convergence according to clean-round limits.
  - Existing contract review behaviours remain covered and passing for no reviewers, single reviewer, multiple reviewers, partial lens failure, clean convergence, max rounds, and stale-version stalemate. For the partial-lens-failure test case, the test additionally asserts: (a) the JSON response contains a non-empty warnings array reflecting the reviewer failure; (b) findings from the failed reviewer do not appear in the aggregated findings for that round; (c) a round with a skipped lens does not affect RoundResult.Clean (determined solely by blocking_findings count) or RoundResult.Progress (determined solely by fixer invocation and content-hash change). For the JSON response shape, existing test assertions on field names and structure must continue to pass without modification—terminal_reason, round_summaries, and per-round subfields (including clean_streak, unchanged_version_streak, and blocking_findings) must remain present and identically named; no fields are added or removed by the refactor. Human-readable output is driven by the same response struct and is unaffected by the loop engine change; if existing tests cover human output they continue to pass unchanged.
  - Contract review loop events (contract_review_loop_started and contract_review_loop_finished) are emitted on every contract_review run, including runs where no reviewers are configured; event emission is unconditional on reviewer count—the phrase 'when configured contract reviewers run' in earlier drafts described the surrounding context (what other events appear nearby), not a precondition for the loop events themselves. events.jsonl contains exactly one contract_review_loop_started event before contract reviewer/fixer attempt events and exactly one contract_review_loop_finished event after the loop terminates, whether by reaching a terminal reason (settled, stalemate, max, or human) or by returning an error; both events carry only the three fields of the existing ledger.Event shape (type, timestamp, run_id) with no additional fields; when loop.Run returns an error, the implementation still emits contract_review_loop_finished (so no run leaves a dangling contract_review_loop_started event) and writes the JSON response to stdout via the same writeJSONResponse call used on the success path with terminal_reason "error"—not in the ledger event—before propagating the error to the caller; TestContractReviewLoopFinishedOnError drives a contract review run that triggers a fixer error, captures stdout, asserts that events.jsonl contains a contract_review_loop_finished event, asserts that the parsed stdout JSON response carries terminal_reason "error", and asserts that no summary artifact file was written to disk as a side effect of the run; adding a separate on-disk summary artifact is not required and must not be added; the run_id is populated in both events; no per-round ledger events are emitted by the contract review loop. (Note: the goal's phrase "round/finished ledger events" refers to these two loop-level events—loop-started and loop-finished—not to one-per-round events.)
- Validation commands:
  - go test ./internal/loop
  - go test -run ^TestDepsIsolation$ ./internal/loop
  - go test ./internal/app -run TestContractReview
  - go test ./internal/app -run TestReviewLoop
  - go build ./...
  - go test ./...
  - make check
  - go test -run ^TestContractReviewLoopFinishedOnError$ ./internal/app
  - git diff --exit-code origin/main -- internal/app/review_loop.go internal/app/clarify_loop.go

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - internal/app/app.go
  - internal/app/contract_review.go
  - internal/app/contract_review_test.go
- New files:
  - internal/loop/loop.go
  - internal/loop/loop_test.go
- Missing files: none

## Clarifications
- None

## Review Decisions
- f_001 [medium] resolved internal/app/contract_review.go:516: Partial reviewer failures are recorded in skipped_lenses but not surfaced in the round warnings array required by the contract.
  Resolution: Added `skipWarnings []string` in `runContractReviewRound` (contract_review.go). For each failed lens, a warning message is appended alongside the skipped_lenses entry. The return combines skip warnings with parse warnings: `append(skipWarnings, parseWarnings...)`. After the fix, the round Warnings field is non-empty whenever any lens is skipped.
- f_002 [medium] resolved internal/app/contract_review.go:430: Fatal contract-review loop errors produce two stdout JSON documents when invoked with --json.
  Resolution: Defined `contractReviewLoopFatalError` in contract_review.go (wraps the cause error with Error/Unwrap). `runContractReviewLoop` now returns `contractReviewLoopFatalError{cause: loopErr}` instead of bare `loopErr` after writing the JSON response. In app.go, added an `errors.As(err, &loopFatal)` check before `writeErrorEnvelope`, so App.Run skips the second envelope. Validated by TestContractReviewLoopFinishedOnError.
- f_003 [medium] resolved internal/app/contract_review.go:519: Partial lens failures are not surfaced in the round warnings array.
  Resolution: Same root as f_001 — the `Warnings: parseWarnings` field in the return of `runContractReviewRound` was changed to `append(skipWarnings, parseWarnings...)`, surfacing partial lens failures in the warnings array.
- f_004 [medium] resolved internal/app/contract_review_test.go:427: TestContractReviewFailedLensSkipped does not assert that the JSON round warnings array is non-empty for a reviewer/lens failure, even though the approved contract requires that partial-lens-failure test assertion.
  Resolution: Added assertion in `TestContractReviewFailedLensSkipped` (contract_review_test.go): `if len(round.Warnings) == 0 { t.Fatalf(...) }` immediately after the existing skipped_lenses assertions. Test passes with the f_001/f_003 fix in place.
- f_005 [low] open docs/contract-review-design.md:29: Empty `contract.reviewers` is documented as skipping the step with unchanged behavior, but contract review now appends loop lifecycle events even when no reviewers are configured.
- Proposal summary: pending=0 accepted=5 rejected=0

## Reusable Project Knowledge
- scope: in scope: Create internal/loop with the specified Run, Limits, RoundContext, HumanExit, RoundResult, Outcome, and Step API.
- scope: in scope: Add table-driven tests for internal/loop covering settled, stalemate, max, human exit, error propagation, and streak reset behavior.
- scope: in scope: Refactor internal/app/contract_review.go so the configured reviewer/fixer convergence loop delegates round counting and stop decisions to loop.Run.
- scope: in scope: Preserve contract review per-round behavior: reviewer fan-out, skipped lens handling, finding parsing, blocking/advisory distinction, fixer invocation, contract revise application, warnings, and JSON/human output shape.
- scope: in scope: Emit contract review loop ledger events using the existing ledger.Event shape and contract-review-specific event names.
- scope: in scope: Update focused contract review tests to assert preserved stop semantics and the new ledger events.
- scope: out of scope: Changing the contract goal.
- scope: out of scope: Changing config files, config defaults, CLI flags, or config schema.
- scope: out of scope: Changing internal/app/review_loop.go or internal/app/clarify_loop.go.
- scope: out of scope: Adding multi-model, best-of-N, judge, or new agent-selection behavior.
- scope: out of scope: Changing reviewer, fixer, gate, or attempt artifact schemas outside what is required to use the shared loop.
- scope: out of scope: Adding a new contract-review loop summary artifact unless already required by existing code paths.
- review_resolution: f_001 resolved: Partial reviewer failures are recorded in skipped_lenses but not surfaced in the round warnings array required by the contract.; resolution: Added `skipWarnings []string` in `runContractReviewRound` (contract_review.go). For each failed lens, a warning message is appended alongside the skipped_lenses entry. The return combines skip warnings with parse warnings: `append(skipWarnings, parseWarnings...)`. After the fix, the round Warnings field is non-empty whenever any lens is skipped.
- review_resolution: f_002 resolved: Fatal contract-review loop errors produce two stdout JSON documents when invoked with --json.; resolution: Defined `contractReviewLoopFatalError` in contract_review.go (wraps the cause error with Error/Unwrap). `runContractReviewLoop` now returns `contractReviewLoopFatalError{cause: loopErr}` instead of bare `loopErr` after writing the JSON response. In app.go, added an `errors.As(err, &loopFatal)` check before `writeErrorEnvelope`, so App.Run skips the second envelope. Validated by TestContractReviewLoopFinishedOnError.
- review_resolution: f_003 resolved: Partial lens failures are not surfaced in the round warnings array.; resolution: Same root as f_001 — the `Warnings: parseWarnings` field in the return of `runContractReviewRound` was changed to `append(skipWarnings, parseWarnings...)`, surfacing partial lens failures in the warnings array.
- review_resolution: f_004 resolved: TestContractReviewFailedLensSkipped does not assert that the JSON round warnings array is non-empty for a reviewer/lens failure, even though the approved contract requires that partial-lens-failure test assertion.; resolution: Added assertion in `TestContractReviewFailedLensSkipped` (contract_review_test.go): `if len(round.Warnings) == 0 { t.Fatalf(...) }` immediately after the existing skipped_lenses assertions. Test passes with the f_001/f_003 fix in place.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- validation: go test ./internal/loop passed
- validation: go test -run ^TestDepsIsolation$ ./internal/loop passed
- validation: go test ./internal/app -run TestContractReview passed
- validation: go test ./internal/app -run TestReviewLoop passed
- validation: go build ./... passed
- validation: go test ./... passed
- validation: make check passed
- validation: go test -run ^TestContractReviewLoopFinishedOnError$ ./internal/app passed
- validation: git diff --exit-code origin/main -- internal/app/review_loop.go internal/app/clarify_loop.go passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
