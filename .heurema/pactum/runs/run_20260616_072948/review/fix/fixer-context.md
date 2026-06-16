# Review Fixer Context

## Run
- Run id: run_20260616_072948
- Run status: contract_approved

## Approved contract
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

## Current review findings
- Summary: findings=4 open=4 resolved=0 blocking_open=3
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: Args and Env entries are only sanitized when the whole string starts with the home directory, leaving home paths embedded in KEY=value env entries or flag/config args unchanged.
    location: internal/agents/dryrun.go:28
  - f_002 severity=medium category=correctness blocking=true status=open: Recorded args/env values are only sanitized when the whole string starts with the home directory, so composite values like `model="~/me/..."` or `ANTHROPIC_MODEL=~/me/...` can still leak absolute home paths in BuildDryRunPlan/BuildACPWouldRun output.
    location: internal/agents/dryrun.go:28
  - f_003 severity=medium category=quality blocking=true status=open: BuildDryRunPlan and recorded Args/Env sanitization are not covered by the added tests. The new test exercises BuildACPWouldRun.Command and the scalar helper, but no test drives the BuildDryRunPlan WouldRun path or asserts sanitization of path-containing Args/Env entries in recorded dry-run output.
    location: internal/agents/dryrun_test.go:10
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_004 severity=low category=quality blocking=false status=open: The docs still describe execute plan as writing the exact command, but dry-run recording now sanitizes home-directory prefixes to '~' in Command, Args, and Env. Update the user-facing docs to clarify that the recorded would_run command may be home-path redacted.
    location: docs/flow.md:170

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
