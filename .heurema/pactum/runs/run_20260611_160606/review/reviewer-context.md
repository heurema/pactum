# Reviewer Context

## Run
- Run id: run_20260611_160606
- Run status: contract_approved

## Contract
- Goal: Smooth the pipeline so no command is pure ritual, then compress the agent skill against the final grammar. This is the last grammar break; hard removals, no aliases. (1) review run absorbs the loop: pactum review run becomes today's review loop semantics — implicit prepare when the gate report exists, panel-times-lenses rounds, fixer, severity-gated convergence; the single panel round without a fixer remains available as review run --no-fix --max-rounds 1; the review loop command is removed (parser error). (2) review prepare is removed: review run prepares implicitly, and manual review mutations (review finding add) self-scaffold the review when a gate report exists; the review_not_prepared error code disappears — the remaining precondition is gate_report_missing, and affordance fix/next values must reflect that. (3) prompt build self-heals a stale project map: when the map is stale it runs the refresh itself, says so in human output, exposes it in --json, and records it in the prompt manifest; the project_map_stale failure disappears from prompt build (other commands keep it). (4) clarify suggest is removed: its semantics fold into clarify run --no-auto --max-rounds 1 (new --no-auto flag disables auto-resolution of high-confidence recommendations). (5) clarify answer gains --recommended: pactum clarify answer <q_id> --recommended records that question's stored recommended answer as the answer (error when the question has no recommendation), and pactum clarify answer --all-recommended answers every open question that carries a recommendation, skipping and reporting those without; both are decision verbs honoring --by and recording answer provenance distinguishable from a typed answer. (6) The skill is rewritten against the final grammar: assets/agent-skills/pactum/SKILL.md, references/workflow.md, references/safety.md, and docs/agent-skill.md compress to the final convention — every stage exposes run and show, decision verbs relay explicit human decisions with --by, agents read next and error.fix instead of memorizing stage order, execute run is unsandboxed per SECURITY.md. (7) All Next: hints, affordance next/fix command strings, docs (flow.md, agents.md, README if affected), and tests speak only the new surface; negative parser coverage for the removed spellings (review loop, review prepare, clarify suggest) and forbidden-phrase guards in the docs/skill tests. Existing JSON schema names and artifact paths unchanged; review plan and review fix run/apply stay as surgical commands.
- In scope:
  - Remove the `pactum review loop`, `pactum review prepare`, and `pactum clarify suggest` command spellings from the CLI grammar so they fail as parser errors with no aliases.
  - Make `pactum review run` absorb the existing review loop behavior, including reviewer panel and lenses rounds, fixer invocation, convergence over open blocking findings, and support for `--reviewer`, `--agent`, `--max-rounds`, `--patience`, `--clean-rounds`, `--timeout`, `--json`, and new `--no-fix`.
  - Make `review run`, `review finding add`, and `review approve` auto-scaffold `review/review.json` when `gate/report.json` exists; keep `review status/show` read-only and show a derived empty pending review state when no review artifact exists.
  - Remove `review_not_prepared` as a user-facing error path and update remaining review preconditions, affordance `next`, and affordance `fix` values to use `gate_report_missing` or ordinary missing-record errors as appropriate.
  - Make `pactum prompt build` refresh a stale project map itself, report the refresh in human output, expose a `map_refresh` object in `--json`, and write the same additive object to `contract/prompt-manifest.json` while preserving existing schema names and artifact paths.
  - Replace `clarify suggest` behavior with `pactum clarify run --no-auto --max-rounds 1`, using the clarify-loop output and `clarify/loop-summary.json` artifact surface.
  - Add `pactum clarify answer <q_id> --recommended` and `pactum clarify answer --all-recommended` decision verbs that honor `--by`, record recommended answers, preserve typed/manual and auto-loop provenance, and persist distinguishable recommended-answer provenance.
  - Rewrite `assets/agent-skills/pactum/SKILL.md`, `assets/agent-skills/pactum/references/workflow.md`, `assets/agent-skills/pactum/references/safety.md`, and `docs/agent-skill.md` to describe only the final grammar and current safety model.
  - Update `docs/flow.md`, `docs/agents.md`, `README.md` if affected, CLI help, `Next:` hints, affordance command strings, and tests so they mention only the new command surface.
- Out of scope:
  - Do not add broad new stage aliases or unrelated command families such as `pactum contract run`, `pactum prompt run`, or `pactum memory run`.
  - Do not rename unrelated existing stage verbs such as `prompt build`, `execute plan`, `gate run`, `memory propose`, or `memory accept`.
  - Do not change existing JSON schema names or artifact paths except for additive fields required by this contract.
  - Do not introduce a new severity threshold for review convergence; convergence and fixer invocation remain based on open findings with `blocking=true`.
  - Do not change `review plan` or the surgical `review fix run/apply` commands except where needed for help text, precondition errors, and removed `review prepare` references.
  - Do not preserve compatibility aliases for `review loop`, `review prepare`, or `clarify suggest`.
- Acceptance criteria:
  - `pactum review loop`, `pactum review prepare`, and `pactum clarify suggest` fail at parse time, and negative CLI grammar tests cover all three removed spellings.
  - `pactum review run` accepts the former loop control flags plus `--no-fix`, runs loop semantics by default, writes the loop summary, and converges based on open blocking findings rather than raw severity values.
  - `pactum review run --no-fix --max-rounds 1` runs one reviewer panel/lenses pass, accepts valid finding proposals into review findings, skips fixer execution, exits 0 when findings remain open, records a terminal reason such as `findings_open` or `no_fix`, and points `next` to `pactum review show <run_id>`.
  - `pactum review run --no-fix --max-rounds 2` stops after the first round that leaves open blocking findings instead of continuing reviewer-only churn.
  - `review run`, `review finding add`, and `review approve` create the review scaffold when `gate/report.json` exists; `review finding resolve`, proposal accept/reject, and `review fix run/apply` still require relevant existing review records.
  - `review status` and `review show` do not mutate files and show a derived empty pending review state for a gated run with no review artifact.
  - `project_map_stale` is no longer a `prompt build` failure; when a stale map is detected, `prompt build` refreshes the map, human output names the refresh and new map run id, `--json` includes `map_refresh`, and `contract/prompt-manifest.json` includes the same additive `map_refresh` object.
  - `map_refresh` is `{ "triggered": false }` when no prompt-build refresh was needed, and `{ "triggered": true, "reason": "project_map_stale", "previous_map_run_id": "...", "run_id": "..." }` when a refresh occurred.
  - `pactum clarify run --no-auto --max-rounds 1` runs one clarifier round, records created questions, performs no auto-resolution, writes `clarify/loop-summary.json`, and reports `terminal_reason: needs_human` when open blocking questions remain.
  - `pactum clarify answer <q_id> --recommended` errors when the question is not currently open, has no non-empty stored recommendation, or is blocked by unanswered dependencies; it records `source: manual_recommended` in `answers.jsonl` and `source: manual_recommended_answer` in `decisions.jsonl` when successful.
  - `pactum clarify answer --all-recommended` answers currently open recommended questions in dependency order, skips open questions without recommendations and dependency-blocked questions, reports skipped IDs in human and JSON output, and records `source: manual_all_recommended` / `manual_all_recommended_answer` for each recorded answer.
  - All recommended-answer decision paths normalize and persist `decided_by` from `--by`.
  - Docs and agent skill files contain no instructions to use `review loop`, `review prepare`, or `clarify suggest`, and describe agents reading `next` and `error.fix` rather than memorizing stage order.
  - `execute run` safety language in the skill and docs matches `SECURITY.md` and states that real agent execution is unsandboxed.
- Validation commands:
  - make check
  - go test ./...
  - rg "review loop|review prepare|clarify suggest|review_not_prepared|project_map_stale" docs assets/agent-skills README.md internal/app

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 3
- Stale: 2
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: failed
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: make check (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./... (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: rg "review loop|review prepare|clarify suggest|review_not_prepared|project_map_stale" docs assets/agent-skills README.md internal/app (exit 2, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - CHANGELOG.md
    - README.md
    - SECURITY.md
    - assets/agent-skills/pactum/SKILL.md
    - assets/agent-skills/pactum/references/safety.md
    - assets/agent-skills/pactum/references/workflow.md
    - docs/agent-skill.md
    - docs/agents.md
    - docs/backlog.md
    - docs/cost-budget-design.md
    - docs/flow.md
    - docs/loop-architecture-design.md
    - internal/agents/acp_transport.go
    - internal/agents/types.go
    - internal/app/affordances_test.go
    - internal/app/agent_attempt.go
    - internal/app/agent_attempt_timeout_test.go
    - internal/app/agent_attempt_transport_test.go
    - internal/app/agent_resolve.go
    - internal/app/clarify.go
    - internal/app/clarify_loop.go
    - internal/app/clarify_test.go
    - internal/app/cli.go
    - internal/app/cli_grammar_test.go
    - internal/app/cli_v2_test.go
    - internal/app/commands.go
    - internal/app/errors.go
    - internal/app/memory.go
    - internal/app/memory_prompt_boundary_test.go
    - internal/app/memory_test.go
    - internal/app/prompt.go
    - internal/app/prompt_test.go
    - internal/app/resolve.go
    - internal/app/review.go
    - internal/app/review_fix.go
    - internal/app/review_fix_outcomes.go
    - internal/app/review_loop.go
    - internal/app/review_loop_test.go
    - internal/app/review_proposals.go
    - internal/app/review_test.go
    - internal/docs/docs_test.go
    - internal/docs/skill_test.go
  - new files:
    - internal/app/clarify_round.go
    - internal/app/clarify_round_test.go
  - missing files:
    - internal/app/clarify_suggest.go
    - internal/app/clarify_suggest_test.go

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
