# Reviewer Context

## Run
- Run id: run_20260617_115334
- Run status: contract_approved

## Contract
- Goal: Add a new top-level CLI command 'pactum usage [run_id]' that summarizes agent token usage from a run's ledger/usage.jsonl — a plain 'where do tokens go and how much' view. No cost in dollars, no tier/role/economics classification.

It reads usage records (one JSON line per agent attempt) with fields: run_id, stage, provider, agent, agent_name, request_model, input_tokens, output_tokens, total_tokens, captured (bool). Default: summarize the current run (or the given run_id), grouped by stage. Flags: --by stage|model|agent|provider (default stage) selects the grouping dimension; --all aggregates across all runs (runs/*/ledger/usage.jsonl); --json emits schema pactum.usage_summary.v1alpha1.

Human output: a table of the chosen grouping (per group: input/output/total tokens and 'captured N/M' records), a TOTAL row, and a coverage line. When any record has captured=false, tag the total as a LOWER BOUND and list which provider/stages are uncaptured. A fully-captured run prints no lower-bound warning.

JSON (pactum.usage_summary.v1alpha1): {schema, scope{kind: run|all, run_id}, coverage{records, captured_records, uncaptured_records, complete, uncaptured:[{provider,stage,records}]}, totals{input_tokens, output_tokens, total_tokens, captured_only: true, lower_bound: bool}, groups:[{by, key, input_tokens, output_tokens, total_tokens, records, captured_records}], warnings:[]}.

Robustness: tolerate a torn/partial last JSONL line in a live run (skip it + emit a warning, count it as unparseable); a malformed earlier line is an error; a missing usage.jsonl for a run yields zero rows + a warning, not a crash.

Constraints: NEW command only; do NOT change the usage-recording path or any existing command; do NOT add dollar cost or pricing; NO tier/role/economics dimension (this is a plain usage view). Validation: go build ./..., go test ./..., make check, plus a focused go test for the new command covering aggregation, coverage/lower-bound, --by, --json, and a partial-file case.
- In scope:
  - Adapt the top-level `pactum usage [run_id]` CLI command (updating the existing command at this path) to the behavior described in this contract, including `--by stage|model|agent|provider`, `--all`, and `--json` flags.
  - Read usage records from `runs/<run_id>/ledger/usage.jsonl`, defaulting to the current run when no run_id is provided, and aggregate `--all` from all `runs/*/ledger/usage.jsonl` files.
  - Render human output as a deterministic table for the selected grouping with input, output, total tokens, captured N/M records, a TOTAL row, and a coverage line.
  - Emit `pactum.usage_summary.v1alpha1` JSON with `scope`, `coverage` (extended with an `unparseable_records` integer field counting torn/partial final JSONL lines that were skipped), `totals`, `groups`, and `warnings` fields matching the contract goal.
  - Handle uncaptured records as lower-bound coverage: totals are captured-only, `coverage.complete` is false, `totals.lower_bound` is true, and uncaptured provider/stage combinations are listed.
  - Handle missing usage ledgers and torn final JSONL lines according to the goal, while treating malformed earlier JSONL lines as errors.
  - Add focused tests for run-level aggregation, `--all`, all `--by` dimensions, JSON output, lower-bound coverage, missing ledgers, malformed records, partial final JSONL lines, the run_id+`--all` conflict error, and the absence of any workspace mutation.
- Out of scope:
  - Changing the usage-recording path, agent execution lifecycle, token capture/parsing behavior, or `usage.jsonl` append format.
  - Adding dollar costs, pricing, budget, quota, tier, role, economics, or effective-unit reporting.
  - Changing unrelated commands such as status, execute, review, gate, contract, prompt, or memory.
  - Persisting derived usage summaries or migrating historical ledger files.
- Acceptance criteria:
  - `pactum usage` resolves the current run read-only and prints a grouped usage table, TOTAL row, and coverage line without mutating the workspace; if no current run can be resolved, it exits with a non-zero status and a clear error message that suggests providing an explicit run_id.
  - `pactum usage <run_id>` summarizes that run's ledger, and `pactum usage --all` aggregates all run usage ledgers.
  - Passing both a positional run_id and `--all` fails with a clear usage error.
  - `--by stage`, `--by model`, `--by agent`, and `--by provider` each produce groups keyed by the selected dimension, with deterministic output. For `--by agent`, the group key is the `agent` field (internal identifier); human output displays `agent_name` as the label, falling back to `agent` when `agent_name` is absent or empty.
  - Fully captured data prints no lower-bound warning and emits JSON with `coverage.complete: true` and `totals.lower_bound: false`.
  - Any `captured:false` record makes the total a lower bound, reports captured N/M counts, and lists uncaptured provider/stage combinations in human and JSON output.
  - `--json` emits only valid `pactum.usage_summary.v1alpha1` JSON with the schema described in the goal. `scope.run_id` is the resolved run ID for `pactum usage [run_id]`, and `null` for `--all` (where `scope.kind` is `"all"`). Group token totals (`groups[*].input_tokens`, `groups[*].output_tokens`, `groups[*].total_tokens`) sum only `captured:true` records within that group, matching the same captured-only rule as `totals`; `groups[*].records` counts all parseable records in the group and `groups[*].captured_records` counts those with `captured:true`. `warnings` is an array of human-readable strings. The `coverage` object includes an `unparseable_records` integer field (count of final JSONL lines that failed parsing and were skipped).
  - The final line of a usage JSONL file is always treated as potentially torn: if it fails JSON parsing for any reason (truncated, malformed, or otherwise unparseable), it is skipped, a warning string is emitted, and it is counted in `coverage.unparseable_records`; it is excluded from `coverage.records` and does not influence `coverage.complete` or `totals.lower_bound`, which are determined solely by the `captured` field on parseable records. A non-final line that fails JSON parsing is an error.
  - A missing run usage ledger returns zero totals and a warning instead of crashing.
  - A malformed non-final JSONL line returns an error.
  - The command does not create, modify, or delete any file or directory during execution; a focused test verifies that no workspace path changes between before and after invoking `pactum usage`.
- Validation commands:
  - go test ./internal/app -run Usage
  - go test ./...
  - go build ./...
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app -run Usage (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./... (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: go build ./... (exit 0, timed out: false, result: gate/validation/command_003/result.json)
  - command_004: make check (exit 0, timed out: false, result: gate/validation/command_004/result.json)
- Change summary:
  - changed files:
    - docs/backlog.md
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
- If you are not certain an issue is real after verification, do not flag it.
