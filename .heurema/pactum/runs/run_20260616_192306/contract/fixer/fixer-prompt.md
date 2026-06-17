# Contract Review Fixer Prompt

You are fixing a software change contract to address blocking review findings.

Current contract version: 542fe28610babee87491bddee8774fffa3caf9adc85a21356c4322de0368c3dd

## Current Contract

**Goal**: Extract a shared bounded-loop engine into a new internal/loop package and port the contract_review loop onto it, behaviour-preserving, plus add the ledger events contract_review currently lacks.

internal/loop provides Run(ctx, Limits, Step) (Outcome, error). Types: Limits{Max,Patience,Settle int}; RoundContext{Round int}; HumanExit{Reason string}; RoundResult{Clean bool, Progress bool, Human *HumanExit, Summary string}; Outcome{Reason string, Rounds int, Last RoundResult}; Step func(ctx, RoundContext)(RoundResult,error). Run owns ONLY the round counter and the stop machine, blind to performers and domain: per round call step; a Clean round grows a clean-streak (reset on non-clean) and Settle>0 && streak>=Settle => Reason 'settled'; a non-Clean non-Progress round grows a stale-streak (reset otherwise) and Patience>0 && streak>=Patience => 'stalemate'; Human!=nil => immediate 'human'; reaching Max => 'max'. Table tests for settled, stalemate, max, human, and error propagation.

Then replace contract_review.go's hand-rolled 'for round := 1; round <= maxRounds; round++' loop with a loop.Run call whose Step closure runs the existing per-round work (reviewer fan-out, finding merge, fixer apply, stop-signal computation) unchanged; stop semantics must match today exactly. Also emit the round/finished ledger events contract_review lacks today, mirroring what review_loop.go emits, adapted to the contract-review schema.

Constraints: do NOT change any config or config schema; do NOT touch review_loop.go or clarify_loop.go; do NOT add multi-model/best-of-N/judge behaviour. Validation: go build ./..., go test ./..., make check must pass.

**Scope in**:
  - Create internal/loop with the specified Run, Limits, RoundContext, HumanExit, RoundResult, Outcome, and Step API.
  - Add table-driven tests for internal/loop covering settled, stalemate, max, human exit, error propagation, and streak reset behavior.
  - Refactor internal/app/contract_review.go so the configured reviewer/fixer convergence loop delegates round counting and stop decisions to loop.Run.
  - Preserve contract review per-round behavior: reviewer fan-out, skipped lens handling, finding parsing, blocking/advisory distinction, fixer invocation, contract revise application, warnings, and JSON/human output shape.
  - Emit contract review loop ledger events using the existing ledger.Event shape and contract-review-specific event names.
  - Update focused contract review tests to assert preserved stop semantics and the new ledger events.

**Scope out**:
  - Changing the contract goal.
  - Changing config files, config defaults, CLI flags, or config schema.
  - Changing internal/app/review_loop.go or internal/app/clarify_loop.go.
  - Adding multi-model, best-of-N, judge, or new agent-selection behavior.
  - Changing reviewer, fixer, gate, or attempt artifact schemas outside what is required to use the shared loop.
  - Adding a new contract-review loop summary artifact unless already required by existing code paths.

**Acceptance criteria**:
  - internal/loop.Run calls Step with 1-based consecutive RoundContext.Round values and stops without domain knowledge of reviewers, fixers, contracts, files, or ledger events; the internal/loop package must import only standard-library packages (including "context") and must not import any sibling package within the same module; a TestDepsIsolation test in the internal/loop package must enforce this constraint by running go list -deps on the package and (a) asserting that no output line contains the module's internal path prefix (e.g. "github.com/heurema/pactum/internal/") except for the internal/loop package path itself—this catches all current and future sibling packages without enumerating them—and (b) cross-referencing the full dependency list against go list std and asserting that every dependency that is not the internal/loop package itself is present in the standard-library set, so that external non-stdlib dependencies are also rejected. internal/loop.Run checks ctx.Err() before invoking Step each round; if the context is already done when a round is about to start, Run returns the context error without calling Step.
  - internal/loop.Run returns Outcome.Reason "settled" after Settle consecutive Clean rounds when Settle > 0, resets the clean streak on a non-clean round, and records the stopping round count and last result.
  - internal/loop.Run returns Outcome.Reason "stalemate" after Patience consecutive rounds where Clean is false and Progress is false when Patience > 0; the stale streak resets when Progress is true; the stale streak is left unchanged (neither incremented nor reset) when Clean is true, regardless of the Progress value; the stale streak increments only when both Clean is false and Progress is false.
  - internal/loop.Run returns Outcome.Reason "human" immediately when Step returns a RoundResult with Human != nil, returns Outcome.Reason "max" after exactly Max rounds when no earlier stop applies, and returns Step errors without calling Step again.
  - contract review still reports the existing public terminal_reason values: loop settled maps to "resolved", loop stalemate maps to "stalemate", loop max maps to "max_rounds", and loop human maps to "human".
  - Contract review clean and stale streak fields in each round summary match existing semantics: blocking_findings == 0 increments clean_streak; blocking findings reset clean_streak; unchanged contract-file content hash after a fixer round increments unchanged_version_streak; changed content hash resets unchanged_version_streak; a clean round (no blocking findings) leaves unchanged_version_streak unmodified. In the Step closure supplied by contract_review, RoundResult.Progress is true if and only if the contract fixer was invoked and the SHA-256 hash of ContractJSON (as computed by the existing storeFileSHA256 function) changed from before to after the fixer call; Progress is false on clean rounds (fixer not invoked) and when the fixer was invoked but the content hash is unchanged; when the fixer returns an error, the Step closure returns that error rather than a RoundResult, loop.Run propagates it, and the loop terminates—fixer errors are not converted to non-progress rounds and do not cause the loop to continue; skipped lenses and partial reviewer failures do not independently determine Progress.
  - Blocking findings still invoke the contract fixer; advisory-only findings do not invoke the fixer and allow convergence according to clean-round limits.
  - Existing contract review behaviours remain covered and passing for no reviewers, single reviewer, multiple reviewers, partial lens failure, clean convergence, max rounds, and stale-version stalemate. For the partial-lens-failure test case, the test additionally asserts: (a) the JSON response contains a non-empty warnings array reflecting the reviewer failure; (b) findings from the failed reviewer do not appear in the aggregated findings for that round; (c) a round with a skipped lens does not affect RoundResult.Clean (determined solely by blocking_findings count) or RoundResult.Progress (determined solely by fixer invocation and content-hash change). For the JSON response shape, existing test assertions on field names and structure must continue to pass without modification—terminal_reason, round_summaries, and per-round subfields (including clean_streak, unchanged_version_streak, and blocking_findings) must remain present and identically named; no fields are added or removed by the refactor. Human-readable output is driven by the same response struct and is unaffected by the loop engine change; if existing tests cover human output they continue to pass unchanged.
  - When configured contract reviewers run, events.jsonl contains exactly one contract_review_loop_started event before contract reviewer/fixer attempt events and exactly one contract_review_loop_finished event after the loop terminates, whether by reaching a terminal reason (settled, stalemate, max, or human) or by returning an error; both events carry only the three fields of the existing ledger.Event shape (type, timestamp, run_id) with no additional fields; when loop.Run returns an error, the implementation still emits contract_review_loop_finished (so no run leaves a dangling contract_review_loop_started event) and writes the JSON response to stdout via the same writeJSONResponse call used on the success path with terminal_reason "error"—not in the ledger event—before propagating the error to the caller; TestContractReviewLoopFinishedOnError drives a contract review run that triggers a fixer error, captures stdout, asserts that events.jsonl contains a contract_review_loop_finished event, and asserts that the parsed stdout JSON response carries terminal_reason "error"; adding a separate on-disk summary artifact is not required and must not be added; the run_id is populated in both events; no per-round ledger events are emitted by the contract review loop. (Note: the goal's phrase "round/finished ledger events" refers to these two loop-level events—loop-started and loop-finished—not to one-per-round events.)

**Validation commands**:
  - go test ./internal/loop
  - go test -run ^TestDepsIsolation$ ./internal/loop
  - go test ./internal/app -run TestContractReview
  - go test ./internal/app -run TestReviewLoop
  - go build ./...
  - go test ./...
  - make check
  - go test -run ^TestContractReviewLoopFinishedOnError$ ./internal/app

**Assumptions**:
  - Mirroring review_loop.go ledger events means adding contract_review_loop_started and contract_review_loop_finished records with only type, timestamp, and run_id fields, not adding per-round ledger payloads.
  - Contract review continues to reuse existing review loop limit configuration through resolveContractReviewLoopLimits; no new config keys are introduced.
  - Callers provide Limits.Max > 0; Patience <= 0 and Settle <= 0 disable their respective stop conditions.
  - HumanExit support is required for the shared loop API but contract_review does not need a new human-exit trigger unless existing contract review logic already has one.
  - Contract-file content change is the canonical and sufficient progress signal for fixer rounds in contract_review, matching the existing implementation. Progress is measured by computing the SHA-256 hash of ContractJSON immediately before and after the fixer runs (using the existing storeFileSHA256 function). A fixer invocation that produces a differing hash constitutes progress (RoundResult.Progress is true; unchanged_version_streak resets); a fixer invocation that produces the same hash is non-progress (Progress is false; unchanged_version_streak increments). Clean rounds where no fixer is invoked leave unchanged_version_streak unmodified. No separate version field on the contract object is read or compared for this determination.

## Blocking Findings to Address

1. [codex-xhigh/completeness] The contract does not unambiguously define stop-condition precedence when a single Step result satisfies multiple conditions, such as Human != nil on the same round that would also satisfy Settle or Max. This makes the shared loop behavior incompletely specified and hard to gate consistently.
   Evidence: Goal says: "a Clean round grows a clean-streak ... => Reason 'settled'; ... Human!=nil => immediate 'human'; reaching Max => 'max'." Acceptance also says: "returns Outcome.Reason \"human\" immediately when Step returns a RoundResult with Human != nil" and "returns Outcome.Reason \"max\" after exactly Max rounds when no earlier stop applies," but does not explicitly define ordering for simultaneous human/settled/stalemate/max conditions.
2. [codex-xhigh/testability] The contract has scope-out constraints that are not directly backed by a machine-checkable validation command.
   Evidence: Scope out says: "Changing config files, config defaults, CLI flags, or config schema", "Changing internal/app/review_loop.go or internal/app/clarify_loop.go", and acceptance says "Adding a separate on-disk summary artifact is not required and must not be added". The validation commands only run build/tests/make check and do not include a diff/path guard or artifact absence check.
3. [codex-xhigh/assumptions-surfaced] The contract does not explicitly state whether contract_review runs with no configured reviewers should emit the new loop ledger events or bypass them. This assumption affects implementation and test expectations because no-reviewer behavior is in scope.
   Evidence: Acceptance criteria says "When configured contract reviewers run, events.jsonl contains exactly one contract_review_loop_started event..." while scope also requires preserving behavior for "no reviewers".

## Fixer Instructions

- Address each blocking finding by updating the relevant contract field.
- Do NOT change the goal field — it is out of scope for the fixer.
- Only include the contract fields you are changing in the output.
- base_version must exactly match the version shown above.

## Output

Output your reasoning, then a single JSON block with the revise payload:

```json
{
  "schema": "pactum.contract_revise.v1alpha1",
  "base_version": "542fe28610babee87491bddee8774fffa3caf9adc85a21356c4322de0368c3dd",
  "contract": {
    "acceptance_criteria": ["...updated criteria..."],
    "validation": {"commands": ["...updated commands..."]}
  }
}
```

Omit any contract field you are not changing. Do not include the goal field.
