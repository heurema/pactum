# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260615_192013
- Approval: approved
- Contract hash: 0ba253fffb7184c25c0c600ff8dbe2649008c657bb76614226649dd07f5a0164

## Goal
Add gofmt enforcement to the build gate. Update the Makefile so that 'make check' fails when any tracked Go file is not gofmt-formatted (add a gofmt -l step that errors on non-empty output), and apply gofmt -w to fix any existing formatting drift. Do not change unrelated gate steps.

## In scope
- Update the Makefile check gate to run a gofmt listing check over tracked Go files.
- Make the gofmt check fail with a nonzero exit when gofmt -l reports any tracked Go file.
- Run gofmt -w on existing tracked Go files to remove current formatting drift.

## Out of scope
- Changing the contract goal.
- Changing unrelated Makefile targets or existing check steps such as test, vet, deadcode, or git diff --check.
- Changing CI workflow behavior except through CI's existing use of make check.
- Making non-formatting source behavior changes.

## Acceptance criteria
- make check still runs the existing test, vet, deadcode, and git diff --check gates.
- make check includes a gofmt -l based gate over tracked Go files and exits nonzero when that output is non-empty.
- After the change, all tracked Go files are gofmt-formatted and the direct gofmt listing check produces no output.
- The Makefile change is limited to adding gofmt enforcement to the build gate.

## Validation commands
- make check
- test -z "$(gofmt -l $(git ls-files '*.go'))"

## Assumptions
- Tracked Go files means the files returned by git ls-files '*.go' at validation time.
- The repository's active Go toolchain gofmt is the formatting authority.
- CI already invokes make check, so updating the Makefile is sufficient to enforce this in CI.

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
- mem_005 [fresh] score=23 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_009 [fresh] score=18 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_007 [fresh] score=15 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_001 [fresh] score=11 — Add an export command that dumps a run's full record as a single archive
- mem_014 [unknown] score=10 — Run a no-edit Pactum execution through the rebuilt Pactum binary and local co...
  reason: No tracked files

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
