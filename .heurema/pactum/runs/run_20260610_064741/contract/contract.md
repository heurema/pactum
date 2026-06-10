# Contract Draft

## Goal
Sketch a plan for improving error messages

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260610_060539
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — Which 'error messages' should the plan target? Candidates in this repo: (a) user-facing CLI command errors — the human stderr line and the pactum.error.v1 JSON envelope produced in internal/app/errors.go; (b) internal Go error wrapping — ~96 fmt.Errorf sites in internal/app and 16 in internal/agents; (c) agent-subprocess failure reporting — executor/transport errors, idle-timeout messages, and doctor diagnostics in internal/agents; or (d) all of the above.
  Rationale: The goal names no error surface, and the repo has at least three distinct ones with different owners and improvement levers. The choice determines which files the plan inventories and what 'improvement' even means (user remediation hints vs. wrap-chain hygiene vs. subprocess failure presentation).
  Answer: pending
- q_002 [blocking] — What artifact does 'sketch a plan' deliver: a new design document following the existing docs/*-design.md convention (like docs/cost-budget-design.md and docs/loop-architecture-design.md), an entry in docs/backlog.md, or a plan plus some initial implementation?
  Rationale: The contract has empty scope and the deliverable form changes everything downstream: which files the executor may write, what acceptance looks like, and whether validation commands are meaningful. The repo's *-design.md convention is the closest existing pattern for a 'plan'.
  Answer: pending
- q_003 [blocking] — Are any code changes in scope for this run — for example the quick win of replacing the substring-matching classifyErrorCode in internal/app/errors.go with typed sentinel errors — or is this run strictly the planning document?
  Rationale: Without an explicit answer the executor could either write code the human did not want or skip a quick win the human expected. The goal verb 'sketch a plan' suggests docs-only, but the contract's empty scope.out does not forbid code.
  Answer: pending
- q_004 [blocking] — What must the plan contain for the run to be accepted? Candidate required sections: an inventory of current error surfaces with file references; message style principles (e.g. the cause-plus-remediation pattern of errNotInitialized: 'pactum is not initialized; run: pactum init'); a stable error-code taxonomy for the pactum.error.v1 JSON envelope; and a prioritized list of improvement work items with affected files.
  Rationale: acceptance_criteria is empty, so there is currently no way to verify completion. A 'plan' can range from three bullet points to a full design; the human must pick the bar.
  Answer: pending
- q_005 — What should validation.commands contain for a docs-only deliverable: leave it empty (human review only), a file-existence check such as 'test -f docs/error-messages-design.md', and/or 'go test ./...' as a no-regression guard confirming no code changed?
  Rationale: validation.commands is empty and the gate step needs something runnable, but most existing validation conventions in this repo assume code changes. For a document, mechanical validation can only check existence/format, not quality.
  Answer: pending
- q_006 — When a spawned executor subprocess (claude/codex) fails with its own error text — for example a pactum execute run that hits the idle timeout, or an agent binary missing from PATH — is improving how Pactum wraps and presents that failure part of this plan, or only errors Pactum itself authors?
  Rationale: Agent-originated failures are a major real-world error path in this repo (internal/agents runner/transport, doctor), but the text originates outside Pactum. The contract is silent on whether presentation of third-party failure output counts as 'error messages'.
  Answer: pending
- q_007 — classifyErrorCode in internal/app/errors.go derives the machine-readable JSON code by substring-matching the message (e.g. 'contract is not approved', 'not found'), so rewording a message can silently change the code a --json consumer sees. Should the plan treat stable machine-readable codes as a hard constraint — i.e. include migrating to typed errors so codes are independent of wording — before any message rewording happens?
  Rationale: This is the concrete failure mode of the whole project: the very act of improving message wording can break the pactum.error.v1 contract for scripted consumers. Whether code stability is a prerequisite changes the plan's ordering and priorities.
  Answer: pending
- q_008 — Should the plan be structured as contract-sized work items, each executable as a future Pactum dogfood run, or as a free-form design narrative?
  Rationale: This repo plans work via docs/backlog.md milestones executed as Pactum runs. If the plan's items are meant to feed that pipeline, each must be independently scopeable with its own acceptance criteria — a different document shape than a prose sketch.
  Answer: pending

## In scope
TBD

## Out of scope
TBD

## Acceptance criteria
TBD

## Validation commands
TBD

## Assumptions
TBD

## Open questions
- Which 'error messages' should the plan target? Candidates in this repo: (a) user-facing CLI command errors — the human stderr line and the pactum.error.v1 JSON envelope produced in internal/app/errors.go; (b) internal Go error wrapping — ~96 fmt.Errorf sites in internal/app and 16 in internal/agents; (c) agent-subprocess failure reporting — executor/transport errors, idle-timeout messages, and doctor diagnostics in internal/agents; or (d) all of the above.
- What artifact does 'sketch a plan' deliver: a new design document following the existing docs/*-design.md convention (like docs/cost-budget-design.md and docs/loop-architecture-design.md), an entry in docs/backlog.md, or a plan plus some initial implementation?
- Are any code changes in scope for this run — for example the quick win of replacing the substring-matching classifyErrorCode in internal/app/errors.go with typed sentinel errors — or is this run strictly the planning document?
- What must the plan contain for the run to be accepted? Candidate required sections: an inventory of current error surfaces with file references; message style principles (e.g. the cause-plus-remediation pattern of errNotInitialized: 'pactum is not initialized; run: pactum init'); a stable error-code taxonomy for the pactum.error.v1 JSON envelope; and a prioritized list of improvement work items with affected files.
- What should validation.commands contain for a docs-only deliverable: leave it empty (human review only), a file-existence check such as 'test -f docs/error-messages-design.md', and/or 'go test ./...' as a no-regression guard confirming no code changed?
- When a spawned executor subprocess (claude/codex) fails with its own error text — for example a pactum execute run that hits the idle timeout, or an agent binary missing from PATH — is improving how Pactum wraps and presents that failure part of this plan, or only errors Pactum itself authors?
- classifyErrorCode in internal/app/errors.go derives the machine-readable JSON code by substring-matching the message (e.g. 'contract is not approved', 'not found'), so rewording a message can silently change the code a --json consumer sees. Should the plan treat stable machine-readable codes as a hard constraint — i.e. include migrating to typed errors so codes are independent of wording — before any message rewording happens?
- Should the plan be structured as contract-sized work items, each executable as a future Pactum dogfood run, or as a free-form design narrative?
