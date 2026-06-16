# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260616_131033
- Approval: approved
- Contract hash: ef628c06928bce2e448dcd1f19f69f43a62f125deccf78cccb427ce329a91415

## Goal
Implement slice 2 of contract cross-review, per docs/contract-review-design.md: the auto-fixer and convergence loop. Today 'contract review' (slice 1) only emits findings. Add: when the contract reviewers produce accepted findings, a fixer applies them to the contract via the declarative 'contract revise --from -' primitive (partial-replace, version-guarded — read the current contract version, apply the fix, never reset approval since this runs pre-approve), then the panel re-reviews, converging until a clean round or max rounds — mirroring the code review loop. Reuse the convergence machinery in internal/app/review_loop.go (rounds / patience / clean_rounds) and the graceful skip-on-failed-lens behavior; the fixer is the drafter-role agent resolved like elsewhere. When contract.reviewers is empty/absent the whole step stays a no-op (slice-1 behavior). The loop must surface per-round results (findings, accepted fixes, skipped lenses) and a terminal reason, in human-readable and --json form. Add tests for: a finding is fixed via revise and the re-review converges clean; convergence stops at max rounds with findings still open; the version guard is used by the fixer so a concurrent change is not clobbered; empty reviewers stays a no-op. Do not change the code review loop. Follow docs/contract-review-design.md.

## In scope
- Implement slice 2 for `pactum contract review`: configured contract reviewers produce accepted contract findings, a fixer applies valid fixes, and the panel re-reviews until convergence or a configured stop.
- Add contract-review fixer prompt/context/result handling that uses `pactum contract show --json` and applies edits only through `pactum contract revise <run> --from -` with `base_version`.
- Reuse the existing review loop concepts for max rounds, patience/stalemate, clean rounds, skipped lenses, per-round summaries, and terminal reasons without changing code-review loop behavior.
- Surface contract review loop results in both human-readable output and `--json`, including per-round findings, accepted fixes, skipped lenses, and terminal reason.
- Add focused tests for clean convergence after a fixer revise, max-round termination with findings still open, stale version protection, and empty `contract.reviewers` no-op behavior.

## Out of scope
- Changing the contract goal.
- Changing code review loop behavior or code review artifacts except for narrowly shared/extracted reusable helpers.
- Running real agent subprocesses in tests; tests should use helper processes or fakes.
- Changing `contract revise --from` partial-replace or version-guard semantics except as needed to consume the existing primitive.
- Supporting approval-resetting contract review fixes; this slice runs before contract approval.

## Acceptance criteria
- With non-empty `contract.reviewers`, `pactum contract review <run> --json` returns a contract-review loop response containing `rounds`, per-round findings/fix data, skipped lenses, and `terminal_reason`.
- Human-readable `pactum contract review <run>` output shows each round's findings/fixes/skipped lenses and the terminal reason.
- When a blocking contract finding is emitted, the fixer is invoked, reads the current contract version, calls `contract revise --from -` with `base_version`, and the next reviewer round runs against the revised contract.
- A successful fixer revise can lead to a subsequent clean round and a clean terminal reason without requiring human approval during the loop.
- When findings remain through the configured round cap, the loop stops with `terminal_reason` of `max_rounds` and reports the remaining open findings.
- A stale `base_version` from the fixer path does not overwrite a concurrent contract change; the failure is surfaced and the contract remains unchanged.
- When `contract.reviewers` is empty or absent, `pactum contract review` remains a no-op: no reviewer or fixer attempts are created and existing slice-1 behavior is preserved.
- Existing code review loop tests continue to pass unchanged.
- Fixer-failure path is explicit: a stale base_version (concurrent contract change) or a fixer agent error/timeout/unparseable-revise causes that finding to be skipped for the round, recorded in the round result, and the loop refreshes the version and continues — it does not abort or overwrite the concurrent change.
- Terminal reasons are named and asserted: 'resolved' (no open blocking findings), 'max_rounds' (rounds exhausted with open blocking findings), and 'stalemate' (patience exhausted with no progress).
- The fixer is invoked for blocking findings only; advisory (non-blocking) findings are surfaced but do not drive a revise.
- The fixer never modifies the contract goal field (scope.out); a revise that would change goal is rejected.
- A test exercises a failed/skipped reviewer lens so the skipped_lenses output element is populated and asserted.

## Validation commands
- go test ./internal/app -run TestContractReview
- go test ./internal/app -run TestReviewLoop
- make check

## Assumptions
- Contract review findings may be accepted automatically inside the loop, mirroring `pactum review run`, rather than adding a separate human accept/reject proposal flow for this slice.
- The contract-review fixer should resolve through the same registry semantics currently used for contract drafting unless a distinct drafter role already exists during implementation.
- Contract review loop limits should reuse the existing review max-rounds, patience, and clean-rounds settings and CLI flag style unless implementation discovers a contract-specific config already exists.

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
- total: 5
- fresh: 5
- stale: 0
- unknown: 0

Items:
- mem_007 [fresh] score=63 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_009 [fresh] score=52 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_005 [fresh] score=49 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_011 [fresh] score=46 — Stagger the cold start of same-model reviewer groups in the review panel fan-...
- mem_013 [fresh] score=38 — Dogfood Pactum with a local codex-acp adapter that returns official ACP Promp...

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
