# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260616_072948/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260616_072948/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260616_072948/review/review.json, .heurema/pactum/runs/run_20260616_072948/review/findings.jsonl, .heurema/pactum/runs/run_20260616_072948/review/resolutions.jsonl

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
