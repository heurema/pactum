# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260615_222003
- Approval: approved
- Contract hash: 4bef822d15d55d6cfd9055f58e3a41da72bf430980b0114e3743d12d61a1a972

## Goal
Complete the ACP-only transport: remove CLITransport entirely so every agent runs over ACP, and delete the now-dead codex CLI machinery. After #152 claude is ACP-only, but agents.CLITransport, the codex CLI descriptor, the PACTUM_AGENT_TRANSPORT=cli escape, and the CLI codex usage parser remain — all dead under the ACP-default transport (codex already runs over ACP via acpAdapterCommand). Changes: (1) remove agents.CLITransport (internal/agents/transport.go) and make App.agentTransport() (internal/app/app.go) always return agents.ACPTransport, dropping acpTransportEnabled() and the PACTUM_AGENT_TRANSPORT=cli branch; (2) remove the codex CLI descriptor Command/Args ('codex exec --json ...') in internal/agents/config.go so the codex descriptor no longer carries CLI flags, mirroring how claude was stripped in #152; (3) remove parseCodexUsage and the codex branch of CLI stdout usage parsing in internal/agents/usage.go — codex usage now comes only from the ACP codex/token_usage _meta (parseCodexACPUsageMeta is already present); (4) fix the dry-run / would_run command representation so it reflects the ACP adapter command (acpAdapterCommand) instead of a stale 'codex exec'/CLI command, for both claude and codex; (5) keep the per-engine ACP adapter resolution (PACTUM_CLAUDE_ACP_COMMAND / PACTUM_CODEX_ACP_COMMAND overrides) and the read-only vs write-stage permission handling unchanged; (6) update or remove any tests that exercise CLITransport or PACTUM_AGENT_TRANSPORT=cli. Do not change ACP protocol handling, usage normalization, or codex usage _meta parsing. Codex usage capture in production still needs the forked adapter at runtime via PACTUM_CODEX_ACP_COMMAND (a runtime dependency, not code).

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
- total: 5
- fresh: 4
- stale: 1
- unknown: 0

Items:
- mem_012 [fresh] score=75 — Capture Codex token usage from ACP usage_update metadata and add per-engine A...
- mem_002 [stale] score=72 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go
- mem_013 [fresh] score=53 — Dogfood Pactum with a local codex-acp adapter that returns official ACP Promp...
- mem_009 [fresh] score=53 — Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-n...
- mem_005 [fresh] score=53 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...

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
