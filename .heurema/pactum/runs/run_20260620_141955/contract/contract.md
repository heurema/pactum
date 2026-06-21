# Contract Draft

## Goal
Add anti-false-positive fields to the code-review reviewer findings schema and validate them, modifying the existing schema pactum.reviewer_findings.v1alpha1 IN PLACE (deliberate breaking change — the project has no external users yet, so do NOT introduce a v1alpha2 or any dual-version/migration support). New fields on the reviewer findings JSON (parsed by reviewerFindingProposalInput in internal/app/review_proposals.go and carried onto reviewProposalRecord and reviewFindingRecord in internal/app/review.go): state ("candidate"|"confirmed"), trigger (concrete runtime condition or "always"), fix_direction, uncertainty, current_code_only (bool). evidence and confidence already exist and stay. Validation in proposalRecordFromReviewerInput: require non-empty trigger, evidence, fix_direction and a valid state; a blocking finding must have current_code_only=true (a current_code_only=false finding must not be blocking); a finding that cannot fill the required fields is dropped with a loud parse warning, never silently accepted. Update the code-review reviewer prompt renderReviewerPrompt in internal/app/review.go to emit and explain the new fields and reframe guidance to recall-first (report tightly-evidenced candidates, state uncertainty, drop findings that cannot fill the fields tightly). Hard constraint: the reviewer findings parser is shared with contract-review (contract_review.go reuses reviewerFindingsSchema) — this change must not break contract review; keep parsing tolerant or enforce the new required-field rules only on the code-review path. Scope: internal/app/review_proposals.go, internal/app/review.go and their tests. Do NOT implement the critic pass (a separate slice). Acceptance: schema id stays pactum.reviewer_findings.v1alpha1; new fields parsed, carried onto records, validated; reviewer prompt emits/explains them recall-first; contract review still works; tests cover the new validation and the current_code_only-cannot-be-blocking rule; make check passes.

## Current status
Contract status: approved
Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.

## Relevant repository context
- Map run: map_20260620_115144
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json (0 result(s))

## Clarifications
- None

## In scope
- Modify the existing pactum.reviewer_findings.v1alpha1 code-review reviewer finding input, proposal records, and accepted finding records to parse and carry state, trigger, fix_direction, uncertainty, and current_code_only.
- Add code-review proposal validation in proposalRecordFromReviewerInput for valid state, non-empty trigger, non-empty evidence, non-empty fix_direction, and the rule that persisted blocking findings must have current_code_only=true.
- Update renderReviewerPrompt so the required JSON example and rules explain the new fields, trigger semantics, current-code-only blocking rule, and recall-first candidate reporting with explicit uncertainty.
- Add or update internal/app tests covering code-review parsing, validation warnings, record persistence, prompt output, proposal acceptance carry-through, and contract-review compatibility with the shared reviewer findings schema.

## Out of scope
- Changing the contract goal or answering clarification questions.
- Introducing pactum.reviewer_findings.v1alpha2, dual-version parsing, migration support, or a second schema ID.
- Implementing a critic pass, critic findings slice, or any separate reviewer output stream.
- Broad refactors or production behavior changes outside internal/app/review_proposals.go and internal/app/review.go except minimal compatibility work required to keep contract review passing.

## Acceptance criteria
- The reviewer findings schema identifier remains exactly pactum.reviewer_findings.v1alpha1 everywhere it is emitted or parsed.
- A code-review reviewer finding with valid state, trigger, fix_direction, uncertainty, current_code_only, evidence, and confidence is collected into a review proposal preserving those fields.
- Accepting a valid proposal creates a review finding that preserves the new anti-false-positive fields and existing confidence behavior while keeping existing evidence handling unchanged.
- Code-review findings missing trigger, evidence, or fix_direction, or using a state other than candidate or confirmed, are dropped with field-specific parse warnings and are not written to proposals.jsonl.
- A reviewer output item with current_code_only=false and blocking=true cannot produce a persisted blocking proposal or finding, and collection emits a warning for that invalid combination.
- Contract review still parses an otherwise valid pactum.reviewer_findings.v1alpha1 block that omits the new anti-false-positive fields; missing new fields do not by themselves break contract-review collection.
- The reviewer prompt includes all new fields in the JSON example and rules, explains trigger as a concrete runtime condition or always, and no longer instructs reviewers to discard every uncertain candidate.
- Tests cover the new validation rules, the current_code_only blocking constraint, prompt rendering, and shared-parser contract-review compatibility.
- make check passes.

## Validation commands
- go test ./internal/app
- make check

## Assumptions
- The breaking change applies to code-review reviewer output only; contract-review parsing remains tolerant because it shares reviewerFindingsSchema.
- state is the only new enum and is limited to candidate or confirmed.
- uncertainty is parsed and preserved as a free-form field unless a later clarification adds an enum or non-empty validation rule.
- current_code_only=false represents a pre-existing or non-current-code issue and therefore must never be blocking.

## Open questions
- None
