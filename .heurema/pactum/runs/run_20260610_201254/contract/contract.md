# Contract Draft

## Goal
Integrate the autonomous clarify loop into task creation. Today task new only creates the run and the contract draft; the operator then separately runs clarify loop, answers what remains, and approves. An opt-in --clarify flag on task new runs the clarify loop (M17.0) immediately after the run is created: suggest -> auto-resolve high-confidence recommendations -> re-suggest, until converged, needs_human, or the round cap. The command's output then shows the created run, the loop summary, and the remaining OPEN BLOCKING questions with their recommended answers — the human answers only what automation could not resolve and proceeds to contract approve. Without --clarify, task new behaves byte-identically to today.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260610_201254
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
