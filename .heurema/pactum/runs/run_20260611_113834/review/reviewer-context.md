# Reviewer Context

## Run
- Run id: run_20260611_113834
- Run status: contract_approved

## Contract
- Goal: Make the CLI announce legal moves so an agent never guesses the pipeline state machine: (1) structured error envelopes — when a command fails on a recognizable precondition (workspace not initialized, project map stale, contract not approved, prompt not built, no execution attempt, gate report missing, review not prepared, open blocking clarifications, run not found, pending proposals), the --json output emits a versioned error envelope carrying a stable machine-readable reason code, the human message, and a fix field holding the exact remedial pactum command when one exists; human output keeps the existing Suggested:/guidance text; exit codes stay nonzero and unchanged. (2) next affordances — every mutating command's --json response gains a next array of full pactum command strings mirroring the human Next: block (commands that already print Next: hints must emit the same set in JSON), and pactum status --json gains a next array for the current run's stage. There is precedent to build on: the pactum.not_ready.v1 envelope with suggested_command, next_command fields in run resolution, and the human Next: blocks — unify these into one consistent affordance convention rather than inventing a parallel one. The bundled skill and docs describe the convention briefly (agents should read next/fix instead of memorizing stage order). Tests pin: a representative precondition failure per stage emits the envelope with the right reason and fix; next arrays match the human hints; non-error and non-mutating outputs are unchanged.
- In scope:
  - Add `pactum.error.v1` JSON envelopes for the named failing preconditions with stable `error.code`, existing human-readable `error.message`, and optional `error.fix` when a single exact runnable remedial command exists.
  - Add `error.fix` inside the existing `error` object; preserve existing `suggested_command` and `next_command` fields where they already exist.
  - Add `fix` to exit-0 `pactum.not_ready.v1` read-only guidance responses while preserving `suggested_command`.
  - Add top-level `next: []` arrays to in-scope workflow-state mutating `--json` responses, `pactum status --json`, and `pactum task show --json`.
  - Populate JSON `next` only with safe, concrete, directly runnable pactum command strings; fill known run IDs and omit placeholder-only templates.
  - Document the `fix` and `next` convention in `assets/agent-skills/pactum/SKILL.md`, `assets/agent-skills/pactum/references/workflow.md`, and `docs/agent-skill.md`.
- Out of scope:
  - Do not rename `error.code` to `error.reason` or add a parallel `error.reason` field.
  - Do not remove existing compatibility fields such as `suggested_command` or `next_command`.
  - Do not add top-level `next` to `pactum task list --json` beyond preserving existing per-run `next_command` fields.
  - Do not require `next` for `export` or other commands that write artifacts without advancing Pactum workflow state.
  - Do not emit `pactum execute run` as `error.fix`; real agent execution remains human-approved.
  - Do not expand the stable reason-code taxonomy to secondary artifact-integrity or boundary-mismatch failures unless they already map to a named precondition such as `project_map_stale`.
- Acceptance criteria:
  - `--json` failures for `not_initialized`, `project_map_stale`, `contract_not_approved`, `blocking_clarifications_open`, `prompt_not_built`, `no_execution_attempt`, `gate_report_missing`, `review_not_prepared`, `pending_review_proposals`, and `run_not_found` emit schema `pactum.error.v1` when the command fails.
  - Each pinned failure test asserts `error.code`, `error.message`, optional `error.fix`, unchanged nonzero exit code, and empty stderr in `--json` mode.
  - No `error.fix` is emitted when no single exact runnable remedial command exists; no `fix` value contains placeholders.
  - `pactum gate run --json` with no completed execution attempt omits `error.fix` and may expose safe preparation through `next`, but never suggests `pactum execute run` as a fix.
  - `pactum task new --clarify --json` partial clarify-loop failure exits nonzero with `schema: pactum.error.v1`, `error.code: clarify_loop_failed`, a message that the run was created, and `error.fix: pactum clarify run <run_id>`.
  - Read-only not-ready JSON responses keep schema `pactum.not_ready.v1`, keep exit code 0, preserve `suggested_command`, and add `fix` when an exact remedial command exists.
  - In-scope JSON responses expose a top-level `next` array; responses with no meaningful next action expose `next: []`.
  - `pactum status --json` and `pactum task show --json` expose top-level `next` while preserving existing `next_command` compatibility fields.
  - For open blocking clarifications, JSON `next` contains safe inspection commands such as `pactum clarify status <run_id>` and does not contain answer templates.
  - Human output keeps existing Suggested:/guidance/Next: behavior except for any necessary consistency fixes.
  - Every command string emitted in JSON next arrays and error.fix values uses the current command grammar (pactum clarify show, not the removed clarify status; pactum execute plan, not execute dry-run) — pinned by a test that walks the emitted affordances
- Validation commands:
  - go test ./...
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 4
- Fresh: 2
- Stale: 2
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./... (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
- Change summary:
  - changed files:
    - assets/agent-skills/pactum/SKILL.md
    - assets/agent-skills/pactum/references/workflow.md
    - docs/agent-skill.md
    - internal/app/agent_attempt.go
    - internal/app/clarify.go
    - internal/app/clarify_loop.go
    - internal/app/clarify_suggest.go
    - internal/app/contract.go
    - internal/app/contract_draft.go
    - internal/app/errors.go
    - internal/app/execute.go
    - internal/app/gate.go
    - internal/app/memory.go
    - internal/app/prompt.go
    - internal/app/readiness.go
    - internal/app/resolve.go
    - internal/app/review.go
    - internal/app/review_fix.go
    - internal/app/review_fix_outcomes.go
    - internal/app/review_loop.go
    - internal/app/review_proposals.go
    - internal/app/status.go
    - internal/app/task.go
  - new files:
    - internal/app/affordances_test.go
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
