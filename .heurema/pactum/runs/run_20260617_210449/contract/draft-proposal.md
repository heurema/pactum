# Contract Draft Proposal

## Status
- Run id: run_20260617_210449
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-17T21:08:57Z

## In scope
- Harden `renderReviewerPrompt` so every reviewer lens prompt makes the fenced `pactum.reviewer_findings.v1alpha1` JSON block mandatory, always emitted, and the only parsed reporting channel.
- Include a worked clean reviewer-output example with `"findings": []` and state that prose is supplemental only and ignored by the parser.
- Change reviewer finding block parsing so an explicit empty `findings` array is distinguishable from a missing `findings` key.
- Treat no valid reviewer findings block, malformed block JSON, or a schema block missing `findings` as a structural parse miss with a warning.
- Enforce the valid-block requirement per reviewer lens attempt, including mixed rounds where some lenses emit proposals and another lens emits no valid block.
- Add a bounded per-lens corrective retry for missing or invalid findings blocks before escalating to `reviewer_findings_unparsed`.
- Keep corrective retry accounting below the outer review-loop round accounting so `MaxRounds`, patience, and clean-round logic still count logical review rounds.
- Update focused Go tests and review-loop helper fixtures so clean reviewer output emits an explicit `"findings": []` block.

## Out of scope
- Changing the contract goal.
- Routing or defaulting review to a different reviewer agent or model.
- Parsing prose findings into proposals.
- Changing fixer behavior, gate behavior, or proposal auto-accept semantics beyond preventing silently partial review rounds.
- Running real reviewer or fixer agents as part of validation.

## Acceptance criteria
- Generated reviewer prompts no longer describe structured finding proposals as optional and require exactly one fenced `pactum.reviewer_findings.v1alpha1` block for both finding and no-finding outcomes.
- A reviewer output containing residual-risk prose plus a valid fenced block with `"findings": []` is treated as clean: zero proposals, zero parse warnings, and no corrective retry.
- A prose-only reviewer attempt with no valid findings block triggers a corrective retry; if every retry also lacks a valid block, the review run terminates loudly with `terminal_reason` set to `reviewer_findings_unparsed`.
- A reviewer attempt whose fenced block has the correct schema but omits the `findings` key is a parse miss, not a clean empty review.
- A reviewer attempt with a valid block and one or more valid findings still creates review proposals from those findings.
- If a first reviewer attempt has no valid block and the bounded retry emits a valid block with findings, those findings are captured and the run does not stop as `reviewer_findings_unparsed` because of the first attempt.
- If one lens emits valid findings and another lens persistently emits no valid block, the round is not clean, not approvable, and does not silently proceed as a partial review.
- A clean round is counted only when every successful reviewer lens attempt emits a valid findings block whose `findings` array is empty.
- Warnings for unparsed reviewer output identify the affected reviewer attempt or lens closely enough for an operator to inspect the relevant attempt artifact.
- Existing valid proposal validation behavior for severity, category, file path, blocking, confidence, and evidence remains covered by tests.

## Validation commands
- go test ./internal/app -run Review
- go test ./...
- go build ./...
- make check

## Assumptions
- If same-session ACP follow-up is not available, launching a fresh reviewer attempt with a corrective prompt satisfies the retry requirement.
- A retry cap of one or two attempts is acceptable as long as persistent misses escalate loudly.
- Tests can use existing helper-process reviewers and must not require real external agents.

