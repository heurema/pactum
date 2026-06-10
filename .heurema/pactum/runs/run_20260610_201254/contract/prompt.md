# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_201254
- Approval: approved
- Contract hash: 2059b89afb0eb39fe316e92c711333682fa6b87c3096298c91aecb65e597616c

## Goal
Integrate the autonomous clarify loop into task creation. Today task new only creates the run and the contract draft; the operator then separately runs clarify loop, answers what remains, and approves. An opt-in --clarify flag on task new runs the clarify loop (M17.0) immediately after the run is created: suggest -> auto-resolve high-confidence recommendations -> re-suggest, until converged, needs_human, or the round cap. The command's output then shows the created run, the loop summary, and the remaining OPEN BLOCKING questions with their recommended answers — the human answers only what automation could not resolve and proceeds to contract approve. Without --clarify, task new behaves byte-identically to today.

## In scope
- CLI (taskNewCmd in internal/app/task.go and the cli wiring): add --clarify (bool, opt-in), and the loop pass-throughs --reviewer, --max-rounds, --timeout; running agents requires confirmation, so --clarify demands --yes exactly like the other agent-running commands (a clear error when --clarify is given without --yes on a non-interactive invocation, mirroring the clarify loop's own rule). Without --clarify the new flags are inert and the behavior is unchanged.
- TaskNew: after the run is created (and its artifacts written exactly as today), when --clarify is set invoke the existing ClarifyLoop against the new run with the passed options (reusing its machinery wholesale — no duplicated loop logic); the human output renders the task-created section as today, then the loop summary, then a focused 'questions awaiting you' section listing each OPEN blocking question with its kind, confidence, and recommended answer (the human's working set), and the next-step hint becomes answering them / contract approve; the JSON output embeds the loop summary document alongside today's task response.
- Failure semantics: a clarify-loop failure after successful run creation must NOT roll back or orphan the run — the run stays created, the error is reported, and the operator can re-run pactum clarify loop on it; the ledger keeps the loop's own started/finished events (no new event types).
- Tests: task new --clarify converges via the helper-process clarifier (loop summary in output, auto-resolved answers recorded, remaining blocking questions listed with recommendation/confidence); --clarify without --yes errors; without --clarify the behavior and output are unchanged (pin against the existing task-new expectations); a loop failure leaves the run intact with the error surfaced. Reuse the clarify-loop helper-process patterns.
- Docs: agents.md / flow.md task-creation sections gain the --clarify flow (one command from task to a pre-interrogated contract); docs/backlog.md marks the Phase 1 task-new integration slice shipped and lists what remains of Phase 1.

## Out of scope
- No default-on clarify (agent execution stays an explicit opt-in); no changes to the clarify loop itself, its terminals, auto-resolve rules, or budgets; no auto-approve of the contract; no changes to clarify answer/status commands.

## Paths in scope
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- task new --clarify --yes creates the run, runs the clarify loop on it, and renders the loop summary plus the open blocking questions with kind/confidence/recommended answer; --clarify without --yes errors; without --clarify the command is byte-identical to today; a loop failure leaves the created run usable.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; docs describe the integrated flow and the backlog reflects Phase 1 progress.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Opt-in --clarify with the --yes requirement preserves the explicit-consent principle for agent execution while making the one-command task-to-interrogated-contract flow available; reusing ClarifyLoop wholesale keeps one source of truth for loop semantics.

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
