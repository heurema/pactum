# Contract Draft

## Goal
Add a retry mechanism for flaky operations

## Current status
Contract status: draft
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260609_121416
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- q_001 [blocking] — Which Pactum operation should get the retry behavior?
  Rationale: The goal says "flaky operations", but the repo has several external operations: execute agents, review/fix agents, clarifier/drafter agents, gate validation commands, ACP adapter calls, and filesystem/project-map work. The repo's loop design specifically says execute is one-shot with an allowed single infrastructure-only retry, so the narrowest repo-backed scope is execute run.
  Answer: pending
- q_002 [blocking] — What failures should count as retryable?
  Rationale: The design docs distinguish infrastructure failure from an agent result that merely needs improvement. Retrying after partial edits or a bad result could violate Pactum's one-shot execution model and bypass the review loop.
  Answer: pending
- q_003 [blocking] — How many retries and what configuration/artifact behavior should the feature have?
  Rationale: The default config already contains `limits.execute.max_iterations: 10`, but the architecture docs say execute is one-shot with a single infrastructure-only retry. Reusing that existing limit would allow many agent reruns and would change the workflow more broadly.
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
- Which Pactum operation should get the retry behavior?
- What failures should count as retryable?
- How many retries and what configuration/artifact behavior should the feature have?
