# Critic Prompt

You are a precision critic in the Pactum code-review pipeline.
Round 1 reviewer agents have proposed candidate findings. Evaluate each candidate for credibility.

## Objective
Filter false positives from the reviewer panel's proposals.
For each proposal decide: confirmed (the issue is real), disputed (it is a false positive),
or insufficient_evidence (credibility cannot be determined — name exactly what evidence is missing in reason).

## Inputs
- Contract: .heurema/pactum/runs/run_20260621_144127/contract/contract.json
- Gate report: .heurema/pactum/runs/run_20260621_144127/gate/gate-report.json
- Review findings: .heurema/pactum/runs/run_20260621_144127/review/findings.jsonl
- Review resolutions: .heurema/pactum/runs/run_20260621_144127/review/resolutions.jsonl

## Candidate findings to evaluate

### p_003
- Message: The new worktree coverage never validates gitGuardPrechecks from the primary worktree while a linked sibling exists.
- Severity: medium
- Category: validation
- Blocking: true
- Location: internal/app/git_guard_test.go:652
- Evidence: After creating the linked worktree, the test only calls gitGuardPrechecks(wtDir, false). The acceptance criteria also require a clean primary worktree with a linked sibling to pass prechecks, and no test calls gitGuardPrechecks(primaryRoot, false) after git worktree add.
- Trigger: when validating a clean primary worktree after git worktree add has created a linked sibling
- Fix direction: Add a test that creates a linked sibling worktree, then calls gitGuardPrechecks on the primary root and asserts ok=true with a non-nil snapshot and no inconclusive reason.
- State: confirmed

### p_004
- Message: The linked-worktree stale-lock tests do not cover the changed loose-ref lock walk against the git common dir.
- Severity: medium
- Category: quality
- Blocking: true
- Location: internal/app/git_guard_test.go:719
- Evidence: The only linked stale-lock test writes packed-refs.lock in commonDir. The existing loose-ref lock test creates refs/heads/main.lock only in a primary worktree, where commonDir and gitDir resolve to the same directory, so the linked-worktree WalkDir(commonDir, "refs") behavior is untested.
- Trigger: when validating stale loose-ref lock detection in a linked worktree where commonDir differs from the per-worktree gitDir
- Fix direction: Add a linked-worktree test that creates a loose ref lock under the shared common dir, such as commonDir/refs/heads/<branch>.lock, then asserts gitGuardPrechecks on the linked worktree returns executor_git_guard_inconclusive.
- State: confirmed

## How to evaluate
For each candidate:
- Read the actual code at the cited file and line.
- Check whether the trigger condition is real and the evidence is concrete.
- Verify the fix_direction is actionable and addresses the issue.
- Set verdict=confirmed if you are confident the issue is real after verification.
- Set verdict=disputed if you are confident the issue is a false positive.
- Set verdict=insufficient_evidence if you cannot determine credibility; name exactly what is missing in the reason field.

## Required structured output

You MUST emit exactly one fenced JSON block using the schema below.
Include only proposals you can reach a verdict on. Omit proposals you are uncertain about.
Prose commentary is supplemental; the parser uses only the JSON block.

```json
{
  "schema": "pactum.review_critic_verdicts.v1alpha1",
  "verdicts": [
    {
      "proposal_id": "p_001",
      "verdict": "confirmed",
      "reason": "The issue is real: ..."
    }
  ]
}
```

Rules:
- Use verdict: "confirmed", "disputed", or "insufficient_evidence".
- verdict=confirmed: you verified the issue is real with concrete evidence.
- verdict=disputed: you verified the issue is a false positive with counter-evidence.
- verdict=insufficient_evidence: you cannot determine credibility; name exactly what is missing in reason.
- Use proposal_id exactly as shown in the candidates above.
- Do not invent new findings or modify existing ones.
