# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260607_224239
- Approval: approved
- Contract hash: 266d713596ba85fbfe1e3f66354ffe086266929f76dfdd6e9e75fac69ba31693

## Goal
Add an optional cross-model review panel to the autonomous review loop. When config agents.review_panel lists two or more reviewer agents, each review round runs all of them CONCURRENTLY against the same diff and contract, merges their proposals via the existing fingerprint dedup, and reconciles severity by taking the maximum. Concurrency must be race-free under go test -race. When review_panel is empty or absent the behavior is exactly the current single-reviewer path.

## In scope
- New config field agents.review_panel as a list of reviewer agent names (yaml/json review_panel) on the AgentConfig struct in internal/agents/types.go; default empty/absent. Each name must resolve via the reviewer registry; an unknown name is a clear configuration error.
- Panel resolution precedence in the review round: an explicit --reviewer flag forces a single reviewer (panel disabled); else a non-empty agents.review_panel is the panel; else the current single cross-model reviewer fallback.
- runReviewLoopReviewRound runs each panel reviewer attempt concurrently (goroutines + barrier), then runs propose-findings SEQUENTIALLY per attempt in panel order and concatenates the proposals so the existing driver dedup merges cross-reviewer duplicates. Returns all reviewer attempt ids plus the merged proposals.
- Concurrency safety: guard the agentAttemptLifecycle shared-state sections (attempt-id allocation plus mkdir, the events and usage ledger appends, and the shared last-result.json write) with a package-level mutex so the long agent subprocess still runs lock-free. Concurrent reviewers sharing the operator live-output writer must serialize their writes through a synchronized writer (no data race, no torn output).
- Severity reconciliation: when a proposal duplicates an open finding and its severity outranks the finding (order low,medium,high,critical), upgrade the stored finding severity to the proposal severity and append a review_finding_severity_upgraded event. Same-or-lower severity leaves the finding unchanged (current behavior plus the duplicate decision).
- Audit: the loop summary reviewer identity reflects the full panel (all reviewer names), and each round summary records all reviewer attempt ids (plural).

## Out of scope
- Token estimation and cost-in-dollars overlay (separate cost-budget roadmap).
- Agreement-count as a first-class field and severity-threshold gating (require K reviewers); cross-model corroboration stays visible through the duplicate proposal decisions.
- The L2 severity-composition final pass; the clarify loop; gate changes; parallelizing the fix or gate stages (only the reviewer fan-out is concurrent).
- Changing single-reviewer behavior in any observable way.

## Paths in scope
- internal/agents/*.go
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- With agents.review_panel set to two reviewers, each review round runs both; with it empty or absent the single-reviewer path is unchanged (existing tests still pass).
- An explicit --reviewer disables the panel and runs that single reviewer.
- Two reviewers reporting the same file+line+message collapse to one finding; the second proposal is recorded as a duplicate decision.
- When two reviewers report the same finding at different severities, the kept finding carries the maximum severity and a review_finding_severity_upgraded event is recorded.
- go test -race ./... passes with the panel running reviewers concurrently (a test exercises concurrent reviewer attempts and would trip the race detector if the shared appends or attempt-id allocation were unguarded).
- make check passes (go test, go vet, deadcode, git diff --check). A clean round where all panel reviewers report nothing still terminates the loop as today.

## Validation commands
- make build
- make check
- make test-race

## Assumptions
- All panel reviewers review the same reviewer prompt and diff: build the reviewer prompt once and run N agents against it.
- The token budget stop (M12.2) already bounds the N-times per-round cost; no new budget logic is needed.
- No backward-compatibility constraints (no external users yet); additive schema and summary fields are free.
- Panel reviewer names resolve via the existing reviewer registry (the codex and claude builtins).

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
