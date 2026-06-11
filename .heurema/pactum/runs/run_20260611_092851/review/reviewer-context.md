# Reviewer Context

## Run
- Run id: run_20260611_092851
- Run status: contract_approved

## Contract
- Goal: Remove the interactive confirmation layer from the CLI: the consumer is an AI agent relaying decisions already made in conversation, so the CLI must never prompt. Delete interactive confirm prompts and the --yes flag from every command that has one (execute run, review run, review loop via clarify run, clarify run, clarify suggest, contract draft, task new --clarify, and any other); after the change the commands simply run, and --yes is rejected as an unknown flag (hard removal, no users yet). Delete gate run --allow-commands: running the contract validation commands is the gate's purpose, so gate run always runs them. Make the --by principal flag uniform across all decision verbs: optional with default value manual on contract approve, review approve, memory accept (today it may be required there), and extend it to contract accept, clarify answer, review proposal accept, review proposal reject — recorded in the respective decision artifacts and ledger events the same way approved_by is recorded today. The task new --clarify guard requiring --yes is removed together with the flag. All Next: hints, hand-written errors, helper text, docs (README, AGENTS.md, docs tree, bundled skill under assets/agent-skills/pactum), and scripts must stop mentioning --yes and --allow-commands as current guidance. Tests updated: confirmation-prompt tests removed or inverted, --by recording covered for the newly extended verbs, and negative coverage that --yes and --allow-commands are rejected.
- In scope:
  - Remove Pactum's own interactive confirmation implementation and every current `--yes` CLI flag/guard from agent-running commands, including clarify suggest/run, contract draft, execute run, review run, review fix run, review loop, task new --clarify, and any other current `Yes` command field.
  - Remove `gate run --allow-commands` so `gate run` always executes contract validation commands and reports `validation.commands_allowed: true` with an empty commands array when no validation commands exist.
  - Add optional `--by` with default `manual` to contract accept, clarify answer, review proposal accept, and review proposal reject; keep/default it on contract approve, review approve, and memory accept.
  - Persist explicit CLI principals only in semantic decision artifacts: `accepted_by` for contract draft proposal acceptance and `decided_by` for clarification and review proposal decisions; leave ledger event schema unchanged.
  - Update current CLI help, Next hints, hand-written errors, README, AGENTS guidance, docs current guidance, bundled Pactum skill references, scripts, and tests so `--yes` and `--allow-commands` are not presented as current guidance.
- Out of scope:
  - Changing built-in agent transport permission prompts, sandbox behavior, read-only/write-scope behavior, or external agent approval semantics.
  - Adding principal fields to automatic loop-created decisions such as clarify-loop auto answers or review-loop duplicate proposal records.
  - Adding `--by` to commands other than contract approve, review approve, memory accept, contract accept, clarify answer, review proposal accept, and review proposal reject.
  - Rewriting clearly historical backlog or dogfood transcripts solely because they contain old `--yes` or `--allow-commands` examples.
- Acceptance criteria:
  - `pactum --help` and subcommand help no longer expose `--yes` or `--allow-commands`; passing either removed flag to formerly affected commands is rejected as an unknown flag.
  - Formerly guarded Pactum commands run without Pactum confirmation prompts or non-interactive `--yes` refusal errors; no code path calls `confirmDirectExecution`.
  - `gate run` runs configured validation commands without an allow flag and successful gate reports always include `validation.commands_allowed: true`.
  - `contract accept`, `clarify answer`, `review proposal accept`, and `review proposal reject` accept optional `--by`, default to `manual`, trim whitespace, sanitize repo-root absolute path text consistently with memory acceptance, and persist the resulting principal in the clarified artifact fields.
  - Tests cover removed flag rejection, no-prompt execution behavior, gate execution without `--allow-commands`, and `--by` persistence/defaulting for all explicitly principal-bearing decision verbs.
  - Current guidance in README, AGENTS.md, docs/agents.md, docs/flow.md, docs/agent-skill.md, bundled Pactum skill files, helper text, Next hints, and scripts no longer instruct users to pass `--yes` or `--allow-commands`.
- Validation commands:
  - go test ./internal/app ./internal/docs
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 2
- Fresh: 1
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app ./internal/docs (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
- Change summary:
  - changed files:
    - README.md
    - assets/agent-skills/pactum/SKILL.md
    - assets/agent-skills/pactum/references/safety.md
    - docs/agent-skill.md
    - docs/agents.md
    - docs/flow.md
    - go.mod
    - internal/app/agent_attempt.go
    - internal/app/agent_attempt_timeout_test.go
    - internal/app/agent_attempt_transport_test.go
    - internal/app/clarify.go
    - internal/app/clarify_loop.go
    - internal/app/clarify_loop_test.go
    - internal/app/clarify_suggest.go
    - internal/app/clarify_suggest_test.go
    - internal/app/clarify_test.go
    - internal/app/cli.go
    - internal/app/cli_v2_test.go
    - internal/app/commands.go
    - internal/app/contract.go
    - internal/app/contract_draft.go
    - internal/app/contract_draft_test.go
    - internal/app/execute.go
    - internal/app/execute_report_test.go
    - internal/app/execute_test.go
    - internal/app/gate.go
    - internal/app/gate_test.go
    - internal/app/memory.go
    - internal/app/resolve.go
    - internal/app/review.go
    - internal/app/review_fix.go
    - internal/app/review_loop.go
    - internal/app/review_loop_test.go
    - internal/app/review_proposals.go
    - internal/app/review_test.go
    - internal/app/task.go
    - internal/app/task_clarify_test.go
    - internal/app/usage_test.go
    - internal/docs/docs_test.go
  - new files:
    - internal/app/principal.go
  - missing files:
    - internal/app/confirm.go

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
