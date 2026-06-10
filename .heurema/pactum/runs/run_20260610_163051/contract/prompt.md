# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_163051
- Approval: approved
- Contract hash: cdca4396b1a9285072b18e1a753d2f75fd49396fcf204f48712d07ad362ea488

## Goal
Completion-aware timeout finalize (backlog timeout follow-up a). Today an idle-timeout kill always yields TimedOut=true / exit -1, so a run whose agent had already finished the work — the M14.1 dogfood applied every edit, then the watchdog killed the lingering process — looks failed, and the operator must inspect the diff to learn otherwise. The agent's terminal output is a reliable completion signal: the claude CLI emits its final result envelope only at the end (is_error distinguishes success), codex emits a terminal turn.completed event, and over ACP a recorded prompt response means the turn finished. When the watchdog fires AND the captured output carries a successful terminal marker, finalize the attempt as completed-with-warning: exit code 0, TimedOut stays true for the record, a new CompletedDespiteTimeout signal on the result, and a visible warning — instead of failing the run.

## In scope
- internal/agents: a per-agent completion detector over the captured stdout (reusing/extending the existing envelope parsing in usage.go rather than duplicating it): claude — a final result envelope present with is_error false; codex — a terminal turn.completed event present; the detector returns false on partial/absent/error envelopes. RunResult gains CompletedDespiteTimeout bool. Both transports apply it: after an idle kill (timedOut true), run the detector for the agent over the attempt stdout; when it reports complete, set ExitCode 0 and CompletedDespiteTimeout true (TimedOut remains true). The ACP transport additionally treats a recorded prompt response (stop reason captured before the kill) as completion. Append a clear line to the attempt stderr and the live writer (e.g. 'idle timeout fired after the agent completed; treating as completed with warning').
- internal/app lifecycle (agent_attempt.go): the timed-out error path applies only when the result is NOT CompletedDespiteTimeout — a completed-despite-timeout attempt proceeds exactly like a success (AfterSuccess runs, artifacts written), with the warning surfaced in the human output and the flag carried in the attempt result document (json completed_despite_timeout) so the run record keeps the truth (timed_out true + completed true). The processResult plumbing carries the new flag.
- Tests: CLI transport — a runner that produces a complete claude success envelope then trips the idle timeout yields ExitCode 0 + TimedOut + CompletedDespiteTimeout (and the warning on stderr), an error envelope (is_error true) or partial output keeps today's failure; same pair for the codex turn.completed marker; lifecycle — a completed-despite-timeout result takes the success path (AfterSuccess runs, no timeout error) while a plain timeout still fails with the timeout message; detector unit tests over envelope fixtures.
- Docs: agents.md timeout paragraph documents the completed-with-warning finalize; docs/backlog.md narrows the timeout follow-up item (a is done; b absolute cap, c config defaults, d stream-json remain).

## Out of scope
- No absolute total cap (b), no config timeout defaults (c), no claude stream-json migration (d) — they stay recorded; no changes to the watchdog arming/reset mechanics, the ACP activity ticking, usage parsing semantics, or convergence/gate logic; the kill itself still happens (we finalize honestly, we do not extend the deadline).

## Paths in scope
- internal/agents/*.go
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- An idle-killed attempt whose captured output carries a successful terminal marker finalizes with ExitCode 0, TimedOut true, CompletedDespiteTimeout true, a visible warning, and the lifecycle success path; partial or error-terminal output keeps the current timed-out failure; the attempt result document records completed_despite_timeout.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the backlog item is narrowed and agents.md documents the behavior.

## Validation commands
- go build ./...
- go test ./internal/agents/... ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- A successful terminal envelope in the captured stdout is a truthful completion signal: claude emits its result envelope only at the very end, codex emits turn.completed only when the turn finished, and an ACP prompt response is recorded only after the turn — so completed-despite-timeout cannot misfire on partial work.
- Keeping TimedOut true alongside CompletedDespiteTimeout preserves the honest record (the watchdog did fire) while the exit code and lifecycle path reflect the real outcome.

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
