# Memory Candidate

## Run
- Run id: run_20260624_143547
- Source: deterministic

## Contract
- Goal: Harden the contract-review loop (internal/app/contract_review.go) so a contract reviewer/lens whose SUCCESSFUL (exit 0) attempt fails to emit a valid findings block is no longer silently dropped, adapting the #196 'force the format, retry, fail loud' mechanism from the code-review loop (internal/app/review_loop.go).

PROBLEM (external pactum user issue #5): When a contract reviewer/lens attempt exits 0 but its output does not parse into a valid findings block, runContractReviewRound merely appends a 'parse miss' warning and continues — that lens's findings are silently dropped. The clean-round decision in runContractReviewLoop declares a round Clean purely on blockingCount==0, ignoring parse misses, so a reviewer whose output failed to parse is treated as if it found nothing and the loop silently converges to resolved. This is the 'lying about convergence' failure pactum's loud loops exist to prevent.

PREREQUISITE — a grammar asymmetry that must be fixed first: the code-review reviewer prompt mandates an empty findings block even when clean ('even when you have no findings, emit "findings": []'), but the contract reviewer prompt currently says the opposite ('If no issues, say so clearly. Do not include an empty findings block.'). Because a clean contract review therefore emits NO block, a clean review is INDISTINGUISHABLE from a parse miss. So this slice must first make the contract reviewer findings block mandatory (emit "findings": [] when clean), mirroring code-review. Only then is 'exit 0 with no valid block' an unambiguous parse miss.

DESIRED BEHAVIOR (adapt the #196 mechanism, scoped to SUCCESSFUL attempts):
- A SOFT parse miss = a successful (exit 0) attempt with non-empty stdout and no valid findings block → exactly ONE corrective retry that re-prompts the same reviewer/lens for the required format, then re-parse.
- A HARD parse miss = a successful (exit 0) attempt with EMPTY stdout → parse miss with NO corrective retry.
- An unresolved parse miss (soft retry still no block, or any hard miss) sets a ParseMiss signal, makes the round non-clean (blockingCount==0 is no longer sufficient), and the loop exits immediately with terminal_reason=reviewer_findings_unparsed (mirroring review_loop.go's errReviewerUnparsed path).
- A lens that FAILED TO RUN — transport error / non-zero exit, or a wall-clock-timeout kill — is NOT a parse miss: it KEEPS the existing skip behavior (recorded in skipped_lenses, no corrective retry, no fail-loud). A parse miss applies only to successful attempts. This deliberately diverges from the code-review loop (which treats transport/empty as hard parse misses) to keep this slice minimal and to preserve TestContractReviewFailedLensSkipped.

SCOPE: internal/app only (contract_review.go: the reviewer prompt grammar, the round parse loop, the round-result struct, the clean-round decision and terminal wiring) plus contract_review_test.go (extend the fake-reviewer harness and add coverage). Do NOT change the code-review loop, config, loop limits, or the reviewer result schema beyond the mandatory-empty-block grammar and the corrective re-prompt.
- In scope:
  - PREREQUISITE GRAMMAR CHANGE: make the contract reviewer findings block mandatory — change the contract reviewer prompt line in internal/app/contract_review.go that currently says 'If no issues, say so clearly. Do not include an empty findings block.' so a clean review emits exactly one fenced JSON block with "findings": [], mirroring the code-review reviewer prompt in internal/app/review.go.
  - Add a corrective-retry path in internal/app/contract_review.go for a SOFT parse miss (a successful exit-0 attempt with non-empty stdout and no valid findings block): run exactly one corrective attempt for the same reviewer/lens re-prompting for the required format, then re-parse. The mechanism mirrors code-review's runReviewerCorrectiveAttempt with one deliberate divergence: a corrective attempt that returns a valid findings block — including "findings": [] — is treated as a successfully parsed result and is NOT an unresolved parse miss. (In code-review, "findings": [] from a corrective attempt is treated as unresolved/unverifiable; in contract-review, "findings": [] is the mandatory canonical clean signal after the prerequisite grammar change, so emitting it after a corrective prompt confirms the reviewer parsed the contract and found nothing.)
  - Classify a successful (exit-0) attempt with EMPTY stdout as a HARD parse miss: a parse miss with NO corrective retry.
  - Add a ParseMiss signal to contractReviewRoundResult and expose parse_miss=true per round in the contract review loop round summary JSON, with warnings identifying the reviewer/lens and attempt outcome.
  - Update the clean-round decision and wire the loop so a round containing an unresolved parse miss is not clean (blockingCount==0 no longer sufficient), returns a sentinel mirroring errReviewerUnparsed, and the loop maps it to terminal_reason=reviewer_findings_unparsed and exits immediately (mirroring review_loop.go lines 242-273 and 647-648).
  - Extend the fake contract-reviewer test harness in internal/app/contract_review_test.go: (i) change its CLEAN path to emit a valid empty block ("findings": []) so clean fake reviews match the new mandatory-block grammar and are NOT parse misses; (ii) add env hooks to simulate a successful exit-0 attempt with non-empty unparseable stdout (soft miss), a successful exit-0 attempt with empty stdout (hard miss), a corrective-retry-succeeds sequence (first attempt unparseable, corrective attempt emits a valid non-empty findings block), a corrective-retry-succeeds-with-empty-block sequence (first attempt unparseable, corrective attempt emits "findings": []), and a corrective-retry-fails-to-run sequence (first attempt unparseable, corrective attempt exits non-zero).
  - Add focused unit coverage in internal/app/contract_review_test.go for: (a) a soft parse miss triggering exactly one corrective retry that then parses (findings included, no fail-loud); (b) a successful empty-stdout attempt being a hard parse miss with no retry; (c) an unresolved soft parse miss (corrective retry returns non-empty unparseable stdout) producing terminal_reason=reviewer_findings_unparsed and a non-clean round; (d) a regression guard that a clean review emitting "findings": [] parses as zero findings, is NOT a parse miss, and converges normally; (e) a corrective retry that fails to run (exits non-zero) is treated as an unresolved parse miss and produces terminal_reason=reviewer_findings_unparsed; and (f) a soft-parse-miss corrective attempt that returns "findings": [] is treated as a successfully parsed clean result (not an unresolved parse miss), the round converges normally, and zero findings are recorded for that lens.
- Out of scope:
  - Do not change the code-review loop behavior in internal/app/review_loop.go.
  - Do not change Pactum configuration schema, reviewer panel configuration, or loop limit defaults.
  - Do not reclassify a lens that FAILED TO RUN — a transport error / non-zero exit, or a wall-clock-timeout kill — as a parse miss. Such lenses retain their existing behavior: recorded in skipped_lenses, no corrective retry, no fail-loud exit. A parse miss applies ONLY to a successful (exit 0) attempt that failed to emit a valid block. TestContractReviewFailedLensSkipped must continue to pass with its existing assertions (failed lens skipped, loop exits 0).
  - Do not introduce a new retry framework or any retry count beyond the single corrective retry for the soft case.
  - Do not change the contract reviewer result schema; the only reviewer-prompt changes are the mandatory empty-block grammar and the corrective re-prompt.
- Acceptance criteria:
  - After the prerequisite grammar change, a clean contract review emits exactly one fenced JSON block containing "findings": []; it parses as zero findings, is NOT a parse miss, and the round converges normally. The existing clean-convergence BEHAVIOR is preserved; the fake-reviewer fixtures used by the existing clean/convergence tests are updated to emit the empty block, and those tests pass.
  - A focused runnable unit test asserts the real contract-reviewer prompt text or prompt builder mandates the fenced JSON findings block for clean reviews, including "findings": [], and no longer contains the old instruction to omit an empty findings block.
  - A SOFT parse miss — a successful (exit 0) reviewer/lens attempt with non-empty stdout and no valid findings block — creates exactly one corrective attempt for the same reviewer/lens; when the corrective attempt returns a valid findings block — including "findings": [] (the canonical clean signal per AC1) — its findings (zero or more) are included and the lens is not an unresolved parse miss. A corrective attempt returning "findings": [] is explicitly valid and not an unresolved parse miss; this deliberately diverges from code-review's runReviewerCorrectiveAttempt, which treats "findings": [] as unresolved (unverifiable clean), a distinction that does not apply after the mandatory-block grammar change makes "findings": [] the sole valid clean emission.
  - A successful (exit 0) reviewer/lens attempt with EMPTY stdout is a HARD parse miss: marked as a parse miss with NO corrective attempt created or run.
  - A lens whose INITIAL attempt FAILED TO RUN (transport error / non-zero exit, or wall-clock-timeout kill) is NOT a parse miss: it keeps the existing skip behavior (skipped_lenses, no corrective retry), does not set parse_miss, and does not trigger the fail-loud exit. This carve-out applies only to the initial attempt. TestContractReviewFailedLensSkipped passes unchanged.
  - If the corrective retry itself fails to run (transport error / non-zero exit, or wall-clock-timeout kill), the lens is treated as an unresolved parse miss. The initial attempt was successful but produced no valid findings block; no subsequent attempt resolved it. The 'failed-to-run = skip' carve-out does NOT extend to corrective retries — the lens does not revert to skip behavior at this point.
  - An unresolved parse miss (soft corrective retry still no valid block, soft corrective retry failed to run, or any hard miss) sets parse_miss=true in the round summary JSON with warnings identifying the lens and attempt outcome.
  - A contract review round with any unresolved parse miss is not reported as clean even when blocking_findings is 0, does not advance the clean streak, and does not terminate with terminal_reason=resolved. The loop instead exits immediately with terminal_reason=reviewer_findings_unparsed (reusing the code-review value), mirroring review_loop.go lines 242-273. The parse miss is scoped to its round; because the exit is immediate the loop never continues to clean-streak or max_rounds termination while a parse miss is outstanding.
  - Existing contract-review behavior for valid findings, blocking-finding fixer flow, clean convergence, skipped lenses (transport error and wall-clock-timeout on initial attempt), and the no-reviewer no-op remains covered and passing (with clean fixtures updated to the mandatory-empty-block grammar as noted in AC1).
- Validation commands:
  - go test ./internal/app -run TestContractReviewerPromptRequiresMandatoryFindingsBlock -count=1
  - go test ./internal/app -run TestContractReview -count=1
  - go test ./internal/app -count=1
  - make check

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - internal/app/contract_review.go
  - internal/app/contract_review_test.go
- New files: none
- Missing files: none

## Clarifications
- None

## Review Decisions
- f_001 [low] open internal/app/contract_review.go:1422: The new corrective-prompt builder renderContractReviewerCorrectivePrompt has no direct unit test for its body. The primary prompt is guarded by TestContractReviewerPromptRequiresMandatoryFindingsBlock (asserts schema + mandatory "findings": [] + removal of the old omit instruction), but the corrective prompt — whose sole purpose is to re-instruct the reviewer to emit a valid block on a soft miss — has no equivalent. The fake reviewer subprocess matches only the '# Corrective Contract Review:' heading and responds via env vars, so the corrective prompt's instruction content (schema name, mandatory empty-block directive) is never asserted; a regression that weakens it would degrade the production remediation path with no failing test.
- f_002 [low] open docs/flow.md:231: The contract-review loop gains a new operator-visible terminal reason `reviewer_findings_unparsed` (internal/app/contract_review.go:514-516) and a `parse_miss` field in the per-round loop-summary JSON, but no user-facing doc enumerates contract-review terminal reasons. By contrast, docs/flow.md has a dedicated 'Review run terminal reasons' section for `pactum review run` that already documents `reviewer_findings_unparsed`. An operator who hits this new terminal in a contract review's loop-summary.json has no documentation to consult.
- Proposal summary: pending=0 accepted=2 rejected=0

## Reusable Project Knowledge
- scope: in scope: PREREQUISITE GRAMMAR CHANGE: make the contract reviewer findings block mandatory — change the contract reviewer prompt line in internal/app/contract_review.go that currently says 'If no issues, say so clearly. Do not include an empty findings block.' so a clean review emits exactly one fenced JSON block with "findings": [], mirroring the code-review reviewer prompt in internal/app/review.go.
- scope: in scope: Add a corrective-retry path in internal/app/contract_review.go for a SOFT parse miss (a successful exit-0 attempt with non-empty stdout and no valid findings block): run exactly one corrective attempt for the same reviewer/lens re-prompting for the required format, then re-parse. The mechanism mirrors code-review's runReviewerCorrectiveAttempt with one deliberate divergence: a corrective attempt that returns a valid findings block — including "findings": [] — is treated as a successfully parsed result and is NOT an unresolved parse miss. (In code-review, "findings": [] from a corrective attempt is treated as unresolved/unverifiable; in contract-review, "findings": [] is the mandatory canonical clean signal after the prerequisite grammar change, so emitting it after a corrective prompt confirms the reviewer parsed the contract and found nothing.)
- scope: in scope: Classify a successful (exit-0) attempt with EMPTY stdout as a HARD parse miss: a parse miss with NO corrective retry.
- scope: in scope: Add a ParseMiss signal to contractReviewRoundResult and expose parse_miss=true per round in the contract review loop round summary JSON, with warnings identifying the reviewer/lens and attempt outcome.
- scope: in scope: Update the clean-round decision and wire the loop so a round containing an unresolved parse miss is not clean (blockingCount==0 no longer sufficient), returns a sentinel mirroring errReviewerUnparsed, and the loop maps it to terminal_reason=reviewer_findings_unparsed and exits immediately (mirroring review_loop.go lines 242-273 and 647-648).
- scope: in scope: Extend the fake contract-reviewer test harness in internal/app/contract_review_test.go: (i) change its CLEAN path to emit a valid empty block ("findings": []) so clean fake reviews match the new mandatory-block grammar and are NOT parse misses; (ii) add env hooks to simulate a successful exit-0 attempt with non-empty unparseable stdout (soft miss), a successful exit-0 attempt with empty stdout (hard miss), a corrective-retry-succeeds sequence (first attempt unparseable, corrective attempt emits a valid non-empty findings block), a corrective-retry-succeeds-with-empty-block sequence (first attempt unparseable, corrective attempt emits "findings": []), and a corrective-retry-fails-to-run sequence (first attempt unparseable, corrective attempt exits non-zero).
- scope: in scope: Add focused unit coverage in internal/app/contract_review_test.go for: (a) a soft parse miss triggering exactly one corrective retry that then parses (findings included, no fail-loud); (b) a successful empty-stdout attempt being a hard parse miss with no retry; (c) an unresolved soft parse miss (corrective retry returns non-empty unparseable stdout) producing terminal_reason=reviewer_findings_unparsed and a non-clean round; (d) a regression guard that a clean review emitting "findings": [] parses as zero findings, is NOT a parse miss, and converges normally; (e) a corrective retry that fails to run (exits non-zero) is treated as an unresolved parse miss and produces terminal_reason=reviewer_findings_unparsed; and (f) a soft-parse-miss corrective attempt that returns "findings": [] is treated as a successfully parsed clean result (not an unresolved parse miss), the round converges normally, and zero findings are recorded for that lens.
- scope: out of scope: Do not change the code-review loop behavior in internal/app/review_loop.go.
- scope: out of scope: Do not change Pactum configuration schema, reviewer panel configuration, or loop limit defaults.
- scope: out of scope: Do not reclassify a lens that FAILED TO RUN — a transport error / non-zero exit, or a wall-clock-timeout kill — as a parse miss. Such lenses retain their existing behavior: recorded in skipped_lenses, no corrective retry, no fail-loud exit. A parse miss applies ONLY to a successful (exit 0) attempt that failed to emit a valid block. TestContractReviewFailedLensSkipped must continue to pass with its existing assertions (failed lens skipped, loop exits 0).
- scope: out of scope: Do not introduce a new retry framework or any retry count beyond the single corrective retry for the soft case.
- scope: out of scope: Do not change the contract reviewer result schema; the only reviewer-prompt changes are the mandatory empty-block grammar and the corrective re-prompt.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- validation: go test ./internal/app -run TestContractReviewerPromptRequiresMandatoryFindingsBlock -count=1 passed
- validation: go test ./internal/app -run TestContractReview -count=1 passed
- validation: go test ./internal/app -count=1 passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
