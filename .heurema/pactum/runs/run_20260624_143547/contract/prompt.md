# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract and memory boundaries before execution.

## Contract status
- Run: run_20260624_143547
- Approval: approved
- Contract hash: 95a4b866498fcf5c445e7141355896920d26e374b86e641e4f644b413fdfa65a

## Goal
Harden the contract-review loop (internal/app/contract_review.go) so a contract reviewer/lens whose SUCCESSFUL (exit 0) attempt fails to emit a valid findings block is no longer silently dropped, adapting the #196 'force the format, retry, fail loud' mechanism from the code-review loop (internal/app/review_loop.go).

PROBLEM (external pactum user issue #5): When a contract reviewer/lens attempt exits 0 but its output does not parse into a valid findings block, runContractReviewRound merely appends a 'parse miss' warning and continues — that lens's findings are silently dropped. The clean-round decision in runContractReviewLoop declares a round Clean purely on blockingCount==0, ignoring parse misses, so a reviewer whose output failed to parse is treated as if it found nothing and the loop silently converges to resolved. This is the 'lying about convergence' failure pactum's loud loops exist to prevent.

PREREQUISITE — a grammar asymmetry that must be fixed first: the code-review reviewer prompt mandates an empty findings block even when clean ('even when you have no findings, emit "findings": []'), but the contract reviewer prompt currently says the opposite ('If no issues, say so clearly. Do not include an empty findings block.'). Because a clean contract review therefore emits NO block, a clean review is INDISTINGUISHABLE from a parse miss. So this slice must first make the contract reviewer findings block mandatory (emit "findings": [] when clean), mirroring code-review. Only then is 'exit 0 with no valid block' an unambiguous parse miss.

DESIRED BEHAVIOR (adapt the #196 mechanism, scoped to SUCCESSFUL attempts):
- A SOFT parse miss = a successful (exit 0) attempt with non-empty stdout and no valid findings block → exactly ONE corrective retry that re-prompts the same reviewer/lens for the required format, then re-parse.
- A HARD parse miss = a successful (exit 0) attempt with EMPTY stdout → parse miss with NO corrective retry.
- An unresolved parse miss (soft retry still no block, or any hard miss) sets a ParseMiss signal, makes the round non-clean (blockingCount==0 is no longer sufficient), and the loop exits immediately with terminal_reason=reviewer_findings_unparsed (mirroring review_loop.go's errReviewerUnparsed path).
- A lens that FAILED TO RUN — transport error / non-zero exit, or a wall-clock-timeout kill — is NOT a parse miss: it KEEPS the existing skip behavior (recorded in skipped_lenses, no corrective retry, no fail-loud). A parse miss applies only to successful attempts. This deliberately diverges from the code-review loop (which treats transport/empty as hard parse misses) to keep this slice minimal and to preserve TestContractReviewFailedLensSkipped.

SCOPE: internal/app only (contract_review.go: the reviewer prompt grammar, the round parse loop, the round-result struct, the clean-round decision and terminal wiring) plus contract_review_test.go (extend the fake-reviewer harness and add coverage). Do NOT change the code-review loop, config, loop limits, or the reviewer result schema beyond the mandatory-empty-block grammar and the corrective re-prompt.

## In scope
- PREREQUISITE GRAMMAR CHANGE: make the contract reviewer findings block mandatory — change the contract reviewer prompt line in internal/app/contract_review.go that currently says 'If no issues, say so clearly. Do not include an empty findings block.' so a clean review emits exactly one fenced JSON block with "findings": [], mirroring the code-review reviewer prompt in internal/app/review.go.
- Add a corrective-retry path in internal/app/contract_review.go for a SOFT parse miss (a successful exit-0 attempt with non-empty stdout and no valid findings block): run exactly one corrective attempt for the same reviewer/lens re-prompting for the required format, then re-parse. The mechanism mirrors code-review's runReviewerCorrectiveAttempt with one deliberate divergence: a corrective attempt that returns a valid findings block — including "findings": [] — is treated as a successfully parsed result and is NOT an unresolved parse miss. (In code-review, "findings": [] from a corrective attempt is treated as unresolved/unverifiable; in contract-review, "findings": [] is the mandatory canonical clean signal after the prerequisite grammar change, so emitting it after a corrective prompt confirms the reviewer parsed the contract and found nothing.)
- Classify a successful (exit-0) attempt with EMPTY stdout as a HARD parse miss: a parse miss with NO corrective retry.
- Add a ParseMiss signal to contractReviewRoundResult and expose parse_miss=true per round in the contract review loop round summary JSON, with warnings identifying the reviewer/lens and attempt outcome.
- Update the clean-round decision and wire the loop so a round containing an unresolved parse miss is not clean (blockingCount==0 no longer sufficient), returns a sentinel mirroring errReviewerUnparsed, and the loop maps it to terminal_reason=reviewer_findings_unparsed and exits immediately (mirroring review_loop.go lines 242-273 and 647-648).
- Extend the fake contract-reviewer test harness in internal/app/contract_review_test.go: (i) change its CLEAN path to emit a valid empty block ("findings": []) so clean fake reviews match the new mandatory-block grammar and are NOT parse misses; (ii) add env hooks to simulate a successful exit-0 attempt with non-empty unparseable stdout (soft miss), a successful exit-0 attempt with empty stdout (hard miss), a corrective-retry-succeeds sequence (first attempt unparseable, corrective attempt emits a valid non-empty findings block), a corrective-retry-succeeds-with-empty-block sequence (first attempt unparseable, corrective attempt emits "findings": []), and a corrective-retry-fails-to-run sequence (first attempt unparseable, corrective attempt exits non-zero).
- Add focused unit coverage in internal/app/contract_review_test.go for: (a) a soft parse miss triggering exactly one corrective retry that then parses (findings included, no fail-loud); (b) a successful empty-stdout attempt being a hard parse miss with no retry; (c) an unresolved soft parse miss (corrective retry returns non-empty unparseable stdout) producing terminal_reason=reviewer_findings_unparsed and a non-clean round; (d) a regression guard that a clean review emitting "findings": [] parses as zero findings, is NOT a parse miss, and converges normally; (e) a corrective retry that fails to run (exits non-zero) is treated as an unresolved parse miss and produces terminal_reason=reviewer_findings_unparsed; and (f) a soft-parse-miss corrective attempt that returns "findings": [] is treated as a successfully parsed clean result (not an unresolved parse miss), the round converges normally, and zero findings are recorded for that lens.

## Out of scope
- Do not change the code-review loop behavior in internal/app/review_loop.go.
- Do not change Pactum configuration schema, reviewer panel configuration, or loop limit defaults.
- Do not reclassify a lens that FAILED TO RUN — a transport error / non-zero exit, or a wall-clock-timeout kill — as a parse miss. Such lenses retain their existing behavior: recorded in skipped_lenses, no corrective retry, no fail-loud exit. A parse miss applies ONLY to a successful (exit 0) attempt that failed to emit a valid block. TestContractReviewFailedLensSkipped must continue to pass with its existing assertions (failed lens skipped, loop exits 0).
- Do not introduce a new retry framework or any retry count beyond the single corrective retry for the soft case.
- Do not change the contract reviewer result schema; the only reviewer-prompt changes are the mandatory empty-block grammar and the corrective re-prompt.

## Acceptance criteria
- After the prerequisite grammar change, a clean contract review emits exactly one fenced JSON block containing "findings": []; it parses as zero findings, is NOT a parse miss, and the round converges normally. The existing clean-convergence BEHAVIOR is preserved; the fake-reviewer fixtures used by the existing clean/convergence tests are updated to emit the empty block, and those tests pass.
- A focused runnable unit test asserts the real contract-reviewer prompt text or prompt builder mandates the fenced JSON findings block for clean reviews, including "findings": [], and no longer contains the old instruction to omit an empty findings block.
- A SOFT parse miss — a successful (exit 0) reviewer/lens attempt with non-empty stdout and no valid findings block — creates exactly one corrective attempt for the same reviewer/lens; when the corrective attempt returns a valid findings block — including "findings": [] (the canonical clean signal per AC1) — its findings (zero or more) are included and the lens is not an unresolved parse miss. A corrective attempt returning "findings": [] is explicitly valid and not an unresolved parse miss; this deliberately diverges from code-review's runReviewerCorrectiveAttempt, which treats "findings": [] as unresolved (unverifiable clean), a distinction that does not apply after the mandatory-block grammar change makes "findings": [] the sole valid clean emission.
- A successful (exit 0) reviewer/lens attempt with EMPTY stdout is a HARD parse miss: marked as a parse miss with NO corrective attempt created or run.
- A lens whose INITIAL attempt FAILED TO RUN (transport error / non-zero exit, or wall-clock-timeout kill) is NOT a parse miss: it keeps the existing skip behavior (skipped_lenses, no corrective retry), does not set parse_miss, and does not trigger the fail-loud exit. This carve-out applies only to the initial attempt. TestContractReviewFailedLensSkipped passes unchanged.
- If the corrective retry itself fails to run (transport error / non-zero exit, or wall-clock-timeout kill), the lens is treated as an unresolved parse miss. The initial attempt was successful but produced no valid findings block; no subsequent attempt resolved it. The 'failed-to-run = skip' carve-out does NOT extend to corrective retries — the lens does not revert to skip behavior at this point.
- An unresolved parse miss (soft corrective retry still no valid block, soft corrective retry failed to run, or any hard miss) sets parse_miss=true in the round summary JSON with warnings identifying the lens and attempt outcome.
- A contract review round with any unresolved parse miss is not reported as clean even when blocking_findings is 0, does not advance the clean streak, and does not terminate with terminal_reason=resolved. The loop instead exits immediately with terminal_reason=reviewer_findings_unparsed (reusing the code-review value), mirroring review_loop.go lines 242-273. The parse miss is scoped to its round; because the exit is immediate the loop never continues to clean-streak or max_rounds termination while a parse miss is outstanding.
- Existing contract-review behavior for valid findings, blocking-finding fixer flow, clean convergence, skipped lenses (transport error and wall-clock-timeout on initial attempt), and the no-reviewer no-op remains covered and passing (with clean fixtures updated to the mandatory-empty-block grammar as noted in AC1).

## Validation commands
- go test ./internal/app -run TestContractReviewerPromptRequiresMandatoryFindingsBlock -count=1
- go test ./internal/app -run TestContractReview -count=1
- go test ./internal/app -count=1
- make check

## Assumptions
- The code-review loop in internal/app/review_loop.go is the source of truth for the corrective-retry mechanism (single retry), the soft-vs-hard distinction among SUCCESSFUL attempts (non-empty-no-block = soft/retry, empty-stdout = hard/no-retry), the warning style, the ParseMiss signal, and the immediate fail-loud exit to terminal_reason=reviewer_findings_unparsed.
- Deliberate divergence from code-review (first): a lens whose INITIAL attempt fails to RUN (transport error / non-zero exit, or wall-clock-timeout kill) is NOT reclassified as a parse miss in the contract-review loop; it keeps the existing skip behavior. This carve-out is explicitly scoped to the initial attempt only. A corrective retry that fails to run does NOT inherit this carve-out: the initial attempt already established a successful-but-unparseable soft miss, so if the corrective attempt cannot run either, the lens remains an unresolved parse miss and the round exits with terminal_reason=reviewer_findings_unparsed. This keeps the slice minimal and preserves TestContractReviewFailedLensSkipped.
- Deliberate divergence from code-review (second): a corrective attempt that returns "findings": [] is treated as a successfully parsed clean result, not an unresolved parse miss. Code-review's runReviewerCorrectiveAttempt classifies "findings": [] as unresolved (unverifiable clean) because in that grammar a clean reviewer is expected to emit a non-empty block, making an empty corrective response ambiguous. In the contract-review grammar, "findings": [] is the mandatory canonical clean signal (prerequisite grammar change); a corrective attempt emitting it has correctly parsed the contract and found nothing — it is unambiguously valid, not an evasion.
- The existing clean-convergence and related tests rely on a fake reviewer that currently emits NO block when clean; those fixtures will be updated to emit "findings": [] to match the new mandatory-block grammar, preserving the clean-convergence behavior they assert.
- Tests use the existing contract-reviewer subprocess harness (PACTUM_CONTRACT_REVIEWER_* env hooks), extended with new hooks for the soft/hard/corrective-succeeds/corrective-succeeds-with-empty-block/corrective-fails-to-run cases, plus skipIfNoGit(t).

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 5
- fresh: 5
- stale: 0
- unknown: 0

Items:
- mem_021 [fresh] score=81 — Make pactum's code-review loop never silently drop reviewer findings, and rec...
- mem_025 [fresh] score=70 — Make the review loop (both contract_review and code_review, which share inter...
- mem_007 [fresh] score=65 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_026 [fresh] score=63 — Add an absolute per-attempt WALL-CLOCK CAP to the ACP agent transport so an a...
- mem_027 [fresh] score=57 — Give the CONTRACT-REVIEW loop the same operator finding-resolution that CODE-...

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing exported functions and types when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.

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
