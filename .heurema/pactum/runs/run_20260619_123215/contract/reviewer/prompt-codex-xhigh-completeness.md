# Contract Review: Completeness

You are reviewing a software change contract through the **contract-completeness** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Make the review loop (both contract_review and code_review, which share internal/app/review_loop.go) never silently terminate with an OPEN BLOCKING finding as if the run were approvable, and stop the fixer from burning all rounds when it makes no progress on a blocker.

Observed problem (live): on a large contract the contract-review loop ran the full max_rounds, hit a stalemate/max_rounds terminal, and HANDED BACK a contract with a real blocking finding still open — appearing as a normal stop, so an operator could approve a contract that still has an unresolved blocker. Separately, the fixer kept failing to land the one blocking finding round after round, burning all rounds rewording around it.

In scope:
1. Loud, non-approvable terminal on open blockers: when the loop terminates via stalemate or max_rounds while there is at least one OPEN BLOCKING finding, the terminal_reason / status must be a DISTINCT, clearly non-approvable terminal (e.g. blockers_open) rather than the generic stalemate/max_rounds that reads as a clean stop. The summary/output must make the open blocking count prominent.
2. Approve guard holds: contract approve and review approve must REFUSE while any blocking finding is open (verify the existing guard; if it already refuses, ensure the loop's terminal state and human/JSON output surface the open blocker loudly and point the operator at it via next-commands, instead of looking like a successful convergence). The two-layer point: the loop must not pretend to have resolved a blocker it did not.
3. Fixer-no-progress escalation: if the fixer fails to change the set/status of OPEN BLOCKING findings for K consecutive rounds (K small, e.g. 2), stop early with a distinct escalation terminal rather than running the remaining rounds rewording around the same blocker. Record the escalation reason.
4. Advisory findings must not drive non-convergence: the loop converges once all BLOCKING findings are resolved; advisory (non-blocking) findings are recorded but do not extend the loop or count toward non-convergence. If this is already the behavior, add a test that locks it.

Keep max_rounds default at 10 (the round count is not the lever). Do not change the reviewer panel composition, the reviewer-findings capture/parse (that is the shipped #196 behavior), or add reviewer-attempt timeouts (separate slice). Do not reintroduce any plan-DAG concept (removed).

Tests: a loop run that ends at stalemate/max_rounds with an open blocking finding produces the distinct non-approvable terminal and approve refuses; a fixer that makes no progress on a blocker for K rounds escalates early instead of running all rounds; advisory-only remaining findings converge clean; existing convergence (blockers cleared) still reaches the resolved/clean terminal. Cover both contract_review and the shared review_loop paths with helper-process fixtures; do not invoke real agents.

Validation: go test ./internal/app -run 'Review|Loop|Contract', go test ./..., go build ./..., make check.

**Scope in**:
  - Update contract_review and code_review loop termination so stalemate or max_rounds with at least one open blocking finding is reported as a distinct non-approvable terminal_reason, using blockers_open.
  - Add fixer no-progress detection for open blocking findings with K=2 consecutive no-change fixer rounds, ending early with terminal_reason fixer_no_progress and recording the reason/streak in loop output.
  - Surface open blocking finding counts prominently in JSON responses, durable summaries where present, and human-readable output for blockers_open and fixer_no_progress terminals.
  - Ensure contract approve and review approve refuse while blocking findings for their phase remain open, and next commands point to inspection or rerun/fix commands instead of suggesting approval.
  - Add helper-process tests for both contract_review and code_review/review_loop paths; tests must not invoke real agents.

**Scope out**:
  - Changing the default max_rounds value from 10.
  - Changing reviewer panel composition or lens selection.
  - Changing reviewer finding capture/parsing behavior.
  - Adding reviewer-attempt timeout behavior.
  - Reintroducing plan-DAG concepts.
  - Running real agent execution in tests.

**Acceptance criteria**:
  - A contract_review loop that reaches stalemate or max rounds while the final round contains at least one finding with blocking=true returns terminal_reason blockers_open, not stalemate or max_rounds, and its JSON and human output include the count of open (blocking=true) findings.
  - A code_review review loop that reaches stalemate or max rounds while the count of findings with blocking=true in the latest round is greater than zero returns terminal_reason blockers_open, not stalemate or max_rounds, and its JSON response, summary artifact, and human output include the open blocking count.
  - When the fixer leaves the set of canonical keys (reviewer-lens, normalized-title) of findings with blocking=true unchanged for 2 consecutive fixer rounds, the loop stops before consuming remaining max_rounds and returns terminal_reason fixer_no_progress with the no-progress reason and streak count recorded in loop output.
  - Advisory-only findings do not prevent convergence: contract_review resolves when all findings with blocking=true are gone (findings with blocking=false may remain recorded without blocking convergence), and code_review reaches a clean round when zero findings with blocking=true remain in that round (findings with blocking=false in the round do not count toward the clean-round threshold and do not extend the loop). A test must lock the clean-round-counts-only-blocking-findings behavior, covering the case where advisory findings (blocking=false) are present in the round but no findings with blocking=true remain.
  - Existing successful convergence remains unchanged: loops still return resolved or clean_round when all findings with blocking=true are cleared.
  - review approve exits nonzero and does not mark the review approved while any finding with blocking=true is present in the latest review run's stored output or summary artifact.
  - contract approve exits nonzero and does not mark the contract approved when the latest contract review state contains any unresolved finding with blocking=true.
  - Next-command output for blockers_open and fixer_no_progress terminals directs the operator to show/review/fix the run and does not present approval as the safe next action.

**Validation commands**:
  - go test ./internal/app
  - go test ./...
  - go build ./...
  - make check

**Assumptions**:
  - K for fixer no-progress escalation is 2.
  - blockers_open is the distinct terminal_reason for open blocking findings at stalemate or max_rounds.
  - fixer_no_progress is the distinct terminal_reason for repeated fixer no-progress on open blocking findings.
  - Contract-review blocking findings can be persisted or derived durably enough for contract approve and next-command logic to enforce the approval guard.
  - Code-review blocking findings (findings with blocking=true in the latest review run's output) can be persisted or derived durably enough for review approve and next-command logic to enforce the approval guard. The durable source of truth is the latest review run's stored output or summary artifact, which records each finding's blocking field; review approve reads this artifact and refuses while any finding with blocking=true is present.
  - Finding identity for cross-round comparison: a finding is treated as a blocking finding for streak-detection purposes when its `blocking` field is true (independent of its `severity` value). Its canonical key is the tuple (reviewer-lens, normalized-title), where normalized-title is derived from the raw title by: (1) lowercasing the entire string, (2) trimming leading and trailing whitespace, and (3) collapsing any run of internal whitespace characters to a single space. Two findings from different rounds are the same open blocker when their canonical keys match. This key is used only for fixer-no-progress streak detection; it is not a persistent finding ID and does not affect reviewer-output capture or parse.
  - A finding is OPEN in round R if it appears in round R's reviewer output with the `blocking` field set to true. The `blocking` boolean is the sole predicate for classifying a finding as a blocker; the `severity` enum (low/medium/high/critical) is independent and does not determine blocker status. A finding with blocking=true and any severity value (including low or medium) is an open blocking finding. Absence from the current round's reviewer output — meaning no finding in that round's output has blocking=true and a matching canonical key — is the resolution predicate; there is no separate resolution event.
  - A clean round in code_review is defined as a round in which no reviewer output contains a finding with blocking=true. Findings with blocking=false (advisory findings) present in the round do not count toward the clean-round threshold and do not extend the loop. If the existing implementation counts all findings regardless of the blocking field toward the clean-round threshold, this slice must update that definition to gate only on findings where blocking=true and lock the behavior with a test.

## Lens: Completeness

Checklist:
- Does the contract fully cover its goal? Are there gaps in scope or acceptance_criteria?
- Is every acceptance criterion specific and observable enough to verify?

## Output

State your analysis in prose. If you find issues, also include a structured block:

```json
{
  "schema": "pactum.reviewer_findings.v1alpha1",
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
