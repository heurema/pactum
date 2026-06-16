# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260616_115349
- Approval: approved
- Contract hash: 3cfa03e42ac4dd83bc8ce09fbd7ff0799f7d514348065681cb308ff234539824

## Goal
Implement slice 1 of contract cross-review, specified in docs/contract-review-design.md. Add an optional panel that reviews the CONTRACT (not code) before the human approve gate. Scope of slice 1: (1) a 'contract.reviewers' config array of registry agent names (empty or absent = off, so current behavior is unchanged). (2) A 'contract review <run>' command that runs the configured reviewer panel on the contract document (goal, scope.in/out, acceptance_criteria, validation.commands, assumptions) using CONTRACT lenses, distinct from the code-review lenses: completeness (does the contract cover its goal; gaps in scope/acceptance), testability (is each acceptance criterion backed by or expressible as a runnable validation command), validation-soundness (are validation.commands gate-runnable, non-vacuous, and not self-contradictory with the tests), scope-fidelity (scope.in/out coherent and neither over- nor under-broad), assumptions-surfaced (risky assumptions called out). It emits STRUCTURED FINDINGS in both human-readable and --json form, surfaced before approve. (3) Reuse the reviewer fan-out and lens machinery from the existing code review loop in internal/app/review_loop.go, but review the contract document instead of a git diff; reuse resolveReviewerEntry / the registry resolution and the graceful-degradation skip-on-failed-lens behavior already in review_loop.go. (4) When contract.reviewers is empty/absent, 'contract review' is a no-op that reports nothing to review. DO NOT implement in slice 1: the auto-fixer that applies findings via 'contract revise --from', and multi-round convergence — slice 1 is reviewers + findings only (the human reads the findings and revises via the existing 'contract revise --from', then approves). Add tests for: panel runs and emits findings on a contract with a known gap; empty contract.reviewers is a no-op; a failed reviewer lens is skipped (graceful) rather than aborting. Follow docs/contract-review-design.md.

## In scope
- Add a `contract.reviewers` configuration array of registered agent names, with an absent or empty array disabling contract review.
- Add `pactum contract review <run>` with human-readable output and `--json` output.
- Run the configured contract reviewer panel against the contract document fields: goal, scope.in, scope.out, acceptance_criteria, validation.commands, and assumptions.
- Use contract-specific lenses: completeness, testability, validation-soundness, scope-fidelity, and assumptions-surfaced.
- Reuse the existing review fan-out, registry resolution, reviewer attempt, lens prompt, and partial failed-lens skip behavior from the code review loop where applicable.
- Emit structured contract-review findings that identify reviewer, lens, severity or blocking status, message, and evidence or rationale.
- Surface contract review before contract approval when `contract.reviewers` is configured.

## Out of scope
- Do not implement an auto-fixer that applies contract-review findings through `contract revise --from`.
- Do not implement multi-round contract-review convergence or re-review loops.
- Do not review git diffs, execution attempts, gate reports, or code changes from `pactum contract review`.
- Do not change the existing code-review `pactum review run` behavior except for shared helper extraction required by contract review.
- Do not add plan-DAG-only contract lenses such as dependency-correctness or granularity.

## Acceptance criteria
- With `contract.reviewers` absent or empty, `pactum contract review <run>` exits successfully, reports that there is nothing to review, emits no reviewer attempts or findings, and existing contract approval behavior remains unchanged.
- `contract.reviewers` accepts only unique, non-blank names registered in `agents`; unknown, duplicate, or blank entries fail config loading with an error naming `contract.reviewers`.
- With one configured reviewer, `pactum contract review <run> --json` runs one read-only reviewer attempt per contract lens and returns a parseable JSON document containing schema, run_id, reviewers, lenses, findings, skipped_lenses, and next fields.
- With multiple configured reviewers, the command fans out across every reviewer and every contract lens, and the output records which reviewer and lens produced each finding.
- A contract with a known missing scope, acceptance, or validation gap produces at least one structured finding in both human-readable and JSON output.
- A single failed reviewer/lens attempt is skipped and reported in `skipped_lenses` without aborting the whole contract review when at least one other lens attempt succeeds.
- Contract review does not mutate the contract fields, does not approve the contract, does not invoke `contract revise`, and does not run any fixer.
- When reviewers are configured and the contract is otherwise ready for approval, the next-step affordance surfaces `pactum contract review <run>` before `pactum contract approve <run>`; when reviewers are off, the next affordance remains approval.

## Validation commands
- go test ./internal/app -count=1
- make check

## Assumptions
- `contract.reviewers` is a new `contract` config section and does not reuse `review.panel`.
- Slice 1 only surfaces findings for human action; unresolved contract-review findings do not create a new persistent approval-blocking resolution workflow.
- Tests can use the existing fake reviewer/helper transport patterns and must not run real agents.

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
- fresh: 4
- stale: 1
- unknown: 0

Items:
- mem_011 [fresh] score=61 — Stagger the cold start of same-model reviewer groups in the review panel fan-...
- mem_005 [fresh] score=60 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_002 [stale] score=60 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go
- mem_009 [fresh] score=55 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_006 [fresh] score=52 — Smooth the pipeline so no command is pure ritual, then compress the agent ski...

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
