# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260608_135839
- Approval: approved
- Contract hash: 947937f64362c95d9f15cc5f599c7952ca450dc04c6cac5b44375fab237b2eff

## Goal
Add a workspace-wide cross-run token usage aggregate. pactum usage --all scans every run ledger/usage.jsonl and reports total token usage with by-run, by-stage, by-agent, and by-model breakdowns plus the cache-read ratio. It reads the per-run ledgers as a derived view (never the source of truth) and is best-effort: a missing or corrupt ledger is skipped, never fatal, consistent with the M12.0 readUsageRecords degradation.

## In scope
- usageCmd (internal/app/cli.go + commands.go) gains an --all bool flag; pactum usage --all produces the workspace aggregate. The existing pactum usage <run_id> per-run behavior is unchanged. Passing both a run_id and --all is a clear usage error.
- Aggregation (internal/app/usage.go): enumerate the run directories (reuse the existing runs listing), read each runs/<id>/ledger/usage.jsonl via the existing best-effort readUsageRecords, and sum the normalized counts (input_tokens, output_tokens, total_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens). Produce breakdowns by run, by stage, by agent, and by model, plus the overall cache-read ratio (cache_read / input) guarded against divide-by-zero.
- Output: a human-readable summary (workspace totals + the breakdown groupings) and a machine-readable --json form. A run with no/empty/corrupt usage.jsonl contributes nothing and is skipped without error; an empty workspace (no runs / no usage) reports a clean zero result.
- Tests: the aggregate sums multiple runs ledgers correctly; the by-stage/by-agent/by-model breakdowns are correct; a corrupt or missing per-run ledger is skipped (best-effort, no error); the cache-read ratio divide-by-zero is guarded; an empty workspace reports zero. Reuse the existing usage test helpers/fixtures.

## Out of scope
- Cost in dollars, price tables, and estimation/forecasting (separate cost-budget slices).
- Trend-over-time series or charts — only the static cross-run aggregate.
- Changing the per-run pactum usage <run_id> output or behavior, or how usage is captured/recorded; native LLM API.

## Paths in scope
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- pactum usage --all reports workspace total tokens plus by-run / by-stage / by-agent / by-model breakdowns and the cache-read ratio, in both human and --json output.
- Best-effort: a corrupt or missing per-run ledger is skipped and the command still succeeds; the cache ratio never divides by zero; an empty workspace reports zero cleanly.
- pactum usage <run_id> behavior is unchanged; passing both a run_id and --all errors clearly.
- make check (incl. deadcode + git diff --check) and make test-race pass.

## Validation commands
- make build
- make check
- make test-race

## Assumptions
- Reuse the M12.0/M12.1 building blocks: readUsageRecords (best-effort, swallows errors), the UsageRecord schema, and the per-run summarizeUsage logic.
- The aggregate is a derived view recomputed on demand from the per-run usage.jsonl ledgers; it is never an authoritative store.
- No backward-compatibility constraints; additive flag and output.

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
