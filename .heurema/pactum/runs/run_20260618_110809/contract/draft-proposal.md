# Contract Draft Proposal

## Status
- Run id: run_20260618_110809
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-18T11:11:54Z

## In scope
- Add deterministic per-task context-pack construction for one plan task, resolving path+lines selectors and symbol selectors into bounded markdown evidence with selector why text, task acceptance, frozen task validation commands, relevant constitution constraints, a SHA-256, and output at execute/context/<task_id>.md.
- Add per-task frozen-validation execution that reuses the gate validation command runner semantics: sh -c from repo root, timeout, captured stdout/stderr, exit code, and pass/fail reporting.
- Add baseline-red classification for a task validation run against the unchanged working tree, including red test validations as proceed, already-green test validations as block candidates, and green build/lint validations as advisory signals.
- Allow pipeline.execute.loop.max in config while rejecting non-default pipeline.execute.loop.patience and pipeline.execute.loop.settle at config load.
- Cover the node primitives with Go unit tests in internal/app using hand-written single-task plans and fixtures; do not run real agents.

## Out of scope
- Do not add the topological scheduler, graph-drain loop, per-node loop.Run wiring, retry feedback, tasks-state execution loop, or any task-state mutation.
- Do not add workspace snapshot/restore, clean-in-scope preconditions, whole-tree out-of-scope guards, commit/contain behavior, or per-task ACP executor attempts.
- Do not change execute run's current single-shot behavior, final gate behavior, code_review behavior, memory behavior, or agent execution behavior.
- Do not add GO/NO-GO instrumentation, out-of-scope structured blockers, parallel execution, --task execution, or contract replan behavior.

## Acceptance criteria
- For a hand-written single-task plan with path+lines and symbol context selectors, the context-pack builder writes execute/context/<task_id>.md, returns its repo-relative path and SHA-256, and produces identical bytes and SHA-256 for identical contract and working-tree inputs.
- Context packs include each selector's why text, fenced source slices tagged with path:start-end, task acceptance criteria, task validation commands, and relevant contract goal/scope/path constraints.
- Context-pack truncation is explicit: oversized slices or packs include a visible truncation marker and a pactum search pointer rather than silently dropping content.
- Per-task validation reports success only when every frozen validation command exits 0 without timeout, and reports failure with the failing command plus captured stdout/stderr/exit details.
- Baseline-red classification returns structured status, shape, command, and recommendation fields, including block for already-green test-shaped validations and the Go -run no-matching-test exit-0 trap, proceed for red test validations, and signal for green build/lint validations.
- Config loading accepts pipeline.execute.loop.max and rejects pipeline.execute.loop.patience or pipeline.execute.loop.settle when they are set to non-default values.
- All new behavior is exercised by internal/app unit tests without invoking real agents.

## Validation commands
- go test ./internal/app -run 'Plan|Context|Pack|Baseline|Validation|Config|Execute'
- go test ./...
- go build ./...
- make check

## Assumptions
- The existing project map search/code-index behavior used by pactum search --symbol is the canonical symbol-resolution mechanism for context-pack symbol selectors.
- The exact context-pack byte caps may be fixed internal constants unless an existing config surface is a better fit; the observable requirement is deterministic explicit truncation.
- The 4a primitives may remain internal Go helpers with unit-test coverage; no public CLI for running an individual node is required in this slice.
- If paths_in_scope is later added before accepting this work, it should include internal/app/** so the planned expected_files remain valid.


## Plan (4 tasks)

### context_pack: Context Pack Builder
Context:
- docs/contract-plan-dag-design.md lines 40-79 — Defines self-contained task fields, context selectors, frozen validation, and baseline-red intent.
- docs/contract-plan-dag-design.md lines 137-150 — Shows context-pack generation as the per-node evidence step before future execution.
- internal/app/run.go lines 106-124 — Existing contract plan task and context selector data structures.
- internal/app/app.go lines 237-282 — Existing search path whose symbol lookup semantics should be reused.
Expected files: internal/app/execute_context.go, internal/app/execute_context_test.go
Acceptance:
- A single-task context pack resolves path+lines and symbol selectors into deterministic markdown with why text, source slices, task acceptance, task validation, constitution constraints, explicit truncation markers, and a SHA-256.
Validation:
- rg -n "func TestPlanTaskContextPack" internal/app/execute_context_test.go && go test ./internal/app -run TestPlanTaskContextPack

### task_validation_runner: Per-Task Validation Runner
Context:
- internal/app/gate.go lines 511-625 — Existing gate validation runner behavior to reuse for per-task validation.
- internal/app/process.go lines 1-68 — Shared process result shape for captured subprocess outcomes.
- internal/app/run.go lines 110-118 — Plan task validation command field to execute.
Expected files: internal/app/gate.go, internal/app/execute_validation.go, internal/app/execute_validation_test.go
Acceptance:
- Per-task validation uses the gate command runner semantics and reports all-pass or first failure with command, stdout/stderr artifacts, exit code, and timeout.
Validation:
- rg -n "func TestPlanTaskValidationRunner" internal/app/execute_validation_test.go && go test ./internal/app -run TestPlanTaskValidationRunner

### baseline_red: Baseline-Red Classification
Depends on: task_validation_runner
Context:
- docs/contract-plan-dag-design.md lines 63-79 — Defines the validation non-vacuity and baseline-red requirements.
- internal/app/gate.go lines 511-625 — Baseline-red classification runs task validation using the shared command runner.
Expected files: internal/app/execute_validation.go, internal/app/execute_validation_test.go
Acceptance:
- Baseline classification returns status red or green, shape test or other, command, and recommendation proceed, block, or signal for red tests, green tests including no-matching Go -run, and green build/lint commands.
Validation:
- rg -n "func TestPlanTaskBaseline" internal/app/execute_validation_test.go && go test ./internal/app -run TestPlanTaskBaseline

### execute_loop_config: execute.loop Config Gate
Context:
- docs/contract-plan-dag-design.md lines 137-157 — Documents execute.loop.max as the future per-node retry cap.
- internal/app/config.go lines 216-276 — Existing pipeline loop stage validation to extend for execute.
- internal/app/config_test.go lines 200-240 — Existing loop validation tests currently rejecting execute.loop.
Expected files: internal/app/config.go, internal/app/config_test.go
Acceptance:
- Config loading accepts pipeline.execute.loop.max and rejects execute.loop.patience or execute.loop.settle when either is non-default, without changing other stage loop validation.
Validation:
- rg -n "func TestReadConfigExecuteLoopGate" internal/app/config_test.go && go test ./internal/app -run TestReadConfigExecuteLoopGate
