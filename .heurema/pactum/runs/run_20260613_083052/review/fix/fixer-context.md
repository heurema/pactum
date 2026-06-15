# Review Fixer Context

## Run
- Run id: run_20260613_083052
- Run status: contract_approved

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

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
