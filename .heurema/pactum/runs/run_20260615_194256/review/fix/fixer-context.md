# Review Fixer Context

## Run
- Run id: run_20260615_194256
- Run status: contract_approved

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
