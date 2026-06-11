# Memory Candidate

## Run
- Run id: run_20260611_080712
- Source: deterministic

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

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
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
- New files:
  - internal/app/cli_grammar_test.go
  - internal/app/doctor.go
  - internal/app/doctor_test.go
- Missing files:
  - internal/app/agents_doctor.go
  - internal/app/agents_doctor_test.go

## Clarifications
- q_001: What concrete scope is intended by “every stage exposes a uniform verb set”: only the explicit rename/drop list in the goal, or also redesign unlisted command groups such as `task`, `prompt`, `gate`, `memory`, `map`, `search`, `status`, `usage`, `version`, and `export`?
  Answer: Limit this run to the explicit rename/drop list in the goal and the directly affected help, human output, errors, docs, and tests; do not redesign unlisted command groups.
- q_002: For “duplicates and aliases are removed,” should that mean only removing old command spellings such as `clarify list`, `execute dry-run`, and `review add-finding`, or should flexible positional forms like `pactum review resolve f_001` vs `pactum review resolve <run_id> f_001` also be removed?
  Answer: Remove only command-name aliases and old renamed command spellings; keep current-run resolution and flexible optional `[run_id]` forms because they are argument resolution behavior, not duplicate command grammar.
- q_003: Should durable artifact names and JSON field names containing old terms, such as `execute/dry-run.json`, `review/reviewer-dry-run.json`, `review/fix/fixer-dry-run.json`, and `dry_run` fields, remain unchanged?
  Answer: Keep durable artifact filenames, JSON schema names, and JSON field names unchanged; update only command spellings and human-facing text that presents commands or current workflow instructions.
- q_004: In the concrete scenario where a run has a built prompt but no execution attempts, what should `pactum execute show` do after `execute status` is removed: show the old status summary, or keep the old `execute show` behavior of saying no attempts were found?
  Answer: `pactum execute show` with no attempt id should absorb the old status summary behavior; `pactum execute show [run_id] <attempt_id>` should keep the old attempt-detail behavior, with `--logs` applying only when attempt details are shown.
- q_005: Which documentation should be updated: only README, `docs/flow.md`, `docs/agents.md`, and `docs/agent-skill.md`, or also current workflow docs in `assets/agent-skills/pactum/`, `docs/install.md`, `docs/skill-install.md`, and historical files such as dogfood reports and backlog notes?
  Answer: Update all current user-facing instructions and bundled skill workflow files, including README, `docs/flow.md`, `docs/agents.md`, `docs/agent-skill.md`, `docs/install.md`, `docs/skill-install.md`, and `assets/agent-skills/pactum/**`; leave explicitly historical dogfood/backlog text unchanged unless it is presented as current guidance.
- q_006: For acceptance, should tests verify only that the new command spellings work, or also that every removed old spelling fails and no help/usage output advertises the old names?
  Answer: Require positive tests for the new spellings, negative parser/help tests for the removed old spellings and `clarify list`, and assertions that human `Next:` hints and usage strings advertise only the new command names; validate with `make check`.
- q_007: When a user invokes an old removed command such as `pactum execute dry-run` or `pactum contract show-draft`, may the parser error mention the invalid old token, or must all error output avoid old command names entirely?
  Answer: Allow parser diagnostics to echo the invalid old token as the cause of failure, but ensure usage text, suggestions, `Next:` hints, and hand-written errors list only the new command names.
- q_008: After `execute dry-run -> execute plan` and `review dry-run -> review plan`, should human-facing labels/headings continue to use “dry-run” as a workflow term, or should “dry-run” be reserved only for durable artifact/schema names such as `execute/dry-run.json` and `dry_run` JSON fields?
  Answer: Use `plan` in current human-facing command/workflow labels, headings, `Next:` hints, usage text, and current docs; allow `dry-run` only when referring to preserved artifact paths, JSON fields/schema names, historical records, or explanatory storage details.
- q_009: If `pactum execute show` absorbs the old `execute status` behavior, what JSON shape should `pactum execute show --json` with no `attempt_id` return when attempts may or may not exist?
  Answer: `pactum execute show --json` with no `attempt_id` should always return the old execute-status summary shape; `pactum execute show --json [run_id] <attempt_id>` should return the old execute-show attempt-detail shape, with log excerpts only when `--logs` is passed.
- q_010: Should active command consumers outside the named docs, specifically `AGENTS.md` and `scripts/smoke.sh`, be updated to the new grammar, while historical files such as `CHANGELOG.md` remain unchanged?
  Answer: Update active agent guidance and executable/dev workflow scripts such as `AGENTS.md` and `scripts/smoke.sh` to the new grammar; leave changelog and explicitly historical reports unchanged unless they present current instructions.
- q_011: Should command strings embedded in machine-readable or generated agent-facing outputs also be updated to the new grammar, even though JSON field names and schema names remain unchanged?
  Answer: Update all current command-string values and generated agent prompt/context text that present legal commands to the new grammar, including JSON `next_command` and `suggested_command` values and generated fixer/reviewer/executor instructions; keep JSON field names, schema names, and durable artifact paths unchanged.

## Review Decisions
- f_001 [low] resolved docs/agents.md:346: docs/agents.md still says attempt logs are "surfaced by `pactum execute show --logs`", but after the execute status/show merge that invocation (no attempt id) takes the run-summary path and silently ignores --logs (internal/app/execute_report.go:81-91). The documented command no longer does what the sentence claims; it should read `pactum execute show <attempt_id> --logs`.
- f_002 [low] resolved internal/app/commands.go:57: All nine renamed hand-written usage-error strings in commands.go (clarify add/answer, execute show, review finding add/resolve, review proposal collect/accept/reject, review fix apply) have zero test assertions; reverting one to an old spelling would keep make check green. Acceptance criteria 4 and 6 name hand-written errors and usage strings as needing new-grammar assertions; the new help-output tests do not reach these paths.
- f_003 [low] resolved internal/app/execute_report_test.go:28: No test exercises `execute show <run_id> --logs` in summary mode (no attempt id), so the acceptance criterion '`--logs` affects only attempt-detail output' is untested on the summary side; a refactor leaking logs into the summary path would not fail any test.
- f_004 [low] resolved internal/docs/skill_test.go:196: TestSkillDocsAvoidStaleAndPrematureClaims forbids the removed spellings in skill docs and AGENTS.md but omits 'pactum clarify list', which docs_test.go:45 forbids for the main docs and which acceptance criterion 1 explicitly names; the alias could be reintroduced into agent-facing skill docs without failing tests.
- f_005 [low] resolved docs/loop-architecture-design.md:261: docs/loop-architecture-design.md describes current shipped behavior using removed command spellings: line 261 says clarify.max_rounds is 'live again since M17.0' and 'enforced by `pactum clarify loop`' (renamed to `clarify run`), and lines 286-287 under 'Already matches the target' describe the shipped reviewer flow as '`review run` -> `propose-findings`, plus `resolve` / `approve`' (now `review proposal collect` and `review finding resolve`). The doc header says 'Draft / proposal (not yet implemented)', but these passages present current state, which the contract's historical-text carve-out ('unless it presents current guidance') covers. Advisory: the file was not in the contract's named doc-update list.
- f_006 [medium] resolved internal/app/cli_grammar_test.go:60: Negative parser coverage omits the removed `review fix` spelling.
  Resolution: Added the bare {"review", "fix"} invocation to the negative parser table in TestRemovedCommandSpellingsAreRejected (internal/app/cli_grammar_test.go:74). The removed single-verb spelling is now pinned as a parser error (exit 2, non-empty stderr) since review fix is a group requiring run/apply. Verified with the targeted test and full make check.
- Proposal summary: pending=0 accepted=6 rejected=0

## Reusable Project Knowledge
- scope: in scope: Implement only the explicit CLI rename and removal list from the goal, with hard removal of old command-name spellings and no deprecation aliases.
- scope: in scope: Merge `execute status` behavior into `execute show` when no attempt id is provided, while preserving attempt-detail behavior for `execute show [run_id] <attempt_id>`.
- scope: in scope: Update help, usage, `Next:` hints, hand-written errors, generated agent prompt/context text, and machine-readable command-string values such as `next_command` and `suggested_command` to advertise only the new command grammar.
- scope: in scope: Update active current documentation and workflow consumers, including README, AGENTS.md, docs/flow.md, docs/agents.md, docs/agent-skill.md, docs/install.md, docs/skill-install.md, assets/agent-skills/pactum/**, and scripts/smoke.sh.
- scope: in scope: Update tests to use the new grammar and add positive, negative, help/usage, and `Next:` hint coverage for the renamed and removed commands.
- scope: out of scope: Redesigning unlisted command groups such as task, prompt, gate, memory, map, search, status, usage, version, and export.
- scope: out of scope: Renaming durable artifact paths, JSON schema names, or JSON field names, including dry-run artifact filenames and `dry_run` fields.
- scope: out of scope: Removing flexible optional `[run_id]` argument resolution forms.
- scope: out of scope: Updating changelog, dogfood reports, backlog notes, or other explicitly historical text unless it presents current guidance.
- scope: out of scope: Running real unsandboxed agents such as `pactum execute run` or `pactum review run`.
- clarification: q_001: What concrete scope is intended by “every stage exposes a uniform verb set”: only the explicit rename/drop list in the goal, or also redesign unlisted command groups such as `task`, `prompt`, `gate`, `memory`, `map`, `search`, `status`, `usage`, `version`, and `export`? Answer: Limit this run to the explicit rename/drop list in the goal and the directly affected help, human output, errors, docs, and tests; do not redesign unlisted command groups.
- clarification: q_002: For “duplicates and aliases are removed,” should that mean only removing old command spellings such as `clarify list`, `execute dry-run`, and `review add-finding`, or should flexible positional forms like `pactum review resolve f_001` vs `pactum review resolve <run_id> f_001` also be removed? Answer: Remove only command-name aliases and old renamed command spellings; keep current-run resolution and flexible optional `[run_id]` forms because they are argument resolution behavior, not duplicate command grammar.
- clarification: q_003: Should durable artifact names and JSON field names containing old terms, such as `execute/dry-run.json`, `review/reviewer-dry-run.json`, `review/fix/fixer-dry-run.json`, and `dry_run` fields, remain unchanged? Answer: Keep durable artifact filenames, JSON schema names, and JSON field names unchanged; update only command spellings and human-facing text that presents commands or current workflow instructions.
- clarification: q_004: In the concrete scenario where a run has a built prompt but no execution attempts, what should `pactum execute show` do after `execute status` is removed: show the old status summary, or keep the old `execute show` behavior of saying no attempts were found? Answer: `pactum execute show` with no attempt id should absorb the old status summary behavior; `pactum execute show [run_id] <attempt_id>` should keep the old attempt-detail behavior, with `--logs` applying only when attempt details are shown.
- clarification: q_005: Which documentation should be updated: only README, `docs/flow.md`, `docs/agents.md`, and `docs/agent-skill.md`, or also current workflow docs in `assets/agent-skills/pactum/`, `docs/install.md`, `docs/skill-install.md`, and historical files such as dogfood reports and backlog notes? Answer: Update all current user-facing instructions and bundled skill workflow files, including README, `docs/flow.md`, `docs/agents.md`, `docs/agent-skill.md`, `docs/install.md`, `docs/skill-install.md`, and `assets/agent-skills/pactum/**`; leave explicitly historical dogfood/backlog text unchanged unless it is presented as current guidance.
- clarification: q_006: For acceptance, should tests verify only that the new command spellings work, or also that every removed old spelling fails and no help/usage output advertises the old names? Answer: Require positive tests for the new spellings, negative parser/help tests for the removed old spellings and `clarify list`, and assertions that human `Next:` hints and usage strings advertise only the new command names; validate with `make check`.
- clarification: q_007: When a user invokes an old removed command such as `pactum execute dry-run` or `pactum contract show-draft`, may the parser error mention the invalid old token, or must all error output avoid old command names entirely? Answer: Allow parser diagnostics to echo the invalid old token as the cause of failure, but ensure usage text, suggestions, `Next:` hints, and hand-written errors list only the new command names.
- clarification: q_008: After `execute dry-run -> execute plan` and `review dry-run -> review plan`, should human-facing labels/headings continue to use “dry-run” as a workflow term, or should “dry-run” be reserved only for durable artifact/schema names such as `execute/dry-run.json` and `dry_run` JSON fields? Answer: Use `plan` in current human-facing command/workflow labels, headings, `Next:` hints, usage text, and current docs; allow `dry-run` only when referring to preserved artifact paths, JSON fields/schema names, historical records, or explanatory storage details.
- clarification: q_009: If `pactum execute show` absorbs the old `execute status` behavior, what JSON shape should `pactum execute show --json` with no `attempt_id` return when attempts may or may not exist? Answer: `pactum execute show --json` with no `attempt_id` should always return the old execute-status summary shape; `pactum execute show --json [run_id] <attempt_id>` should return the old execute-show attempt-detail shape, with log excerpts only when `--logs` is passed.
- clarification: q_010: Should active command consumers outside the named docs, specifically `AGENTS.md` and `scripts/smoke.sh`, be updated to the new grammar, while historical files such as `CHANGELOG.md` remain unchanged? Answer: Update active agent guidance and executable/dev workflow scripts such as `AGENTS.md` and `scripts/smoke.sh` to the new grammar; leave changelog and explicitly historical reports unchanged unless they present current instructions.
- clarification: q_011: Should command strings embedded in machine-readable or generated agent-facing outputs also be updated to the new grammar, even though JSON field names and schema names remain unchanged? Answer: Update all current command-string values and generated agent prompt/context text that present legal commands to the new grammar, including JSON `next_command` and `suggested_command` values and generated fixer/reviewer/executor instructions; keep JSON field names, schema names, and durable artifact paths unchanged.
- review_resolution: f_001 resolved: docs/agents.md still says attempt logs are "surfaced by `pactum execute show --logs`", but after the execute status/show merge that invocation (no attempt id) takes the run-summary path and silently ignores --logs (internal/app/execute_report.go:81-91). The documented command no longer does what the sentence claims; it should read `pactum execute show <attempt_id> --logs`.
- review_resolution: f_002 resolved: All nine renamed hand-written usage-error strings in commands.go (clarify add/answer, execute show, review finding add/resolve, review proposal collect/accept/reject, review fix apply) have zero test assertions; reverting one to an old spelling would keep make check green. Acceptance criteria 4 and 6 name hand-written errors and usage strings as needing new-grammar assertions; the new help-output tests do not reach these paths.
- review_resolution: f_003 resolved: No test exercises `execute show <run_id> --logs` in summary mode (no attempt id), so the acceptance criterion '`--logs` affects only attempt-detail output' is untested on the summary side; a refactor leaking logs into the summary path would not fail any test.
- review_resolution: f_004 resolved: TestSkillDocsAvoidStaleAndPrematureClaims forbids the removed spellings in skill docs and AGENTS.md but omits 'pactum clarify list', which docs_test.go:45 forbids for the main docs and which acceptance criterion 1 explicitly names; the alias could be reintroduced into agent-facing skill docs without failing tests.
- review_resolution: f_005 resolved: docs/loop-architecture-design.md describes current shipped behavior using removed command spellings: line 261 says clarify.max_rounds is 'live again since M17.0' and 'enforced by `pactum clarify loop`' (renamed to `clarify run`), and lines 286-287 under 'Already matches the target' describe the shipped reviewer flow as '`review run` -> `propose-findings`, plus `resolve` / `approve`' (now `review proposal collect` and `review finding resolve`). The doc header says 'Draft / proposal (not yet implemented)', but these passages present current state, which the contract's historical-text carve-out ('unless it presents current guidance') covers. Advisory: the file was not in the contract's named doc-update list.
- review_resolution: f_006 resolved: Negative parser coverage omits the removed `review fix` spelling.; resolution: Added the bare {"review", "fix"} invocation to the negative parser table in TestRemovedCommandSpellingsAreRejected (internal/app/cli_grammar_test.go:74). The removed single-verb spelling is now pinned as a parser error (exit 2, non-empty stderr) since review fix is a group requiring run/apply. Verified with the targeted test and full make check.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
