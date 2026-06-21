# Reviewer Context

## Run
- Run id: run_20260620_195608
- Run status: contract_approved

## Contract
- Goal: Apply precision discipline to the CONTRACT reviewer (the panel that reviews the contract document before approval), mirroring the anti-false-positive discipline code-review got, but adapted for a spec document. The contract reviewer is a SEPARATE mechanism from code review: it uses pactum.contract_reviewer_result.v1alpha1 (NOT the code-review reviewer_findings schema), its own lenses (scope-fidelity, completeness, testability, validation-soundness, assumptions-surfaced), and renderContractReviewerPrompt in internal/app/contract_review.go. Modify that schema/prompt/parser IN PLACE (deliberate breaking change — no external users yet; no v2 or migration). Do NOT touch the code-review reviewer.

Problem this fixes: contract review currently grinds to blockers_open / max_rounds on low-signal WORDING nitpicks about the contract document (e.g. "acceptance criteria are prose-only", "term X is not defined", "criterion wording could be tighter"), because nothing forces the reviewer to distinguish a material spec defect from a style/completeness preference, and any nit can be marked blocking.

Changes:
1. Add fields to the contract reviewer finding (the struct parsed from the panel output and persisted): material_impact (string — for a blocking finding, the concrete way this spec defect would make the IMPLEMENTATION wrong, ambiguous, or stuck), fix_direction (string), uncertainty (string), and state ("candidate" or "confirmed"). Keep existing message/evidence/severity/category/blocking; evidence already exists.
2. Update renderContractReviewerPrompt to: (a) frame reviewers recall-first but precision-gated — report likely-real defects as state=candidate with explicit uncertainty; (b) state the HARD RULE: a finding may set blocking=true ONLY if it is a material spec defect that would make the implementation wrong, ambiguous, or stuck — wording, style, naming, redundancy, and completeness/thoroughness PREFERENCES must be blocking=false (advisory); (c) require every blocking finding to fill material_impact concretely; (d) instruct reviewers to mark advisory (not blocking) any finding they cannot tie to a material implementation consequence.
3. Enforce in the parser/validation of contract reviewer findings: a finding with blocking=true MUST have a non-empty material_impact; if a blocking finding lacks material_impact, DOWNGRADE it to advisory (blocking=false) and record a reason (do not silently keep it blocking, and do not silently drop it). Keep findings that omit the new fields parseable, but apply the blocking rule on the contract-review path.

Scope: internal/app/contract_review.go (the contract reviewer finding schema/struct, renderContractReviewerPrompt, and the parse/validation of contract reviewer findings) and its tests (internal/app/contract_review_test.go). Do NOT modify the code-review reviewer (review.go / review_proposals.go), the loop engine, transport, or the contract-fixer revise behavior. Acceptance: the contract reviewer finding carries material_impact/fix_direction/uncertainty/state; renderContractReviewerPrompt states the material-defect-only blocking rule and recall-first framing and requires material_impact for blocking findings; the parser downgrades a blocking finding lacking material_impact to advisory with a recorded reason; a wording/completeness-preference finding cannot remain blocking; tests cover the downgrade path and the prompt rule; make check passes.
- In scope:
  - Update only the contract-review path in internal/app/contract_review.go to carry material_impact, fix_direction, uncertainty, and state on contract reviewer findings.
  - Introduce a contract-local schema constant (emitting pactum.contract_reviewer_result.v1alpha1) and a contract-local parse-input struct (carrying material_impact, fix_direction, uncertainty, and state) in internal/app/contract_review.go, so that the new fields can be parsed without modifying internal/app/review_proposals.go.
  - Update contract-review prompt rendering so reviewer output uses the contract-local schema constant and no longer uses the shared reviewerFindingsSchema from review_proposals.go.
  - Update contract-review parsing and validation so old findings that omit the new fields remain parseable, while blocking findings without non-empty material_impact are downgraded to advisory with a recorded reason.
  - Update internal/app/contract_review_test.go helpers and tests to cover prompt text (including a positive assertion that the rendered output contains pactum.contract_reviewer_result.v1alpha1), new field propagation, material_impact downgrade behavior, and wording/completeness-preference findings staying non-blocking.
- Out of scope:
  - Changing the contract goal.
  - Changing the code-review reviewer path, including internal/app/review.go and internal/app/review_proposals.go.
  - Changing the loop engine, agent transport, contract-fixer revise behavior, or manual finding resolution semantics.
  - Adding a v2 schema, migration path, or backwards-compatibility layer for external contract-review consumers.
  - Running real Pactum agents.
- Acceptance criteria:
  - contractReviewFinding serializes and persists material_impact, fix_direction, uncertainty, and state for contract-review findings where those fields are provided.
  - The contract reviewer prompt sample JSON includes material_impact, fix_direction, uncertainty, and state, and documents state as candidate or confirmed.
  - The contract reviewer prompt states that reviewers should report likely-real defects recall-first, using state=candidate with explicit uncertainty when not confirmed.
  - The contract reviewer prompt states the hard rule that blocking=true is allowed only for material spec defects that would make implementation wrong, ambiguous, or stuck.
  - The contract reviewer prompt states that wording, style, naming, redundancy, and completeness/thoroughness preferences must be blocking=false advisory findings.
  - The contract reviewer prompt requires every blocking finding to include a concrete material_impact and instructs reviewers to mark findings advisory when no material implementation consequence can be stated.
  - The contract-review parser keeps findings that omit the new fields parseable instead of dropping the whole finding.
  - A contract-review finding parsed with blocking=true and blank or omitted material_impact is emitted with blocking=false, and the round warnings record the downgrade reason.
  - A wording or completeness-preference finding emitted as blocking without material_impact does not increment blocking_findings, does not invoke the fixer as a blocker, and does not prevent a clean review round.
  - Existing tests that intentionally exercise real blocking contract-review loop behavior emit material_impact so the fixer, max-rounds, blockers-open, and no-progress paths remain covered.
  - No code-review reviewer prompt, parser, schema, or tests are changed except as indirectly exercised by make check.
  - A contract-local schema constant holding pactum.contract_reviewer_result.v1alpha1 and a contract-local parse-input struct carrying material_impact, fix_direction, uncertainty, and state are introduced in internal/app/contract_review.go; the implementation does not require any changes to internal/app/review_proposals.go.
  - The schema field in renderContractReviewerPrompt output is pactum.contract_reviewer_result.v1alpha1; a test in contract_review_test.go positively asserts the rendered output contains the string pactum.contract_reviewer_result.v1alpha1, providing a discriminating check that the contract-local schema constant is used rather than the shared code-review schema; and the string pactum.reviewer_findings.v1alpha1 does not appear as a string literal anywhere in internal/app/contract_review.go.
  - A contract-review finding parsed with a blank, omitted, or unrecognized state value is stored with state set to candidate.
- Validation commands:
  - go test ./internal/app -run 'TestContractReview'
  - bash -c '! grep -qF "reviewer_findings.v1alpha1" internal/app/contract_review.go'
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
  - command_001: go test ./internal/app -run 'TestContractReview' (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: bash -c '! grep -qF "reviewer_findings.v1alpha1" internal/app/contract_review.go' (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: make check (exit 0, timed out: false, result: gate/validation/command_003/result.json)
- Change summary:
  - changed files:
    - internal/app/contract_review.go
    - internal/app/contract_review_test.go
  - new files:
    - none
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

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
