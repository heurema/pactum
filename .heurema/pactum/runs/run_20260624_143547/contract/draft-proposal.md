# Contract Draft Proposal

## Status
- Run id: run_20260624_143547
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-24T14:39:04Z

## In scope
- Update internal/app/contract_review.go so non-empty contract reviewer stdout with no valid contract reviewer findings block triggers exactly one corrective retry for the same reviewer and lens, modeled on the code-review corrective retry path.
- Update internal/app/contract_review.go so transport errors and empty stdout are recorded as parse misses without a corrective retry.
- Add a ParseMiss signal to the contract review round result and expose unresolved parse misses in the contract review loop round summary JSON.
- Update the contract review clean-round decision so a round is clean only when there are zero blocking findings and no unresolved parse miss.
- Add focused unit coverage in internal/app/contract_review_test.go for corrective retry success, hard failures without retry, and unresolved parse misses preventing clean convergence.

## Out of scope
- Do not change the code-review loop behavior in internal/app/review_loop.go.
- Do not change Pactum configuration schema, reviewer panel configuration, or loop limit defaults.
- Do not change the contract reviewer result schema or ordinary reviewer prompt grammar except for the new corrective retry prompt needed to force the existing findings-block format.
- Do not introduce a new retry framework or retry count beyond the single corrective retry.

## Acceptance criteria
- A soft contract reviewer parse miss, defined as non-empty stdout with zero valid contract reviewer findings blocks, creates exactly one corrective attempt for the same reviewer and lens; when that corrective attempt parses, its findings are included in the round and the round is not marked as an unresolved parse miss for that lens.
- A hard contract reviewer failure, defined as a transport error or empty stdout, is marked as a parse miss and does not create or run a corrective attempt.
- If the corrective attempt does not produce a valid contract reviewer findings block, the lens remains an unresolved parse miss and the round summary JSON contains parse_miss=true with warnings identifying the lens and attempt outcome.
- A contract review round with any unresolved parse miss is not reported as clean even when blocking_findings is 0, does not advance the clean streak, and does not terminate with terminal_reason=resolved.
- The JSON response for contract review exposes parse-miss state per round, and unresolved parse misses are visible through warnings and a non-resolved terminal_reason such as reviewer_findings_unparsed.
- Existing contract-review behavior for valid findings, blocking finding fixer flow, clean convergence, skipped lenses, and no-reviewer no-op remains covered by the current tests.

## Validation commands
- go test ./internal/app -run 'TestContractReview(CorrectiveRetrySuccess|HardFailureNoRetry|UnresolvedParseMissPreventsCleanRound)'
- go test ./internal/app
- make check

## Assumptions
- The contract-review fail-loud terminal reason may reuse the code-review value reviewer_findings_unparsed unless maintainers prefer a contract-specific name.
- The code-review loop in internal/app/review_loop.go is the source of truth for retry count, hard-versus-soft failure classification, and warning style.

