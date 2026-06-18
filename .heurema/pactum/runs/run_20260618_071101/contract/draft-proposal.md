# Contract Draft Proposal

## Status
- Run id: run_20260618_071101
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_002
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-18T07:14:37Z

## In scope
- Extend contract plan validation so any plan task with non-empty expected_files must have at least one validation command scoped to an expected file path or an enclosing path/package segment.
- Reject unscoped per-task validation with an actionable VACUOUS_VALIDATION issue during both contract load and contract revise.
- Add pipeline.plan_review configuration using the existing stageBy scalar-or-list parsing; absent or empty by means no automated plan review.
- Add pactum plan review [run_id] [--json] as a single-pass plan DAG reviewer over contract.plan.tasks, distinct from the existing pactum review plan code-review dry-run command.
- Persist plan-review prompts, attempts, results, and structured findings under a run-local plan-review artifact area.
- Use the existing reviewer_findings structured-output capture behavior, including parse-miss detection and corrective retry, so prose-only reviewer output is never silently treated as clean.

## Out of scope
- Do not implement baseline-red validation enforcement.
- Do not implement frozen-edit detection for task validation changes.
- Do not change execute run behavior, add a topological execute loop, add tasks-state.json, context-pack resolution, execute.loop.max, single-writer leases, or --task.
- Do not change gate, code_review, memory, or existing pactum review plan behavior.
- Do not make plan_review a convergence loop and do not invoke any fixer from plan_review.

## Acceptance criteria
- A plan task with expected_files and only global validation such as go build ./... or make check is rejected with VACUOUS_VALIDATION on load and revise.
- A plan task with expected_files is accepted when at least one validation command references an expected file path, an enclosing directory, or an enclosing package segment; mixed global plus scoped validation is accepted.
- A plan task with empty expected_files remains exempt from the non-vacuous validation rule.
- Config parsing accepts pipeline.plan_review.by as a scalar or list of registered agents, allows multiple agents as a reviewer panel, normalizes empty entries away, and treats absent or empty by as a no-op.
- pactum plan review exits 0 with a clear no-plan message when the approved contract has no plan.
- With no configured plan_review.by, pactum plan review exits 0 without launching reviewer attempts.
- With configured reviewers and a plan, pactum plan review runs one single-pass reviewer panel over the plan DAG, reports the plan-review lenses, writes structured artifacts, and does not revise the contract or run a fixer.
- The plan-review prompt documents the granularity, dependency-correctness, testability, non-vacuity, and scope-fidelity lenses and requires exactly one pactum.reviewer_findings.v1alpha1 JSON block.
- A plan-review reviewer response missing the required findings block triggers the existing parse-miss/corrective-retry behavior instead of silently passing.

## Validation commands
- go test ./internal/app -run 'Contract|Plan|Config|Review'
- go test ./...
- go build ./...
- make check

## Assumptions
- The non-vacuous validation check is intentionally static and conservative; it does not execute commands or prove baseline-red behavior.
- Tests may simulate reviewers with existing helper-process patterns and must not require real agent execution.
- plan_review.loop should not be introduced as meaningful configuration because this slice is single-pass only.


## Plan (4 tasks)

### t1_non_vacuous_validation: Static non-vacuous task validation
Context:
- symbol validateContractPlan internal/app/contract.go — Owns structural plan validation at load and revise.
- internal/app/contract_plan_test.go — Existing plan DAG validation tests cover accepted and rejected plan shapes.
Expected files: internal/app/contract.go, internal/app/contract_plan_test.go
Acceptance:
- Plan validation rejects all-global validation for tasks with expected_files using VACUOUS_VALIDATION and accepts scoped, mixed, and empty-expected_files cases.
Validation:
- go test ./internal/app -run 'TestPlanDAG.*Vacuous|TestPlanDAG.*Validation|TestPlanDAGValid'

### t2_plan_review_config_cli: Plan review config and CLI entry
Context:
- symbol pipelineConfig internal/app/config.go — Defines the closed set of pipeline stages and by parsing behavior.
- symbol planCmd internal/app/cli.go — Owns the pactum plan command surface.
- internal/app/commands.go — Wires CLI commands to App methods.
Expected files: internal/app/config.go, internal/app/config_test.go, internal/app/cli.go, internal/app/commands.go, internal/app/plan.go, internal/app/plan_test.go
Acceptance:
- pipeline.plan_review.by parses as scalar or list and pactum plan review is available without changing pactum review plan.
Validation:
- go test ./internal/app -run 'TestReadConfig.*PlanReview|TestPlanReview.*CLI|TestPlanReview.*NoPlan|TestPlanReview.*NoReviewers'

### t3_plan_review_single_pass_capture: Single-pass plan-review reviewer panel
Depends on: t2_plan_review_config_cli
Context:
- internal/app/contract_review.go — Existing reviewer panel/prompt/artifact patterns to mirror without adding a convergence loop.
- symbol parseReviewerFindingBlocks internal/app/review_proposals.go — Existing structured reviewer findings parser and parse-miss behavior.
- symbol contractRunPathSet internal/app/run.go — Run-local artifact path registry.
Expected files: internal/app/plan_review.go, internal/app/plan_review_test.go, internal/app/run.go, internal/app/review_proposals.go
Acceptance:
- Configured plan reviewers run one panel pass over the contract plan, persist prompts/results/findings, use mandatory structured findings capture, and never invoke a fixer or loop.
Validation:
- go test ./internal/app -run 'TestPlanReview.*Panel|TestPlanReview.*Findings|TestPlanReview.*ParseMiss|TestPlanReview.*Prompt'

### t4_plan_review_regression_gate: Integrated regression coverage
Depends on: t1_non_vacuous_validation, t2_plan_review_config_cli, t3_plan_review_single_pass_capture
Context:
- docs/contract-plan-dag-design.md — Design source for plan-review lenses and slice boundaries.
- internal/app/config_test.go — Regression surface for pipeline config behavior.
- internal/app/plan_test.go — Regression surface for plan command behavior.
Expected files: internal/app/contract_plan_test.go, internal/app/config_test.go, internal/app/plan_test.go, internal/app/plan_review_test.go
Acceptance:
- The targeted Contract, Plan, Config, and Review test selection covers the new validation, config, CLI, prompt, artifact, no-op, and parse-miss behaviors.
Validation:
- go test ./internal/app -run 'Contract|Plan|Config|Review'
