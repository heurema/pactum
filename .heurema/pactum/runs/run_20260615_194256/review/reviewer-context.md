# Reviewer Context

## Run
- Run id: run_20260615_194256
- Run status: contract_approved

## Contract
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

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 0
- Unknown: 1
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app -run TestGateValidationCommandRunsThroughShell -count=1 (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./internal/app -run 'TestGateRunExecutesValidationCommand|TestGateRunValidationFailureWritesReportAndReturnsNonZero|TestGateValidationCommandRunsThroughShell' -count=1 (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: make check (exit 0, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - internal/app/gate.go
    - internal/app/gate_test.go
  - new files:
    - none
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
