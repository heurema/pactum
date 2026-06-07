# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_203005
- Approval: approved
- Contract hash: 886fc6083a575ca9367eaae2895b24b58199b48575a0a5e1910b95ddfc568c28

## Goal
Add a 'pactum clarify suggest' command: a read-only agent proposes clarification questions for the run from the contract goal and repo context, and pactum records them as open clarification questions for the human to answer. The agent does not answer questions or edit code — clarifications are human-answered

## In scope
- Add a 'clarify suggest <run_id>' command with --reviewer (the clarifier agent), --yes, --timeout, and --json flags
- Build a clarifier prompt from the contract goal + repo context (project map / search excerpt) + existing clarifications; instruct the agent to propose clarification questions (each: text, blocking flag, brief rationale) and NOT to answer them or edit code
- Run a READ-ONLY agent (reviewer role / read-only sandbox), capture attempt artifacts; parse its structured output into clarification questions and add them as OPEN questions answerable via the existing 'clarify answer'
- Require --yes (it runs an agent); reuse cross-model selection; tests with a fake agent; docs/agents.md note

## Out of scope
- Agent contract drafting / refining contract fields from answers (a later slice)
- The full clarify loop driver (suggest -> answer -> refine -> repeat)
- The agent answering clarification questions (they are human-answered by design)
- Native LLM API or model/provider abstraction; touching generated .heurema artifacts

## Acceptance criteria
- clarify suggest runs a read-only agent and adds its proposed questions as OPEN clarifications without answering them or editing code
- It captures attempt artifacts and requires --yes for non-interactive use
- Covered by tests (fake agent output parsed into open questions)

## Validation commands
- make check

## Assumptions
TBD

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 0
- fresh: 0
- stale: 0
- unknown: 0

Items:
- none

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
