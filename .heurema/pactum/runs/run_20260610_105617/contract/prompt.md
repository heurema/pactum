# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_105617
- Approval: approved
- Contract hash: 4141f766b2754cf0af1fa2e0f029bc5ccddaa32e37b3a7beda61e7706cb0602f

## Goal
Harden the built-in reviewer prompt per docs/review-prompt-design.md slice 1. Today renderReviewerPrompt gives boundaries and a findings schema but no review methodology: no confidence bar, no NOT-to-flag list, no lens checklists, no self-verification step. Bake in: (1) the high-signal contract — a finding must be certainly real (certain-or-silent), an explicit NOT-to-flag list, problems-only output; (2) five lens checklists (correctness/quality, implementation-vs-contract, test quality with fake-test detection, over-engineering, documentation gaps); (3) verify-then-report — classify every candidate CONFIRMED vs FALSE POSITIVE before emitting, only CONFIRMED findings are reported; (4) findings-first output ordering with honest empties; (5) pre-existing issues reported as non-blocking advisory; (6) a per-finding confidence field (high|medium|low) on the structured proposal schema, recorded and displayed but not yet gating anything.

## In scope
- renderReviewerPrompt (internal/app/review.go): add the methodology sections, condensed from docs/review-prompt-design.md. High-signal contract: report a finding only when certain it is real after verification — 'if you are not certain an issue is real, do not flag it'; an explicit NOT-to-flag list (style/formatting preferences; anything the gate's machine checks already catch — tests, vet, deadcode; input-dependent hypotheticals without a concrete failure path; subjective redesign suggestions); report problems only, no positive observations or praise. Five lenses as compact checklists: correctness (logic errors, edge cases incl. empty/nil/boundary/concurrent, error handling without silent failures, resource cleanup, races); implementation-vs-contract (does the diff achieve the contract goal, every in-scope item and acceptance criterion covered, wiring/integration complete, no missing pieces); test quality (new paths and error paths tested; fake-test detection: always-pass tests, hardcoded-value checks, asserting mock behavior, ignored errors, commented-out cases); over-engineering (wrapper-adds-nothing, factory/abstraction for a single case, premature generalization, unused extension points, dual implementations, silent fallbacks); documentation gaps (user-visible changes missing docs vs internal changes that need none). Verify-then-report: for every candidate finding read the actual code at file:line plus 20-30 surrounding lines, check for existing mitigations and for duplicates among existing findings/proposals, classify CONFIRMED or FALSE POSITIVE, and emit only CONFIRMED. Output ordering: findings first ordered by severity with file/line, then open questions/assumptions, summary last; an empty review must say so explicitly AND name residual risks or testing gaps. Pre-existing issues (present before this change) are reported as non-blocking advisory findings, never blocking.
- Finding proposal schema: add a confidence field (high|medium|low) to the structured reviewer finding proposals (the pactum.reviewer_findings.v1 block parsing in review_proposals.go) and carry it onto the stored finding records and into the review status/display where findings are rendered. Validation mirrors the clarify precedent after its compatibility fix: a missing/empty confidence defaults to medium; a non-empty value outside high|medium|low skips that proposal with a clear warning. The prompt's example JSON gains the confidence field with the allowed set documented, and instructs that confidence reflects how certain the reviewer is the finding is real after verification. Confidence does NOT gate the fix loop or convergence this slice — recorded and displayed only.
- Prompt-content tests guarding each new section (high-signal heading, the NOT-to-flag list, each lens heading, verify-then-report, findings-first/honest-empties, pre-existing-advisory, confidence in the example JSON) so they cannot be silently dropped; parsing tests for confidence (default on missing, recorded on valid, skip-with-warning on invalid); display test that a finding's confidence is shown.
- Docs: docs/agents.md review section notes the hardened reviewer methodology briefly (pointing at review-prompt-design.md); docs/review-prompt-design.md and docs/backlog.md mark slice 1 shipped.

## Out of scope
- The executor prompt house style (slice 2); per-panel-member lenses (recorded follow-up); any gating of the fixer, convergence, or severity by confidence; numeric confidence scores (the in-house convention is high|medium|low); changes to the finding fingerprint/dedup, the gate, the review loop mechanics, or the fixer prompt.

## Paths in scope
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- renderReviewerPrompt contains the high-signal contract with the NOT-to-flag list and problems-only rule, the five lens checklists, the verify-then-report CONFIRMED/FALSE-POSITIVE step, findings-first ordering with honest empties, and the pre-existing-as-advisory policy — each guarded by a prompt-content test.
- Structured finding proposals carry confidence (missing defaults to medium; invalid skips with a warning); stored findings and the review display surface it; nothing is gated by it.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; the design note and backlog mark slice 1 shipped.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- The methodology lives in the prompt template (not a config knob), per the earlier decision to drop a custom review-guidelines knob; docs/review-prompt-design.md is the design source and the prompt encodes its condensed form.
- Confidence uses the in-house high|medium|low convention (clarify precedent) with default-to-medium for compatibility, mirroring the kind-field lesson: required new enum fields silently zero out v1-shaped producers.

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 0
- fresh: 0
- stale: 0
- unknown: 0

Items:
- none

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Use `pactum search "<term>"` and inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing code items when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.
