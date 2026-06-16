# Reviewer Context

## Run
- Run id: run_20260616_072948
- Run status: contract_approved

## Contract
- Goal: Sanitize absolute home paths in the recorded would_run command so committed dry-run artifacts never leak a home path. When PACTUM_CLAUDE_ACP_COMMAND or PACTUM_CODEX_ACP_COMMAND is set to an absolute path under the user's home (e.g. ~/<user>/repos/.../codex-acp), acpAdapterCommand returns it verbatim and BuildACPWouldRun / BuildDryRunPlan (internal/agents/dryrun.go) record it in the would_run.command of request.json artifacts. Those artifacts are committed under .heurema and then fail 'make heurema-hygiene' with a home-path leak (this actually happened for run_20260615_222003's codex reviewer attempts). Fix: when building the would_run DryRunCommand, replace the user's home-directory prefix with '~' in Command (and Args/Env, where an absolute home path can also appear), so the recorded representation has no absolute home path. Use os.UserHomeDir() to get the prefix; if it cannot be determined, or the value is not under the home directory, leave it unchanged. The ACTUAL executed adapter command must stay byte-for-byte unchanged — only the recorded would_run representation is sanitized (ACPTransport.Run still uses the real path). Add a unit test for BuildACPWouldRun that an absolute home-path override is recorded as '~/...' while a non-home value (e.g. 'npx') is left as-is. Keep claude and codex behavior identical; do not change acpAdapterCommand's actual command resolution.
- In scope:
  - Sanitize the recorded DryRunCommand produced by BuildACPWouldRun and BuildDryRunPlan by replacing an os.UserHomeDir() prefix in Command, Args, and Env with '~'.
  - Add focused unit coverage for home-path overrides and non-home command values for both codex and claude ACP would_run recording.
- Out of scope:
  - Changing acpAdapterCommand resolution semantics or the PACTUM_CLAUDE_ACP_COMMAND and PACTUM_CODEX_ACP_COMMAND override format.
  - Changing the actual ACPTransport.Run process launch command, args, or env.
  - Weakening cmd/heurema-hygiene home-path detection.
  - Rewriting historical .heurema run records as part of this feature change.
- Acceptance criteria:
  - When PACTUM_CODEX_ACP_COMMAND or PACTUM_CLAUDE_ACP_COMMAND is set to an absolute path under os.UserHomeDir(), BuildACPWouldRun records would_run.command as '~/...' with no absolute home prefix.
  - BuildDryRunPlan records sanitized Command, Args, and Env values whenever those recorded values contain a path under os.UserHomeDir().
  - Non-home values such as 'npx', 'codex-acp', and relative override paths are recorded unchanged.
  - Values are left unchanged when os.UserHomeDir() cannot be determined or when the value is not equal to or below the home directory.
  - Existing acpAdapterCommand override tests still pass, demonstrating the actual adapter command resolution remains unchanged.
- Validation commands:
  - go test ./internal/agents -run TestBuildACPWouldRun
  - make check
  - make heurema-hygiene

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/agents -run TestBuildACPWouldRun (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: make heurema-hygiene (exit 0, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - internal/agents/dryrun.go
  - new files:
    - internal/agents/dryrun_test.go
  - missing files:
    - none

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
