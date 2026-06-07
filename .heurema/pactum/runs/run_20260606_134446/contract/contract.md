# Contract Draft

## Goal
Add a 'pactum review fix' command: an executor agent (write-enabled, fresh context) that addresses the current run's review findings — fixing valid ones in place and explaining a rebuttal for false positives — capturing attempt artifacts. One invocation, human-driven (no loop yet)

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260606_134446
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
