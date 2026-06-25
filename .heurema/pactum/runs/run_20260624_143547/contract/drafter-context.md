# Contract Drafter Context

## Run
- Run id: run_20260624_143547
- Run status: contract_draft

## Contract goal
Harden the contract-review loop against reviewer parse-misses, porting the #196 "force the format, retry, fail loud" mechanism that already protects the code-review loop (internal/app/review_loop.go) to the contract-review loop (internal/app/contract_review.go).

PROBLEM (the linchpin bug, reported by an external pactum user as issue #5): When a contract reviewer/lens attempt produces output that does not parse into a valid findings block, the contract-review round (runContractReviewRound in internal/app/contract_review.go, the parse loop around line 617-659) merely appends a "parse miss" warning via parseContractReviewerFindingBlocks and CONTINUES — that lens's findings are silently dropped. Worse, the loop's clean-round decision (runContractReviewLoop, internal/app/contract_review.go around line 426) declares a round Clean purely on `blockingCount == 0`, with no regard for parse-misses. Net effect: a reviewer whose output failed to parse is treated as if it found nothing, and if the surviving (parsed) findings are non-blocking, the loop silently converges to `resolved`. This drops half the lenses' signal and lets the loop "lie about convergence" — the exact failure pactum's loud-loops are meant to prevent.

REFERENCE: The code-review loop already solved this in PR #196. In internal/app/review_loop.go (the per-lens attempt handling around lines 883-940 plus the helper runReviewerCorrectiveAttempt around line 1251), it:
- distinguishes HARD failures (transport error, or empty stdout) — NOT eligible for a corrective retry, recorded as a parse miss — from SOFT failures (non-empty stdout with no valid findings block) — eligible for exactly ONE corrective retry that re-prompts the reviewer to force the result format, then re-parses;
- carries a ParseMiss flag on the round/batch result;
- and treats a parse miss as fail-loud: a round with a parse miss is NOT a clean round (see review_loop.go's clean-round path which requires "no parse-miss").

DESIRED BEHAVIOR (mirror the #196 code-review semantics in the contract-review loop):
- Add a corrective-retry path for a SOFT contract-reviewer parse miss (non-empty stdout, no valid findings block): run exactly one corrective attempt that re-prompts the same reviewer/lens to emit the required findings-block format, then re-parse. Model it on runReviewerCorrectiveAttempt. If the corrective attempt yields a valid block, use it; otherwise the lens remains a parse miss.
- Distinguish HARD failures (transport error, empty stdout) from SOFT ones exactly as the code-review loop does: hard failures are recorded as parse misses WITHOUT a corrective retry; only soft failures get the one retry.
- Surface a ParseMiss signal on contractReviewRoundResult (and in the round summary JSON) so it is observable.
- FAIL LOUD: a round in which any reviewer/lens remains a parse miss after the corrective retry must NOT be declared a clean round — the clean-round decision around contract_review.go line 426 must require both `blockingCount == 0` AND no unresolved parse miss. The loop must therefore not converge to `resolved` on a round that dropped a lens to a parse miss; such a state surfaces loudly (it keeps the loop from declaring convergence and is visible in warnings / round summary / terminal_reason), consistent with how the code-review loop behaves.

SCOPE: internal/app only — primarily internal/app/contract_review.go (the round parse loop, the round-result struct, the clean-round decision), mirroring internal/app/review_loop.go. Add unit coverage for: (a) a soft parse miss triggering exactly one corrective retry that then parses, (b) a hard failure (empty stdout / transport error) NOT triggering a retry, and (c) an unresolved parse miss preventing a clean round / convergence. Do NOT change the code-review loop, do NOT change config, do NOT alter the reviewer prompt grammar or schemas beyond what the corrective re-prompt needs. Keep the change faithful to the existing #196 pattern; do not invent a new mechanism.

## Current contract fields
- In scope:
  - none
- Out of scope:
  - none
- Acceptance criteria:
  - none
- Validation commands:
  - none
- Assumptions:
  - none

## Answered clarifications
- None

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Verify any file references by reading the current source.
