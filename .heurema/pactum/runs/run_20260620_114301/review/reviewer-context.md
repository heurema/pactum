# Reviewer Context

## Run
- Run id: run_20260620_114301
- Run status: contract_approved

## Contract
- Goal: De-hardcode `--agent codex` from pactum's lifecycle `next` affordances and human "Next:" output so the suggested execute-plan command respects the user's CONFIGURED executor instead of steering every user — including a claude-only user — to codex.

Background: the skill-install slice (#214) shipped a SKILL.md that correctly uses an `<agent>` placeholder and tells the agent to use the configured executor, but pactum's own machine/human surfaces still emit `pactum execute plan <run> --agent codex` literally. This was flagged by that slice's two non-blocking review findings (f_005: the lifecycle `next` affordance hardcodes codex; f_007: the human-facing output hardcodes it). Because an agent driving pactum runs the `next` array VERBATIM, a claude-only user is silently steered to `--agent codex`, contradicting the cross-agent guidance. The fix is to DROP the hardcoded `--agent codex` so the command defers to execute-plan's existing default-agent resolution (an omitted `--agent` already resolves the configured executor via `prepareExecution`). This is the pattern the codebase already uses elsewhere: `nextCommandForStatus` and `resolve.go`'s `nextCommandForStatus` sibling already emit the bare `pactum execute plan` form, and `execute plan` with no `--agent` is already runnable.

In scope (exactly three production affordance sites + their tests; change ONLY the emitted command string, not resolution logic):
1. internal/app/resolve.go — the `prompt_built` case in the `next`-affordance builder currently returns `[]string{"pactum execute plan " + runID + " --agent codex"}`. Drop ` --agent codex` so it returns `"pactum execute plan " + runID`.
2. internal/app/errors.go — `noExecutionAttemptError` sets `next: []string{"pactum execute plan " + runID + " --agent codex"}`. Drop ` --agent codex` likewise.
3. internal/app/prompt.go — the human "Next:" line in the prompt-build writer prints `"  pactum execute plan %s --agent codex\n"`. Drop ` --agent codex` so it prints `"  pactum execute plan %s\n"`.
4. Update the tests that assert the OLD string to assert the new bare form: internal/app/prompt_test.go (the `pactum execute plan <run> --agent codex` expectation) and internal/app/affordances_test.go (all three: the `wantNext` builder, the `assertNext` line, and the human-output `Next:` substring check).

The result: every lifecycle affordance for the execute-plan step emits `pactum execute plan <run>` (no `--agent`), so it respects `pipeline.execute.by` (the configured executor) for whoever is running — a claude user gets their claude executor, a codex user gets codex — instead of a hardcoded codex. This is consistent with the SKILL.md `<agent>` guidance and with the already-bare affordances elsewhere in resolve.go.

Out of scope (do NOT do here):
- Do NOT change execute-plan's default-agent RESOLUTION logic (`prepareExecution` / `ExecutePlan`); only the emitted affordance/human strings change. The omitted-`--agent` default already works.
- Do NOT touch internal/app/skill.go's "no agent targets detected; use --agent claude, --agent codex, or --agent all" help text — it legitimately lists all install targets and is not a lifecycle affordance.
- Do NOT introduce a new `--agent` placeholder token in the machine `next` array (an agent must be able to run it verbatim; a literal `<agent>` would not run). Dropping the flag is the correct machine-runnable form.
- No schema change, no new flag, no change to the contract-review "no nitpicks" calibration (a separate slice), no docs/SKILL.md changes (the skill is already correct).

Tests: the updated affordance/prompt tests assert the bare `pactum execute plan <run>` form (no `--agent codex`) in both the machine `next` arrays (resolve.go and errors.go paths) and the human "Next:" output (prompt.go). Verify no other test still asserts the hardcoded `--agent codex` for these lifecycle affordances (the skill_test.go references to `--agent codex` are for `skill install` and must stay).

Validation: go build ./..., go vet ./..., go test ./internal/..., go test ./..., make check.
- In scope:
  - Change internal/app/resolve.go so the prompt_built lifecycle next affordance emits pactum execute plan <runID> without --agent codex.
  - Change internal/app/errors.go so noExecutionAttemptError emits pactum execute plan <runID> without --agent codex.
  - Change internal/app/prompt.go so the human Next: line after prompt build prints pactum execute plan <runID> without --agent codex.
  - Update internal/app/prompt_test.go and internal/app/affordances_test.go assertions for these lifecycle affordance and human-output surfaces to expect the bare execute-plan command.
- Out of scope:
  - Changing execute-plan default agent resolution, including prepareExecution or ExecutePlan behavior.
  - Changing CLI schema, flags, prompt manifest structure, or adding a placeholder agent token to machine next arrays.
  - Changing internal/app/skill.go install-target help text or unrelated skill-install tests that legitimately mention --agent codex.
  - Changing docs, SKILL.md guidance, or contract-review calibration behavior.
- Acceptance criteria:
  - All lifecycle next arrays for the prompt-built execute-plan step emit exactly pactum execute plan <runID> with no --agent codex suffix.
  - The no-execution-attempt error next array emits exactly pactum execute plan <runID> with no --agent codex suffix.
  - The human prompt-build Next: output emits exactly pactum execute plan <runID> with no --agent codex suffix.
  - Tests covering the affected affordance and prompt output paths assert the bare execute-plan command.
  - No affected lifecycle affordance test or production affordance string still asserts or emits pactum execute plan <runID> --agent codex.
  - Existing execute-plan default-executor behavior remains unchanged and covered by the existing execute tests.
- Validation commands:
  - go build ./...
  - go vet ./...
  - go test ./internal/...
  - go test ./...
  - sh -c 'if rg -n "pactum execute plan .*--agent codex" internal/app/resolve.go internal/app/errors.go internal/app/prompt.go internal/app/prompt_test.go internal/app/affordances_test.go; then exit 1; fi'
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 5
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go build ./... (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go vet ./... (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: go test ./internal/... (exit 0, timed out: false, result: gate/validation/command_003/result.json)
  - command_004: go test ./... (exit 0, timed out: false, result: gate/validation/command_004/result.json)
  - command_005: sh -c 'if rg -n "pactum execute plan .*--agent codex" internal/app/resolve.go internal/app/errors.go internal/app/prompt.go internal/app/prompt_test.go internal/app/affordances_test.go; then exit 1; fi' (exit 0, timed out: false, result: gate/validation/command_005/result.json)
  - command_006: make check (exit 0, timed out: false, result: gate/validation/command_006/result.json)
- Change summary:
  - changed files:
    - internal/app/affordances_test.go
    - internal/app/errors.go
    - internal/app/prompt.go
    - internal/app/prompt_test.go
    - internal/app/resolve.go
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
