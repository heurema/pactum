# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260620_141955/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260620_141955/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260620_141955/review/review.json, .heurema/pactum/runs/run_20260620_141955/review/findings.jsonl, .heurema/pactum/runs/run_20260620_141955/review/resolutions.jsonl

## Approved contract
- Goal: Add anti-false-positive fields to the code-review reviewer findings schema and validate them, modifying the existing schema pactum.reviewer_findings.v1alpha1 IN PLACE (deliberate breaking change — the project has no external users yet, so do NOT introduce a v1alpha2 or any dual-version/migration support). New fields on the reviewer findings JSON (parsed by reviewerFindingProposalInput in internal/app/review_proposals.go and carried onto reviewProposalRecord and reviewFindingRecord in internal/app/review.go): state ("candidate"|"confirmed"), trigger (concrete runtime condition or "always"), fix_direction, uncertainty, current_code_only (bool). evidence and confidence already exist and stay. Validation in proposalRecordFromReviewerInput: require non-empty trigger, evidence, fix_direction and a valid state; a blocking finding must have current_code_only=true (a current_code_only=false finding must not be blocking); a finding that cannot fill the required fields is dropped with a loud parse warning, never silently accepted. Update the code-review reviewer prompt renderReviewerPrompt in internal/app/review.go to emit and explain the new fields and reframe guidance to recall-first (report tightly-evidenced candidates, state uncertainty, drop findings that cannot fill the fields tightly). Hard constraint: the reviewer findings parser is shared with contract-review (contract_review.go reuses reviewerFindingsSchema) — this change must not break contract review; keep parsing tolerant or enforce the new required-field rules only on the code-review path. Scope: internal/app/review_proposals.go, internal/app/review.go and their tests. Do NOT implement the critic pass (a separate slice). Acceptance: schema id stays pactum.reviewer_findings.v1alpha1; new fields parsed, carried onto records, validated; reviewer prompt emits/explains them recall-first; contract review still works; tests cover the new validation and the current_code_only-cannot-be-blocking rule; make check passes.
- In scope:
  - Modify the existing pactum.reviewer_findings.v1alpha1 code-review reviewer finding input, proposal records, and accepted finding records to parse and carry state, trigger, fix_direction, uncertainty, and current_code_only.
  - Add code-review proposal validation in proposalRecordFromReviewerInput for valid state, non-empty trigger, non-empty evidence, non-empty fix_direction, and the rule that persisted blocking findings must have current_code_only=true.
  - Update renderReviewerPrompt so the required JSON example and rules explain the new fields, trigger semantics, current-code-only blocking rule, and recall-first candidate reporting with explicit uncertainty.
  - Add or update internal/app tests covering code-review parsing, validation warnings, record persistence, prompt output, proposal acceptance carry-through, and contract-review compatibility with the shared reviewer findings schema.
- Out of scope:
  - Changing the contract goal or answering clarification questions.
  - Introducing pactum.reviewer_findings.v1alpha2, dual-version parsing, migration support, or a second schema ID.
  - Implementing a critic pass, critic findings slice, or any separate reviewer output stream.
  - Broad refactors or production behavior changes outside internal/app/review_proposals.go and internal/app/review.go except minimal compatibility work required to keep contract review passing.
- Acceptance criteria:
  - The reviewer findings schema identifier remains exactly pactum.reviewer_findings.v1alpha1 everywhere it is emitted or parsed.
  - A code-review reviewer finding with valid state, trigger, fix_direction, uncertainty, current_code_only, evidence, and confidence is collected into a review proposal preserving those fields.
  - Accepting a valid proposal creates a review finding that preserves the new anti-false-positive fields and existing confidence behavior while keeping existing evidence handling unchanged.
  - Code-review findings missing trigger, evidence, or fix_direction, or using a state other than candidate or confirmed, are dropped with field-specific parse warnings and are not written to proposals.jsonl.
  - A reviewer output item with current_code_only=false and blocking=true cannot produce a persisted blocking proposal or finding, and collection emits a warning for that invalid combination.
  - Contract review still parses an otherwise valid pactum.reviewer_findings.v1alpha1 block that omits the new anti-false-positive fields; missing new fields do not by themselves break contract-review collection.
  - The reviewer prompt includes all new fields in the JSON example and rules, explains trigger as a concrete runtime condition or always, and no longer instructs reviewers to discard every uncertain candidate.
  - Tests cover the new validation rules, the current_code_only blocking constraint, prompt rendering, and shared-parser contract-review compatibility.
  - make check passes.
- Validation commands:
  - go test ./internal/app
  - make check

## Current review findings
- Summary: findings=9 open=3 resolved=6 blocking_open=2
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_008 severity=medium category=correctness blocking=true status=open: renderReviewerContext still emits a certain-or-silent instruction in the Reviewer guidance section (review.go:1173: if you are not certain an issue is real after verification, do not flag it). This contradicts the new recall-first model. Replace it with recall-first guidance consistent with renderReviewerPrompt: report likely-real issues as state=candidate and drop only when trigger, evidence, and fix_direction cannot be filled concretely.
    location: internal/app/review.go:1173
  - f_009 severity=medium category=quality blocking=true status=open: docs/agents.md still describes the reviewer methodology as certain-or-silent, contradicting the new recall-first candidate/confirmed model. Update the reviewer methodology description to recall-first: reviewers report likely-real issues as candidates with explicit uncertainty and drop only when trigger, evidence, and fix_direction cannot be filled.
    location: docs/agents.md
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_007 severity=medium category=quality blocking=false status=open: The user-facing agents documentation still describes reviewer methodology as certain-or-silent and CONFIRMED-only, so it is stale after adding candidate findings and anti-false-positive fields.
    location: docs/agents.md:557
- Resolved findings (already addressed — context only):
  - f_001 severity=medium category=correctness blocking=true status=resolved: renderReviewerPrompt still tells reviewers not to flag uncertain issues, contradicting the new candidate-state workflow.
    location: internal/app/review.go:1213
    latest resolution: Replaced the certain-or-silent lines in the '## High-signal contract' section of renderReviewerPrompt (review.go:1211-1213) with recall-first guidance: 'Report every issue you believe is likely real: use state=candidate for uncertain findings…' and 'Do not drop a finding solely because you are uncertain…'.
  - f_002 severity=medium category=scope blocking=true status=resolved: The reviewer prompt still tells reviewers not to report findings unless they are certain, which contradicts the contract's recall-first candidate reporting requirement.
    location: internal/app/review.go:1213
    latest resolution: Same change as f_001 — both findings pointed at the same two lines (1212-1213). The new High-signal contract text explicitly directs reviewers to use candidate state rather than staying silent when uncertain.
  - f_003 severity=medium category=process blocking=true status=resolved: The new uncertainty field is persisted without repo-root sanitization.
    location: internal/app/review_proposals.go:606
    latest resolution: Wrapped the uncertainty assignment in review_proposals.go:606 with sanitizeRepoRootInText(root, uncertainty), consistent with how Trigger and FixDirection are already sanitized on the adjacent lines.
  - f_004 severity=high category=quality blocking=true status=resolved: The prompt methodology test still requires the old certain-or-silent instruction, so it cannot catch the contract violation where reviewers are still told not to report uncertain candidates.
    location: internal/app/review_test.go:2392
    latest resolution: In TestReviewerPromptIncludesReviewMethodology: removed the old certain-or-silent string ('If you are not certain an issue is real, do not flag it…') from the want list and added the two new recall-first strings instead. Added 'If you are not certain an issue is real, do not flag it.' to the gone list so the old text cannot regress, and updated the error message to say 'contradicts recall-first candidate reporting'.
  - f_005 severity=medium category=quality blocking=true status=resolved: The proposal acceptance tests do not exercise or assert accepted-finding carry-through for the new anti-false-positive fields.
    location: internal/app/review_test.go:2779
    latest resolution: Added TestReviewAcceptProposalCarriesAntiFPFields in review_test.go after TestReviewShowDisplaysFindingConfidence. The test collects a reviewer finding with all new anti-FP fields (state, trigger, fix_direction, uncertainty, current_code_only), accepts the proposal, and asserts each field is present on the accepted finding record.
  - f_006 severity=high category=scope blocking=true status=resolved: The generated reviewer prompt still tells reviewers to report findings only when certain, which contradicts the new recall-first candidate workflow.
    location: internal/app/review.go:1212
    latest resolution: Same root change as f_001/f_002 — the two lines the reviewer cited (1212-1213) are the same High-signal contract lines replaced in that fix. The generated reviewer prompt no longer tells reviewers to report findings only when certain.

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
