# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260606_190824
- Approval: approved
- Contract hash: 350abc6ff5bf0982ed3e4f7e6be33f6216fb2b94f4f48d00606be837dec87843

## Goal
Stream the agent's live stdout/stderr to the operator's terminal during execute run, review run, and review fix (and per-round in review loop), in addition to capturing it to the per-attempt log files, so these commands are not a silent black box during multi-minute runs

## In scope
- Add an optional live-output writer to agents.RunRequest; in the subprocess runner, tee the agent's stdout and stderr to it via io.MultiWriter alongside the existing per-attempt log files (unset = unchanged behavior)
- Stream the live output to the operator's STDERR so stdout stays the clean result channel (human summary or --json) in all modes
- Wire it from execute run / review run / review fix (pass the command stderr), and from review loop for its per-round reviewer/fixer sub-runs, so the sub-command JSON the loop parses on stdout stays unpolluted
- Tests: live output appears on stderr, stdout result stays clean/parseable (including --json), attempt log files are still written, review loop sub-command JSON parsing unaffected; plus a docs/agents.md note

## Out of scope
- Streaming/structured JSON parsing of agent output (raw tee only); progress bars or TUI
- Changing what is captured to the attempt log files
- Native LLM API or model/provider abstraction
- Touching generated .heurema artifacts

## Acceptance criteria
- During execute/review/fix the agent output appears live on the operator's stderr AND is still written to the attempt log files
- stdout stays clean in all modes: the --json response remains parseable and review loop sub-command JSON parsing is unaffected
- When no live writer is set, behavior is unchanged; covered by tests

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
