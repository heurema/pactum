# Contract Draft Proposal

## Status
- Run id: run_20260611_113834
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-11T11:47:46Z

## In scope
- Add `pactum.error.v1` JSON envelopes for the named failing preconditions with stable `error.code`, existing human-readable `error.message`, and optional `error.fix` when a single exact runnable remedial command exists.
- Add `error.fix` inside the existing `error` object; preserve existing `suggested_command` and `next_command` fields where they already exist.
- Add `fix` to exit-0 `pactum.not_ready.v1` read-only guidance responses while preserving `suggested_command`.
- Add top-level `next: []` arrays to in-scope workflow-state mutating `--json` responses, `pactum status --json`, and `pactum task show --json`.
- Populate JSON `next` only with safe, concrete, directly runnable pactum command strings; fill known run IDs and omit placeholder-only templates.
- Document the `fix` and `next` convention in `assets/agent-skills/pactum/SKILL.md`, `assets/agent-skills/pactum/references/workflow.md`, and `docs/agent-skill.md`.

## Out of scope
- Do not rename `error.code` to `error.reason` or add a parallel `error.reason` field.
- Do not remove existing compatibility fields such as `suggested_command` or `next_command`.
- Do not add top-level `next` to `pactum task list --json` beyond preserving existing per-run `next_command` fields.
- Do not require `next` for `export` or other commands that write artifacts without advancing Pactum workflow state.
- Do not emit `pactum execute run` as `error.fix`; real agent execution remains human-approved.
- Do not expand the stable reason-code taxonomy to secondary artifact-integrity or boundary-mismatch failures unless they already map to a named precondition such as `project_map_stale`.

## Acceptance criteria
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

## Validation commands
- go test ./...
- make check

## Assumptions
- The accepted stable reason code for pending proposal failures in this slice is `pending_review_proposals`.
- Exact command strings in `next` should include the resolved run ID whenever the command otherwise would rely on current-run inference.

