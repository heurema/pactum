# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260618_110809
- Approval: approved
- Contract hash: 2500bb1bbf2cc7bec4ebd8410d1403f6e533f9c05e4104ded8793c0743082f24

## Goal
Plan-DAG slice 4a: the per-task execution NODE primitives, WITHOUT the topological loop. This is slice 4a of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 4 and the "Slice 4, finalized" section). Build and unit-test the building blocks the 4b loop will drive, on a hand-written single-task plan. NO topological scheduler, NO workspace snapshot/restore, NO commits, NO tasks-state execution loop, NO agent execution change (all slice 4b).

Context: slice 1 added plan.tasks[] to the hashed contract + structural validation; slice 2 has the drafter emit plans + `pactum plan show`; slice 3 added non-vacuous validation + single-pass `plan_review`. Each plan task has: id, title, depends_on[], context[] (evidence selectors: each with path + optional lines like "60-100" and/or symbol, plus why), expected_files[] (advisory), acceptance[], validation[] (frozen commands). This slice builds the deterministic node primitives.

In scope:
1. Context-pack builder: a deterministic function that resolves one task's context[] selectors into an evidence pack (markdown). For a path+lines selector, read the repo-relative file and emit the line-range slice in a fenced block tagged path:start-end; for a symbol selector, resolve the symbol to a path:start-end via the existing search/code-index (the same mechanism `pactum search --symbol` uses) and emit that slice; carry each selector's `why`. Append the task's acceptance, the task's frozen validation commands, and the relevant constitution constraints (goal, scope, paths_in_scope). Bound the output: cap each slice and the whole pack with an explicit truncation marker and a `pactum search` pointer (never silently truncate). The pack is deterministic given (contract, working tree). Compute its SHA-256. Write it to execute/context/<task_id>.md and return {path, sha256}. Use os.Lstat/read for file content; do not go through the ACP text-write path.

2. Per-task frozen-validation runner: run a task's validation[] commands via the same mechanism the gate already uses (sh -c with cwd=root, timeout, captured stdout/stderr/exit) and report pass (all commands exit 0) or fail with the failing command + output. Reuse the gate's command-runner; do not invent a new one.

3. Baseline-red checker: run a task's validation against the UNCHANGED working tree (before any executor attempt) and classify the result. A validation that is test-shaped (a test command, e.g. `go test ... -run ...`) and already passes (green) is reported as baseline_green = a hard block candidate, since a check that passes before the work is not a real check (this also catches the Go `go test -run NoSuchTest` exit-0 trap — a -run pattern matching no test exits 0). A build/lint validation that is green-before is reported as a softer signal (advisory, recorded), not an auto-block. Return a structured baseline result {status: red|green, shape: test|other, command, recommendation: proceed|block|signal}. This is a pure classification step in 4a; the 4b loop decides what to do with it.

4. execute.loop config gate: make the existing-but-unused pipeline.execute.loop slot valid and bounded. Add `execute` to the set of stages that accept a loop block, but for execute REJECT non-default patience/settle at config load (a binary node only has a meaningful `max`; settle is fixed at 1 and patience at 0). Only `execute.loop.max` is configurable; a config with execute.loop.patience or execute.loop.settle set to a non-default value is a clear load error.

5. Tests (Go, internal/app): context-pack resolves path+lines and symbol selectors deterministically (same input -> same bytes -> same sha256), includes acceptance/validation/why, and truncates with the marker when over the cap; the validation runner reports pass and fail correctly; baseline-red classifies a red test validation as proceed, an already-green test validation as block (including the `-run NoSuchTest` exit-0 case), and a green build/lint as signal; the execute.loop config gate accepts execute.loop.max and rejects execute.loop.patience/settle. Use hand-written single-task plans / fixtures; do not run real agents.

Out of scope (slice 4b and later): the topological scheduler / graph-drain; per-node loop.Run wiring + retry-with-feedback; the workspace content snapshot + restore-on-block + the two guards (whole-tree out-of-scope detector, clean-in-scope precondition); tasks-state.json execution state + the execution loop that mutates it; per-task ACP attempts / actually running the executor over the DAG; commit/contain semantics; the drain -> constitution gate integration; GO/NO-GO instrumentation; out-of-scope structured blockers. Do NOT change execute run's current single-shot behavior, gate, code_review, or memory.

Validation: go test ./internal/app -run 'Plan|Context|Pack|Baseline|Validation|Config|Execute', go test ./..., go build ./..., make check.

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
- Context-pack selector resolution degrades gracefully and deterministically: a path selector whose file is missing, whose line range is out of range/invalid, or which is binary/unreadable emits an explicit note marker (with a pactum search pointer) in the pack rather than crashing or silently skipping; the pack is still produced.
- Context-pack symbol selectors handle non-unique resolution deterministically: a symbol with no match emits an explicit no-match note plus a pactum search pointer; a symbol with multiple matches resolves in deterministic path/range order and notes the ambiguity, rather than crashing or producing nondeterministic output.
- Baseline-red classification is defined for a task with multiple validation commands: each command is classified individually and the task-level recommendation is conservative — a task is block when any test-shaped command is already green, proceed only when every test-shaped command is red, and green build/lint commands are recorded as signal; this aggregation is covered by a test.

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
- mem_022 [fresh] score=87 — Plan-DAG slice 2: the contract drafter emits an optional plan.tasks[] DAG, an...
- mem_005 [fresh] score=80 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_021 [fresh] score=79 — Make pactum's code-review loop never silently drop reviewer findings, and rec...
- mem_017 [fresh] score=79 — Rework the pactum config to the new pipeline shape and wire it through the ex...
- mem_020 [fresh] score=78 — Add the plan-DAG schema and structural validation to the contract — slice 1 o...

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
