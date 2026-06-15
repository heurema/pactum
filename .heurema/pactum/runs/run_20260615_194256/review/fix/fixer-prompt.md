# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260615_194256/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260615_194256/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260615_194256/review/review.json, .heurema/pactum/runs/run_20260615_194256/review/findings.jsonl, .heurema/pactum/runs/run_20260615_194256/review/resolutions.jsonl

## Approved contract
- Goal: Run contract validation commands through a shell so shell features work. In internal/app/gate.go the gate tokenizes each validation command with strings.Fields and runs it directly via exec.Command(fields[0], fields[1:]...), so commands using shell features (command substitution $(...), quotes, pipes, globs, &&) are mis-parsed and fail. Change the gate to execute each validation command through the system shell (sh -c <command>) so the shell interprets the string. Preserve timeout/context handling and existing behavior for simple commands, and add a unit test covering a shell-feature command (e.g. command substitution or a quoted argument).
- In scope:
  - Update internal/app/gate.go so each non-empty validation command is executed as one intact command string through sh -c, not by splitting it with strings.Fields.
  - Preserve gate validation plumbing: repository-root working directory, inherited environment, stdout/stderr logs, result JSON artifacts, original command text in reports, exit-code capture, and context timeout handling.
  - Add or rewrite internal/app/gate_test.go coverage for a validation command requiring shell interpretation, such as command substitution or a quoted single argument containing spaces.
  - Update the existing whitespace-parsing regression test so it expects shell-preserved quoted arguments instead of strings.Fields behavior.
- Out of scope:
  - Changing the contract schema or adding validation command types, negative-match semantics, command allowlists, or configurable shells.
  - Changing gate status rules, path-scope enforcement, execution-attempt selection, prompt or contract approval behavior, or review behavior.
  - Running real Pactum executor or reviewer agents.
  - Changing generated Pactum run records except for normal artifacts produced by local tests.
- Acceptance criteria:
  - internal/app/gate.go no longer tokenizes validation commands with strings.Fields before execution; the exact validation command text is passed as the -c argument to sh.
  - A unit test named TestGateValidationCommandRunsThroughShell demonstrates that a validation command with shell syntax completes successfully during gate run and captured stdout proves the shell interpreted it correctly.
  - Existing simple validation command behavior still passes, including repository-root execution, inherited environment, stdout/stderr/result artifacts, and original command text in the gate report.
  - A failing validation command still records its non-zero exit code in command_001/result.json and the gate report, and the overall gate status remains failed.
  - Validation timeout behavior remains context-based and continues to report timed_out with a non-success exit code when the command exceeds its timeout.
- Validation commands:
  - go test ./internal/app -run TestGateValidationCommandRunsThroughShell -count=1
  - go test ./internal/app -run 'TestGateRunExecutesValidationCommand|TestGateRunValidationFailureWritesReportAndReturnsNonZero|TestGateValidationCommandRunsThroughShell' -count=1
  - make check

## Current review findings
- Summary: findings=4 open=4 resolved=0 blocking_open=1
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: Validation timeout kills only the sh wrapper, so foreground child commands can continue running after the gate records a timeout.
    location: internal/app/gate.go:543
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_002 severity=medium category=quality blocking=false status=open: Validation-command timeout behavior is not covered by tests after switching validation execution to `sh -c`.
    location: internal/app/gate_test.go:523
  - f_003 severity=low category=quality blocking=false status=open: The failing validation-command test does not assert the non-zero exit code written to `gate/validation/command_001/result.json`.
    location: internal/app/gate_test.go:110
  - f_004 severity=low category=quality blocking=false status=open: The backlog still describes validation command quote parsing as an active strings.Fields bug even though this change now runs validation commands through sh -c.
    location: docs/backlog.md:375

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
