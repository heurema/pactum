# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260617_182429
- Approval: approved
- Contract hash: 8d26748fd3ff3b25c7115aa0d7cc08a75b58a371a85af1473acac5ba1919c6a8

## Goal
Add the plan-DAG schema and structural validation to the contract — slice 1 of the plan-DAG arc (see docs/contract-plan-dag-design.md). SCHEMA + VALIDATION ONLY: no drafter emission, no execution change, no plan-rendering command, no tasks-state, no execution loop (all later slices).

Add an optional 'plan' object to the contract (pactum.contract.v1alpha1), INSIDE the hashed contract (extend draftContract in internal/app/run.go, do not duplicate). plan.tasks is a list; each task: { id (string, required, non-empty), title (string), depends_on ([]string of task ids), context ([] of structured evidence selectors, each {path (string), lines (string, optional e.g. "60-100"), symbol (string, optional), why (string)}), expected_files ([]string, advisory), acceptance ([]string), validation ([]string) }. A contract MAY carry no plan (optional for now); when present, plan is part of the hashed contract and is preserved through contract show/revise like the other fields.

Add structural validation of the plan, enforced at contract load AND on 'contract revise', rejecting with a clear actionable error: (a) duplicate task id; (b) a depends_on entry referencing a task id that does not exist in plan.tasks; (c) a cycle in the depends_on DAG; (d) an expected_files entry outside paths_in_scope when paths_in_scope is non-empty; (e) an empty acceptance list or empty validation list on any task; (f) an empty or missing task id. A plan with zero tasks is allowed (treat as no plan).

Do NOT: change the drafter or any prompt (no auto-emission of plan.tasks), change execute/prompt build behaviour, add a 'plan show' command, add execute/tasks-state.json, or add the topological execution loop. Those are explicitly later slices.

Add focused Go tests: a valid plan is accepted and survives the contract hash + show/revise round-trip; and one test per rejection case (duplicate id, missing dependency, cycle, out-of-scope expected_file, empty acceptance, empty validation, empty id).

Validation: go test ./internal/app -run 'Plan', go test ./internal/app -run 'Contract', go test ./..., go build ./..., make check.

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

## Validation commands
- go test ./internal/app -run TestPlanDAG
- go test ./internal/app -run Contract
- go test ./...
- go build ./...
- make check

## Assumptions
- The goal overrides the broader design doc for this slice: plan remains optional now even though the final design expects every contract to carry a plan.
- Plan validation in this slice is structural only; context selectors, line ranges, symbols, and expected_files are not required to resolve to existing repository content.
- expected_files scope validation uses existing paths_in_scope glob semantics and does not add new paths_out_of_scope behavior unless reused helper code already enforces it.
- Actionable errors may use the existing contract revise issue format with field, code, and message; exact wording is not fixed as long as the failing plan field and reason are clear.
- When plan is absent OR when plan.tasks is an empty slice (a non-nil Plan with zero Tasks), the Plan pointer must be normalized to nil before marshaling so it is omitted entirely from the serialized draftContract — not as `plan: null` or `plan: {}` — ensuring both plan-less and empty-tasks contracts produce a SHA-256 hash identical to the pre-slice plan-less hash and no previously-approved approval hash is invalidated.

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
- mem_009 [fresh] score=63 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_007 [fresh] score=48 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_016 [fresh] score=44 — Port the code-review loop (internal/app/review_loop.go) onto the existing int...
- mem_019 [fresh] score=42 — Surface cache reuse and effective cost in the existing 'pactum usage' command...
- mem_005 [fresh] score=40 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...

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
