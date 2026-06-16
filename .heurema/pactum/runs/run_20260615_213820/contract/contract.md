# Contract Draft

## Goal
Make the review loop degrade gracefully when a reviewer lens attempt fails (model unavailable / rate-limited / process dies) instead of aborting the whole review run. Today in internal/app/review_loop.go the round collects a per-(reviewer, lens) error grid and the FIRST error wins (around line 558-564): it returns 'reviewer <name> lens <lens>: <err>' and fails the entire run. This was observed when claude-fable-5 returned a rate_limit error ('Claude Fable 5 is currently unavailable') and the run had to be manually re-run with fable removed from the panel. Instead: a failed lens attempt should be recorded as a SKIPPED lens with a warning, and the round should continue with the lenses/reviewers that succeeded. Only fail the round when NO lens produced a result (a fully-unavailable panel). The round summary and review artifacts must surface which (reviewer, lens) attempts were skipped and why. Requirements: (1) a single failing reviewer/lens must not abort the round if at least one other lens succeeded; (2) all-lenses-failing still returns an error; (3) skipped lenses are recorded with their error/warning in the round result and surfaced in the review output; (4) do not change convergence semantics beyond treating skipped lenses as non-fatal. Add tests for: one reviewer/lens failing while others succeed (round proceeds, warning recorded), and every lens failing (round errors).

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260615_211242
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- None

## In scope
- Update internal/app/review_loop.go so a failed reviewer/lens attempt is treated as a skipped lens when at least one reviewer/lens attempt in the round succeeds.
- Record skipped reviewer/lens attempts with reviewer name, lens key, and failure reason in the round result and persisted review loop summary.
- Surface skipped reviewer/lens warnings in both JSON review output and human-readable review run output.
- Add focused review-loop tests for partial lens failure and all-lens failure behavior.

## Out of scope
- Do not change the review lens set, reviewer panel resolution, stagger scheduling policy, or reviewer prompt content except as needed to report skipped attempts.
- Do not change fixer, gate, proposal acceptance, or convergence semantics beyond treating skipped reviewer/lens attempts as non-fatal when another lens succeeds.
- Do not make failed reviewer/lens attempts appear as accepted findings or successful proposal sources.

## Acceptance criteria
- When one reviewer/lens attempt fails and at least one other reviewer/lens attempt succeeds, `pactum review run` completes the round instead of returning the failing attempt error.
- For a partial failure round, successful lens results are still collected into proposals exactly as before, and skipped lenses do not call proposal collection.
- For a partial failure round, the round summary artifact contains an observable warning or structured skipped entry identifying the skipped reviewer, lens, and reason.
- For a partial failure round, CLI JSON output and non-JSON summary output surface the skipped reviewer/lens and reason.
- When every reviewer/lens attempt in a review round fails, the round returns an error and does not report a successful review round.
- Existing convergence behavior remains unchanged for clean rounds, open findings, fixer rounds, gate failures, and stalemate handling except for non-fatal skipped lens attempts.

## Validation commands
- go test ./internal/app -run TestReviewLoop -count=1
- make check

## Assumptions
- The preferred persisted representation can be either structured skipped attempt entries or warning strings, as long as tests can assert reviewer, lens, and reason from the review loop summary artifact and CLI output.
- A reviewer/lens attempt counts as successful only when it produces a usable reviewer result document that can be processed for proposals.
- Failure reasons may be normalized from process errors, timeouts, unavailable-model errors, or agent transport errors, but must preserve enough detail for a human to understand why the lens was skipped.

## Open questions
- None
