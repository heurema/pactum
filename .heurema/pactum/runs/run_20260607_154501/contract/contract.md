# Contract Draft

## Goal
Make path-scope enforcement BLOCKING by default. When a contract declares path globs (paths_in_scope / paths_out_of_scope), a changed or new file outside that scope makes the gate FAIL (status 'failed'), not just an advisory warning. A new config 'gate.scope_enforcement' (default 'block') can downgrade to 'warn' (the current M11.5 advisory behavior). When no path globs are declared, scope is not checked (unchanged)

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260607_154501
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
