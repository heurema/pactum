# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260615_222003/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260615_222003/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260615_222003/review/review.json, .heurema/pactum/runs/run_20260615_222003/review/findings.jsonl, .heurema/pactum/runs/run_20260615_222003/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=12 open=12 resolved=0 blocking_open=11
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: Clarifier attempts record an empty would_run for built-in ACP reviewers because the stale empty-command branch returns an empty DryRunCommand.
    location: internal/app/clarify_round.go:149
  - f_002 severity=medium category=correctness blocking=true status=open: Contract drafter attempts record an empty would_run for built-in ACP reviewers because the stale empty-command branch returns an empty DryRunCommand.
    location: internal/app/contract_draft.go:167
  - f_003 severity=medium category=correctness blocking=true status=open: Human would_run output omits ACP adapter env entries, so Claude model/effort pins are not shown in required launch details.
    location: internal/app/execute.go:306
  - f_004 severity=medium category=scope blocking=true status=open: Clarifier attempts record an empty would_run for built-in ACP agents because empty Command now means ACP-only, but the prepare path returns a zero DryRunCommand instead of resolving the ACP adapter.
    location: internal/app/clarify_round.go:149
  - f_005 severity=medium category=scope blocking=true status=open: Contract drafter attempts record an empty would_run for built-in ACP agents instead of the read-only ACP adapter command.
    location: internal/app/contract_draft.go:167
  - f_006 severity=medium category=scope blocking=true status=open: Execute attempt request JSON drops ACP adapter env entries from would_run.
    location: internal/app/execute.go:113
  - f_007 severity=medium category=scope blocking=true status=open: Human would_run output omits ACP adapter env entries.
    location: internal/app/execute.go:306
  - f_008 severity=medium category=quality blocking=true status=open: Clarifier attempt tests still assert the stale CLI-style WouldRun.Stdin path instead of the ACP adapter command/env representation required by the contract.
    location: internal/app/clarify_round_test.go:113
  - f_009 severity=medium category=quality blocking=true status=open: Contract-drafter attempt tests still assert the stale CLI-style WouldRun.Stdin path instead of ACP adapter launch details.
    location: internal/app/contract_draft_test.go:108
  - f_011 severity=medium category=quality blocking=true status=open: SECURITY.md still documents the removed CLI transport and PACTUM_AGENT_TRANSPORT=cli, including stale codex exec / claude -p commands and a false claim that execute plan records the CLI-transport invocation.
    location: SECURITY.md:20
  - f_012 severity=medium category=quality blocking=true status=open: docs/install.md still tells users the optional execution prerequisites are the codex and claude CLIs and says pactum doctor checks those commands, but ACP-only built-ins now require the adapter launcher path checked by doctor.
    location: docs/install.md:15
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_010 severity=low category=quality blocking=false status=open: promptExecutor/newPromptExecutor is now a single-case wrapper around promptFileExecutor.command after CLI transport removal.
    location: internal/agents/executor.go:8

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

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

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
