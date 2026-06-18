# Contract Draft Proposal

## Status
- Run id: run_20260618_054400
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-18T05:46:51Z

## In scope
- Add optional plan support to contract draft proposal parsing, persistence, JSON output, markdown preview, and summaries so draft proposals can round-trip plan.tasks[] before acceptance.
- Extend the contract drafter prompt/context to document optional plan emission rules and every supported task field without enforcing task granularity in code.
- Update ContractAcceptDraft so accepted draft plans populate contractPartialUpdate.Plan, count as a real change even when plan is the only proposed field, and pass through existing plan validation.
- Add a read-only pactum plan show [run_id] [--json] command that renders the contract's static plan DAG in text and JSON and is registered in CLI help/command parsing.
- Add focused internal/app tests for drafted plan parsing, accepted plan persistence, invalid drafted plan rejection, plan show text/json/no-plan output, and plan-less proposal compatibility.

## Out of scope
- Do not make execute, gate, review, or prompt execution consume plan.tasks[].
- Do not add topological execution, tasks-state.json, context-pack resolution, plan_review, validation freeze, single-writer leases, --task, or task-level execution state.
- Do not enforce the drafter prompt's task granularity guidance in code; structural plan validation remains the existing slice-1 validation surface.
- Do not change the contract goal or existing approved plan validation semantics beyond carrying drafted plans into that validation path.

## Acceptance criteria
- A drafter fenced JSON proposal containing a valid plan.tasks[] is recorded in contract/draft-proposal.json and visible through draft proposal JSON without mutating contract/contract.json before accept.
- Accepting a valid drafted plan writes the same plan.tasks[] into the contract, and contract show --json plus pactum plan show --json expose task id, title, depends_on, context, expected_files, acceptance, and validation fields.
- A draft proposal whose only substantive field is plan is accepted as a contract change rather than rejected as having no contract fields to apply.
- Accepting a drafted plan with a structural error such as a cycle or unresolved depends_on fails non-zero with the existing actionable plan validation issue code and does not silently drop the plan.
- A proposal with no plan still accepts as before, produces a plan-less contract, and preserves existing plan-less hash behavior.
- The drafter prompt documents that plan.tasks[] should be emitted only for real fan-in or independently-validatable surfaces, targets 3-10 leaf tasks, requires falsifiable validation per task, and lists all task fields.
- pactum plan show renders every task field in a readable static DAG form for contracts with a plan.
- pactum plan show on a contract without a plan prints a clear no-plan message and exits 0.
- pactum plan show --json emits structured plan data when present and an explicit empty or absent plan representation when no plan exists.
- pactum plan show is available through the CLI grammar/help and does not alter execution, gate, review, or ledger state.

## Validation commands
- go test ./internal/app -run 'Contract|Plan|Draft'
- go test ./...
- go build ./...
- make check

## Assumptions
- The existing contractPlan, planTask, planContextSelector, normalizeDraftContractPlan, validateContractPlan, and contract plan renderer are the intended source of truth for plan shape and structural validation.
- plan.tasks[] remains optional in this slice; nil or empty plans should continue to normalize to no plan.
- pactum plan show is intended as an inspection command and should be read-only.

