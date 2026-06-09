# Contract Draft

## Goal
Make the agent faster

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_182340
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — When the goal says "the agent," which concrete thing should be made faster? Candidates in this repo: (a) the Pactum CLI orchestrator itself (Go code in cmd/pactum + internal/app: map scan, search index, prompt build, gate); (b) a specific agent role Pactum runs — executor / reviewer / clarifier / drafter / fixer (internal/agents.AgentDescriptor); (c) the external agent CLIs codex/claude that Pactum invokes (docs/agents.md states Pactum does NOT install, configure, or wrap these, so their speed is largely outside Pactum's control); or (d) the end-to-end run pipeline as the operator experiences it.
  Rationale: Pactum has no single 'the agent.' docs/agents.md and docs/flow.md show an orchestrator that runs multiple agent ROLES against external CLIs. Each candidate points at a completely different change surface, and option (c) is mostly out of Pactum's control by design. The contract's empty scope means nothing in the repo disambiguates this — it is the human's intent to settle. Every later answer depends on it.
  Answer: pending
- q_002 [blocking] — What does "faster" measure? Candidates: (a) wall-clock duration of a run/stage (RunResult.DurationMillis); (b) token usage / cost as a latency-and-spend proxy (TokenUsage, docs/cost-budget-design.md); (c) fewer agent round-trips / subprocess invocations (e.g. fewer review-loop rounds); or (d) the Pactum Go process's own overhead independent of any agent call.
  Rationale: 'Faster' is overloaded across at least four measurable dimensions that the repo already instruments separately (DurationMillis vs TokenUsage vs loop rounds). Optimizing one can be neutral or harmful to another (e.g. more caching tokens to cut latency). Which dimension is primary determines what code to touch and how to verify, and the empty acceptance section gives no signal.
  Answer: pending
- q_003 [blocking] — How will 'faster' be verified as done? The contract has no acceptance_criteria and no validation commands, so 'faster' is currently unverifiable. What baseline, target, and measurement method define completion — e.g. a named representative scenario, a captured baseline, a concrete target (absolute or percentage reduction), and a repeatable command that measures it?
  Rationale: docs/flow.md shows acceptance criteria and validation commands are the gate for completion, and both are empty here. Without a baseline + target + measurement, the executor cannot know when it is done and the gate cannot check it. This is a decision the repo cannot make: it needs a chosen scenario and target number.
  Answer: pending
- q_004 [blocking] — Is changing observable behavior or output acceptable as a trade-off for speed, or must speed improve while preserving existing behavior and correctness? For example: may a stage be dropped, review rounds reduced (limits.review.*), or context/prompt tokens trimmed if that changes what the agent produces — or must all existing tests and outputs stay unchanged?
  Rationale: Several plausible speedups (fewer review-loop rounds, smaller prompt context, skipping a stage) trade correctness/coverage for speed. With empty scope, an executor could legitimately regress behavior to hit a speed target. Whether such trade-offs are allowed is a product call the repo cannot infer.
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
- When the goal says "the agent," which concrete thing should be made faster? Candidates in this repo: (a) the Pactum CLI orchestrator itself (Go code in cmd/pactum + internal/app: map scan, search index, prompt build, gate); (b) a specific agent role Pactum runs — executor / reviewer / clarifier / drafter / fixer (internal/agents.AgentDescriptor); (c) the external agent CLIs codex/claude that Pactum invokes (docs/agents.md states Pactum does NOT install, configure, or wrap these, so their speed is largely outside Pactum's control); or (d) the end-to-end run pipeline as the operator experiences it.
- What does "faster" measure? Candidates: (a) wall-clock duration of a run/stage (RunResult.DurationMillis); (b) token usage / cost as a latency-and-spend proxy (TokenUsage, docs/cost-budget-design.md); (c) fewer agent round-trips / subprocess invocations (e.g. fewer review-loop rounds); or (d) the Pactum Go process's own overhead independent of any agent call.
- How will 'faster' be verified as done? The contract has no acceptance_criteria and no validation commands, so 'faster' is currently unverifiable. What baseline, target, and measurement method define completion — e.g. a named representative scenario, a captured baseline, a concrete target (absolute or percentage reduction), and a repeatable command that measures it?
- Is changing observable behavior or output acceptable as a trade-off for speed, or must speed improve while preserving existing behavior and correctness? For example: may a stage be dropped, review rounds reduced (limits.review.*), or context/prompt tokens trimmed if that changes what the agent produces — or must all existing tests and outputs stay unchanged?
