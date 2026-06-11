# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260611_092851
- Approval: approved
- Contract hash: d45525ca3c0703eeb324b80d78656a2b92317bc162791ab989eae78512ff830f

## Goal
Remove the interactive confirmation layer from the CLI: the consumer is an AI agent relaying decisions already made in conversation, so the CLI must never prompt. Delete interactive confirm prompts and the --yes flag from every command that has one (execute run, review run, review loop via clarify run, clarify run, clarify suggest, contract draft, task new --clarify, and any other); after the change the commands simply run, and --yes is rejected as an unknown flag (hard removal, no users yet). Delete gate run --allow-commands: running the contract validation commands is the gate's purpose, so gate run always runs them. Make the --by principal flag uniform across all decision verbs: optional with default value manual on contract approve, review approve, memory accept (today it may be required there), and extend it to contract accept, clarify answer, review proposal accept, review proposal reject — recorded in the respective decision artifacts and ledger events the same way approved_by is recorded today. The task new --clarify guard requiring --yes is removed together with the flag. All Next: hints, hand-written errors, helper text, docs (README, AGENTS.md, docs tree, bundled skill under assets/agent-skills/pactum), and scripts must stop mentioning --yes and --allow-commands as current guidance. Tests updated: confirmation-prompt tests removed or inverted, --by recording covered for the newly extended verbs, and negative coverage that --yes and --allow-commands are rejected.

## In scope
- Remove Pactum's own interactive confirmation implementation and every current `--yes` CLI flag/guard from agent-running commands, including clarify suggest/run, contract draft, execute run, review run, review fix run, review loop, task new --clarify, and any other current `Yes` command field.
- Remove `gate run --allow-commands` so `gate run` always executes contract validation commands and reports `validation.commands_allowed: true` with an empty commands array when no validation commands exist.
- Add optional `--by` with default `manual` to contract accept, clarify answer, review proposal accept, and review proposal reject; keep/default it on contract approve, review approve, and memory accept.
- Persist explicit CLI principals only in semantic decision artifacts: `accepted_by` for contract draft proposal acceptance and `decided_by` for clarification and review proposal decisions; leave ledger event schema unchanged.
- Update current CLI help, Next hints, hand-written errors, README, AGENTS guidance, docs current guidance, bundled Pactum skill references, scripts, and tests so `--yes` and `--allow-commands` are not presented as current guidance.

## Out of scope
- Changing built-in agent transport permission prompts, sandbox behavior, read-only/write-scope behavior, or external agent approval semantics.
- Adding principal fields to automatic loop-created decisions such as clarify-loop auto answers or review-loop duplicate proposal records.
- Adding `--by` to commands other than contract approve, review approve, memory accept, contract accept, clarify answer, review proposal accept, and review proposal reject.
- Rewriting clearly historical backlog or dogfood transcripts solely because they contain old `--yes` or `--allow-commands` examples.

## Acceptance criteria
- `pactum --help` and subcommand help no longer expose `--yes` or `--allow-commands`; passing either removed flag to formerly affected commands is rejected as an unknown flag.
- Formerly guarded Pactum commands run without Pactum confirmation prompts or non-interactive `--yes` refusal errors; no code path calls `confirmDirectExecution`.
- `gate run` runs configured validation commands without an allow flag and successful gate reports always include `validation.commands_allowed: true`.
- `contract accept`, `clarify answer`, `review proposal accept`, and `review proposal reject` accept optional `--by`, default to `manual`, trim whitespace, sanitize repo-root absolute path text consistently with memory acceptance, and persist the resulting principal in the clarified artifact fields.
- Tests cover removed flag rejection, no-prompt execution behavior, gate execution without `--allow-commands`, and `--by` persistence/defaulting for all explicitly principal-bearing decision verbs.
- Current guidance in README, AGENTS.md, docs/agents.md, docs/flow.md, docs/agent-skill.md, bundled Pactum skill files, helper text, Next hints, and scripts no longer instruct users to pass `--yes` or `--allow-commands`.

## Validation commands
- go test ./internal/app ./internal/docs
- make check

## Assumptions
- The CLI parser's normal unknown-flag behavior is sufficient for removed `--yes` and `--allow-commands` flags.
- Historical documents may retain old command transcripts when they are clearly archival rather than current instructions.

## Clarifications
- q_001 [blocking] When the contract says the CLI must never prompt, is the scope limited to Pactum's own interactive confirmation prompts and `--yes` guards, leaving underlying agent transport behavior unchanged?
  Rationale: The repo's only direct stdin confirmation path is `internal/app/confirm.go`, used by agent-running Pactum commands. Docs also mention agent transports and permission behavior, so changing underlying agent prompting or sandbox/permission semantics would be a much larger scope than removing Pactum's confirmation layer.
  Decision: Limit the change to Pactum's own confirmation layer: remove `confirmDirectExecution`, all `Yes` command fields, and all Pactum errors/help/docs requiring `--yes`; do not change built-in agent transport permission, sandbox, read-only, or write-scope behavior except where those commands now run without Pactum prompting.
- q_002 [blocking] For the new `--by <principal>` support, what concrete artifact fields should store the principal, and should `ledger.Event` gain principal metadata?
  Rationale: Existing approvals persist `approved_by` in approval artifacts, but `internal/ledger/events.go` events only contain `type`, `timestamp`, and `run_id`. `contract accept` already has `accepted_by`; `clarify answer` and `review proposal accept|reject` currently have only `source` in their decision records.
  Decision: Persist the principal in semantic decision-artifact fields only: keep/use `accepted_by` on `contract/draft-proposal.json`, add `decided_by` to clarification decision records and review proposal decision records, and leave `source` as provenance. Do not extend the shared ledger event schema; ledger events continue recording that the decision occurred, matching today's approval events.
- q_003 Should automatic loop-created decisions get a `--by` principal field, for example clarify-loop auto answers and review-loop duplicate proposal decisions?
  Rationale: The repo has non-CLI decision writes: `autoResolveClarifications` records `source: auto_recommended` / `clarify_loop_auto`, and `recordDuplicateReviewLoopProposal` records `source: review_loop`. The contract names user-facing decision verbs, not these internal loop decisions.
  Decision: Do not add a principal to automatic loop decisions. Keep their existing `source` values as the actor/provenance signal, and only record `decided_by`/`accepted_by` for explicit CLI decision verbs that accept `--by`.
- q_004 How should empty, whitespace-only, or path-like `--by` values be normalized across all decision verbs?
  Rationale: Contract/review approval currently default blank values to `manual` but do not trim stored non-empty values; memory accept trims and sanitizes repo-root paths. Uniform `--by` behavior needs one rule.
  Decision: Trim whitespace before persistence; if the trimmed value is empty, record `manual`; sanitize any repo-root absolute path text the same way memory acceptance does; otherwise preserve the non-empty principal string.
- q_005 After deleting `gate run --allow-commands`, should the existing gate report field `validation.commands_allowed` remain in the JSON schema, and what value should it have when there are zero validation commands?
  Rationale: `gateReportDocument` currently includes `validation.commands_allowed`, driven by the removed flag. The contract specifies command behavior but not whether this report schema field is obsolete or retained.
  Decision: Keep `validation.commands_allowed` for now and set it to `true` on every successful `gate run`; with zero validation commands, emit `commands_allowed: true` and an empty `commands` array.
- q_006 Should historical docs such as backlog and dogfood records be rewritten to remove old `--yes` and `--allow-commands` examples, or only current guidance?
  Rationale: The draft says README, AGENTS, docs tree, bundled skill, and scripts must stop mentioning the flags as current guidance. Searches show historical files like `docs/backlog.md` and dogfood notes contain old command transcripts.
  Decision: Update all current user/agent guidance, helper text, generated hints, tests, scripts, README, AGENTS, docs/agents, docs/flow, docs/agent-skill, and bundled skill references. Leave clearly historical backlog/dogfood transcripts intact only if they are not presented as current instructions.
- q_007 [blocking] When the contract says `--by` should be uniform across all decision verbs, should that mean only the explicitly named principal-bearing commands (`contract approve`, `review approve`, `memory accept`, `contract accept`, `clarify answer`, `review proposal accept`, and `review proposal reject`), or should it also include other mutating decision-like commands such as `clarify add`, `contract revise`, `review finding add`, `review finding resolve`, `review prepare`, `memory propose`, or `memory refresh`?
  Rationale: The existing open `q_002` asks where to store the principal, but not which concrete commands count as `decision verbs`. The repo has many mutating commands that append ledger events or decision-like artifacts; extending `--by` to all of them would substantially widen the CLI and test scope beyond the commands named in the contract.
  Decision: Limit `--by` support to the explicitly named commands: keep/default it on `contract approve`, `review approve`, and `memory accept`, and add/default it on `contract accept`, `clarify answer`, `review proposal accept`, and `review proposal reject`. Do not add `--by` to `clarify add`, `contract revise`, `review finding add`, `review finding resolve`, `review prepare`, `memory propose`, `memory refresh`, or other mutating commands unless separately requested.

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 2
- fresh: 1
- stale: 1
- unknown: 0

Items:
- mem_002 [stale] score=75 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go
- mem_001 [fresh] score=32 — Add an export command that dumps a run's full record as a single archive

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Use `pactum search "<term>"` and inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing code items when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.
