# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260607_194548
- Approval: approved
- Contract hash: 7166424fef98e4ef37e5dd93ac4519b84c0529fb72430391d014b731190dec4b

## Goal
Slice 2 of token accounting (docs/cost-budget-design.md): capture token usage from the READ-stage agents (reviewer, clarify suggest, contract draft) too. Switch their invocation to json output and adapt the findings/questions/draft parsers to extract the agent's message text from the json envelope before parsing fenced JSON. The runner's slice-1 usage capture then covers read stages automatically. Best-effort: text extraction must never panic or break findings parsing / the review loop

## In scope
- Flip reviewerBuiltins (internal/agents/config.go) to structured json output: codex 'exec --json --sandbox read-only' and claude '-p --output-format json' (keep them read-only; these stages do not write)
- Add an agent-output text extractor, e.g. agentMessageText(stdout []byte) string, that returns the agent's message text by AUTO-DETECTING the format: claude --output-format json -> the single result object's 'result' string field; codex --json -> the concatenation of agent_message item texts from the JSONL item.completed events; fallback -> the raw stdout unchanged (so plain-text / non-json / unparseable output still works). Never panics
- In the three read-stage parsers (review propose-findings in review_proposals.go, clarify suggest in clarify_suggest.go, contract draft in contract_draft.go), pass the stdout bytes through agentMessageText before extractFencedJSONBlocks, so fenced-JSON findings/questions/draft are extracted from the agent text regardless of the json wrapping
- Verify read-stage usage is captured: with the json args, the existing runner path (structuredUsageEnabled/parseAgentUsage) and the shared lifecycle record a UsageRecord (captured=true) for the reviewer/clarify/draft stages; full-run totals in status / pactum usage now include read stages
- Tests: agentMessageText extraction (codex JSONL with agent_message item.completed -> text; claude result object -> .result; plain-text/non-json -> raw fallback; malformed/empty -> raw fallback, no panic); the findings/questions/draft parsers still extract correctly from json-wrapped reviewer output; a reviewer run configured with json args records captured reviewer usage

## Out of scope
- Cost in dollars, budget enforcement, estimation (later slices)
- Changing the fenced-JSON block format or the findings/clarification/draft schemas
- Decoupling the clarifier/drafter run+parse fusion (separate backlog item); changing how live agent output is rendered
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Paths in scope
- internal/agents/**
- internal/app/**
- docs/**


## Acceptance criteria
- reviewer / clarify suggest / contract draft run with json output, and their token usage is captured (captured=true) and recorded to runs/<id>/ledger/usage.jsonl, surfaced by status / pactum usage; full-run totals now include read stages
- Findings / clarification questions / contract draft fields are still correctly extracted from the json-wrapped agent output; review loop and clarify/draft behavior is otherwise unchanged
- Plain-text / non-json agent output still parses via the raw fallback (existing fake-agent tests pass); extraction never panics or fails a run on malformed output
- make check green (incl deadcode); go test -race ./... clean

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
