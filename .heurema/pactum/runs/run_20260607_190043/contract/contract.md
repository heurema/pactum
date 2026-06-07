# Contract Draft

## Goal
Slice 1 of token accounting (see docs/cost-budget-design.md): capture per-call token usage from the write-stage agents (executor and fixer), normalize it across providers, record it to the run's usage ledger, and surface tokens-per-task. Tokens are the unit; cost/budget/estimation are out of scope. Capture is best-effort and MUST NEVER fail a run

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260607_190042
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (10 result(s))

## Clarifications
- None

## In scope
- Add a normalized token-usage type. RunResult (internal/agents/types.go) gains a Usage field carrying normalized counts: input_tokens, output_tokens, total_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens, plus a captured bool and the raw provider usage blob. Follow the schema and the OTel-inclusive normalization in docs/cost-budget-design.md
- Per-agent usage parser in the runner (internal/agents/runner.go), selected per agent. codex: parse the JSONL event stream, take the LAST turn.completed.usage (it is cumulative per session) -> {input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens}; OutputTokens = output_tokens + reasoning_output_tokens, InputTokens = input_tokens (already cache-inclusive), CacheReadTokens = cached_input_tokens. claude: parse the single --output-format json result object usage{input_tokens,output_tokens,cache_creation_input_tokens,cache_read_input_tokens}; InputTokens = input_tokens + cache_read + cache_creation (Anthropic excludes cache). Best-effort: on any parse failure set captured=false and continue (NEVER error the run)
- Make the write-stage agents emit structured usage: in ListBuiltins (the executor/fixer descriptors), add 'codex exec --json ...' and 'claude -p --output-format json ...' while KEEPING the existing write-bypass flags (--dangerously-bypass-approvals-and-sandbox / --dangerously-skip-permissions). Do NOT change reviewerBuiltins (reviewer/clarify/draft stay as-is; their usage is captured in a later slice)
- Record usage once in the shared runAgentAttemptLifecycle (internal/app/agent_attempt.go): assemble a UsageRecord (run_id, attempt_id, stage, provider, agent, model, the normalized counts, captured, raw, schema_version, created_at) and append it to a per-run usage ledger (runs/<id>/ledger/usage.jsonl). All five commands flow through this lifecycle; write stages will have captured usage, read stages captured=false
- Surface: replace the hardcoded zero usage in status (internal/app/status.go usageStatus + the status output) with the live per-run total summed from usage.jsonl; add a 'pactum usage [run_id]' command printing the per-run token total and a breakdown by stage and agent, plus the cache-read ratio. JSON output supported
- Tests: parser unit tests with real example payloads (codex JSONL with multiple turn.completed events -> takes the last/cumulative; claude result object; a malformed/empty output -> captured=false, no error); the lifecycle appends a usage record; status and 'pactum usage' sum a usage.jsonl correctly; a write-stage run records non-zero captured usage

## Out of scope
- Read-stage usage capture (reviewer / clarify suggest / contract draft) — they keep their current invocation and parser; their usage is captured in a later slice
- Cost in dollars / price tables, budget enforcement / max_tokens / budget_exceeded, and estimation/forecasting — all later slices
- Changing how live agent output is rendered (write-stage stdout becomes JSONL/json; that is acceptable for this slice)
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Paths in scope
- internal/agents/**
- internal/app/**
- docs/**


## Acceptance criteria
- A codex or claude execute/fix run records a UsageRecord with non-zero normalized token counts (input/output/total, captured=true) to runs/<id>/ledger/usage.jsonl; the per-provider cache normalization matches docs/cost-budget-design.md
- status shows the live per-run token total (not a hardcoded 0); 'pactum usage' shows the per-run total + by-stage/by-agent breakdown
- A parse miss (unexpected/empty agent output) records captured=false with a warning and the run still SUCCEEDS; read-stage runs do not error
- Existing execute/review/fix behavior is otherwise unchanged (write-bypass preserved; files still written); make check green incl deadcode; go test -race ./... clean

## Validation commands
- make check

## Assumptions
TBD

## Open questions
- None
