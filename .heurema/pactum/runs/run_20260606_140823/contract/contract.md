# Contract Draft

## Goal
Add a 'pactum review loop' command that drives the review->fix cycle to convergence: run the reviewer, collect findings, and while there are open findings run the fixer then re-validate and re-review, terminating on a review round that finds nothing (clean round) or on max_rounds. Reuse the existing review run / review fix / gate primitives; do not add a new agent-execution path

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260606_140823
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- Add a 'review loop <run_id>' command with --reviewer, --agent (fixer), --max-rounds (default from config review.max_rounds), --yes, --timeout, and --json flags
- Each round: run the reviewer (review run + propose-findings, accepting the proposals into findings), collect open findings; if any are open, run the fixer (review fix) then re-run gate validation; loop
- Terminate on a clean review round (no open findings) OR when max_rounds is reached; capture a loop summary artifact (rounds, per-round open-finding counts, terminal reason)
- Reuse existing primitives and require --yes (the fixer performs unsandboxed writes); do NOT auto-approve the review (the human approve gate remains)
- Unit tests using fake reviewer/fixer agents (findings then clean), plus a docs/agents.md note

## Out of scope
- Stalemate-by-fingerprint, budget stop, K>1 consecutive-clean, and rebuttal-feedback-to-the-reviewer (a later slice L3b)
- Severity composition (broad vs critical-only) and multi-reviewer panel
- Native LLM API or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- The loop runs review->(fix if open findings)->re-review and terminates on a clean review round or max_rounds
- It reuses the existing review run / review fix / gate primitives and adds no new subprocess path
- It does not auto-approve the review; it writes a loop summary; it requires --yes
- Covered by tests with fake agents (a round with findings then a clean round)

## Validation commands
- make check

## Assumptions
TBD

## Open questions
- None
