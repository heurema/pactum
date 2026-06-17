# Reviewer Context

## Run
- Run id: run_20260617_182429
- Run status: contract_approved

## Contract
- Goal: Add the plan-DAG schema and structural validation to the contract — slice 1 of the plan-DAG arc (see docs/contract-plan-dag-design.md). SCHEMA + VALIDATION ONLY: no drafter emission, no execution change, no plan-rendering command, no tasks-state, no execution loop (all later slices).

Add an optional 'plan' object to the contract (pactum.contract.v1alpha1), INSIDE the hashed contract (extend draftContract in internal/app/run.go, do not duplicate). plan.tasks is a list; each task: { id (string, required, non-empty), title (string), depends_on ([]string of task ids), context ([] of structured evidence selectors, each {path (string), lines (string, optional e.g. "60-100"), symbol (string, optional), why (string)}), expected_files ([]string, advisory), acceptance ([]string), validation ([]string) }. A contract MAY carry no plan (optional for now); when present, plan is part of the hashed contract and is preserved through contract show/revise like the other fields.

Add structural validation of the plan, enforced at contract load AND on 'contract revise', rejecting with a clear actionable error: (a) duplicate task id; (b) a depends_on entry referencing a task id that does not exist in plan.tasks; (c) a cycle in the depends_on DAG; (d) an expected_files entry outside paths_in_scope when paths_in_scope is non-empty; (e) an empty acceptance list or empty validation list on any task; (f) an empty or missing task id. A plan with zero tasks is allowed (treat as no plan).

Do NOT: change the drafter or any prompt (no auto-emission of plan.tasks), change execute/prompt build behaviour, add a 'plan show' command, add execute/tasks-state.json, or add the topological execution loop. Those are explicitly later slices.

Add focused Go tests: a valid plan is accepted and survives the contract hash + show/revise round-trip; and one test per rejection case (duplicate id, missing dependency, cycle, out-of-scope expected_file, empty acceptance, empty validation, empty id).

Validation: go test ./internal/app -run 'Plan', go test ./internal/app -run 'Contract', go test ./..., go build ./..., make check.
- In scope:
  - Add an optional hashed contract plan object to pactum.contract.v1alpha1 by extending draftContract with plan.tasks and the task fields id, title, depends_on, context, expected_files, acceptance, and validation.
  - Preserve plan through contract load, contract show, contract show --json, contract revise, contract version/hash calculation, and approval hash behavior.
  - Add structural plan validation at contract load and contract revise for duplicate task ids, missing dependency ids, dependency cycles, expected_files outside non-empty paths_in_scope, empty acceptance, empty validation, and missing or empty task ids.
  - Allow contracts with no plan and plans with zero tasks, treating both as no plan for this slice.
  - Add focused internal/app Go tests for a valid plan round-trip and each required rejection case.
- Out of scope:
  - Do not change drafter prompts or make the drafter emit plan.tasks.
  - Do not change executor prompt-building, execution behavior, or add a topological execution loop.
  - Do not add a plan show command.
  - Do not add execute/tasks-state.json or any task-state artifact.
  - Do not enforce later-slice validation-freeze, non-vacuous validation, baseline-red validation, plan review, or per-task execution semantics.
- Acceptance criteria:
  - A contract without plan still loads and existing contract workflows continue to pass.
  - A plan-less contract's draftContract serializes with no plan field present — neither `plan: null` nor `plan: {}` — (pointer + omitempty), producing a SHA-256 hash byte-for-byte identical to what it would have been before this slice; no previously-approved contract's approval hash is invalidated.
  - A contract with plan.tasks set to [] is accepted and treated as no plan: the Plan pointer is normalized to nil before marshaling so it is omitted entirely from the serialized draftContract (not as `plan: null` or `plan: {}`), and the resulting SHA-256 hash is byte-for-byte identical to a plan-less contract's hash.
  - A valid plan is accepted, appears in contract show --json, changes the hashed contract content, and survives contract show/revise round-trips without data loss.
  - A valid plan is preserved without data loss in plain contract show output: all task ids, titles, depends_on entries, and other task fields are present in the rendered output; this property is explicitly asserted by TestPlanDAGValid.
  - Contract load and contract revise reject duplicate task ids with an actionable error identifying the plan task id problem.
  - Contract load and contract revise reject depends_on entries that reference missing task ids.
  - Contract load and contract revise reject cycles in the depends_on DAG.
  - Contract load and contract revise reject expected_files outside paths_in_scope when paths_in_scope contains at least one non-empty glob.
  - Contract load and contract revise reject any task with an empty acceptance list, empty validation list, or missing/empty id.
  - The implementation adds Go tests in internal/app using the TestPlanDAG naming prefix: TestPlanDAGHashStability (asserts that a plan-less draftContract serializes with no 'plan' key in the JSON output and its SHA-256 matches a frozen golden value computed before this slice; also asserts that a Plan with Tasks set to [] produces the same SHA-256 as the plan-less case), TestPlanDAGValid (valid plan round-trip through contract show, contract show --json, and contract revise; asserts that all task fields — id, title, depends_on, and other task fields — are present without data loss in both the plain text output of contract show and the JSON output of contract show --json), TestPlanDAGDuplicateID (exercises both contract load and contract revise paths), TestPlanDAGMissingDep (exercises both contract load and contract revise paths), TestPlanDAGCycle (exercises both contract load and contract revise paths), TestPlanDAGScopeViolation (exercises both contract load and contract revise paths), TestPlanDAGEmptyAcceptance (exercises both contract load and contract revise paths), TestPlanDAGEmptyValidation (exercises both contract load and contract revise paths), and TestPlanDAGEmptyID (exercises both contract load and contract revise paths); all must pass under go test ./internal/app -run TestPlanDAG.
- Validation commands:
  - go test ./internal/app -run TestPlanDAG
  - go test ./internal/app -run Contract
  - go test ./...
  - go build ./...
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 5
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app -run TestPlanDAG (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./internal/app -run Contract (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: go test ./... (exit 0, timed out: false, result: gate/validation/command_003/result.json)
  - command_004: go build ./... (exit 0, timed out: false, result: gate/validation/command_004/result.json)
  - command_005: make check (exit 0, timed out: false, result: gate/validation/command_005/result.json)
- Change summary:
  - changed files:
    - docs/flow.md
    - internal/app/clarify.go
    - internal/app/contract.go
    - internal/app/run.go
  - new files:
    - internal/app/contract_plan_test.go
  - missing files:
    - none

## Existing manual review
- Review status: changes_requested
- Current findings summary: findings=9 open=9 resolved=0 blocking_open=8
- Existing findings:
  - f_001 severity=high category=correctness blocking=true status=open: contract revise can persist a plan that becomes invalid after a non-plan update because plan validation only runs when the plan field itself is revised.
  - f_002 severity=medium category=correctness blocking=true status=open: Plain contract show drops context path and lines when a context selector also has symbol.
  - f_003 severity=high category=correctness blocking=true status=open: contract revise can accept a paths_in_scope change that makes an existing plan.expected_files entry invalid, because plan validation only runs when the plan field itself is updated.
  - f_004 severity=medium category=correctness blocking=true status=open: plain contract show omits context path/lines when a context selector also has symbol, so valid plan context fields are not preserved without data loss in rendered output.
  - f_005 severity=medium category=validation blocking=true status=open: TestPlanDAGHashStability does not compare against a frozen pre-slice golden hash as required by the contract.
  - f_006 severity=medium category=quality blocking=true status=open: TestPlanDAGHashStability derives the expected hash from the current implementation instead of comparing against a frozen pre-slice SHA-256, so plan-less contract hash drift would still pass.
  - f_007 severity=medium category=quality blocking=true status=open: TestPlanDAGValid does not assert all plan fields survive show/show --json, and the plain-text depends_on assertion is ambiguous.
  - f_008 severity=medium category=quality blocking=true status=open: The scope-violation revise test does not cover changing paths_in_scope after a valid plan already exists.
  - f_009 severity=medium category=quality blocking=false status=open: User-facing contract docs omit the new contract.plan schema and structural validation rules exposed through contract revise and contract show.
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=9 rejected=0
- Existing proposals:
  - p_001 severity=high category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_005: contract revise can persist a plan that becomes invalid after a non-plan update because plan validation only runs when the plan field itself is revised.
    location: internal/app/contract.go:239
  - p_002 severity=medium category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_005: Plain contract show drops context path and lines when a context selector also has symbol.
    location: internal/app/run.go:813
  - p_003 severity=high category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: contract revise can accept a paths_in_scope change that makes an existing plan.expected_files entry invalid, because plan validation only runs when the plan field itself is updated.
    location: internal/app/contract.go:239
  - p_004 severity=medium category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: plain contract show omits context path/lines when a context selector also has symbol, so valid plan context fields are not preserved without data loss in rendered output.
    location: internal/app/run.go:813
  - p_005 severity=medium category=validation blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: TestPlanDAGHashStability does not compare against a frozen pre-slice golden hash as required by the contract.
    location: internal/app/contract_plan_test.go:40
  - p_006 severity=medium category=quality blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: TestPlanDAGHashStability derives the expected hash from the current implementation instead of comparing against a frozen pre-slice SHA-256, so plan-less contract hash drift would still pass.
    location: internal/app/contract_plan_test.go:40
  - p_007 severity=medium category=quality blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: TestPlanDAGValid does not assert all plan fields survive show/show --json, and the plain-text depends_on assertion is ambiguous.
    location: internal/app/contract_plan_test.go:150
  - p_008 severity=medium category=quality blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: The scope-violation revise test does not cover changing paths_in_scope after a valid plan already exists.
    location: internal/app/contract_plan_test.go:272
  - p_009 severity=medium category=quality blocking=false status=accepted source=reviewer_attempt attempt=reviewer_attempt_001: User-facing contract docs omit the new contract.plan schema and structural validation rules exposed through contract revise and contract show.
    location: docs/flow.md:146

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
