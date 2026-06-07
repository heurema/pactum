# Contract Draft

## Goal
Stream the agent's live stdout/stderr to the operator's terminal during execute run, review run, and review fix (and per-round in review loop), in addition to capturing it to the per-attempt log files, so these commands are not a silent black box during multi-minute runs

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260606_190824
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
