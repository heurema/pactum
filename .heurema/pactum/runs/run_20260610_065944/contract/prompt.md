# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_065944
- Approval: approved
- Contract hash: 5fe5f0a3c3bb766a1bac25d3ca488302bdd59ee973da5e761517820c6644a6d8

## Goal
Make ACP the default agent transport. The two gaps that blocked the flip are closed (M16.1: model pins reach the adapters; read-only stages enforced per leg), and ACP is strictly better where it differs: it streams the agent's output (the CLI claude -p runs repeatedly tripped the idle watchdog AFTER finishing work because -p buffers until the end), and it gives write stages the real-time contract path-scope guard the CLI cannot have. Flip acpTransportEnabled so the default is ACP; PACTUM_AGENT_TRANSPORT=cli remains the debug escape hatch (cli selects the one-shot CLI transport; acp or empty selects ACP; any other value falls back to the ACP default — the env var is a debug hatch, not config).

## In scope
- internal/app/app.go acpTransportEnabled (rename if a clearer name fits, e.g. the function or its comment should state ACP is the default): return false only when PACTUM_AGENT_TRANSPORT equals cli (case-insensitive, trimmed); empty, acp, or anything else returns true. Update the function comment to document the default and the escape hatch.
- Tests: transport selection — empty env selects ACPTransport, PACTUM_AGENT_TRANSPORT=cli selects CLITransport, acp selects ACPTransport (t.Setenv); adjust any existing tests that relied on the CLI default at the transport seam (tests that inject App.AgentTransport explicitly are unaffected).
- Docs: docs/agents.md — the transport section flips (ACP is the default; cli is the debug escape hatch via PACTUM_AGENT_TRANSPORT=cli; the planned-flip wording becomes done), and the execution-model section's 'direct subprocess' description gets a one-line note that by default the subprocess is the ACP adapter; README.md — update the 'invokes agent CLIs directly as subprocesses' wording to mention the ACP adapter default with the CLI escape hatch; docs/backlog.md — record the flip (the ACP arc M13.x->M16.2 is complete; remaining: write-stage shell gating).

## Out of scope
- No changes to the ACP transport implementation, the adapters, agents doctor, the config schema, or the CLI transport; no removal of the CLI transport (it stays as the escape hatch); no isolation/sandbox changes.

## Paths in scope
- internal/app/*.go
- docs/agents.md
- docs/backlog.md
- README.md


## Acceptance criteria
- With no env var set, the agent transport is ACP; PACTUM_AGENT_TRANSPORT=cli selects the CLI transport; covered by tests at the transport seam.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; agents.md/README/backlog reflect the ACP default and the cli escape hatch.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- An unknown PACTUM_AGENT_TRANSPORT value falling back to the ACP default is acceptable because the env var is a debug escape hatch, not workspace config (documented); the loud-failure rule applies to config keys, which the transport no longer is.

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
