# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260613_083052/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260613_083052/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260613_083052/review/review.json, .heurema/pactum/runs/run_20260613_083052/review/findings.jsonl, .heurema/pactum/runs/run_20260613_083052/review/resolutions.jsonl

## Approved contract
- Goal: Dogfood Pactum with a local codex-acp adapter that returns official ACP PromptResponse.Usage, and align Pactum's ACP usage code/docs with response usage as the primary source.
- In scope:
  - ACP usage handling and tests in internal/agents/acp_transport.go and internal/agents/acp_transport_test.go.
  - User-facing ACP/cost documentation in docs/agents.md and docs/cost-budget-design.md.
- Out of scope:
  - Changing ACP schema dependencies or codex-acp source from this Pactum repository task.
  - Persisting local absolute adapter paths in source, docs, or committed run records.
  - Removing the legacy codex/token_usage metadata parser unless it conflicts with official PromptResponse.Usage.
- Acceptance criteria:
  - Pactum treats ACP PromptResponse.Usage as authoritative for token accounting and preserves the existing legacy codex/token_usage metadata path only as a fallback when prompt usage is absent.
  - Docs describe official ACP PromptResponse.Usage as the primary Codex-over-ACP usage source and describe codex/token_usage metadata only as legacy/fork fallback compatibility.
  - The run is executed with a locally built codex-acp adapter via PACTUM_CODEX_ACP_COMMAND so Pactum dogfoods the official usage response path.
  - After execution, Pactum usage reporting for this run records a captured Codex call rather than an 'acp prompt returned no usage' warning.
  - No source or docs file contains an absolute local filesystem path to the adapter binary.
- Validation commands:
  - make check

## Current review findings
- Summary: findings=4 open=3 resolved=1 blocking_open=3
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=validation blocking=true status=open: The executed run did not record captured Codex usage, so the dogfood acceptance criterion is unmet.
    location: .heurema/pactum/runs/run_20260613_083052/ledger/usage.jsonl:1
  - f_002 severity=high category=validation blocking=true status=open: The dogfood usage acceptance criterion is not met: the run ledger records the execute Codex call as uncaptured with zero tokens, so this run does not demonstrate captured PromptResponse.Usage accounting.
    location: .heurema/pactum/runs/run_20260613_083052/ledger/usage.jsonl:1
  - f_004 severity=medium category=correctness blocking=true status=open: parseCodexACPUsageMeta rejects legacy Codex ACP metadata when reasoning_output_tokens is absent, even though the existing Codex usage normalization treats that field as optional/newer. Otherwise valid fallback metadata is discarded and token usage is reported uncaptured.
    location: internal/agents/acp_transport.go:391
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - none
- Resolved findings (already addressed — context only):
  - f_003 severity=medium category=correctness blocking=true status=resolved: PromptResponse.Usage normalization can emit internally inconsistent token accounting by adding cache/reasoning subtotals to input/output while preserving the raw total, producing captured records where input_tokens exceeds total_tokens.
    location: internal/agents/acp_transport.go:334
    latest resolution: Updated internal/agents/acp_transport.go so ACP PromptResponse.Usage treats cache/reasoning as sub-counts instead of adding them into input/output again, and materializes TotalTokens to at least InputTokens + OutputTokens. Updated tests in internal/agents/acp_transport_test.go and docs in docs/agents.md and docs/cost-budget-design.md. Validation: make check passed.

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
  "schema": "pactum.review_fix_outcomes.v1",
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
