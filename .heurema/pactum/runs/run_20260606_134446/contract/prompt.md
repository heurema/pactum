# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_134446
- Approval: approved
- Contract hash: 99c21b035de50874c78f0b776b3fe15746316f48d94bb7021a9e05f23f74a46e

## Goal
Add a 'pactum review fix' command: an executor agent (write-enabled, fresh context) that addresses the current run's review findings — fixing valid ones in place and explaining a rebuttal for false positives — capturing attempt artifacts. One invocation, human-driven (no loop yet)

## In scope
- Add a 'review fix <run_id>' command with --agent (the fixer), --yes, --timeout, and --json flags, mirroring 'execute run'
- Build a fix prompt from the approved contract (goal/scope/acceptance) and the run's review findings (review/findings.jsonl); instruct the fixer to trace each finding to the code, fix valid ones in place, and explain a rebuttal for false positives
- Resolve a write-enabled EXECUTOR agent for the fixer (reuse model[:effort] and the Resolved block), NOT the read-only reviewer
- Run the fixer subprocess and capture attempt artifacts (request/result/stdout/stderr) under the run's review fix directory; require --yes for non-interactive use
- Unit tests + a fix dry-run test, plus a docs/agents.md note

## Out of scope
- The loop driver / automatic re-review / convergence / stop conditions (a later slice)
- Feeding the fixer's rebuttal back to the reviewer (a later slice)
- Severity composition or a multi-reviewer panel
- Native LLM API or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- 'review fix' builds a fix prompt containing the contract goal and the current findings
- It runs a write-enabled executor agent (codex bypass / claude skip-permissions), not the read-only reviewer
- It captures attempt artifacts and requires --yes for non-interactive use
- The resolved fixer model/effort appears in the Resolved block; covered by tests

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
