# Reviewer Context

## Run
- Run id: run_20260615_222003
- Run status: contract_approved

## Contract
- Goal: Complete the ACP-only transport: remove CLITransport entirely so every agent runs over ACP, and delete the now-dead codex CLI machinery. After #152 claude is ACP-only, but agents.CLITransport, the codex CLI descriptor, the PACTUM_AGENT_TRANSPORT=cli escape, and the CLI codex usage parser remain — all dead under the ACP-default transport (codex already runs over ACP via acpAdapterCommand). Changes: (1) remove agents.CLITransport (internal/agents/transport.go) and make App.agentTransport() (internal/app/app.go) always return agents.ACPTransport, dropping acpTransportEnabled() and the PACTUM_AGENT_TRANSPORT=cli branch; (2) remove the codex CLI descriptor Command/Args ('codex exec --json ...') in internal/agents/config.go so the codex descriptor no longer carries CLI flags, mirroring how claude was stripped in #152; (3) remove parseCodexUsage and the codex branch of CLI stdout usage parsing in internal/agents/usage.go — codex usage now comes only from the ACP codex/token_usage _meta (parseCodexACPUsageMeta is already present); (4) fix the dry-run / would_run command representation so it reflects the ACP adapter command (acpAdapterCommand) instead of a stale 'codex exec'/CLI command, for both claude and codex; (5) keep the per-engine ACP adapter resolution (PACTUM_CLAUDE_ACP_COMMAND / PACTUM_CODEX_ACP_COMMAND overrides) and the read-only vs write-stage permission handling unchanged; (6) update or remove any tests that exercise CLITransport or PACTUM_AGENT_TRANSPORT=cli. Do not change ACP protocol handling, usage normalization, or codex usage _meta parsing. Codex usage capture in production still needs the forked adapter at runtime via PACTUM_CODEX_ACP_COMMAND (a runtime dependency, not code).
- In scope:
  - Remove the exported agents.CLITransport implementation and make App.agentTransport() return an injected transport when provided, otherwise agents.ACPTransport.
  - Remove PACTUM_AGENT_TRANSPORT handling and update tests/docs that present cli as a supported transport escape hatch.
  - Strip built-in codex executor and reviewer descriptors of CLI Command/Args while preserving Name, Input, model inference, and RunRequest.Model behavior.
  - Remove codex CLI stdout usage parsing from internal/agents/usage.go while preserving parseCodexACPUsageMeta and ACP token usage capture.
  - Update execute, review, review fix, clarify, and contract draft dry-run/would_run output for built-in codex and claude so it reflects acpAdapterCommand command, args, and adapter env entries instead of codex exec CLI commands.
  - Update affected tests and helper setup so they no longer depend on agents.CLITransport or PACTUM_AGENT_TRANSPORT=cli.
- Out of scope:
  - Changing ACP protocol request/response handling, session update handling, permission request policy, file read/write behavior, or timeout completion semantics.
  - Changing token usage normalization fields or codex ACP _meta parsing behavior.
  - Vendoring, replacing, or removing the runtime ACP adapter dependency, including the forked codex adapter selected with PACTUM_CODEX_ACP_COMMAND.
  - Adding new agent engines or custom CLI agent support.
  - Running real agent execution commands such as pactum execute run or pactum review run.
- Acceptance criteria:
  - No internal code or tests define, return, or reference agents.CLITransport, and App code no longer reads PACTUM_AGENT_TRANSPORT.
  - Built-in codex and claude executor/reviewer descriptors resolve with Input set to prompt_file and empty Command/Args for both roles.
  - Built-in dry-run JSON and human output no longer display codex exec, --json, --dangerously-bypass-approvals-and-sandbox, or --sandbox read-only as agent commands.
  - Built-in dry-run/would_run output shows ACP adapter launch details from acpAdapterCommand; codex read-only stages include sandbox_mode="read-only", codex write stages do not, and adapter command overrides from PACTUM_CODEX_ACP_COMMAND and PACTUM_CLAUDE_ACP_COMMAND are reflected.
  - No parseCodexUsage symbol remains, and codex usage capture continues through parseCodexACPUsageMeta coverage.
  - Tests that previously asserted CLI transport, PACTUM_AGENT_TRANSPORT=cli, or codex CLI dry-run behavior are updated or removed to assert ACP-only behavior.
  - The repository passes make check.
- Validation commands:
  - go test ./internal/agents ./internal/app
  - make check
  - bash -lc 'if rg -n "agents\\.CLITransport|CLITransport|PACTUM_AGENT_TRANSPORT" internal; then exit 1; fi'
  - bash -lc 'if rg -n "parseCodexUsage|codex exec --json|--dangerously-bypass-approvals-and-sandbox|--sandbox read-only" internal/agents internal/app README.md docs/agents.md; then exit 1; fi'

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: failed
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/agents ./internal/app (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: bash -lc 'if rg -n "agents\\.CLITransport|CLITransport|PACTUM_AGENT_TRANSPORT" internal; then exit 1; fi' (exit 0, timed out: false, result: gate/validation/command_003/result.json)
  - command_004: bash -lc 'if rg -n "parseCodexUsage|codex exec --json|--dangerously-bypass-approvals-and-sandbox|--sandbox read-only" internal/agents internal/app README.md docs/agents.md; then exit 1; fi' (exit 1, timed out: false, result: gate/validation/command_004/result.json)
- Change summary:
  - changed files:
    - README.md
    - docs/agents.md
    - docs/backlog.md
    - internal/agents/acp_transport.go
    - internal/agents/config.go
    - internal/agents/doctor.go
    - internal/agents/dryrun.go
    - internal/agents/executor.go
    - internal/agents/executor_test.go
    - internal/agents/model.go
    - internal/agents/runner.go
    - internal/agents/transport.go
    - internal/agents/types.go
    - internal/agents/usage.go
    - internal/agents/usage_test.go
    - internal/app/app.go
    - internal/app/app_test.go
    - internal/app/config_test.go
    - internal/app/doctor_test.go
    - internal/app/execute.go
    - internal/app/execute_test.go
    - internal/app/review.go
    - internal/app/review_fix.go
    - internal/app/review_loop.go
    - internal/app/review_loop_test.go
    - internal/app/review_test.go
    - internal/app/transport_selection_test.go
    - internal/app/usage_test.go
    - internal/docs/docs_test.go
  - new files:
    - none
  - missing files:
    - internal/agents/process_unix.go
    - internal/agents/process_windows.go

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If you are not certain an issue is real after verification, do not flag it.
