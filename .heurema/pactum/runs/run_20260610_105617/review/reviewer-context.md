# Reviewer Context

## Run
- Run id: run_20260610_105617
- Run status: contract_approved

## Contract
- Goal: Harden the built-in reviewer prompt per docs/review-prompt-design.md slice 1. Today renderReviewerPrompt gives boundaries and a findings schema but no review methodology: no confidence bar, no NOT-to-flag list, no lens checklists, no self-verification step. Bake in: (1) the high-signal contract — a finding must be certainly real (certain-or-silent), an explicit NOT-to-flag list, problems-only output; (2) five lens checklists (correctness/quality, implementation-vs-contract, test quality with fake-test detection, over-engineering, documentation gaps); (3) verify-then-report — classify every candidate CONFIRMED vs FALSE POSITIVE before emitting, only CONFIRMED findings are reported; (4) findings-first output ordering with honest empties; (5) pre-existing issues reported as non-blocking advisory; (6) a per-finding confidence field (high|medium|low) on the structured proposal schema, recorded and displayed but not yet gating anything.
- In scope:
  - renderReviewerPrompt (internal/app/review.go): add the methodology sections, condensed from docs/review-prompt-design.md. High-signal contract: report a finding only when certain it is real after verification — 'if you are not certain an issue is real, do not flag it'; an explicit NOT-to-flag list (style/formatting preferences; anything the gate's machine checks already catch — tests, vet, deadcode; input-dependent hypotheticals without a concrete failure path; subjective redesign suggestions); report problems only, no positive observations or praise. Five lenses as compact checklists: correctness (logic errors, edge cases incl. empty/nil/boundary/concurrent, error handling without silent failures, resource cleanup, races); implementation-vs-contract (does the diff achieve the contract goal, every in-scope item and acceptance criterion covered, wiring/integration complete, no missing pieces); test quality (new paths and error paths tested; fake-test detection: always-pass tests, hardcoded-value checks, asserting mock behavior, ignored errors, commented-out cases); over-engineering (wrapper-adds-nothing, factory/abstraction for a single case, premature generalization, unused extension points, dual implementations, silent fallbacks); documentation gaps (user-visible changes missing docs vs internal changes that need none). Verify-then-report: for every candidate finding read the actual code at file:line plus 20-30 surrounding lines, check for existing mitigations and for duplicates among existing findings/proposals, classify CONFIRMED or FALSE POSITIVE, and emit only CONFIRMED. Output ordering: findings first ordered by severity with file/line, then open questions/assumptions, summary last; an empty review must say so explicitly AND name residual risks or testing gaps. Pre-existing issues (present before this change) are reported as non-blocking advisory findings, never blocking.
  - Finding proposal schema: add a confidence field (high|medium|low) to the structured reviewer finding proposals (the pactum.reviewer_findings.v1 block parsing in review_proposals.go) and carry it onto the stored finding records and into the review status/display where findings are rendered. Validation mirrors the clarify precedent after its compatibility fix: a missing/empty confidence defaults to medium; a non-empty value outside high|medium|low skips that proposal with a clear warning. The prompt's example JSON gains the confidence field with the allowed set documented, and instructs that confidence reflects how certain the reviewer is the finding is real after verification. Confidence does NOT gate the fix loop or convergence this slice — recorded and displayed only.
  - Prompt-content tests guarding each new section (high-signal heading, the NOT-to-flag list, each lens heading, verify-then-report, findings-first/honest-empties, pre-existing-advisory, confidence in the example JSON) so they cannot be silently dropped; parsing tests for confidence (default on missing, recorded on valid, skip-with-warning on invalid); display test that a finding's confidence is shown.
  - Docs: docs/agents.md review section notes the hardened reviewer methodology briefly (pointing at review-prompt-design.md); docs/review-prompt-design.md and docs/backlog.md mark slice 1 shipped.
- Out of scope:
  - The executor prompt house style (slice 2); per-panel-member lenses (recorded follow-up); any gating of the fixer, convergence, or severity by confidence; numeric confidence scores (the in-house convention is high|medium|low); changes to the finding fingerprint/dedup, the gate, the review loop mechanics, or the fixer prompt.
- Acceptance criteria:
  - renderReviewerPrompt contains the high-signal contract with the NOT-to-flag list and problems-only rule, the five lens checklists, the verify-then-report CONFIRMED/FALSE-POSITIVE step, findings-first ordering with honest empties, and the pre-existing-as-advisory policy — each guarded by a prompt-content test.
  - Structured finding proposals carry confidence (missing defaults to medium; invalid skips with a warning); stored findings and the review display surface it; nothing is gated by it.
  - go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the design note and backlog mark slice 1 shipped.
- Validation commands:
  - go build ./...
  - go test ./internal/app/...
  - go vet ./...
  - go test -race ./...

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 0
- Fresh: 0
- Stale: 0
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go build ./... (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: go test ./internal/app/... (exit 0, timed out: false, result: gate/validation/command_002/result.json)
  - command_003: go vet ./... (exit 0, timed out: false, result: gate/validation/command_003/result.json)
  - command_004: go test -race ./... (exit 0, timed out: false, result: gate/validation/command_004/result.json)
- Change summary:
  - changed files:
    - docs/agents.md
    - docs/backlog.md
    - docs/review-prompt-design.md
    - internal/app/review.go
    - internal/app/review_proposals.go
    - internal/app/review_test.go
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
