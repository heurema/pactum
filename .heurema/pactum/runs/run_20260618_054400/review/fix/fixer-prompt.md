# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260618_054400/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260618_054400/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260618_054400/review/review.json, .heurema/pactum/runs/run_20260618_054400/review/findings.jsonl, .heurema/pactum/runs/run_20260618_054400/review/resolutions.jsonl

## Approved contract
- Goal: Plan-DAG slice 2: the contract drafter emits an optional plan.tasks[] DAG, and add `pactum plan show` to render the static DAG. This is slice 2 of the plan-DAG arc (see docs/contract-plan-dag-design.md, build plan item 2). DRAFTER EMISSION + STATIC RENDER ONLY: no execution change, no plan_review, no validation-freeze, no tasks-state, no topological loop (all later slices).

Context: slice 1 (shipped) already put an optional, hashed plan.tasks[] on the v1alpha1 contract and validates it structurally at contract load and on contract revise (duplicate ids, cycles, unresolved depends_on, expected_files outside paths_in_scope, empty acceptance/validation). The contract revise --from path already accepts a contract.plan object (parseContractReviseInput maps it into contractPartialUpdate.Plan, and contractReviseWithUpdate validates it). What is MISSING and is this slice: the DRAFTER cannot emit a plan (the drafter proposal structs contractDraftProposalBlock and contractDraftProposalDocument in internal/app/contract_draft.go have no plan/tasks field), the accept path does not carry a drafted plan into the contract, and there is no command to render the DAG.

In scope:
1. Drafter proposal schema: add an optional plan field (a plan object with a tasks array, reusing the existing contractPlan / planTask types) to contractDraftProposalBlock and contractDraftProposalDocument so a drafter can propose plan.tasks[] in its fenced JSON block, and so the proposal artifact (memory-candidate / draft proposal json) round-trips it.
2. Drafter prompt: extend renderContractDrafterPrompt (and the drafter context if needed) to document the optional plan and instruct the drafter to emit plan.tasks[] ONLY when the work has real intra-contract fan-in (a task with more than one dependency) or independently-validatable surfaces; target 3-10 leaf tasks; a leaf is one independently reviewable patch, not one edit; each task needs one falsifiable validation referencing its expected_files. Linear or simple work stays plan-less (no plan emitted). Do NOT auto-split in code -- the drafter decides; the granularity rule is guidance in the prompt, not enforced code (enforcement is a later plan_review slice). Document each task field: id (required, unique), title, depends_on (ids of other tasks), context (evidence selectors: each path with optional lines and/or symbol, plus why), expected_files (advisory), acceptance (non-empty), validation (non-empty).
3. Accept path: in ContractAcceptDraft, map a proposed plan into contractPartialUpdate.Plan so it lands in the contract and is validated by the existing slice-1 plan validation; if the drafted plan is structurally invalid, accept fails with the existing actionable plan validation issues (no silent drop).
4. New command `pactum plan show [run_id] [--json]`: render the contract's plan.tasks[] as a static DAG -- task ids, titles, depends_on edges, context selectors, expected_files, acceptance, validation -- in a readable text form, and the structured plan in --json. When the contract has no plan, say so clearly (not an error). Reuse the existing plan rendering helper where possible (the contract.md plan section renderer in run.go already renders tasks). Register the command in the CLI and the command table.

Tests must cover: a drafter proposal containing a valid plan is accepted and the plan lands in the contract (visible via contract show --json and plan show); a drafter proposal with a structurally invalid plan (e.g. a cycle or unresolved depends_on) makes accept fail with the corresponding plan validation issue, not silently dropped; plan show renders all task fields for a contract that has a plan; plan show on a plan-less contract prints a clear no-plan message and exits 0; plan show --json emits the structured plan (or an explicit empty/absent plan) for both cases. A proposal with no plan still accepts exactly as before (plan-less contract, unchanged hash behavior).

Out of scope: making execute consume the plan, the topological execution loop, tasks-state.json, context-pack resolution (resolving context selectors into file slices -- that is slice 4), plan_review / the granularity-enforcement lens (slice 3), validation freeze / non-vacuous / baseline-red enforcement, single-writer lease, --task. Do not change execution, gate, or review behavior.

Validation: go test ./internal/app -run 'Contract|Plan|Draft', go test ./..., go build ./..., make check.
- In scope:
  - Add optional plan support to contract draft proposal parsing, persistence, JSON output, markdown preview, and summaries so draft proposals can round-trip plan.tasks[] before acceptance.
  - Extend the contract drafter prompt/context to document optional plan emission rules and every supported task field without enforcing task granularity in code.
  - Update ContractAcceptDraft so accepted draft plans populate contractPartialUpdate.Plan, count as a real change even when plan is the only proposed field, and pass through existing plan validation.
  - Add a read-only pactum plan show [run_id] [--json] command that renders the contract's static plan DAG in text and JSON and is registered in CLI help/command parsing.
  - Add focused internal/app tests for drafted plan parsing, accepted plan persistence, invalid drafted plan rejection, plan show text/json/no-plan output, and plan-less proposal compatibility.
- Out of scope:
  - Do not make execute, gate, review, or prompt execution consume plan.tasks[].
  - Do not add topological execution, tasks-state.json, context-pack resolution, plan_review, validation freeze, single-writer leases, --task, or task-level execution state.
  - Do not enforce the drafter prompt's task granularity guidance in code; structural plan validation remains the existing slice-1 validation surface.
  - Do not change the contract goal or existing approved plan validation semantics beyond carrying drafted plans into that validation path.
- Acceptance criteria:
  - After processing a draft proposal containing a valid plan.tasks[], contract/draft-proposal.json contains the plan with all proposed tasks, draft proposal JSON output exposes the plan and all task field values, and contract/contract.json is unchanged from its pre-draft state.
  - Accepting a valid drafted plan writes the same plan.tasks[] into the contract, and contract show --json plus pactum plan show --json expose task id, title, depends_on, context, expected_files, acceptance, and validation fields.
  - A draft proposal whose only substantive field is plan is accepted as a contract change rather than rejected as having no contract fields to apply.
  - Accepting a drafted plan with a structural error such as a cycle or unresolved depends_on fails non-zero with the existing actionable plan validation issue code and does not silently drop the plan.
  - A proposal with no plan still accepts as before, produces a plan-less contract, and preserves existing plan-less hash behavior.
  - The drafter prompt documents that plan.tasks[] should be emitted only for real fan-in or independently-validatable surfaces, targets 3-10 leaf tasks, requires falsifiable validation per task, and lists all task fields; a test calls renderContractDrafterPrompt and asserts the returned prompt contains the fan-in/independently-validatable condition, the 3-10 leaf task target, the falsifiable validation requirement, and each of the task field names: id, title, depends_on, context, expected_files, acceptance, validation.
  - pactum plan show renders every task field in a readable static DAG form for contracts with a plan.
  - pactum plan show on a contract without a plan prints a clear no-plan message and exits 0.
  - pactum plan show --json emits a JSON object with a top-level "plan" key whose value is an object containing a "tasks" array when a plan is present, with each task's id, title, depends_on, context, expected_files, acceptance, and validation fields; when no plan exists, the output is {"plan": null} — a valid JSON object with an explicit null plan field; a test asserts both the with-plan and no-plan shapes match these exact top-level keys.
  - pactum plan show is registered in the CLI grammar and appears in CLI help output; a test asserts the command name is recognised by the command parser without error; a separate test invokes the CLI with a help flag or help subcommand and asserts the string "plan show" appears in the produced output. Running pactum plan show and pactum plan show --json leaves ledger state, execution records, gate records, and review records unchanged, asserted by test comparing state before and after each invocation.
  - The draft proposal markdown preview and any draft proposal summary output render the proposed plan.tasks[] when present and do not write to contract/contract.json before accept.
  - Tests assert each of the following conditions independently: (a) contract/draft-proposal.json contains the plan field with all proposed tasks after recording a draft with plan.tasks[]; (b) draft proposal JSON output returns the plan and all task field values before accept is called; (c) contract/contract.json does not contain a plan field, or is otherwise byte-for-byte unchanged from its pre-draft state, between recording a draft and calling accept.
- Validation commands:
  - go test ./internal/app -run 'Contract|Plan|Draft'
  - go test ./...
  - go build ./...
  - make check

## Current review findings
- Summary: findings=7 open=1 resolved=6 blocking_open=0
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - none
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_007 severity=low category=quality blocking=false status=open: The new user-visible `pactum plan show` command is not documented in the workflow command table, leaving the public command reference stale.
    location: docs/flow.md:18
- Resolved findings (already addressed — context only):
  - f_001 severity=medium category=correctness blocking=true status=resolved: ContractAcceptDraft returns planValidationError directly, so accepting an invalid drafted plan loses the structured plan validation issue codes.
    location: internal/app/contract_draft.go:270
    latest resolution: Added `errors` import to contract_draft.go and added a planValidationError type-check in ContractAcceptDraft (after line ~270). When contractReviseWithUpdate returns a planValidationError, the structured failure (with Issues carrying the error codes) is written to stdout via writeReviseFailure, and contractReviseFailureError{} is returned — matching the pattern used in ContractRevise.
  - f_002 severity=medium category=scope blocking=true status=resolved: Accepting an invalid drafted plan rejects the change but drops the existing plan validation issue details, so callers do not see codes like CYCLE_IN_DAG or MISSING_DEPENDENCY as required.
    location: internal/app/contract_draft.go:270
    latest resolution: Same fix as f_001: the planValidationError handler in ContractAcceptDraft now emits the full contractReviseFailure JSON (including issue Codes like CYCLE_IN_DAG / MISSING_DEPENDENCY) to stdout rather than returning the raw error, ensuring callers can read the structured issue codes.
  - f_003 severity=medium category=quality blocking=true status=resolved: The drafted-plan round-trip test bypasses the drafter output parsing and proposal-recording path by writing contractDraftProposalDocument directly, so the new plan field on contractDraftProposalBlock and recordContractDraftProposal are not tested for plan.tasks[] input.
    location: internal/app/plan_test.go:309
    latest resolution: Rewrote the proposal setup in TestDraftedPlanProposalRoundTrip to construct a fenced JSON block (as a drafter would emit), then parse it through parseContractDraftProposalBlocks and contractDraftProposalFromBlock, then persist via writeContractDraftProposalArtifacts. The test now asserts that parseContractDraftProposalBlocks picks up the plan field from the block and that contractDraftProposalFromBlock carries it into the proposal document. Added time import for time.Now().UTC().
  - f_004 severity=medium category=quality blocking=true status=resolved: The invalid drafted-plan accept tests only require any non-zero exit and do not assert the required actionable plan validation issue code.
    location: internal/app/plan_test.go:505
    latest resolution: Added JSON parsing of stdout after the non-zero exit check in both subtests of TestAcceptInvalidDraftedPlanFails. The cycle subtest asserts CYCLE_IN_DAG is present in failure.Issues; the unresolved_dep subtest asserts MISSING_DEPENDENCY. These assertions work because the f_001/f_002 fix now writes the structured JSON failure to stdout before returning.
  - f_005 severity=medium category=quality blocking=true status=resolved: The plan show --json tests do not verify that all task fields are emitted.
    location: internal/app/plan_test.go:431
    latest resolution: Extended the plan show --json section in TestAcceptDraftedPlanAppliesItToContract to assert all task fields: t1 title, context path and why, expected_files, acceptance, validation; t2 depends_on, acceptance, validation — matching the task data set up in the test's proposal.
  - f_006 severity=medium category=quality blocking=true status=resolved: The read-only test for plan show does not cover all records named by the contract.
    location: internal/app/plan_test.go:181
    latest resolution: Extended readState in TestPlanShowIsReadOnly to capture two additional records: gate/gate-report.json and review/review.json (via mustReadFileOrAbsent, returning '<absent>' if not present). Added gateBefore/gateAfter and reviewBefore/reviewAfter comparisons after the plan show invocations.

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

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
  "schema": "pactum.review_fix_outcomes.v1alpha1",
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
