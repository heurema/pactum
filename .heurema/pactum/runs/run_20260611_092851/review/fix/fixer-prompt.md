# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260611_092851/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260611_092851/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260611_092851/review/review.json, .heurema/pactum/runs/run_20260611_092851/review/findings.jsonl, .heurema/pactum/runs/run_20260611_092851/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=9 open=9 resolved=0 blocking_open=2
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_007 severity=medium category=correctness blocking=true status=open: review proposal accept --by "" skips principal normalization and omits decided_by instead of recording manual.
    location: internal/app/review_proposals.go:233
  - f_008 severity=medium category=validation blocking=true status=open: Missing sanitization coverage for --by on newly extended decision verbs. contract accept and clarify answer persist principals through normalizePrincipal, but their tests only assert trimming/defaulting; only review proposal decisions exercise repo-root path sanitization.
    location: internal/app/contract_draft_test.go:233
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=low category=correctness blocking=false status=open: Review-loop auto-accepted proposals are persisted with source: "manual": ReviewAcceptProposal hardcodes Source on the decision record, and the loop's acceptReviewLoopProposal reuses it. The loop's duplicate path writes source: "review_loop", so automatic accepts are the one loop decision misattributed as manual. The change's own comments now describe Source as the sole provenance signal for automatic decisions, making the mislabel actively misleading; the only differentiator left is the implicit absence of decided_by. Pre-existing behavior (verified against HEAD), therefore advisory.
    location: internal/app/review_proposals.go:244
  - f_002 severity=low category=correctness blocking=false status=open: review proposal accept treats an explicitly empty --by value (`--by=""`) as 'no principal' instead of defaulting to 'manual': the `if decidedBy != ""` guard added so the review loop's auto-accept can pass "" also skips normalizePrincipal for empty CLI input, and the omitempty tag then drops decided_by from the decision record. All other principal-bearing verbs (clarify answer, review proposal reject, contract accept, contract approve, review approve, memory accept) call normalizePrincipal unconditionally, so explicit-empty records 'manual' there, per the contract's q_004 rule ('if the trimmed value is empty, record manual'). Only the exact-empty input on this one verb deviates; the omitted-flag path is correct via the CLI default:"manual" and whitespace-only input is correctly normalized.
    location: internal/app/review_proposals.go:233
  - f_003 severity=low category=validation blocking=false status=open: TestDocsHaveNoStaleCommandConcepts now forbids --yes and --allow-commands, but its requiredDocFiles scan set covers only README.md and five docs/ files. Acceptance criterion 6 also names docs/agent-skill.md, AGENTS.md, and the bundled skill under assets/agent-skills/pactum; those were cleaned in this change but are outside the test's scan set, so the removed flags could silently reappear there without failing any test. Pre-existing scope limitation of the docs test, advisory only.
    location: internal/docs/docs_test.go:14
  - f_004 severity=low category=quality blocking=false status=open: ReviewAcceptProposal uses an in-band empty-string sentinel on decidedBy to distinguish the review loop's auto-accept from the CLI path, so it only normalizes when the value is non-empty. This makes 'review proposal accept' the single --by verb where an explicit --by="" skips the default-to-manual rule (the record omits decided_by instead of recording 'manual'; all six other verbs normalize unconditionally). The resulting record is also indistinguishable from a loop auto-accept, since the loop path writes Source: "manual" (pre-existing) with decided_by omitted — contradicting the new comment at review_proposals.go:48 that automatic loop decisions 'record only Source as their provenance'. Consider the structure clarify.go already uses: the loop writes the decision via an internal helper and the CLI verb normalizes unconditionally.
    location: internal/app/review_proposals.go:233
  - f_005 severity=low category=quality blocking=false status=open: CHANGELOG.md's Unreleased section still states '`execute run` and `review run` require confirmation or `--yes`.' The changelog header says everything lives under Unreleased (no released version), so this presents the removed confirmation layer as current behavior. The change did not update the entry, and the docs stale-phrase test (internal/docs/docs_test.go) does not scan CHANGELOG.md, so the gate cannot catch it.
    location: CHANGELOG.md:66
  - f_006 severity=low category=quality blocking=false status=open: The new optional `--by` flag (default manual, persisted as decided_by/accepted_by) on contract accept, clarify answer, review proposal accept, and review proposal reject is not mentioned anywhere in the markdown docs, while the same docs show `--by manual` on contract approve, review approve, and memory accept. Since the goal was a uniform principal flag across all decision verbs, the docs now under-represent four of the seven verbs at a detail level (per-flag) these docs otherwise maintain. CLI help text covers all seven verbs, so the gap is markdown-only.
    location: docs/flow.md:186
  - f_009 severity=low category=quality blocking=false status=open: The main flow documentation does not document the newly supported optional --by attribution on clarify answer, contract accept, and review proposal accept/reject, while adjacent decision verbs document --by manual.
    location: docs/flow.md:105

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or any review loop command.

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

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
