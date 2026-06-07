# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260607_154501
- Approval: approved
- Contract hash: 770158ac75c1c9d078661b1643f91165010b5107777c059c04b92beace058117

## Goal
Make path-scope enforcement BLOCKING by default. When a contract declares path globs (paths_in_scope / paths_out_of_scope), a changed or new file outside that scope makes the gate FAIL (status 'failed'), not just an advisory warning. A new config 'gate.scope_enforcement' (default 'block') can downgrade to 'warn' (the current M11.5 advisory behavior). When no path globs are declared, scope is not checked (unchanged)

## In scope
- Config: add a 'gate' section with 'scope_enforcement: block | warn', default 'block' (empty/missing => block; invalid value => error or default to block). Add it to the config struct and the default config that 'init' writes
- gate.go: buildGateScopeReport takes the enforcement mode. On violations (undeclared or out_of_scope non-empty): set scope.Status = 'blocked' when mode is block, 'warnings' when mode is warn. Add gateSummary.ScopeBlocked (true when scope != nil and Status == 'blocked'); gateStatus returns 'failed' when ScopeBlocked. warn mode leaves the gate status unaffected (current behavior). No path globs => nil scope => unchanged
- Gate output (human + JSON) clearly reflects a blocked scope: the scope section shows status blocked and the offending files, and the overall gate status is failed
- Tests: block (default) + out-of-scope/undeclared change => gate status failed; warn + same violation => gate passes with advisory warnings (M11.5 behavior); block + in-scope clean => passed; no path globs => no scope section and gate unaffected; config parsing of block/warn/empty(->block)
- Docs: update docs/flow.md (gate scope enforcement modes) and docs/backlog.md (resolve the blocking path-scope item; note it composes with the M11.8 gate_failed loop terminal)

## Out of scope
- Per-contract scope-enforcement override (config-level only this slice)
- Changing the path-glob matcher (M11.5) or the .heurema VCS policy (M11.9)
- Adding loop code for the blocking case — it composes with the existing M11.8 gate_failed terminal; just verify, do not duplicate
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Paths in scope
- internal/app/**
- docs/**


## Acceptance criteria
- By default (no gate config), a contract with path globs whose run changed an out-of-scope or undeclared file => gate status 'failed'; the scope section reports status 'blocked' with the offending files
- gate.scope_enforcement: warn => the same violation is advisory (gate not failed), matching M11.5
- No path globs declared => gate behaves exactly as before (no scope section, no effect on status)
- In the review loop, a blocking scope failure terminates as gate_failed (composition with M11.8), not a generic error
- make check is green (incl. deadcode); go test -race ./... is clean

## Validation commands
- make check

## Assumptions
TBD

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
