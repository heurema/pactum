# Reviewer Context

## Run
- Run id: run_20260613_083052
- Run status: contract_approved

## Contract
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

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 5
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: make check (exit 0, timed out: false, result: gate/validation/command_001/result.json)
- Change summary:
  - changed files:
    - docs/agents.md
    - docs/cost-budget-design.md
    - internal/agents/acp_transport.go
    - internal/agents/acp_transport_test.go
  - new files:
    - none
  - missing files:
    - none

## Existing manual review
- Review status: changes_requested
- Current findings summary: findings=4 open=2 resolved=2 blocking_open=2
- Existing findings:
  - f_001 severity=medium category=validation blocking=true status=open: The executed run did not record captured Codex usage, so the dogfood acceptance criterion is unmet.
  - f_002 severity=high category=validation blocking=true status=open: The dogfood usage acceptance criterion is not met: the run ledger records the execute Codex call as uncaptured with zero tokens, so this run does not demonstrate captured PromptResponse.Usage accounting.
  - f_003 severity=medium category=correctness blocking=true status=resolved: PromptResponse.Usage normalization can emit internally inconsistent token accounting by adding cache/reasoning subtotals to input/output while preserving the raw total, producing captured records where input_tokens exceeds total_tokens.
  - f_004 severity=medium category=correctness blocking=true status=resolved: parseCodexACPUsageMeta rejects legacy Codex ACP metadata when reasoning_output_tokens is absent, even though the existing Codex usage normalization treats that field as optional/newer. Otherwise valid fallback metadata is discarded and token usage is reported uncaptured.
- Existing resolutions:
  - r_001 finding=f_003 source=review_fix note=Updated internal/agents/acp_transport.go so ACP PromptResponse.Usage treats cache/reasoning as sub-counts instead of adding them into input/output again, and materializes TotalTokens to at least InputTokens + OutputTokens. Updated tests in internal/agents/acp_transport_test.go and docs in docs/agents.md and docs/cost-budget-design.md. Validation: make check passed.
  - r_002 finding=f_004 source=review_fix note=Updated internal/agents/acp_transport.go so parseCodexACPUsageMeta no longer requires reasoning_output_tokens. Added regression coverage in internal/agents/acp_transport_test.go for legacy metadata without reasoning_output_tokens while keeping missing required fields uncaptured. Validation: go test ./internal/agents passed; make check passed.
- Proposal summary: pending=0 accepted=4 rejected=0
- Existing proposals:
  - p_001 severity=medium category=validation blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_003: The executed run did not record captured Codex usage, so the dogfood acceptance criterion is unmet.
    location: .heurema/pactum/runs/run_20260613_083052/ledger/usage.jsonl:1
  - p_002 severity=high category=validation blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_004: The dogfood usage acceptance criterion is not met: the run ledger records the execute Codex call as uncaptured with zero tokens, so this run does not demonstrate captured PromptResponse.Usage accounting.
    location: .heurema/pactum/runs/run_20260613_083052/ledger/usage.jsonl:1
  - p_003 severity=medium category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_007: PromptResponse.Usage normalization can emit internally inconsistent token accounting by adding cache/reasoning subtotals to input/output while preserving the raw total, producing captured records where input_tokens exceeds total_tokens.
    location: internal/agents/acp_transport.go:334
  - p_004 severity=medium category=correctness blocking=true status=accepted source=reviewer_attempt attempt=reviewer_attempt_012: parseCodexACPUsageMeta rejects legacy Codex ACP metadata when reasoning_output_tokens is absent, even though the existing Codex usage normalization treats that field as optional/newer. Otherwise valid fallback metadata is discarded and token usage is reported uncaptured.
    location: internal/agents/acp_transport.go:391

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
