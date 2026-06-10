# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_084300
- Approval: approved
- Contract hash: 6d6391e49e030005a606b02405e383a4bc3447fc7b33de119463eba9f3b60e90

## Goal
Fix an ACP idle-watchdog blind spot observed live in the M17.0 dogfood: over ACP the idle timer is reset only by streamed agent text (the acpClient writes AgentMessageChunk text through the activity-wrapped writer), so an agent that works silently through tool calls — reading files, running tools, thinking — feeds the watchdog nothing and a healthy run can be killed as idle once --timeout elapses (the M17.0 attempt's stdout sat unchanged for minutes while the agent read the repo). Every inbound ACP client callback proves the agent is alive: session updates of every kind (tool calls, tool-call updates, thoughts, plans), permission requests, and client-serviced file reads/writes must all tick the idle watchdog. Log content must NOT change — only streamed agent text keeps being written to the attempt stdout.log; ticking is signal-only.

## In scope
- internal/agents/acp_transport.go: give acpClient an explicit activity callback (e.g. activity func(), no-op when nil) and have ACPTransport.Run wire it to the existing idle-timeout activity channel (same non-blocking send semantics as activityWriter) when a timeout is armed. Invoke the callback at the top of EVERY inbound client method — SessionUpdate (all update kinds, not just AgentMessageChunk), RequestPermission, ReadTextFile, WriteTextFile, and the terminal methods — so any protocol traffic from the agent resets the idle timer.
- Keep the attempt log content unchanged: SessionUpdate still writes ONLY AgentMessageChunk text to the output writer; tool calls/thoughts/plans tick the watchdog without writing anything. The existing activityWriter wrapping stays (text writes still tick as before).
- Unit tests: a SessionUpdate carrying a tool-call (and a tool-call update, and a thought chunk) ticks the activity callback and writes nothing to the output; RequestPermission, ReadTextFile, and WriteTextFile tick it; AgentMessageChunk both ticks (via the callback and/or the activity writer) and writes the text; a nil callback is safe (no panic) for every method.
- Docs: docs/agents.md ACP transport section — note that the idle --timeout over ACP is reset by any agent protocol activity (streamed text, tool calls, thoughts, permission requests, file reads/writes), not only visible output; docs/backlog.md — record the residual observability idea (surfacing compact tool-call progress in the live output) as a separate small item, since this slice fixes the watchdog only.

## Out of scope
- No changes to what is written to stdout.log or the live output (tool-call rendering is the separate backlog item); no changes to the CLI transport, the idle-timeout implementation (startIdleTimeout), the read-only/scope-guard logic, model-pin threading, or the adapters; no timeout default changes.

## Paths in scope
- internal/agents/*.go
- docs/agents.md
- docs/backlog.md


## Acceptance criteria
- With a timeout armed, any inbound ACP client callback (every SessionUpdate kind, RequestPermission, ReadTextFile, WriteTextFile, terminal methods) resets the idle timer; silent tool-call activity can no longer be killed as idle (covered by unit tests on the acpClient activity callback).
- Attempt log content is byte-identical in behavior: only AgentMessageChunk text is written; tool calls/thoughts write nothing.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; agents.md documents the activity semantics and backlog records the observability follow-up.

## Validation commands
- go build ./...
- go test ./internal/agents/...
- go vet ./...
- go test -race ./...

## Assumptions
- Any inbound protocol traffic from the adapter is a truthful liveness signal: the idle watchdog exists to catch hung agents, not quiet ones.

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
