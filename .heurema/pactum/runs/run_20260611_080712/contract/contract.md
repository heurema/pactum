# Contract Draft

## Goal
Normalize the CLI command grammar for agent-first use: every stage exposes a uniform verb set, duplicates and aliases are removed, hyphenated pseudo-subcommands become nested subcommands. Renames (hard, no deprecation aliases — the project has no users yet): agents doctor -> doctor; clarify ask -> clarify add; clarify loop -> clarify run; clarify status (and its list alias) -> clarify show; contract show-draft -> contract show --draft; contract accept-draft -> contract accept; execute dry-run -> execute plan; execute status merges into execute show; review dry-run -> review plan; review add-finding -> review finding add; review resolve -> review finding resolve; review accept-proposal -> review proposal accept; review reject-proposal -> review proposal reject; review propose-findings -> review proposal collect; review fix -> review fix run; review apply-fix-outcomes -> review fix apply; task current is dropped (pactum status already reports the current run). All human-output Next: hints, error messages, and docs (flow.md, agents.md, agent-skill.md, README if it lists commands) must reference the new names. Existing JSON output schemas keep their names. Tests updated to invoke the new grammar.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260611_063009
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — What concrete scope is intended by “every stage exposes a uniform verb set”: only the explicit rename/drop list in the goal, or also redesign unlisted command groups such as `task`, `prompt`, `gate`, `memory`, `map`, `search`, `status`, `usage`, `version`, and `export`?
  Rationale: The current CLI defines many stages in `internal/app/cli.go`; the goal lists specific renames but the phrase “every stage” could expand the change into a full CLI taxonomy redesign.
  Answer: Limit this run to the explicit rename/drop list in the goal and the directly affected help, human output, errors, docs, and tests; do not redesign unlisted command groups.
- q_002 [blocking] — For “duplicates and aliases are removed,” should that mean only removing old command spellings such as `clarify list`, `execute dry-run`, and `review add-finding`, or should flexible positional forms like `pactum review resolve f_001` vs `pactum review resolve <run_id> f_001` also be removed?
  Rationale: The repo has a Kong alias for `clarify list`, old command paths, and flexible `[run_id]` parsing via helpers such as `splitLeadingRunID`; these are different kinds of “alias.”
  Answer: Remove only command-name aliases and old renamed command spellings; keep current-run resolution and flexible optional `[run_id]` forms because they are argument resolution behavior, not duplicate command grammar.
- q_003 [blocking] — Should durable artifact names and JSON field names containing old terms, such as `execute/dry-run.json`, `review/reviewer-dry-run.json`, `review/fix/fixer-dry-run.json`, and `dry_run` fields, remain unchanged?
  Rationale: The draft explicitly says existing JSON output schemas keep their names, but the repo also has durable artifact filenames and JSON fields with `dry-run` terminology; renaming those would be a storage-format migration, not just CLI grammar normalization.
  Answer: Keep durable artifact filenames, JSON schema names, and JSON field names unchanged; update only command spellings and human-facing text that presents commands or current workflow instructions.
- q_004 [blocking] — In the concrete scenario where a run has a built prompt but no execution attempts, what should `pactum execute show` do after `execute status` is removed: show the old status summary, or keep the old `execute show` behavior of saying no attempts were found?
  Rationale: `execute status` currently reports prompt readiness, plan artifact existence, attempt count, and last result; `execute show` currently focuses on a specific/latest attempt and prints “No execution attempts found” when none exist.
  Answer: `pactum execute show` with no attempt id should absorb the old status summary behavior; `pactum execute show [run_id] <attempt_id>` should keep the old attempt-detail behavior, with `--logs` applying only when attempt details are shown.
- q_005 [blocking] — Which documentation should be updated: only README, `docs/flow.md`, `docs/agents.md`, and `docs/agent-skill.md`, or also current workflow docs in `assets/agent-skills/pactum/`, `docs/install.md`, `docs/skill-install.md`, and historical files such as dogfood reports and backlog notes?
  Rationale: Repository search finds old commands in current user docs, the bundled Pactum agent skill, install docs, backlog, and dogfood reports. Historical reports may intentionally preserve past commands, while skill/install docs are active instructions.
  Answer: Update all current user-facing instructions and bundled skill workflow files, including README, `docs/flow.md`, `docs/agents.md`, `docs/agent-skill.md`, `docs/install.md`, `docs/skill-install.md`, and `assets/agent-skills/pactum/**`; leave explicitly historical dogfood/backlog text unchanged unless it is presented as current guidance.
- q_006 — For acceptance, should tests verify only that the new command spellings work, or also that every removed old spelling fails and no help/usage output advertises the old names?
  Rationale: The draft says tests are updated and aliases are hard-removed, but does not state whether negative coverage is required. Existing tests heavily invoke old commands, so the test update scope matters.
  Answer: Require positive tests for the new spellings, negative parser/help tests for the removed old spellings and `clarify list`, and assertions that human `Next:` hints and usage strings advertise only the new command names; validate with `make check`.
- q_007 [blocking] — When a user invokes an old removed command such as `pactum execute dry-run` or `pactum contract show-draft`, may the parser error mention the invalid old token, or must all error output avoid old command names entirely?
  Rationale: The goal says all error messages must reference new names, but parser diagnostics often echo the invalid token. Treating that echo as a violation would require custom error handling beyond removing command definitions.
  Answer: Allow parser diagnostics to echo the invalid old token as the cause of failure, but ensure usage text, suggestions, `Next:` hints, and hand-written errors list only the new command names.
- q_008 [blocking] — After `execute dry-run -> execute plan` and `review dry-run -> review plan`, should human-facing labels/headings continue to use “dry-run” as a workflow term, or should “dry-run” be reserved only for durable artifact/schema names such as `execute/dry-run.json` and `dry_run` JSON fields?
  Rationale: The answered artifact/schema clarification preserves `dry-run` filenames and JSON fields, but current human output and docs also say things like `Execution dry-run prepared`, `Dry-run:`, `Reviewer dry-run prepared`, and `dry-run vs run`; this changes visible UX and test expectations.
  Answer: Use `plan` in current human-facing command/workflow labels, headings, `Next:` hints, usage text, and current docs; allow `dry-run` only when referring to preserved artifact paths, JSON fields/schema names, historical records, or explanatory storage details.
- q_009 [blocking] — If `pactum execute show` absorbs the old `execute status` behavior, what JSON shape should `pactum execute show --json` with no `attempt_id` return when attempts may or may not exist?
  Rationale: `execute status --json` currently returns prompt/plan/attempt summary fields, while `execute show --json` returns a specific attempt’s request/result/log excerpts. Merging the commands without a rule would make JSON consumers and tests ambiguous, especially because existing JSON schemas are meant to keep their names.
  Answer: `pactum execute show --json` with no `attempt_id` should always return the old execute-status summary shape; `pactum execute show --json [run_id] <attempt_id>` should return the old execute-show attempt-detail shape, with log excerpts only when `--logs` is passed.
- q_010 — Should active command consumers outside the named docs, specifically `AGENTS.md` and `scripts/smoke.sh`, be updated to the new grammar, while historical files such as `CHANGELOG.md` remain unchanged?
  Rationale: Repository search finds old command spellings in active agent guidance and the smoke script. `scripts/smoke.sh` currently invokes `pactum agents doctor`, which will break after hard removal, while changelog/history entries may intentionally describe past behavior.
  Answer: Update active agent guidance and executable/dev workflow scripts such as `AGENTS.md` and `scripts/smoke.sh` to the new grammar; leave changelog and explicitly historical reports unchanged unless they present current instructions.
- q_011 — Should command strings embedded in machine-readable or generated agent-facing outputs also be updated to the new grammar, even though JSON field names and schema names remain unchanged?
  Rationale: The answered artifact/schema clarification keeps JSON schemas and durable artifact names unchanged, but repository search shows executable command strings in JSON affordances and generated prompt text, such as `status.runs.next_command`, `suggested_command`, and the review fixer prompt line that currently says not to run `pactum review resolve`. Leaving those values on old command names would give agents stale legal moves even if terminal `Next:` hints and docs are updated.
  Answer: Update all current command-string values and generated agent prompt/context text that present legal commands to the new grammar, including JSON `next_command` and `suggested_command` values and generated fixer/reviewer/executor instructions; keep JSON field names, schema names, and durable artifact paths unchanged.

## In scope
- Implement only the explicit CLI rename and removal list from the goal, with hard removal of old command-name spellings and no deprecation aliases.
- Merge `execute status` behavior into `execute show` when no attempt id is provided, while preserving attempt-detail behavior for `execute show [run_id] <attempt_id>`.
- Update help, usage, `Next:` hints, hand-written errors, generated agent prompt/context text, and machine-readable command-string values such as `next_command` and `suggested_command` to advertise only the new command grammar.
- Update active current documentation and workflow consumers, including README, AGENTS.md, docs/flow.md, docs/agents.md, docs/agent-skill.md, docs/install.md, docs/skill-install.md, assets/agent-skills/pactum/**, and scripts/smoke.sh.
- Update tests to use the new grammar and add positive, negative, help/usage, and `Next:` hint coverage for the renamed and removed commands.

## Out of scope
- Redesigning unlisted command groups such as task, prompt, gate, memory, map, search, status, usage, version, and export.
- Renaming durable artifact paths, JSON schema names, or JSON field names, including dry-run artifact filenames and `dry_run` fields.
- Removing flexible optional `[run_id]` argument resolution forms.
- Updating changelog, dogfood reports, backlog notes, or other explicitly historical text unless it presents current guidance.
- Running real unsandboxed agents such as `pactum execute run` or `pactum review run`.

## Acceptance criteria
- Each new command spelling named in the goal is accepted by the CLI, and each removed old spelling, including `clarify list`, `execute status`, and `task current`, is rejected as a command.
- `pactum execute show` with no attempt id shows the former execute-status summary; `pactum execute show --json` with no attempt id returns the former execute-status summary shape.
- `pactum execute show [run_id] <attempt_id>` keeps the existing attempt-detail behavior, and `--logs` affects only attempt-detail output.
- Human-facing current workflow labels, headings, help, usage text, `Next:` hints, suggestions, and hand-written errors use `plan` and the new command names; parser diagnostics may echo an invalid old token only as the rejected input.
- Current docs, bundled Pactum skill files, active agent guidance, scripts, generated prompt/context text, and JSON command-string values no longer instruct users or agents to run removed command spellings.
- Tests include positive coverage for new spellings, negative parser/help coverage for removed spellings, and assertions that usage strings and `Next:` hints advertise only the new command names.

## Validation commands
- make check

## Assumptions
- Historical files are identified by context and filename purpose; changelog, dogfood, backlog, and dated report content can retain old commands when describing past behavior.
- Preserving existing JSON output schemas includes preserving schema identifiers and field names even when the CLI command that emits them is renamed or merged.

## Open questions
- None
