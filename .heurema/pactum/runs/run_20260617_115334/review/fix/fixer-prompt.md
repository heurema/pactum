# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260617_115334/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260617_115334/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260617_115334/review/review.json, .heurema/pactum/runs/run_20260617_115334/review/findings.jsonl, .heurema/pactum/runs/run_20260617_115334/review/resolutions.jsonl

## Approved contract
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

## Current review findings
- Summary: findings=7 open=7 resolved=0 blocking_open=3
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_002 severity=medium category=correctness blocking=true status=open: Human output falls back to STAGE for empty summaries even when another --by value was selected.
    location: internal/app/usage.go:729
  - f_003 severity=medium category=quality blocking=true status=open: TestUsageSummaryNoWorkspaceMutation does not actually detect file modifications. It snapshots only relative paths, so a pactum usage implementation that rewrites an existing file would still pass.
    location: internal/app/usage_test.go:802
  - f_004 severity=medium category=quality blocking=true status=open: The focused usage tests do not exercise pactum usage with an omitted run_id, so the current-run default and no-current failure path required by the contract are untested for this command.
    location: internal/app/usage_test.go:129
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=low category=correctness blocking=false status=open: Empty-result human output ignores non-default --by and labels the table as STAGE.
    location: internal/app/usage.go:729
  - f_005 severity=low category=quality blocking=false status=open: The --by agent human-label fallback is untested. The only --by agent test records both set AgentName, so the required fallback to agent when agent_name is empty is not covered.
    location: internal/app/usage_test.go:356
  - f_006 severity=low category=quality blocking=false status=open: Empty usage summaries silently render the wrong grouping header because the human renderer falls back to stage when there are no groups.
    location: internal/app/usage.go:729
  - f_007 severity=medium category=quality blocking=false status=open: The usage documentation still describes the removed --top/by_run/effective_units/cache-ratio usage output instead of the new --by-based pactum.usage_summary.v1alpha1 view.
    location: docs/cost-budget-design.md:296

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1alpha1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
