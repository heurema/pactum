# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_183533
- Approval: approved
- Contract hash: 5260cbe6d94828388e265b3ef206635456292aee6aa9df63e9a9acc611513957

## Goal
Add a per-project idle-timeout default (backlog timeout follow-up c). The idle window is a project property — this repository needs 900s for large slices while the built-in flag default is 10m — but today it can only be set per invocation: seven agent-running commands (execute run, review run, review fix, review loop, clarify suggest, clarify loop, contract draft) each hardcode default 10m on their --timeout flag, so the operator repeats --timeout on every run. Introduce a top-level timeouts config section with an idle key: resolution is flag (when explicitly set) -> timeouts.idle -> built-in 10m. The section name is plural deliberately, leaving room for the future absolute total cap (backlog item b) without a new section.

## In scope
- Config (internal/app/config.go): a timeouts section with idle as a duration string (yaml idle, e.g. 15m, parsed with time.ParseDuration); validation at readConfig — a non-empty value must parse as a positive duration, with an error naming the key and the value; empty/absent means no project default. The generated default config includes timeouts: {idle: 10m} so the key is discoverable. Strict parsing covers the new section automatically.
- Resolution: one helper (e.g. resolveIdleTimeout) implementing flag-explicit -> config -> built-in 10m. The seven CLI --timeout flags change their kong default from 10m to 0 (0 = not set) with help text saying the default comes from timeouts.idle in the workspace config (10m when unset); each command resolves through the helper at its entry point; an explicit non-positive --timeout is a clear error (the flag must be positive when given).
- The workspace .heurema/pactum/config.yaml gains timeouts: {idle: 15m} in the same change (additive — old configs without the section still parse), so the dogfood stops carrying --timeout 900s on every command.
- Tests: config validation (valid duration accepted; garbage and non-positive values error naming timeouts.idle); resolution precedence unit-covered (explicit flag wins; config beats built-in; built-in 10m when both unset); at least one command-level test proving an omitted --timeout picks up the config value (assert via the observable the command exposes, e.g. the recorded attempt request or the resolved-timeout output) and one proving an explicit flag still overrides; update any tests pinned to the old kong default.
- Docs: agents.md timeout paragraph documents the resolution order and the config key; docs/backlog.md narrows the timeout follow-up (c done; b absolute cap and d stream-json remain).

## Out of scope
- No absolute total cap (b), no stream-json migration (d), no per-stage timeout keys (one project-wide idle default; per-stage windows can come later if ever needed); no changes to the watchdog mechanics, activity ticking, or completion-aware finalize.

## Paths in scope
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- timeouts.idle exists in the strict config with positive-duration validation; all seven agent-running commands resolve flag -> config -> 10m through one helper; an explicit flag overrides, an omitted flag uses the config value, and both are test-covered at the command level; the generated default config carries timeouts: {idle: 10m} and round-trips.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the workspace config carries idle: 15m; agents.md and the backlog reflect the change.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- A kong flag default of 0 cleanly distinguishes explicitly-set from unset (mirroring how --max-rounds resolves through the config), and a single project-wide idle default fits the minimal-config philosophy.

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
- total: 0
- fresh: 0
- stale: 0
- unknown: 0

Items:
- none

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
