# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260609_121417
- Approval: approved
- Contract hash: 40aa15ec1dac944d1cbf343a48f97b5d122b02d541987e2851bfe8cdc4c7faee

## Goal
Fix a real bug found empirically: the codex clarifier (and any consumer of codex structured output) records zero suggestion blocks despite the agent emitting valid fenced JSON. Root cause: codexAgentMessageText concatenates the codex stream's successive 'agent_message' events with no separator, so a fenced ```json block that begins a later agent_message is glued onto the trailing text of the previous (progress-narration) agent_message. extractFencedJSONBlocks recognizes a fence only when the ```json marker starts a line (via isJSONFenceStart on the trimmed line), so the glued fence start is missed and no block is extracted. Fix by separating concatenated agent_message texts with a newline.

## In scope
- In codexAgentMessageText (internal/app/agent_output.go): insert a newline separator between successive agent_message items so each message's text/content begins on a fresh line, preserving recognition of a fenced block that starts an agent_message. The separator must not add a spurious leading newline before the first message.
- Add a regression test in internal/app/agent_output_test.go reproducing the glued-fence scenario: a codex JSONL stream containing a progress agent_message (plain text, no trailing newline) immediately followed by a final agent_message whose text starts with a fenced ```json block; assert that agentMessageText/codexAgentMessageText output lets extractFencedJSONBlocks (or parseClarifierSuggestionBlocks) recover exactly one JSON block.

## Out of scope
- Do not change extractFencedJSONBlocks, isJSONFenceStart, claudeResultText/the claude path, the codex item-type filtering (still only agent_message items), or any clarifier/review logic. Only the agent_message concatenation separator and the new test.
- Do not change files other than internal/app/agent_output.go and internal/app/agent_output_test.go.

## Paths in scope
- internal/app/agent_output.go
- internal/app/agent_output_test.go


## Acceptance criteria
- codexAgentMessageText joins multiple agent_message texts with a newline (no leading newline before the first); a fenced JSON block that starts a later agent_message is recoverable by extractFencedJSONBlocks.
- The new regression test fails without the fix and passes with it; all existing tests pass (any test asserting the old glued concatenation is updated to expect the newline-separated form).
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Codex's exec --json stream emits multiple agent_message events (progress narration plus a final structured answer); only the concatenation separator is wrong, not the item selection.
- A newline separator is harmless for all other consumers of agentMessageText (it only ensures fenced blocks and line-oriented parsing see clean line boundaries).

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
