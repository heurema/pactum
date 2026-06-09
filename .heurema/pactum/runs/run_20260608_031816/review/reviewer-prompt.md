# Reviewer Prompt

This prompt is prepared for a reviewer agent subprocess.
Pactum captures reviewer output as artifacts and may parse optional structured proposal blocks, but it does not trust reviewer output automatically.

## Objective
Review the executed task against the approved Pactum contract and gate report.

## Inputs
- Reviewer context: .heurema/pactum/runs/run_20260608_031816/review/reviewer-context.md
- Contract: .heurema/pactum/runs/run_20260608_031816/contract/contract.json
- Gate report: .heurema/pactum/runs/run_20260608_031816/gate/gate-report.json
- Review artifacts: .heurema/pactum/runs/run_20260608_031816/review/review.json, .heurema/pactum/runs/run_20260608_031816/review/findings.jsonl, .heurema/pactum/runs/run_20260608_031816/review/resolutions.jsonl, .heurema/pactum/runs/run_20260608_031816/review/proposals.jsonl, .heurema/pactum/runs/run_20260608_031816/review/proposal-decisions.jsonl

## Review boundaries
- Do not apply patches.
- Do not modify files.
- Do not approve the review.
- Do not claim semantic correctness without evidence.
- Prefer concrete findings with file/path evidence.
- Focus on real problems, not style preferences.
- Read the actual file and surrounding context before proposing a finding.
- Check whether the issue is already mitigated or already represented in existing findings/proposals.
- If uncertain, recommend a blocking manual finding.

## Output shape
If you report findings in prose, make them easy for a human to convert manually:
- message
- severity
- category
- file
- line
- blocking

## Optional structured finding proposals

If you propose findings, include a fenced JSON block exactly like:

```json
{
  "schema": "pactum.reviewer_findings.v1",
  "findings": [
    {
      "message": "Explain the issue clearly.",
      "severity": "medium",
      "category": "quality",
      "file": "internal/app/example.go",
      "line": 42,
      "blocking": true,
      "evidence": "Short evidence from reviewed artifacts."
    }
  ]
}
```

Rules:
- Use repo-relative file paths only.
- Do not include absolute paths.
- Use severity: low, medium, high, critical.
- Use category: correctness, scope, quality, validation, process, other.
- If uncertain, set blocking=true and explain uncertainty in evidence.

Important: Pactum does not trust this output automatically. A human must accept proposals.
