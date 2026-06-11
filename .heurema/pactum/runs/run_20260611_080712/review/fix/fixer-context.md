# Review Fixer Context

## Run
- Run id: run_20260611_080712
- Run status: contract_approved

## Approved contract
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

## Current review findings
- Summary: findings=6 open=6 resolved=0 blocking_open=1
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_006 severity=medium category=quality blocking=true status=open: Negative parser coverage omits the removed `review fix` spelling.
    location: internal/app/cli_grammar_test.go:60
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=low category=quality blocking=false status=open: docs/agents.md still says attempt logs are "surfaced by `pactum execute show --logs`", but after the execute status/show merge that invocation (no attempt id) takes the run-summary path and silently ignores --logs (internal/app/execute_report.go:81-91). The documented command no longer does what the sentence claims; it should read `pactum execute show <attempt_id> --logs`.
    location: docs/agents.md:346
  - f_002 severity=low category=quality blocking=false status=open: All nine renamed hand-written usage-error strings in commands.go (clarify add/answer, execute show, review finding add/resolve, review proposal collect/accept/reject, review fix apply) have zero test assertions; reverting one to an old spelling would keep make check green. Acceptance criteria 4 and 6 name hand-written errors and usage strings as needing new-grammar assertions; the new help-output tests do not reach these paths.
    location: internal/app/commands.go:57
  - f_003 severity=low category=quality blocking=false status=open: No test exercises `execute show <run_id> --logs` in summary mode (no attempt id), so the acceptance criterion '`--logs` affects only attempt-detail output' is untested on the summary side; a refactor leaking logs into the summary path would not fail any test.
    location: internal/app/execute_report_test.go:28
  - f_004 severity=low category=quality blocking=false status=open: TestSkillDocsAvoidStaleAndPrematureClaims forbids the removed spellings in skill docs and AGENTS.md but omits 'pactum clarify list', which docs_test.go:45 forbids for the main docs and which acceptance criterion 1 explicitly names; the alias could be reintroduced into agent-facing skill docs without failing tests.
    location: internal/docs/skill_test.go:196
  - f_005 severity=low category=quality blocking=false status=open: docs/loop-architecture-design.md describes current shipped behavior using removed command spellings: line 261 says clarify.max_rounds is 'live again since M17.0' and 'enforced by `pactum clarify loop`' (renamed to `clarify run`), and lines 286-287 under 'Already matches the target' describe the shipped reviewer flow as '`review run` -> `propose-findings`, plus `resolve` / `approve`' (now `review proposal collect` and `review finding resolve`). The doc header says 'Draft / proposal (not yet implemented)', but these passages present current state, which the contract's historical-text carve-out ('unless it presents current guidance') covers. Advisory: the file was not in the contract's named doc-update list.
    location: docs/loop-architecture-design.md:261

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
