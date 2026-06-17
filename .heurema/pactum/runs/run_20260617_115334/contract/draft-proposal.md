# Contract Draft Proposal

## Status
- Run id: run_20260617_115334
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-17T11:55:18Z

## In scope
- Implement or align the top-level `pactum usage [run_id]` CLI command with `--by stage|model|agent|provider`, `--all`, and `--json` flags.
- Read usage records from `runs/<run_id>/ledger/usage.jsonl`, defaulting to the current run when no run_id is provided, and aggregate `--all` from all `runs/*/ledger/usage.jsonl` files.
- Render human output as a deterministic table for the selected grouping with input, output, total tokens, captured N/M records, a TOTAL row, and a coverage line.
- Emit `pactum.usage_summary.v1alpha1` JSON with `scope`, `coverage`, `totals`, `groups`, and `warnings` fields matching the contract goal.
- Handle uncaptured records as lower-bound coverage: totals are captured-only, `coverage.complete` is false, `totals.lower_bound` is true, and uncaptured provider/stage combinations are listed.
- Handle missing usage ledgers and torn final JSONL lines according to the goal, while treating malformed earlier JSONL lines as errors.
- Add focused tests for run-level aggregation, `--all`, all `--by` dimensions, JSON output, lower-bound coverage, missing ledgers, malformed records, and partial final JSONL lines.

## Out of scope
- Changing the usage-recording path, agent execution lifecycle, token capture/parsing behavior, or `usage.jsonl` append format.
- Adding dollar costs, pricing, budget, quota, tier, role, economics, or effective-unit reporting.
- Changing unrelated commands such as status, execute, review, gate, contract, prompt, or memory.
- Persisting derived usage summaries or migrating historical ledger files.

## Acceptance criteria
- `pactum usage` resolves the current run read-only and prints a grouped usage table, TOTAL row, and coverage line without mutating the workspace.
- `pactum usage <run_id>` summarizes that run's ledger, and `pactum usage --all` aggregates all run usage ledgers.
- Passing both a positional run_id and `--all` fails with a clear usage error.
- `--by stage`, `--by model`, `--by agent`, and `--by provider` each produce groups keyed by the selected dimension, with deterministic output.
- Fully captured data prints no lower-bound warning and emits JSON with `coverage.complete: true` and `totals.lower_bound: false`.
- Any `captured:false` record makes the total a lower bound, reports captured N/M counts, and lists uncaptured provider/stage combinations in human and JSON output.
- `--json` emits only valid `pactum.usage_summary.v1alpha1` JSON with the schema described in the goal.
- A missing run usage ledger returns zero totals and a warning instead of crashing.
- A torn or partial final JSONL line is skipped with a warning, while a malformed non-final line returns an error.

## Validation commands
- go test ./internal/app -run Usage
- go test ./...
- go build ./...
- make check

## Assumptions
- The implementation should use existing artifact path and current-run resolution helpers where possible.
- Usage JSONL records may contain extra fields beyond the goal's required fields, and the command should tolerate them.
- Because the repository already contains a `pactum usage` surface, satisfying this contract may mean adapting that command to the requested behavior rather than adding a duplicate command.
- Legacy usage output schemas or flags not named in the goal, such as prior workspace/top reporting, are not required unless the human explicitly keeps them in scope.

