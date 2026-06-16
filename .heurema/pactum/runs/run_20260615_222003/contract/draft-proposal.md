# Contract Draft Proposal

## Status
- Run id: run_20260615_222003
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-15T22:23:37Z

## In scope
- Remove the exported agents.CLITransport implementation and make App.agentTransport() return an injected transport when provided, otherwise agents.ACPTransport.
- Remove PACTUM_AGENT_TRANSPORT handling and update tests/docs that present cli as a supported transport escape hatch.
- Strip built-in codex executor and reviewer descriptors of CLI Command/Args while preserving Name, Input, model inference, and RunRequest.Model behavior.
- Remove codex CLI stdout usage parsing from internal/agents/usage.go while preserving parseCodexACPUsageMeta and ACP token usage capture.
- Update execute, review, review fix, clarify, and contract draft dry-run/would_run output for built-in codex and claude so it reflects acpAdapterCommand command, args, and adapter env entries instead of codex exec CLI commands.
- Update affected tests and helper setup so they no longer depend on agents.CLITransport or PACTUM_AGENT_TRANSPORT=cli.

## Out of scope
- Changing ACP protocol request/response handling, session update handling, permission request policy, file read/write behavior, or timeout completion semantics.
- Changing token usage normalization fields or codex ACP _meta parsing behavior.
- Vendoring, replacing, or removing the runtime ACP adapter dependency, including the forked codex adapter selected with PACTUM_CODEX_ACP_COMMAND.
- Adding new agent engines or custom CLI agent support.
- Running real agent execution commands such as pactum execute run or pactum review run.

## Acceptance criteria
- No internal code or tests define, return, or reference agents.CLITransport, and App code no longer reads PACTUM_AGENT_TRANSPORT.
- Built-in codex and claude executor/reviewer descriptors resolve with Input set to prompt_file and empty Command/Args for both roles.
- Built-in dry-run JSON and human output no longer display codex exec, --json, --dangerously-bypass-approvals-and-sandbox, or --sandbox read-only as agent commands.
- Built-in dry-run/would_run output shows ACP adapter launch details from acpAdapterCommand; codex read-only stages include sandbox_mode="read-only", codex write stages do not, and adapter command overrides from PACTUM_CODEX_ACP_COMMAND and PACTUM_CLAUDE_ACP_COMMAND are reflected.
- No parseCodexUsage symbol remains, and codex usage capture continues through parseCodexACPUsageMeta coverage.
- Tests that previously asserted CLI transport, PACTUM_AGENT_TRANSPORT=cli, or codex CLI dry-run behavior are updated or removed to assert ACP-only behavior.
- The repository passes make check.

## Validation commands
- go test ./internal/agents ./internal/app
- make check
- bash -lc 'if rg -n "agents\\.CLITransport|CLITransport|PACTUM_AGENT_TRANSPORT" internal; then exit 1; fi'
- bash -lc 'if rg -n "parseCodexUsage|codex exec --json|--dangerously-bypass-approvals-and-sandbox|--sandbox read-only" internal/agents internal/app README.md docs/agents.md; then exit 1; fi'

## Assumptions
- The ACP-only requirement applies to built-in resolved agent engines, codex and claude.
- Helper agents in tests may be replaced with local test transports or subprocess test utilities as long as production App transport no longer exposes CLITransport.
- Current-behavior docs such as README.md and docs/agents.md should be updated if they still describe CLI transport or codex exec as current behavior; explicitly historical backlog/design notes may remain.
- No real agent runs are required for validation; deterministic Go tests and make check are sufficient.

