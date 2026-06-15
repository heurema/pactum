# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260615_194256
- Approval: approved
- Contract hash: 16771848dd4891208bb4280fb0697a2d9b9bf19ef792ba678f01d1515a8c2ccc

## Goal
Run contract validation commands through a shell so shell features work. In internal/app/gate.go the gate tokenizes each validation command with strings.Fields and runs it directly via exec.Command(fields[0], fields[1:]...), so commands using shell features (command substitution $(...), quotes, pipes, globs, &&) are mis-parsed and fail. Change the gate to execute each validation command through the system shell (sh -c <command>) so the shell interprets the string. Preserve timeout/context handling and existing behavior for simple commands, and add a unit test covering a shell-feature command (e.g. command substitution or a quoted argument).

## In scope
- Update internal/app/gate.go so each non-empty validation command is executed as one intact command string through sh -c, not by splitting it with strings.Fields.
- Preserve gate validation plumbing: repository-root working directory, inherited environment, stdout/stderr logs, result JSON artifacts, original command text in reports, exit-code capture, and context timeout handling.
- Add or rewrite internal/app/gate_test.go coverage for a validation command requiring shell interpretation, such as command substitution or a quoted single argument containing spaces.
- Update the existing whitespace-parsing regression test so it expects shell-preserved quoted arguments instead of strings.Fields behavior.

## Out of scope
- Changing the contract schema or adding validation command types, negative-match semantics, command allowlists, or configurable shells.
- Changing gate status rules, path-scope enforcement, execution-attempt selection, prompt or contract approval behavior, or review behavior.
- Running real Pactum executor or reviewer agents.
- Changing generated Pactum run records except for normal artifacts produced by local tests.

## Acceptance criteria
- internal/app/gate.go no longer tokenizes validation commands with strings.Fields before execution; the exact validation command text is passed as the -c argument to sh.
- A unit test named TestGateValidationCommandRunsThroughShell demonstrates that a validation command with shell syntax completes successfully during gate run and captured stdout proves the shell interpreted it correctly.
- Existing simple validation command behavior still passes, including repository-root execution, inherited environment, stdout/stderr/result artifacts, and original command text in the gate report.
- A failing validation command still records its non-zero exit code in command_001/result.json and the gate report, and the overall gate status remains failed.
- Validation timeout behavior remains context-based and continues to report timed_out with a non-success exit code when the command exceeds its timeout.

## Validation commands
- go test ./internal/app -run TestGateValidationCommandRunsThroughShell -count=1
- go test ./internal/app -run 'TestGateRunExecutesValidationCommand|TestGateRunValidationFailureWritesReportAndReturnsNonZero|TestGateValidationCommandRunsThroughShell' -count=1
- make check

## Assumptions
- The supported runtime and CI environment for gate validation has sh available on PATH.
- Using sh -c is acceptable because validation commands come from the approved contract and should now be interpreted exactly as authored.
- The existing nonEmptyValidationCommands filtering of blank validation entries remains intended behavior.

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
- fresh: 4
- stale: 0
- unknown: 1

Items:
- mem_014 [unknown] score=35 — Run a no-edit Pactum execution through the rebuilt Pactum binary and local co...
  reason: No tracked files
- mem_005 [fresh] score=35 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_001 [fresh] score=33 — Add an export command that dumps a run's full record as a single archive
- mem_007 [fresh] score=29 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_006 [fresh] score=26 — Smooth the pipeline so no command is pure ritual, then compress the agent ski...

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
