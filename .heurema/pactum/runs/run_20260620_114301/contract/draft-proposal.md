# Contract Draft Proposal

## Status
- Run id: run_20260620_114301
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-20T11:46:44Z

## In scope
- Change internal/app/resolve.go so the prompt_built lifecycle next affordance emits pactum execute plan <runID> without --agent codex.
- Change internal/app/errors.go so noExecutionAttemptError emits pactum execute plan <runID> without --agent codex.
- Change internal/app/prompt.go so the human Next: line after prompt build prints pactum execute plan <runID> without --agent codex.
- Update internal/app/prompt_test.go and internal/app/affordances_test.go assertions for these lifecycle affordance and human-output surfaces to expect the bare execute-plan command.

## Out of scope
- Changing execute-plan default agent resolution, including prepareExecution or ExecutePlan behavior.
- Changing CLI schema, flags, prompt manifest structure, or adding a placeholder agent token to machine next arrays.
- Changing internal/app/skill.go install-target help text or unrelated skill-install tests that legitimately mention --agent codex.
- Changing docs, SKILL.md guidance, or contract-review calibration behavior.

## Acceptance criteria
- All lifecycle next arrays for the prompt-built execute-plan step emit exactly pactum execute plan <runID> with no --agent codex suffix.
- The no-execution-attempt error next array emits exactly pactum execute plan <runID> with no --agent codex suffix.
- The human prompt-build Next: output emits exactly pactum execute plan <runID> with no --agent codex suffix.
- Tests covering the affected affordance and prompt output paths assert the bare execute-plan command.
- No affected lifecycle affordance test or production affordance string still asserts or emits pactum execute plan <runID> --agent codex.
- Existing execute-plan default-executor behavior remains unchanged and covered by the existing execute tests.

## Validation commands
- go build ./...
- go vet ./...
- go test ./internal/...
- go test ./...
- sh -c 'if rg -n "pactum execute plan .*--agent codex" internal/app/resolve.go internal/app/errors.go internal/app/prompt.go internal/app/prompt_test.go internal/app/affordances_test.go; then exit 1; fi'
- make check

## Assumptions
- Omitting --agent from pactum execute plan already resolves to the configured executor through existing prepareExecution behavior.
- pipeline.execute.by remains the source of the configured executor for default execute-plan resolution.
- The --agent codex mentions in internal/app/skill.go and related skill-install tests are not lifecycle affordances and should remain unchanged.
- The existing SKILL.md guidance is already correct and does not need documentation changes for this slice.

