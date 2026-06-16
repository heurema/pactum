# Contract Review: Assumptions surfaced

You are reviewing a software change contract through the **assumptions-surfaced** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Implement slice 2 of contract cross-review, per docs/contract-review-design.md: the auto-fixer and convergence loop. Today 'contract review' (slice 1) only emits findings. Add: when the contract reviewers produce accepted findings, a fixer applies them to the contract via the declarative 'contract revise --from -' primitive (partial-replace, version-guarded — read the current contract version, apply the fix, never reset approval since this runs pre-approve), then the panel re-reviews, converging until a clean round or max rounds — mirroring the code review loop. Reuse the convergence machinery in internal/app/review_loop.go (rounds / patience / clean_rounds) and the graceful skip-on-failed-lens behavior; the fixer is the drafter-role agent resolved like elsewhere. When contract.reviewers is empty/absent the whole step stays a no-op (slice-1 behavior). The loop must surface per-round results (findings, accepted fixes, skipped lenses) and a terminal reason, in human-readable and --json form. Add tests for: a finding is fixed via revise and the re-review converges clean; convergence stops at max rounds with findings still open; the version guard is used by the fixer so a concurrent change is not clobbered; empty reviewers stays a no-op. Do not change the code review loop. Follow docs/contract-review-design.md.

**Scope in**:
  - Implement slice 2 for `pactum contract review`: configured contract reviewers produce accepted contract findings, a fixer applies valid fixes, and the panel re-reviews until convergence or a configured stop.
  - Add contract-review fixer prompt/context/result handling that uses `pactum contract show --json` and applies edits only through `pactum contract revise <run> --from -` with `base_version`.
  - Reuse the existing review loop concepts for max rounds, patience/stalemate, clean rounds, skipped lenses, per-round summaries, and terminal reasons without changing code-review loop behavior.
  - Surface contract review loop results in both human-readable output and `--json`, including per-round findings, accepted fixes, skipped lenses, and terminal reason.
  - Add focused tests for clean convergence after a fixer revise, max-round termination with findings still open, stale version protection, and empty `contract.reviewers` no-op behavior.

**Scope out**:
  - Changing the contract goal.
  - Changing code review loop behavior or code review artifacts except for narrowly shared/extracted reusable helpers.
  - Running real agent subprocesses in tests; tests should use helper processes or fakes.
  - Changing `contract revise --from` partial-replace or version-guard semantics except as needed to consume the existing primitive.
  - Supporting approval-resetting contract review fixes; this slice runs before contract approval.

**Acceptance criteria**:
  - With non-empty `contract.reviewers`, `pactum contract review <run> --json` returns a contract-review loop response containing `rounds`, per-round findings/fix data, skipped lenses, and `terminal_reason`.
  - Human-readable `pactum contract review <run>` output shows each round's findings/fixes/skipped lenses and the terminal reason.
  - When a blocking contract finding is emitted, the fixer is invoked, reads the current contract version, calls `contract revise --from -` with `base_version`, and the next reviewer round runs against the revised contract.
  - A successful fixer revise can lead to a subsequent clean round and a clean terminal reason without requiring human approval during the loop.
  - When findings remain through the configured round cap, the loop stops with `terminal_reason` of `max_rounds` and reports the remaining open findings.
  - A stale `base_version` from the fixer path does not overwrite a concurrent contract change; the failure is surfaced and the contract remains unchanged.
  - When `contract.reviewers` is empty or absent, `pactum contract review` remains a no-op: no reviewer or fixer attempts are created and existing slice-1 behavior is preserved.
  - Existing code review loop tests continue to pass unchanged.

**Validation commands**:
  - go test ./internal/app -run TestContractReview
  - go test ./internal/app -run TestReviewLoop
  - make check

**Assumptions**:
  - Contract review findings may be accepted automatically inside the loop, mirroring `pactum review run`, rather than adding a separate human accept/reject proposal flow for this slice.
  - The contract-review fixer should resolve through the same registry semantics currently used for contract drafting unless a distinct drafter role already exists during implementation.
  - Contract review loop limits should reuse the existing review max-rounds, patience, and clean-rounds settings and CLI flag style unless implementation discovers a contract-specific config already exists.

## Lens: Assumptions surfaced

Checklist:
- Are risky assumptions explicitly called out rather than buried in scope or acceptance criteria?
- Are there implicit assumptions that affect executor behaviour and should be made explicit?

## Output

State your analysis in prose. If you find issues, also include a structured block:

```json
{
  "schema": "pactum.reviewer_findings.v1",
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
