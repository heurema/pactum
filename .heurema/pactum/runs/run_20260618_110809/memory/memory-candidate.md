# Memory Candidate

## Run
- Run id: run_20260618_110809
- Source: deterministic

## Contract
- Goal: Plan-DAG slice 4a: the per-task execution NODE primitives, WITHOUT the topological loop. This is slice 4a of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 4 and the "Slice 4, finalized" section). Build and unit-test the building blocks the 4b loop will drive, on a hand-written single-task plan. NO topological scheduler, NO workspace snapshot/restore, NO commits, NO tasks-state execution loop, NO agent execution change (all slice 4b).

Context: slice 1 added plan.tasks[] to the hashed contract + structural validation; slice 2 has the drafter emit plans + `pactum plan show`; slice 3 added non-vacuous validation + single-pass `plan_review`. Each plan task has: id, title, depends_on[], context[] (evidence selectors: each with path + optional lines like "60-100" and/or symbol, plus why), expected_files[] (advisory), acceptance[], validation[] (frozen commands). This slice builds the deterministic node primitives.

In scope:
1. Context-pack builder: a deterministic function that resolves one task's context[] selectors into an evidence pack (markdown). For a path+lines selector, read the repo-relative file and emit the line-range slice in a fenced block tagged path:start-end; for a symbol selector, resolve the symbol to a path:start-end via the existing search/code-index (the same mechanism `pactum search --symbol` uses) and emit that slice; carry each selector's `why`. Append the task's acceptance, the task's frozen validation commands, and the relevant constitution constraints (goal, scope, paths_in_scope). Bound the output: cap each slice and the whole pack with an explicit truncation marker and a `pactum search` pointer (never silently truncate). The pack is deterministic given (contract, working tree). Compute its SHA-256. Write it to execute/context/<task_id>.md and return {path, sha256}. Use os.Lstat/read for file content; do not go through the ACP text-write path.

2. Per-task frozen-validation runner: run a task's validation[] commands via the same mechanism the gate already uses (sh -c with cwd=root, timeout, captured stdout/stderr/exit) and report pass (all commands exit 0) or fail with the failing command + output. Reuse the gate's command-runner; do not invent a new one.

3. Baseline-red checker: run a task's validation against the UNCHANGED working tree (before any executor attempt) and classify the result. A validation that is test-shaped (a test command, e.g. `go test ... -run ...`) and already passes (green) is reported as baseline_green = a hard block candidate, since a check that passes before the work is not a real check (this also catches the Go `go test -run NoSuchTest` exit-0 trap — a -run pattern matching no test exits 0). A build/lint validation that is green-before is reported as a softer signal (advisory, recorded), not an auto-block. Return a structured baseline result {status: red|green, shape: test|other, command, recommendation: proceed|block|signal}. This is a pure classification step in 4a; the 4b loop decides what to do with it.

4. execute.loop config gate: make the existing-but-unused pipeline.execute.loop slot valid and bounded. Add `execute` to the set of stages that accept a loop block, but for execute REJECT non-default patience/settle at config load (a binary node only has a meaningful `max`; settle is fixed at 1 and patience at 0). Only `execute.loop.max` is configurable; a config with execute.loop.patience or execute.loop.settle set to a non-default value is a clear load error.

5. Tests (Go, internal/app): context-pack resolves path+lines and symbol selectors deterministically (same input -> same bytes -> same sha256), includes acceptance/validation/why, and truncates with the marker when over the cap; the validation runner reports pass and fail correctly; baseline-red classifies a red test validation as proceed, an already-green test validation as block (including the `-run NoSuchTest` exit-0 case), and a green build/lint as signal; the execute.loop config gate accepts execute.loop.max and rejects execute.loop.patience/settle. Use hand-written single-task plans / fixtures; do not run real agents.

Out of scope (slice 4b and later): the topological scheduler / graph-drain; per-node loop.Run wiring + retry-with-feedback; the workspace content snapshot + restore-on-block + the two guards (whole-tree out-of-scope detector, clean-in-scope precondition); tasks-state.json execution state + the execution loop that mutates it; per-task ACP attempts / actually running the executor over the DAG; commit/contain semantics; the drain -> constitution gate integration; GO/NO-GO instrumentation; out-of-scope structured blockers. Do NOT change execute run's current single-shot behavior, gate, code_review, or memory.

Validation: go test ./internal/app -run 'Plan|Context|Pack|Baseline|Validation|Config|Execute', go test ./..., go build ./..., make check.
- In scope:
  - Add deterministic per-task context-pack construction for one plan task, resolving path+lines selectors and symbol selectors into bounded markdown evidence with selector why text, task acceptance, frozen task validation commands, relevant constitution constraints, a SHA-256, and output at execute/context/<task_id>.md.
  - Add per-task frozen-validation execution that reuses the gate validation command runner semantics: sh -c from repo root, timeout, captured stdout/stderr, exit code, and pass/fail reporting.
  - Add baseline-red classification for a task validation run against the unchanged working tree, including red test validations as proceed, already-green test validations as block candidates, and green build/lint validations as advisory signals.
  - Allow pipeline.execute.loop.max in config while rejecting non-default pipeline.execute.loop.patience and pipeline.execute.loop.settle at config load.
  - Cover the node primitives with Go unit tests in internal/app using hand-written single-task plans and fixtures; do not run real agents.
- Out of scope:
  - Do not add the topological scheduler, graph-drain loop, per-node loop.Run wiring, retry feedback, tasks-state execution loop, or any task-state mutation.
  - Do not add workspace snapshot/restore, clean-in-scope preconditions, whole-tree out-of-scope guards, commit/contain behavior, or per-task ACP executor attempts.
  - Do not change execute run's current single-shot behavior, final gate behavior, code_review behavior, memory behavior, or agent execution behavior.
  - Do not add GO/NO-GO instrumentation, out-of-scope structured blockers, parallel execution, --task execution, or contract replan behavior.
- Acceptance criteria:
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
- Validation commands:
  - go test ./internal/app -run 'Plan|Context|Pack|Baseline|Validation|Config|Execute'
  - go test ./...
  - go build ./...
  - make check

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - docs/flow.md
  - internal/app/cli.go
  - internal/app/commands.go
  - internal/app/config.go
  - internal/app/config_test.go
  - internal/app/gate.go
- New files:
  - internal/app/execute_node.go
  - internal/app/execute_node_test.go
- Missing files: none

## Clarifications
- None

## Review Decisions
- f_001 [high] resolved internal/app/execute_node.go:58: Context-pack output paths are built from the raw task id, so a valid task id containing path separators or '..' can write the pack outside execute/context/<task_id>.md.
  Resolution: buildContextPack now strips task.ID to its base component via filepath.Base(filepath.Clean(task.ID)) before constructing the pack file path, preventing path separators or '..' in a task ID from writing outside execute/context/.
- f_002 [high] resolved internal/app/execute_node.go:138: Path context selectors are not constrained to repo-relative paths before reading, so a selector like ../outside.txt can resolve evidence from outside the working tree.
  Resolution: writePathSelector now computes the cleaned absolute path and rejects it with an explicit note marker when it does not have the workspace root as a prefix, preventing selectors like '../outside.txt' from being read.
- f_003 [medium] resolved internal/app/config.go:261: execute.loop.settle rejects the fixed default value of 1 even though the contract says only non-default settle values should be rejected.
  Resolution: config.go check changed from `Settle != 0` to `Settle > 1`: settle=0 (absent) and settle=1 (the fixed default) are both accepted; only values above 1 are rejected. TestConfigExecuteLoopSettleRejected updated to use settle:2, and a new TestConfigExecuteLoopSettleDefaultIsValid test confirms settle:1 is accepted.
- f_004 [medium] resolved internal/app/execute_node.go:138: Context path selectors are joined directly with the repo root, so a selector such as `../outside.txt` can read and include files outside the workspace instead of being treated as an invalid repo-relative selector.
  Resolution: Same root-escape check added in writePathSelector (same location as f_002). f_002 and f_004 describe the same code site; the single fix addresses both.
- f_005 [medium] resolved internal/app/execute_node.go:319: The per-task validation runner invents a new subprocess runner instead of reusing the existing gate validation command runner.
  Resolution: Extracted runShellCommandIO into gate.go as a shared primitive (sh -c, process-group, timeout, exit-code extraction). runValidationCmd in execute_node.go now delegates to it with memory buffers; runGateValidationCommand in gate.go does the same with file writers. The duplicate execution logic is gone.
- f_006 [medium] resolved internal/app/execute_node.go:495: The new plan context command/orchestration path is not covered by tests.
  Resolution: Added TestPlanContextWritesPackAndReturnsJSON in execute_node_test.go. It sets up a full workspace via setupContractRun, adds a single-task plan via contract revise, calls app.Run(["plan", "context", runID, "--json"]), and asserts the JSON response, the written context pack file, baseline recommendation, and validation pass status.
- f_007 [medium] resolved internal/app/execute_node.go:346: The validation runner timeout path is untested.
  Resolution: Added TestTaskValidationTimeout in execute_node_test.go. It calls runTaskValidation with a 100ms timeout and 'sleep 60', then asserts TimedOut=true and Passed=false on the result.
- f_008 [low] resolved internal/app/execute_node.go:157: The binary/unreadable context selector fallback required by the contract is not tested.
  Resolution: Fixed: added TestContextPackBinaryFile (null-byte file → context pack emits the 'binary' note + pactum search pointer), covering the isBinaryContent fallback path.
- f_009 [medium] resolved internal/app/execute_node.go:319: Per-task validation uses a newly hand-rolled command runner instead of reusing the existing gate validation runner, leaving two implementations of the same shell/cwd/env/timeout/process-group/exit-code behavior.
  Resolution: Same fix as f_005 — both findings point to the same duplicate runner. runShellCommandIO is the shared primitive now used by both gate and execute-node paths.
- f_010 [medium] resolved internal/app/execute_node.go:529: PlanContext runs each task's validation commands twice in sequence on the same working tree.
  Resolution: PlanContext no longer calls runTaskValidation separately. runBaselineCheck now delegates to runTaskValidation internally (one execution), and PlanContext derives ValidationNow from the baseline result via taskValidationResultFromBaseline. The Stdout/Stderr fields were added to baselineCommandResult so the derived result carries full output.
- f_011 [medium] resolved docs/flow.md:18: The workflow documentation does not mention the new user-facing `pactum plan context` command or its generated context-pack artifacts.
  Resolution: Fixed: documented 'pactum plan context' in docs/flow.md — added to the Stages command table and a paragraph describing context-pack building, truncation/degrade behavior, and baseline-red.
- Proposal summary: pending=0 accepted=11 rejected=0

## Reusable Project Knowledge
- scope: in scope: Add deterministic per-task context-pack construction for one plan task, resolving path+lines selectors and symbol selectors into bounded markdown evidence with selector why text, task acceptance, frozen task validation commands, relevant constitution constraints, a SHA-256, and output at execute/context/<task_id>.md.
- scope: in scope: Add per-task frozen-validation execution that reuses the gate validation command runner semantics: sh -c from repo root, timeout, captured stdout/stderr, exit code, and pass/fail reporting.
- scope: in scope: Add baseline-red classification for a task validation run against the unchanged working tree, including red test validations as proceed, already-green test validations as block candidates, and green build/lint validations as advisory signals.
- scope: in scope: Allow pipeline.execute.loop.max in config while rejecting non-default pipeline.execute.loop.patience and pipeline.execute.loop.settle at config load.
- scope: in scope: Cover the node primitives with Go unit tests in internal/app using hand-written single-task plans and fixtures; do not run real agents.
- scope: out of scope: Do not add the topological scheduler, graph-drain loop, per-node loop.Run wiring, retry feedback, tasks-state execution loop, or any task-state mutation.
- scope: out of scope: Do not add workspace snapshot/restore, clean-in-scope preconditions, whole-tree out-of-scope guards, commit/contain behavior, or per-task ACP executor attempts.
- scope: out of scope: Do not change execute run's current single-shot behavior, final gate behavior, code_review behavior, memory behavior, or agent execution behavior.
- scope: out of scope: Do not add GO/NO-GO instrumentation, out-of-scope structured blockers, parallel execution, --task execution, or contract replan behavior.
- review_resolution: f_001 resolved: Context-pack output paths are built from the raw task id, so a valid task id containing path separators or '..' can write the pack outside execute/context/<task_id>.md.; resolution: buildContextPack now strips task.ID to its base component via filepath.Base(filepath.Clean(task.ID)) before constructing the pack file path, preventing path separators or '..' in a task ID from writing outside execute/context/.
- review_resolution: f_002 resolved: Path context selectors are not constrained to repo-relative paths before reading, so a selector like ../outside.txt can resolve evidence from outside the working tree.; resolution: writePathSelector now computes the cleaned absolute path and rejects it with an explicit note marker when it does not have the workspace root as a prefix, preventing selectors like '../outside.txt' from being read.
- review_resolution: f_003 resolved: execute.loop.settle rejects the fixed default value of 1 even though the contract says only non-default settle values should be rejected.; resolution: config.go check changed from `Settle != 0` to `Settle > 1`: settle=0 (absent) and settle=1 (the fixed default) are both accepted; only values above 1 are rejected. TestConfigExecuteLoopSettleRejected updated to use settle:2, and a new TestConfigExecuteLoopSettleDefaultIsValid test confirms settle:1 is accepted.
- review_resolution: f_004 resolved: Context path selectors are joined directly with the repo root, so a selector such as `../outside.txt` can read and include files outside the workspace instead of being treated as an invalid repo-relative selector.; resolution: Same root-escape check added in writePathSelector (same location as f_002). f_002 and f_004 describe the same code site; the single fix addresses both.
- review_resolution: f_005 resolved: The per-task validation runner invents a new subprocess runner instead of reusing the existing gate validation command runner.; resolution: Extracted runShellCommandIO into gate.go as a shared primitive (sh -c, process-group, timeout, exit-code extraction). runValidationCmd in execute_node.go now delegates to it with memory buffers; runGateValidationCommand in gate.go does the same with file writers. The duplicate execution logic is gone.
- review_resolution: f_006 resolved: The new plan context command/orchestration path is not covered by tests.; resolution: Added TestPlanContextWritesPackAndReturnsJSON in execute_node_test.go. It sets up a full workspace via setupContractRun, adds a single-task plan via contract revise, calls app.Run(["plan", "context", runID, "--json"]), and asserts the JSON response, the written context pack file, baseline recommendation, and validation pass status.
- review_resolution: f_007 resolved: The validation runner timeout path is untested.; resolution: Added TestTaskValidationTimeout in execute_node_test.go. It calls runTaskValidation with a 100ms timeout and 'sleep 60', then asserts TimedOut=true and Passed=false on the result.
- review_resolution: f_008 resolved: The binary/unreadable context selector fallback required by the contract is not tested.; resolution: Fixed: added TestContextPackBinaryFile (null-byte file → context pack emits the 'binary' note + pactum search pointer), covering the isBinaryContent fallback path.
- review_resolution: f_009 resolved: Per-task validation uses a newly hand-rolled command runner instead of reusing the existing gate validation runner, leaving two implementations of the same shell/cwd/env/timeout/process-group/exit-code behavior.; resolution: Same fix as f_005 — both findings point to the same duplicate runner. runShellCommandIO is the shared primitive now used by both gate and execute-node paths.
- review_resolution: f_010 resolved: PlanContext runs each task's validation commands twice in sequence on the same working tree.; resolution: PlanContext no longer calls runTaskValidation separately. runBaselineCheck now delegates to runTaskValidation internally (one execution), and PlanContext derives ValidationNow from the baseline result via taskValidationResultFromBaseline. The Stdout/Stderr fields were added to baselineCommandResult so the derived result carries full output.
- review_resolution: f_011 resolved: The workflow documentation does not mention the new user-facing `pactum plan context` command or its generated context-pack artifacts.; resolution: Fixed: documented 'pactum plan context' in docs/flow.md — added to the Stages command table and a paragraph describing context-pack building, truncation/degrade behavior, and baseline-red.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- review_resolution: proposal p_007 accepted as f_007
- review_resolution: proposal p_008 accepted as f_008
- review_resolution: proposal p_009 accepted as f_009
- review_resolution: proposal p_010 accepted as f_010
- review_resolution: proposal p_011 accepted as f_011
- validation: go test ./internal/app -run 'Plan|Context|Pack|Baseline|Validation|Config|Execute' passed
- validation: go test ./... passed
- validation: go build ./... passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
