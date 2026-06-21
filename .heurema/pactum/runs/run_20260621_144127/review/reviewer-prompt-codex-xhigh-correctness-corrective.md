# Corrective Reviewer Prompt

This prompt is prepared for a corrective reviewer attempt.
Your previous response did not include a valid `pactum.reviewer_findings.v1alpha1` JSON block.
Findings expressed only in prose in the previous attempt are not recoverable; re-review the task using the inputs below.

## Inputs
- Reviewer context: .heurema/pactum/runs/run_20260621_144127/review/reviewer-context.md
- Contract: .heurema/pactum/runs/run_20260621_144127/contract/contract.json
- Gate report: .heurema/pactum/runs/run_20260621_144127/gate/gate-report.json
- Review artifacts: .heurema/pactum/runs/run_20260621_144127/review/review.json, .heurema/pactum/runs/run_20260621_144127/review/findings.jsonl, .heurema/pactum/runs/run_20260621_144127/review/resolutions.jsonl, .heurema/pactum/runs/run_20260621_144127/review/proposals.jsonl, .heurema/pactum/runs/run_20260621_144127/review/proposal-decisions.jsonl

## Review lens: Correctness

You are the correctness reviewer. Review the task against the approved contract and gate report, focusing on your lens:
- Logic errors: off-by-one, wrong operators, inverted conditions.
- Edge cases: empty, nil, boundary, and concurrent inputs.
- Error handling: no silent failures.
- Resource cleanup: leaks, unclosed handles.
- Races and deadlocks.

## Required structured output

You MUST emit exactly one fenced JSON block. If you have no findings, emit `"findings": []`.

```json
{
  "schema": "pactum.reviewer_findings.v1alpha1",
  "findings": []
}
```
