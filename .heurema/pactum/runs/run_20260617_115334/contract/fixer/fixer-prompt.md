# Contract Review Fixer Prompt

You are fixing a software change contract to address blocking review findings.

Current contract version: 53ab6b4367226602a773671d2f7607b53cd77523f7deb8c1cbb15f7ab8893ca9

## Current Contract

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
  - `--by stage`, `--by model`, `--by agent`, and `--by provider` each produce groups keyed by the selected dimension, with deterministic output.
  - Fully captured data prints no lower-bound warning and emits JSON with `coverage.complete: true` and `totals.lower_bound: false`.
  - Any `captured:false` record makes the total a lower bound, reports captured N/M counts, and lists uncaptured provider/stage combinations in human and JSON output.
  - `--json` emits only valid `pactum.usage_summary.v1alpha1` JSON with the schema described in the goal; the `coverage` object includes an `unparseable_records` integer field (count of torn/partial final JSONL lines skipped during this run).
  - A torn or partial final JSONL line is skipped with a warning and counted in `coverage.unparseable_records`; it is excluded from `coverage.records` and does not influence `coverage.complete` or `totals.lower_bound`, which are determined solely by the `captured` field on parseable records.
  - A missing run usage ledger returns zero totals and a warning instead of crashing.
  - A malformed non-final JSONL line returns an error.
  - The command does not create, modify, or delete any file or directory during execution; a focused test verifies that no workspace path changes between before and after invoking `pactum usage`.

**Validation commands**:
  - go test ./internal/app -run Usage
  - go test ./...
  - go build ./...
  - make check

**Assumptions**:
  - The implementation should use existing artifact path and current-run resolution helpers where possible.
  - Usage JSONL records may contain extra fields beyond the goal's required fields, and the command should tolerate them.
  - The repository already contains a `pactum usage` surface. The correct implementation approach is to adapt that existing command to the behavior specified in this contract — not to add a duplicate new command. The phrase 'NEW command' in the goal refers to usage reporting as a new CLI feature, not a mandate to create a second binary or duplicate entry-point. The goal's constraint 'do NOT change any existing command' is directed at unrelated commands (status, execute, review, gate, contract, prompt, memory) and does not apply to `pactum usage` itself, which is the explicit subject of this contract.
  - Legacy usage output schemas or flags not named in the goal, such as prior workspace/top reporting, are not required unless the human explicitly keeps them in scope.
  - When no current run can be resolved (e.g. the run-state file is absent or there is no unambiguous latest run), `pactum usage` without a positional run_id exits with a non-zero status and a clear error message; it does not pick an arbitrary run or silently produce empty output.

## Blocking Findings to Address

1. [opus/completeness] The `--by agent` grouping key is ambiguous: usage records carry both `agent` and `agent_name`, and the contract never says which field is the group key (or which is the display label). The acceptance criterion '--by agent ... produce groups keyed by the selected dimension' is not independently verifiable with two real candidate fields.
   Evidence: Goal lists record fields 'agent, agent_name' and flag '--by stage|model|agent|provider'; acceptance criterion: '`--by agent` ... each produce groups keyed by the selected dimension, with deterministic output.' No field mapping for the agent dimension is given.
2. [opus/completeness] The `--by agent` grouping key is ambiguous: usage records carry both `agent` and `agent_name`, and the contract never says which field is the group key (or which is the display label). The acceptance criterion '--by agent ... produce groups keyed by the selected dimension' is not independently verifiable with two real candidate fields.
   Evidence: Goal lists record fields 'agent, agent_name' and flag '--by stage|model|agent|provider'; acceptance criterion: '`--by agent` ... each produce groups keyed by the selected dimension, with deterministic output.' No field mapping for the agent dimension is given.
3. [codex-xhigh/completeness] `--by agent` is not fully specified because usage records contain both `agent` and `agent_name`, but the contract does not say which field is the group key or whether one falls back to the other.
   Evidence: Usage records include fields: `provider, agent, agent_name...`; flags include `--by stage|model|agent|provider`.
4. [codex-xhigh/completeness] The lower-bound aggregation rules do not explicitly say whether group token totals exclude `captured:false` records the same way `totals` do.
   Evidence: `totals{... captured_only: true, lower_bound: bool}` and `totals are captured-only`, but `groups:[{... input_tokens, output_tokens, total_tokens...}]` has no equivalent rule.
5. [codex-xhigh/completeness] The JSON schema is incomplete for `scope` and `warnings`: it does not specify the `scope.run_id` value for `--all`, nor the element shape of `warnings` even though warnings are required for missing ledgers and partial files.
   Evidence: JSON schema: `{schema, scope{kind: run|all, run_id}, ... warnings:[]}`; acceptance requires `--json` and warning-producing cases.
6. [codex-xhigh/completeness] Final-line parse behavior is under-specified: torn or partial final JSONL lines are skipped, malformed earlier lines error, but a syntactically malformed final line that is not clearly torn/partial is not classified.
   Evidence: `tolerate a torn/partial last JSONL line...`; `a malformed earlier line is an error`.
7. [codex-xhigh/assumptions-surfaced] The contract should explicitly define how `pactum usage` resolves the current run, including the authoritative state file/helper and whether falling back to the latest run directory is allowed.
   Evidence: Assumptions: "The implementation should use existing artifact path and current-run resolution helpers where possible." Acceptance criteria: "if no current run can be resolved, it exits with a non-zero status..."

## Fixer Instructions

- Address each blocking finding by updating the relevant contract field.
- Do NOT change the goal field — it is out of scope for the fixer.
- Only include the contract fields you are changing in the output.
- base_version must exactly match the version shown above.

## Output

Output your reasoning, then a single JSON block with the revise payload:

```json
{
  "schema": "pactum.contract_revise.v1alpha1",
  "base_version": "53ab6b4367226602a773671d2f7607b53cd77523f7deb8c1cbb15f7ab8893ca9",
  "contract": {
    "acceptance_criteria": ["...updated criteria..."],
    "validation": {"commands": ["...updated commands..."]}
  }
}
```

Omit any contract field you are not changing. Do not include the goal field.
