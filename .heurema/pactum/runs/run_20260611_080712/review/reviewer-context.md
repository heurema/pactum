# Reviewer Context

## Run
- Run id: run_20260611_080712
- Run status: contract_approved

## Contract
- Goal: Normalize the CLI command grammar for agent-first use: every stage exposes a uniform verb set, duplicates and aliases are removed, hyphenated pseudo-subcommands become nested subcommands. Renames (hard, no deprecation aliases — the project has no users yet): agents doctor -> doctor; clarify ask -> clarify add; clarify loop -> clarify run; clarify status (and its list alias) -> clarify show; contract show-draft -> contract show --draft; contract accept-draft -> contract accept; execute dry-run -> execute plan; execute status merges into execute show; review dry-run -> review plan; review add-finding -> review finding add; review resolve -> review finding resolve; review accept-proposal -> review proposal accept; review reject-proposal -> review proposal reject; review propose-findings -> review proposal collect; review fix -> review fix run; review apply-fix-outcomes -> review fix apply; task current is dropped (pactum status already reports the current run). All human-output Next: hints, error messages, and docs (flow.md, agents.md, agent-skill.md, README if it lists commands) must reference the new names. Existing JSON output schemas keep their names. Tests updated to invoke the new grammar.
- In scope:
  - Implement only the explicit CLI rename and removal list from the goal, with hard removal of old command-name spellings and no deprecation aliases.
  - Merge `execute status` behavior into `execute show` when no attempt id is provided, while preserving attempt-detail behavior for `execute show [run_id] <attempt_id>`.
  - Update help, usage, `Next:` hints, hand-written errors, generated agent prompt/context text, and machine-readable command-string values such as `next_command` and `suggested_command` to advertise only the new command grammar.
  - Update active current documentation and workflow consumers, including README, AGENTS.md, docs/flow.md, docs/agents.md, docs/agent-skill.md, docs/install.md, docs/skill-install.md, assets/agent-skills/pactum/**, and scripts/smoke.sh.
  - Update tests to use the new grammar and add positive, negative, help/usage, and `Next:` hint coverage for the renamed and removed commands.
- Out of scope:
  - Redesigning unlisted command groups such as task, prompt, gate, memory, map, search, status, usage, version, and export.
  - Renaming durable artifact paths, JSON schema names, or JSON field names, including dry-run artifact filenames and `dry_run` fields.
  - Removing flexible optional `[run_id]` argument resolution forms.
  - Updating changelog, dogfood reports, backlog notes, or other explicitly historical text unless it presents current guidance.
  - Running real unsandboxed agents such as `pactum execute run` or `pactum review run`.
- Acceptance criteria:
  - Each new command spelling named in the goal is accepted by the CLI, and each removed old spelling, including `clarify list`, `execute status`, and `task current`, is rejected as a command.
  - `pactum execute show` with no attempt id shows the former execute-status summary; `pactum execute show --json` with no attempt id returns the former execute-status summary shape.
  - `pactum execute show [run_id] <attempt_id>` keeps the existing attempt-detail behavior, and `--logs` affects only attempt-detail output.
  - Human-facing current workflow labels, headings, help, usage text, `Next:` hints, suggestions, and hand-written errors use `plan` and the new command names; parser diagnostics may echo an invalid old token only as the rejected input.
  - Current docs, bundled Pactum skill files, active agent guidance, scripts, generated prompt/context text, and JSON command-string values no longer instruct users or agents to run removed command spellings.
  - Tests include positive coverage for new spellings, negative parser/help coverage for removed spellings, and assertions that usage strings and `Next:` hints advertise only the new command names.
- Validation commands:
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 1
- Fresh: 1
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: make check (exit 0, timed out: false, result: gate/validation/command_001/result.json)
- Change summary:
  - changed files:
    - AGENTS.md
    - README.md
    - assets/agent-skills/pactum/SKILL.md
    - assets/agent-skills/pactum/references/safety.md
    - assets/agent-skills/pactum/references/workflow.md
    - docs/agent-skill.md
    - docs/agents.md
    - docs/flow.md
    - docs/install.md
    - docs/memory.md
    - docs/skill-install.md
    - internal/app/agent_attempt_transport_test.go
    - internal/app/app_test.go
    - internal/app/clarify_loop.go
    - internal/app/clarify_loop_test.go
    - internal/app/cli.go
    - internal/app/cli_v2_test.go
    - internal/app/commands.go
    - internal/app/contract_draft_test.go
    - internal/app/dogfood_hardening_test.go
    - internal/app/execute.go
    - internal/app/execute_report.go
    - internal/app/execute_report_test.go
    - internal/app/execute_test.go
    - internal/app/memory_prompt_boundary_test.go
    - internal/app/memory_selection_test.go
    - internal/app/memory_test.go
    - internal/app/prompt.go
    - internal/app/prompt_test.go
    - internal/app/resolve.go
    - internal/app/review.go
    - internal/app/review_fix.go
    - internal/app/review_test.go
    - internal/app/task.go
    - internal/app/task_clarify_test.go
    - internal/docs/docs_test.go
    - internal/docs/packaging_test.go
    - internal/docs/skill_test.go
    - scripts/smoke.sh
  - new files:
    - internal/app/cli_grammar_test.go
    - internal/app/doctor.go
    - internal/app/doctor_test.go
  - missing files:
    - internal/app/agents_doctor.go
    - internal/app/agents_doctor_test.go

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
