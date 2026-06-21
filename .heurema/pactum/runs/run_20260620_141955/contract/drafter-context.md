# Contract Drafter Context

## Run
- Run id: run_20260620_141955
- Run status: contract_draft

## Contract goal
Add anti-false-positive fields to the code-review reviewer findings schema and validate them, modifying the existing schema pactum.reviewer_findings.v1alpha1 IN PLACE (deliberate breaking change — the project has no external users yet, so do NOT introduce a v1alpha2 or any dual-version/migration support). New fields on the reviewer findings JSON (parsed by reviewerFindingProposalInput in internal/app/review_proposals.go and carried onto reviewProposalRecord and reviewFindingRecord in internal/app/review.go): state ("candidate"|"confirmed"), trigger (concrete runtime condition or "always"), fix_direction, uncertainty, current_code_only (bool). evidence and confidence already exist and stay. Validation in proposalRecordFromReviewerInput: require non-empty trigger, evidence, fix_direction and a valid state; a blocking finding must have current_code_only=true (a current_code_only=false finding must not be blocking); a finding that cannot fill the required fields is dropped with a loud parse warning, never silently accepted. Update the code-review reviewer prompt renderReviewerPrompt in internal/app/review.go to emit and explain the new fields and reframe guidance to recall-first (report tightly-evidenced candidates, state uncertainty, drop findings that cannot fill the fields tightly). Hard constraint: the reviewer findings parser is shared with contract-review (contract_review.go reuses reviewerFindingsSchema) — this change must not break contract review; keep parsing tolerant or enforce the new required-field rules only on the code-review path. Scope: internal/app/review_proposals.go, internal/app/review.go and their tests. Do NOT implement the critic pass (a separate slice). Acceptance: schema id stays pactum.reviewer_findings.v1alpha1; new fields parsed, carried onto records, validated; reviewer prompt emits/explains them recall-first; contract review still works; tests cover the new validation and the current_code_only-cannot-be-blocking rule; make check passes.

## Current contract fields
- In scope:
  - none
- Out of scope:
  - none
- Acceptance criteria:
  - none
- Validation commands:
  - none
- Assumptions:
  - none

## Answered clarifications
- None

## Repository context
# Repository Context

Generated: 2026-06-20T14:19:55Z

Map run: map_20260620_115144
Repo map path: .heurema/pactum/map/repo-map.md
LLMS path: .heurema/pactum/map/llms.txt
Search index path: .heurema/pactum/map/search.sqlite
Accepted memory context: context/memory-context.md

Notes:
- Pactum has not yet done agentic clarification.
- This is deterministic context assembled from existing map artifacts.

## Project map

Project map is unavailable at .heurema/pactum/map/repo-map.md.

## Search results
{
  "query": "Add anti-false-positive fields to the code-review reviewer findings schema and validate them, modifying the existing schema pactum.reviewer_findings.v1alpha1 IN PLACE (deliberate breaking change — the project has no external users yet, so do NOT introduce a v1alpha2 or any dual-version/migration support). New fields on the reviewer findings JSON (parsed by reviewerFindingProposalInput in internal/app/review_proposals.go and carried onto reviewProposalRecord and reviewFindingRecord in internal/app/review.go): state (\"candidate\"|\"confirmed\"), trigger (concrete runtime condition or \"always\"), fix_direction, uncertainty, current_code_only (bool). evidence and confidence already exist and stay. Validation in proposalRecordFromReviewerInput: require non-empty trigger, evidence, fix_direction and a valid state; a blocking finding must have current_code_only=true (a current_code_only=false finding must not be blocking); a finding that cannot fill the required fields is dropped with a loud parse warning, never silently accepted. Update the code-review reviewer prompt renderReviewerPrompt in internal/app/review.go to emit and explain the new fields and reframe guidance to recall-first (report tightly-evidenced candidates, state uncertainty, drop findings that cannot fill the fields tightly). Hard constraint: the reviewer findings parser is shared with contract-review (contract_review.go reuses reviewerFindingsSchema) — this change must not break contract review; keep parsing tolerant or enforce the new required-field rules only on the code-review path. Scope: internal/app/review_proposals.go, internal/app/review.go and their tests. Do NOT implement the critic pass (a separate slice). Acceptance: schema id stays pactum.reviewer_findings.v1alpha1; new fields parsed, carried onto records, validated; reviewer prompt emits/explains them recall-first; contract review still works; tests cover the new validation and the current_code_only-cannot-be-blocking rule; make check passes.",
  "queries": [
    "dual-version/migration",
    "internal/app/review_proposals.go",
    "internal/app/review.go",
    "contract_review.go",
    "emits/explains",
    "anti-false-positive",
    "code-review",
    "pactum.reviewer_findings.v1alpha1"
  ],
  "query_source": "task",
  "results": [],
  "warnings": [
    "Search index is stale. Run: pactum map refresh."
  ]
}

## Drafter guidance
- Propose only additions to the contract fields listed in the prompt.
- Do not change or restate the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Treat repository map/search context as navigation hints, not semantic truth.
