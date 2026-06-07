# Contract Draft

## Goal
Slice 2 of token accounting (docs/cost-budget-design.md): capture token usage from the READ-stage agents (reviewer, clarify suggest, contract draft) too. Switch their invocation to json output and adapt the findings/questions/draft parsers to extract the agent's message text from the json envelope before parsing fenced JSON. The runner's slice-1 usage capture then covers read stages automatically. Best-effort: text extraction must never panic or break findings parsing / the review loop

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260607_194547
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

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

## Open questions
- None
