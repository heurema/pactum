# Task

Add a new top-level CLI command 'pactum usage [run_id]' that summarizes agent token usage from a run's ledger/usage.jsonl — a plain 'where do tokens go and how much' view. No cost in dollars, no tier/role/economics classification.

It reads usage records (one JSON line per agent attempt) with fields: run_id, stage, provider, agent, agent_name, request_model, input_tokens, output_tokens, total_tokens, captured (bool). Default: summarize the current run (or the given run_id), grouped by stage. Flags: --by stage|model|agent|provider (default stage) selects the grouping dimension; --all aggregates across all runs (runs/*/ledger/usage.jsonl); --json emits schema pactum.usage_summary.v1alpha1.

Human output: a table of the chosen grouping (per group: input/output/total tokens and 'captured N/M' records), a TOTAL row, and a coverage line. When any record has captured=false, tag the total as a LOWER BOUND and list which provider/stages are uncaptured. A fully-captured run prints no lower-bound warning.

JSON (pactum.usage_summary.v1alpha1): {schema, scope{kind: run|all, run_id}, coverage{records, captured_records, uncaptured_records, complete, uncaptured:[{provider,stage,records}]}, totals{input_tokens, output_tokens, total_tokens, captured_only: true, lower_bound: bool}, groups:[{by, key, input_tokens, output_tokens, total_tokens, records, captured_records}], warnings:[]}.

Robustness: tolerate a torn/partial last JSONL line in a live run (skip it + emit a warning, count it as unparseable); a malformed earlier line is an error; a missing usage.jsonl for a run yields zero rows + a warning, not a crash.

Constraints: NEW command only; do NOT change the usage-recording path or any existing command; do NOT add dollar cost or pricing; NO tier/role/economics dimension (this is a plain usage view). Validation: go build ./..., go test ./..., make check, plus a focused go test for the new command covering aggregation, coverage/lower-bound, --by, --json, and a partial-file case.

Generated: 2026-06-17T11:53:34Z
