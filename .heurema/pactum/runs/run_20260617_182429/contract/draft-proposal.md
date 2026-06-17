# Contract Draft Proposal

## Status
- Run id: run_20260617_182429
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-17T18:26:27Z

## In scope
- Add an optional hashed contract plan object to pactum.contract.v1alpha1 by extending draftContract with plan.tasks and the task fields id, title, depends_on, context, expected_files, acceptance, and validation.
- Preserve plan through contract load, contract show, contract show --json, contract revise, contract version/hash calculation, and approval hash behavior.
- Add structural plan validation at contract load and contract revise for duplicate task ids, missing dependency ids, dependency cycles, expected_files outside non-empty paths_in_scope, empty acceptance, empty validation, and missing or empty task ids.
- Allow contracts with no plan and plans with zero tasks, treating both as no plan for this slice.
- Add focused internal/app Go tests for a valid plan round-trip and each required rejection case.

## Out of scope
- Do not change drafter prompts or make the drafter emit plan.tasks.
- Do not change executor prompt-building, execution behavior, or add a topological execution loop.
- Do not add a plan show command.
- Do not add execute/tasks-state.json or any task-state artifact.
- Do not enforce later-slice validation-freeze, non-vacuous validation, baseline-red validation, plan review, or per-task execution semantics.

## Acceptance criteria
- A contract without plan still loads and existing contract workflows continue to pass.
- A contract with plan.tasks set to [] is accepted and treated as no plan.
- A valid plan is accepted, appears in contract show --json, changes the hashed contract content, and survives contract show/revise round-trips without data loss.
- Contract load and contract revise reject duplicate task ids with an actionable error identifying the plan task id problem.
- Contract load and contract revise reject depends_on entries that reference missing task ids.
- Contract load and contract revise reject cycles in the depends_on DAG.
- Contract load and contract revise reject expected_files outside paths_in_scope when paths_in_scope contains at least one non-empty glob.
- Contract load and contract revise reject any task with an empty acceptance list, empty validation list, or missing/empty id.
- The implementation adds tests covering valid plan acceptance plus duplicate id, missing dependency, cycle, out-of-scope expected_file, empty acceptance, empty validation, and empty id rejection.

## Validation commands
- go test ./internal/app -run 'Plan'
- go test ./internal/app -run 'Contract'
- go test ./...
- go build ./...
- make check

## Assumptions
- The goal overrides the broader design doc for this slice: plan remains optional now even though the final design expects every contract to carry a plan.
- Plan validation in this slice is structural only; context selectors, line ranges, symbols, and expected_files are not required to resolve to existing repository content.
- expected_files scope validation uses existing paths_in_scope glob semantics and does not add new paths_out_of_scope behavior unless reused helper code already enforces it.
- Actionable errors may use the existing contract revise issue format with field, code, and message; exact wording is not fixed as long as the failing plan field and reason are clear.

