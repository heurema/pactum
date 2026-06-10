# Contract Draft

## Goal
Make ACP the default agent transport. The two gaps that blocked the flip are closed (M16.1: model pins reach the adapters; read-only stages enforced per leg), and ACP is strictly better where it differs: it streams the agent's output (the CLI claude -p runs repeatedly tripped the idle watchdog AFTER finishing work because -p buffers until the end), and it gives write stages the real-time contract path-scope guard the CLI cannot have. Flip acpTransportEnabled so the default is ACP; PACTUM_AGENT_TRANSPORT=cli remains the debug escape hatch (cli selects the one-shot CLI transport; acp or empty selects ACP; any other value falls back to the ACP default — the env var is a debug hatch, not config).

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260610_065943
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
