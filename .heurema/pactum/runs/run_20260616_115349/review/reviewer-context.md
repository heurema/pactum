# Reviewer Context

## Run
- Run id: run_20260616_115349
- Run status: contract_approved

## Contract
- Goal: Implement slice 1 of contract cross-review, specified in docs/contract-review-design.md. Add an optional panel that reviews the CONTRACT (not code) before the human approve gate. Scope of slice 1: (1) a 'contract.reviewers' config array of registry agent names (empty or absent = off, so current behavior is unchanged). (2) A 'contract review <run>' command that runs the configured reviewer panel on the contract document (goal, scope.in/out, acceptance_criteria, validation.commands, assumptions) using CONTRACT lenses, distinct from the code-review lenses: completeness (does the contract cover its goal; gaps in scope/acceptance), testability (is each acceptance criterion backed by or expressible as a runnable validation command), validation-soundness (are validation.commands gate-runnable, non-vacuous, and not self-contradictory with the tests), scope-fidelity (scope.in/out coherent and neither over- nor under-broad), assumptions-surfaced (risky assumptions called out). It emits STRUCTURED FINDINGS in both human-readable and --json form, surfaced before approve. (3) Reuse the reviewer fan-out and lens machinery from the existing code review loop in internal/app/review_loop.go, but review the contract document instead of a git diff; reuse resolveReviewerEntry / the registry resolution and the graceful-degradation skip-on-failed-lens behavior already in review_loop.go. (4) When contract.reviewers is empty/absent, 'contract review' is a no-op that reports nothing to review. DO NOT implement in slice 1: the auto-fixer that applies findings via 'contract revise --from', and multi-round convergence — slice 1 is reviewers + findings only (the human reads the findings and revises via the existing 'contract revise --from', then approves). Add tests for: panel runs and emits findings on a contract with a known gap; empty contract.reviewers is a no-op; a failed reviewer lens is skipped (graceful) rather than aborting. Follow docs/contract-review-design.md.
- In scope:
  - Add a `contract.reviewers` configuration array of registered agent names, with an absent or empty array disabling contract review.
  - Add `pactum contract review <run>` with human-readable output and `--json` output.
  - Run the configured contract reviewer panel against the contract document fields: goal, scope.in, scope.out, acceptance_criteria, validation.commands, and assumptions.
  - Use contract-specific lenses: completeness, testability, validation-soundness, scope-fidelity, and assumptions-surfaced.
  - Reuse the existing review fan-out, registry resolution, reviewer attempt, lens prompt, and partial failed-lens skip behavior from the code review loop where applicable.
  - Emit structured contract-review findings that identify reviewer, lens, severity or blocking status, message, and evidence or rationale.
  - Surface contract review before contract approval when `contract.reviewers` is configured.
- Out of scope:
  - Do not implement an auto-fixer that applies contract-review findings through `contract revise --from`.
  - Do not implement multi-round contract-review convergence or re-review loops.
  - Do not review git diffs, execution attempts, gate reports, or code changes from `pactum contract review`.
  - Do not change the existing code-review `pactum review run` behavior except for shared helper extraction required by contract review.
  - Do not add plan-DAG-only contract lenses such as dependency-correctness or granularity.
- Acceptance criteria:
  - With `contract.reviewers` absent or empty, `pactum contract review <run>` exits successfully, reports that there is nothing to review, emits no reviewer attempts or findings, and existing contract approval behavior remains unchanged.
  - `contract.reviewers` accepts only unique, non-blank names registered in `agents`; unknown, duplicate, or blank entries fail config loading with an error naming `contract.reviewers`.
  - With one configured reviewer, `pactum contract review <run> --json` runs one read-only reviewer attempt per contract lens and returns a parseable JSON document containing schema, run_id, reviewers, lenses, findings, skipped_lenses, and next fields.
  - With multiple configured reviewers, the command fans out across every reviewer and every contract lens, and the output records which reviewer and lens produced each finding.
  - A contract with a known missing scope, acceptance, or validation gap produces at least one structured finding in both human-readable and JSON output.
  - A single failed reviewer/lens attempt is skipped and reported in `skipped_lenses` without aborting the whole contract review when at least one other lens attempt succeeds.
  - Contract review does not mutate the contract fields, does not approve the contract, does not invoke `contract revise`, and does not run any fixer.
  - When reviewers are configured and the contract is otherwise ready for approval, the next-step affordance surfaces `pactum contract review <run>` before `pactum contract approve <run>`; when reviewers are off, the next affordance remains approval.
- Validation commands:
  - go test ./internal/app -count=1
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app -count=1 (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
- Change summary:
  - changed files:
    - internal/app/attempt_paths_test.go
    - internal/app/cli.go
    - internal/app/commands.go
    - internal/app/config.go
    - internal/app/resolve.go
    - internal/app/run.go
  - new files:
    - internal/app/contract_review.go
    - internal/app/contract_review_test.go
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If you are not certain an issue is real after verification, do not flag it.
