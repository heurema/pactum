# Task

Add the plan-DAG schema and structural validation to the contract — slice 1 of the plan-DAG arc (see docs/contract-plan-dag-design.md). SCHEMA + VALIDATION ONLY: no drafter emission, no execution change, no plan-rendering command, no tasks-state, no execution loop (all later slices).

Add an optional 'plan' object to the contract (pactum.contract.v1alpha1), INSIDE the hashed contract (extend draftContract in internal/app/run.go, do not duplicate). plan.tasks is a list; each task: { id (string, required, non-empty), title (string), depends_on ([]string of task ids), context ([] of structured evidence selectors, each {path (string), lines (string, optional e.g. "60-100"), symbol (string, optional), why (string)}), expected_files ([]string, advisory), acceptance ([]string), validation ([]string) }. A contract MAY carry no plan (optional for now); when present, plan is part of the hashed contract and is preserved through contract show/revise like the other fields.

Add structural validation of the plan, enforced at contract load AND on 'contract revise', rejecting with a clear actionable error: (a) duplicate task id; (b) a depends_on entry referencing a task id that does not exist in plan.tasks; (c) a cycle in the depends_on DAG; (d) an expected_files entry outside paths_in_scope when paths_in_scope is non-empty; (e) an empty acceptance list or empty validation list on any task; (f) an empty or missing task id. A plan with zero tasks is allowed (treat as no plan).

Do NOT: change the drafter or any prompt (no auto-emission of plan.tasks), change execute/prompt build behaviour, add a 'plan show' command, add execute/tasks-state.json, or add the topological execution loop. Those are explicitly later slices.

Add focused Go tests: a valid plan is accepted and survives the contract hash + show/revise round-trip; and one test per rejection case (duplicate id, missing dependency, cycle, out-of-scope expected_file, empty acceptance, empty validation, empty id).

Validation: go test ./internal/app -run 'Plan', go test ./internal/app -run 'Contract', go test ./..., go build ./..., make check.

Generated: 2026-06-17T18:24:29Z
