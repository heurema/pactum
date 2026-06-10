# Contract Draft

## Goal
Add a per-project idle-timeout default (backlog timeout follow-up c). The idle window is a project property — this repository needs 900s for large slices while the built-in flag default is 10m — but today it can only be set per invocation: seven agent-running commands (execute run, review run, review fix, review loop, clarify suggest, clarify loop, contract draft) each hardcode default 10m on their --timeout flag, so the operator repeats --timeout on every run. Introduce a top-level timeouts config section with an idle key: resolution is flag (when explicitly set) -> timeouts.idle -> built-in 10m. The section name is plural deliberately, leaving room for the future absolute total cap (backlog item b) without a new section.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260610_183533
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (5 result(s))

## Clarifications
- None

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

## Open questions
- None
