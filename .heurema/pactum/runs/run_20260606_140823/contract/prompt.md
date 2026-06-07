# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_140823
- Approval: approved
- Contract hash: bcee82b71ba4b1634b35efa1dd2b9a163bd13cedc4e76f29542257ecfa0d4ed5

## Goal
Add a 'pactum review loop' command that drives the review->fix cycle to convergence: run the reviewer, collect findings, and while there are open findings run the fixer then re-validate and re-review, terminating on a review round that finds nothing (clean round) or on max_rounds. Reuse the existing review run / review fix / gate primitives; do not add a new agent-execution path

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

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 0
- fresh: 0
- stale: 0
- unknown: 0

Items:
- none

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Use `pactum search "<term>"` and inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing code items when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.
