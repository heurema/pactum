# Contract Review: Testability

You are reviewing a software change contract through the **acceptance-testability** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Add a new top-level CLI command 'pactum usage [run_id]' that summarizes agent token usage from a run's ledger/usage.jsonl — a plain 'where do tokens go and how much' view. No cost in dollars, no tier/role/economics classification.

It reads usage records (one JSON line per agent attempt) with fields: run_id, stage, provider, agent, agent_name, request_model, input_tokens, output_tokens, total_tokens, captured (bool). Default: summarize the current run (or the given run_id), grouped by stage. Flags: --by stage|model|agent|provider (default stage) selects the grouping dimension; --all aggregates across all runs (runs/*/ledger/usage.jsonl); --json emits schema pactum.usage_summary.v1alpha1.

Human output: a table of the chosen grouping (per group: input/output/total tokens and 'captured N/M' records), a TOTAL row, and a coverage line. When any record has captured=false, tag the total as a LOWER BOUND and list which provider/stages are uncaptured. A fully-captured run prints no lower-bound warning.

JSON (pactum.usage_summary.v1alpha1): {schema, scope{kind: run|all, run_id}, coverage{records, captured_records, uncaptured_records, complete, uncaptured:[{provider,stage,records}]}, totals{input_tokens, output_tokens, total_tokens, captured_only: true, lower_bound: bool}, groups:[{by, key, input_tokens, output_tokens, total_tokens, records, captured_records}], warnings:[]}.

Robustness: tolerate a torn/partial last JSONL line in a live run (skip it + emit a warning, count it as unparseable); a malformed earlier line is an error; a missing usage.jsonl for a run yields zero rows + a warning, not a crash.

Constraints: NEW command only; do NOT change the usage-recording path or any existing command; do NOT add dollar cost or pricing; NO tier/role/economics dimension (this is a plain usage view). Validation: go build ./..., go test ./..., make check, plus a focused go test for the new command covering aggregation, coverage/lower-bound, --by, --json, and a partial-file case.

**Scope in**:
  - Adapt the top-level `pactum usage [run_id]` CLI command (updating the existing command at this path) to the behavior described in this contract, including `--by stage|model|agent|provider`, `--all`, and `--json` flags.
  - Read usage records from `runs/<run_id>/ledger/usage.jsonl`, defaulting to the current run when no run_id is provided, and aggregate `--all` from all `runs/*/ledger/usage.jsonl` files.
  - Render human output as a deterministic table for the selected grouping with input, output, total tokens, captured N/M records, a TOTAL row, and a coverage line.
  - Emit `pactum.usage_summary.v1alpha1` JSON with `scope`, `coverage` (extended with an `unparseable_records` integer field counting torn/partial final JSONL lines that were skipped), `totals`, `groups`, and `warnings` fields matching the contract goal.
  - Handle uncaptured records as lower-bound coverage: totals are captured-only, `coverage.complete` is false, `totals.lower_bound` is true, and uncaptured provider/stage combinations are listed.
  - Handle missing usage ledgers and torn final JSONL lines according to the goal, while treating malformed earlier JSONL lines as errors.
  - Add focused tests for run-level aggregation, `--all`, all `--by` dimensions, JSON output, lower-bound coverage, missing ledgers, malformed records, partial final JSONL lines, the run_id+`--all` conflict error, and the absence of any workspace mutation.

**Scope out**:
  - Changing the usage-recording path, agent execution lifecycle, token capture/parsing behavior, or `usage.jsonl` append format.
  - Adding dollar costs, pricing, budget, quota, tier, role, economics, or effective-unit reporting.
  - Changing unrelated commands such as status, execute, review, gate, contract, prompt, or memory.
  - Persisting derived usage summaries or migrating historical ledger files.

**Acceptance criteria**:
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

**Validation commands**:
  - go test ./internal/app -run Usage
  - go test ./...
  - go build ./...
  - make check

**Assumptions**:
  - The implementation must use the existing canonical run-state resolution helper to determine the current run ID; falling back to the most-recently-modified run directory is not permitted. If the run-state file is absent or contains no unambiguous run ID, `pactum usage` without a positional argument exits with a non-zero status.
  - Usage JSONL records may contain extra fields beyond the goal's required fields, and the command should tolerate them.
  - The repository already contains a `pactum usage` surface. The correct implementation approach is to adapt that existing command to the behavior specified in this contract — not to add a duplicate new command. The phrase 'NEW command' in the goal refers to usage reporting as a new CLI feature, not a mandate to create a second binary or duplicate entry-point. The goal's constraint 'do NOT change any existing command' is directed at unrelated commands (status, execute, review, gate, contract, prompt, memory) and does not apply to `pactum usage` itself, which is the explicit subject of this contract.
  - Legacy usage output schemas or flags not named in the goal, such as prior workspace/top reporting, are not required unless the human explicitly keeps them in scope.
  - When no current run can be resolved (e.g. the run-state file is absent or there is no unambiguous latest run), `pactum usage` without a positional run_id exits with a non-zero status and a clear error message; it does not pick an arbitrary run or silently produce empty output.

## Lens: Testability

Checklist:
- Is each acceptance criterion backed by or expressible as a runnable validation command (not just prose)?
- Are any criteria purely prose with no machine-checkable outcome?

## Output

State your analysis in prose. If you find issues, also include a structured block:

```json
{
  "schema": "pactum.reviewer_findings.v1alpha1",
  "findings": [
    {
      "message": "Describe the contract issue clearly.",
      "severity": "medium",
      "category": "quality",
      "blocking": true,
      "evidence": "Quote or cite the contract field that shows the issue."
    }
  ]
}
```

Rules:
- Use severity: low, medium, high, critical.
- Use category: correctness, scope, quality, validation, process, other.
- Omit file and line (not applicable for contract review).
- Set blocking=true for defects that should block approval: gaps that make the contract unexecutable or ungatable.
- Set blocking=false for advisory issues.
- If no issues, say so clearly. Do not include an empty findings block.
