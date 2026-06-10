# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_195318
- Approval: approved
- Contract hash: 63af17aa10a879070e86a3f404be9c9981c211a83750bbe9d7c95007d75d59b0

## Goal
Apply the config philosophy — a config carries only deviations from sane defaults — to the idle timeout. M20.1 made timeouts.idle configurable but set the built-in fallback to 10m and emitted the key into both the generated default config and the workspace config. Instead: raise the built-in default to 25m (dogfooding showed 10m is too small for large agentic slices — the longest healthy runs streamed 35-45 minutes wall-clock with idle gaps well under 25m — and with ACP streaming plus activity ticking the idle timer resets on any agent activity, so a generous idle window is safe), and stop emitting the timeouts section from writeDefaultConfigIfMissing; the key remains fully supported as an override for projects that want a different window. The workspace config.yaml drops its timeouts section in the same change since the built-in now covers it.

## In scope
- internal/app: defaultIdleTimeout becomes 15m; defaultConfigFile/writeDefaultConfigIfMissing no longer emit the timeouts section (an absent section stays valid and falls back to the built-in — already the case); the seven --timeout flag help texts and any other text naming the 10m fallback say 15m. Resolution order unchanged: explicit flag -> timeouts.idle -> built-in 15m.
- The workspace .heurema/pactum/config.yaml drops timeouts: {idle: 15m} (now redundant with the built-in) in the same change.
- Tests: update the precedence/default tests and the default-config round-trip pin (the generated config must NOT contain a timeouts key; an absent section resolves to 15m; an explicit config value still overrides); update any help-text or 10m pins. Docs: agents.md timeout paragraph (15m built-in; set timeouts.idle only to deviate); backlog M20.1 mention annotated.
- internal/app: defaultIdleTimeout becomes 25m; defaultConfigFile/writeDefaultConfigIfMissing no longer emit the timeouts section (an absent section stays valid and falls back to the built-in — already the case); the seven --timeout flag help texts and any other text naming the 10m fallback say 25m. Resolution order unchanged: explicit flag -> timeouts.idle -> built-in 25m.
- The workspace .heurema/pactum/config.yaml drops timeouts: {idle: 15m} (superseded by the 25m built-in) in the same change.
- Tests: update the precedence/default tests and the default-config round-trip pin (the generated config must NOT contain a timeouts key; an absent section resolves to 25m; an explicit config value still overrides); update any help-text or 10m pins. Docs: agents.md timeout paragraph (25m built-in; set timeouts.idle only to deviate); backlog M20.1 mention annotated.

## Out of scope
- No changes to the resolution mechanics, validation, the watchdog, completion-aware finalize, or the seven command wirings beyond help text; the timeouts.idle key itself stays exactly as implemented.
- No changes to the resolution mechanics, validation, the watchdog, completion-aware finalize, or the seven command wirings beyond help text; the timeouts.idle key itself stays exactly as implemented.

## Paths in scope
- internal/app/*.go
- docs/*.md
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- The built-in idle default is 15m; the generated default config carries no timeouts key; an absent section resolves to 15m and an explicit timeouts.idle still overrides (test-covered); help texts and docs say 15m; the workspace config no longer carries the section.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes.
- The built-in idle default is 25m; the generated default config carries no timeouts key; an absent section resolves to 25m and an explicit timeouts.idle still overrides (test-covered); help texts and docs say 25m; the workspace config no longer carries the section.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- 15m is the sane idle default: the largest dogfood slices needed exactly that window, and an idle (not wall-clock) timer makes a generous default cheap — completion-aware finalize and ACP activity ticking already guard the failure modes.
- 25m is the sane idle default: an idle (not wall-clock) timer makes a generous window cheap, and completion-aware finalize plus ACP activity ticking already guard both failure modes (killed-but-complete and silent hangs).

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
