# Reviewer Context

## Run
- Run id: run_20260608_135839
- Run status: contract_approved

## Contract
- Goal: Add a workspace-wide cross-run token usage aggregate. pactum usage --all scans every run ledger/usage.jsonl and reports total token usage with by-run, by-stage, by-agent, and by-model breakdowns plus the cache-read ratio. It reads the per-run ledgers as a derived view (never the source of truth) and is best-effort: a missing or corrupt ledger is skipped, never fatal, consistent with the M12.0 readUsageRecords degradation.
- In scope:
  - usageCmd (internal/app/cli.go + commands.go) gains an --all bool flag; pactum usage --all produces the workspace aggregate. The existing pactum usage <run_id> per-run behavior is unchanged. Passing both a run_id and --all is a clear usage error.
  - Aggregation (internal/app/usage.go): enumerate the run directories (reuse the existing runs listing), read each runs/<id>/ledger/usage.jsonl via the existing best-effort readUsageRecords, and sum the normalized counts (input_tokens, output_tokens, total_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens). Produce breakdowns by run, by stage, by agent, and by model, plus the overall cache-read ratio (cache_read / input) guarded against divide-by-zero.
  - Output: a human-readable summary (workspace totals + the breakdown groupings) and a machine-readable --json form. A run with no/empty/corrupt usage.jsonl contributes nothing and is skipped without error; an empty workspace (no runs / no usage) reports a clean zero result.
  - Tests: the aggregate sums multiple runs ledgers correctly; the by-stage/by-agent/by-model breakdowns are correct; a corrupt or missing per-run ledger is skipped (best-effort, no error); the cache-read ratio divide-by-zero is guarded; an empty workspace reports zero. Reuse the existing usage test helpers/fixtures.
- Out of scope:
  - Cost in dollars, price tables, and estimation/forecasting (separate cost-budget slices).
  - Trend-over-time series or charts — only the static cross-run aggregate.
  - Changing the per-run pactum usage <run_id> output or behavior, or how usage is captured/recorded; native LLM API.
- Acceptance criteria:
  - pactum usage --all reports workspace total tokens plus by-run / by-stage / by-agent / by-model breakdowns and the cache-read ratio, in both human and --json output.
  - Best-effort: a corrupt or missing per-run ledger is skipped and the command still succeeds; the cache ratio never divides by zero; an empty workspace reports zero cleanly.
  - pactum usage <run_id> behavior is unchanged; passing both a run_id and --all errors clearly.
  - make check (incl. deadcode + git diff --check) and make test-race pass.
- Validation commands:
  - make build
  - make check
  - make test-race

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 0
- Fresh: 0
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: make build (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: make test-race (exit 0, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - docs/backlog.md
    - docs/cost-budget-design.md
    - internal/app/cli.go
    - internal/app/commands.go
    - internal/app/usage.go
    - internal/app/usage_test.go
  - new files:
    - none
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If uncertain, propose a blocking finding that asks for clarification.
