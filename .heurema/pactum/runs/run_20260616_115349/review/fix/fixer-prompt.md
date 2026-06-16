# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260616_115349/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260616_115349/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260616_115349/review/review.json, .heurema/pactum/runs/run_20260616_115349/review/findings.jsonl, .heurema/pactum/runs/run_20260616_115349/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=9 open=9 resolved=0 blocking_open=7
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: Contract review silently drops malformed structured finding output and can report no findings even when the reviewer attempted to emit findings.
    location: internal/app/contract_review.go:248
  - f_002 severity=medium category=correctness blocking=true status=open: Human next-step output after accepting a contract draft still points directly to `pactum contract approve` instead of the configured contract review command, so the required pre-approval contract review affordance is skipped on this path.
    location: internal/app/contract_draft.go:720
  - f_003 severity=medium category=quality blocking=true status=open: Missing tests for contract.reviewers config rejection paths.
    location: internal/app/config.go:277
  - f_004 severity=medium category=quality blocking=true status=open: Missing test for multiple configured contract reviewers.
    location: internal/app/contract_review_test.go:123
  - f_005 severity=medium category=quality blocking=true status=open: Human-readable findings output is not tested for configured contract reviewers.
    location: internal/app/contract_review_test.go:129
  - f_006 severity=medium category=correctness blocking=true status=open: contractReviewersConfigured silently treats readConfig failures as contract review being disabled, so nextCommandsForRun advertises pactum contract approve when config loading should fail. ContractApprove does not load config, so an invalid configured contract.reviewers entry can be bypassed through approval instead of surfacing the review/config failure.
    location: internal/app/resolve.go:303
  - f_008 severity=medium category=quality blocking=true status=open: SECURITY.md omits `pactum contract review` from the agent-launching command list, even though the new implementation launches reviewer subprocesses for configured contract reviewers. The read-only stages sentence also omits contract review, so the safety model no longer covers every unsandboxed agent-running command.
    location: SECURITY.md:11
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_007 severity=low category=quality blocking=false status=open: Contract review duplicates the existing review-loop fan-out machinery instead of sharing it.
    location: internal/app/contract_review.go:495
  - f_009 severity=low category=quality blocking=false status=open: docs/agents.md says registry-name references are limited to `--agent`, `--reviewer`, and `review.panel`, but this change adds `contract.reviewers` as another registry-backed config reference. The canonical agent/config docs are stale and do not tell users how to configure the new contract review panel.
    location: docs/agents.md:29

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

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

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
