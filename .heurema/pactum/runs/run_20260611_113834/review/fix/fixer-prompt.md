# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260611_113834/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260611_113834/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260611_113834/review/review.json, .heurema/pactum/runs/run_20260611_113834/review/findings.jsonl, .heurema/pactum/runs/run_20260611_113834/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=15 open=15 resolved=0 blocking_open=8
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_005 severity=medium category=validation blocking=true status=open: Acceptance criterion #11 ('pinned by a test that walks the emitted affordances') has a hole: TestEmittedAffordancesUseCurrentGrammar walks real emitted values only for the typed precondition errors and the contract_draft stage of nextCommandsForRun. The executed/gated/review_prepared/review_approved/memory_proposed branches (resolve.go:266-275, nextReviewCommands resolve.go:306-321) and the literal next values in ContractDraft, ReviewRun, ReviewFix, and the multi-run status branch are covered only by hand-typed duplicate strings in the test's collect(...) block, and no behavioral test asserts next for those surfaces. A typo or stale verb introduced in those production branches would pass go test ./... while emitting a broken affordance to agents. The duplicated literals match production today, so this is a pinning gap, not a live bug.
    location: internal/app/affordances_test.go:384
  - f_008 severity=medium category=correctness blocking=true status=open: nextCommandsForRun can advertise pactum review approve <run_id> for a review prepared from a failed gate report, but ReviewApprove rejects failed gates.
    location: internal/app/resolve.go:319
  - f_009 severity=medium category=correctness blocking=true status=open: Pending review proposals collected after review approval can leave next pointing at pactum memory propose <run_id>, which MemoryPropose rejects with pending_review_proposals.
    location: internal/app/resolve.go:272
  - f_010 severity=medium category=scope blocking=true status=open: prompt show --json before the prompt is built emits a not-ready guidance response without schema pactum.not_ready.v1.
    location: internal/app/prompt.go:183
  - f_011 severity=medium category=scope blocking=true status=open: map refresh --json omits the required top-level next array for an in-scope mutating workflow-state command.
    location: internal/app/commands.go:468
  - f_012 severity=medium category=scope blocking=true status=open: memory refresh --json omits the required top-level next array for an in-scope mutating workflow-state command.
    location: internal/app/memory_freshness.go:121
  - f_013 severity=medium category=quality blocking=true status=open: The affordance grammar test hardcodes response-specific next commands instead of extracting them from the actual JSON responses, so new emitters can regress without tests failing.
    location: internal/app/affordances_test.go:384
  - f_014 severity=medium category=correctness blocking=true status=open: nextCommandsForRun silently treats unreadable clarification records as zero open blocking questions, so JSON next can advertise pactum contract approve <run_id> even when the CLI cannot prove approval is legal.
    location: internal/app/resolve.go:286
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=medium category=correctness blocking=false status=open: Failed agent attempts in --json mode write two JSON documents to stdout: the run-only result document (agent_attempt.go:159, now carrying next: []) followed by the pactum.error.v1 envelope from app.go:112 (only gateProcessError is exempted). Plain json.Unmarshal of stdout fails. Same pattern in review run / review fix / contract draft / clarify suggest failure paths. Pre-existing at HEAD; advisory.
    location: internal/app/agent_attempt.go:159
  - f_002 severity=low category=quality blocking=false status=open: New docs claim read-only show commands return pactum.not_ready.v1, but pactum prompt show --json emits promptShowNotBuiltResponse with no schema field (internal/app/prompt.go:183-190). Agents branching on the documented schema will not match the prompt show not-built response. The doc sentence is introduced by this change.
    location: assets/agent-skills/pactum/references/workflow.md:89
  - f_003 severity=medium category=correctness blocking=false status=open: classifyErrorCode's fallback `strings.Contains(msg, "not found") -> run_not_found` misclassifies non-run lookup failures as the contract-pinned stable code. With --json, `clarify answer` with a bad question id (clarify.go:183), `review finding resolve` with a bad finding id (review.go:502), `contract accept` with no draft proposal (contract_draft.go:253), and `review fix apply` with a bad attempt id (review_fix_outcomes.go:159) all emit error.code=run_not_found while the run exists. Since this slice typed every genuine run-not-found path, the fallback now fires only for these misleading cases; agents told to trust error.code get the wrong recovery signal.
    location: internal/app/errors.go:159
  - f_004 severity=low category=correctness blocking=false status=open: Failed agent-stage commands in --json mode write two concatenated JSON documents to stdout: the run-only result JSON (agent_attempt.go:158-167 calls writeAgentAttemptRunOnly before returning processExitError), then the top-level pactum.error.v1 envelope (app.go:112, which exempts only gateProcessError). Affects execute run, review run, review fix run, contract draft on agent failure; whole-stdout JSON parsing fails, undermining the convention the updated skill/docs now instruct agents to rely on.
    location: internal/app/app.go:112
  - f_006 severity=medium category=quality blocking=false status=open: New conditional next-selection logic is untested on both arms: GateRun's failed-gate branch (gate.go:198-203), the WrapRunOnly exit-code/timeout conditionals in ExecuteRun (execute.go:134-140) and ReviewFix (review_fix.go:156-164), ReviewRun's runErr conditional (review.go:644-650), and the nextReviewCommands gating on pending proposals / open blocking findings (resolve.go:306-321). Existing JSON tests run these commands and decode the wrapped responses but never assert next, so an inverted condition would pass the suite.
    location: internal/app/resolve.go:306
  - f_007 severity=low category=quality blocking=false status=open: workflow.md claims all read-only `show` commands with a missing artifact return `pactum.not_ready.v1`, but `pactum prompt show --json` on an unbuilt prompt emits promptShowNotBuiltResponse (internal/app/prompt.go:86-95) which has no `schema` field at all. Agents dispatching on schema == "pactum.not_ready.v1" will misclassify this response. Same claim in docs/agent-skill.md:57-59. The shape is pre-existing; the over-general doc sentence is new in this change.
    location: assets/agent-skills/pactum/references/workflow.md:89
  - f_015 severity=low category=quality blocking=false status=open: The docs say `next: []` means the next move needs a human, but the implementation also emits `next: []` for terminal states such as after `pactum memory accept --json`, where there is no next move.
    location: assets/agent-skills/pactum/SKILL.md:50

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
