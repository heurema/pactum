# Contract Drafter Context

## Run
- Run id: run_20260620_195608
- Run status: contract_draft

## Contract goal
Apply precision discipline to the CONTRACT reviewer (the panel that reviews the contract document before approval), mirroring the anti-false-positive discipline code-review got, but adapted for a spec document. The contract reviewer is a SEPARATE mechanism from code review: it uses pactum.contract_reviewer_result.v1alpha1 (NOT the code-review reviewer_findings schema), its own lenses (scope-fidelity, completeness, testability, validation-soundness, assumptions-surfaced), and renderContractReviewerPrompt in internal/app/contract_review.go. Modify that schema/prompt/parser IN PLACE (deliberate breaking change — no external users yet; no v2 or migration). Do NOT touch the code-review reviewer.

Problem this fixes: contract review currently grinds to blockers_open / max_rounds on low-signal WORDING nitpicks about the contract document (e.g. "acceptance criteria are prose-only", "term X is not defined", "criterion wording could be tighter"), because nothing forces the reviewer to distinguish a material spec defect from a style/completeness preference, and any nit can be marked blocking.

Changes:
1. Add fields to the contract reviewer finding (the struct parsed from the panel output and persisted): material_impact (string — for a blocking finding, the concrete way this spec defect would make the IMPLEMENTATION wrong, ambiguous, or stuck), fix_direction (string), uncertainty (string), and state ("candidate" or "confirmed"). Keep existing message/evidence/severity/category/blocking; evidence already exists.
2. Update renderContractReviewerPrompt to: (a) frame reviewers recall-first but precision-gated — report likely-real defects as state=candidate with explicit uncertainty; (b) state the HARD RULE: a finding may set blocking=true ONLY if it is a material spec defect that would make the implementation wrong, ambiguous, or stuck — wording, style, naming, redundancy, and completeness/thoroughness PREFERENCES must be blocking=false (advisory); (c) require every blocking finding to fill material_impact concretely; (d) instruct reviewers to mark advisory (not blocking) any finding they cannot tie to a material implementation consequence.
3. Enforce in the parser/validation of contract reviewer findings: a finding with blocking=true MUST have a non-empty material_impact; if a blocking finding lacks material_impact, DOWNGRADE it to advisory (blocking=false) and record a reason (do not silently keep it blocking, and do not silently drop it). Keep findings that omit the new fields parseable, but apply the blocking rule on the contract-review path.

Scope: internal/app/contract_review.go (the contract reviewer finding schema/struct, renderContractReviewerPrompt, and the parse/validation of contract reviewer findings) and its tests (internal/app/contract_review_test.go). Do NOT modify the code-review reviewer (review.go / review_proposals.go), the loop engine, transport, or the contract-fixer revise behavior. Acceptance: the contract reviewer finding carries material_impact/fix_direction/uncertainty/state; renderContractReviewerPrompt states the material-defect-only blocking rule and recall-first framing and requires material_impact for blocking findings; the parser downgrades a blocking finding lacking material_impact to advisory with a recorded reason; a wording/completeness-preference finding cannot remain blocking; tests cover the downgrade path and the prompt rule; make check passes.

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

Generated: 2026-06-20T19:56:08Z

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
  "query": "Apply precision discipline to the CONTRACT reviewer (the panel that reviews the contract document before approval), mirroring the anti-false-positive discipline code-review got, but adapted for a spec document. The contract reviewer is a SEPARATE mechanism from code review: it uses pactum.contract_reviewer_result.v1alpha1 (NOT the code-review reviewer_findings schema), its own lenses (scope-fidelity, completeness, testability, validation-soundness, assumptions-surfaced), and renderContractReviewerPrompt in internal/app/contract_review.go. Modify that schema/prompt/parser IN PLACE (deliberate breaking change — no external users yet; no v2 or migration). Do NOT touch the code-review reviewer.\n\nProblem this fixes: contract review currently grinds to blockers_open / max_rounds on low-signal WORDING nitpicks about the contract document (e.g. \"acceptance criteria are prose-only\", \"term X is not defined\", \"criterion wording could be tighter\"), because nothing forces the reviewer to distinguish a material spec defect from a style/completeness preference, and any nit can be marked blocking.\n\nChanges:\n1. Add fields to the contract reviewer finding (the struct parsed from the panel output and persisted): material_impact (string — for a blocking finding, the concrete way this spec defect would make the IMPLEMENTATION wrong, ambiguous, or stuck), fix_direction (string), uncertainty (string), and state (\"candidate\" or \"confirmed\"). Keep existing message/evidence/severity/category/blocking; evidence already exists.\n2. Update renderContractReviewerPrompt to: (a) frame reviewers recall-first but precision-gated — report likely-real defects as state=candidate with explicit uncertainty; (b) state the HARD RULE: a finding may set blocking=true ONLY if it is a material spec defect that would make the implementation wrong, ambiguous, or stuck — wording, style, naming, redundancy, and completeness/thoroughness PREFERENCES must be blocking=false (advisory); (c) require every blocking finding to fill material_impact concretely; (d) instruct reviewers to mark advisory (not blocking) any finding they cannot tie to a material implementation consequence.\n3. Enforce in the parser/validation of contract reviewer findings: a finding with blocking=true MUST have a non-empty material_impact; if a blocking finding lacks material_impact, DOWNGRADE it to advisory (blocking=false) and record a reason (do not silently keep it blocking, and do not silently drop it). Keep findings that omit the new fields parseable, but apply the blocking rule on the contract-review path.\n\nScope: internal/app/contract_review.go (the contract reviewer finding schema/struct, renderContractReviewerPrompt, and the parse/validation of contract reviewer findings) and its tests (internal/app/contract_review_test.go). Do NOT modify the code-review reviewer (review.go / review_proposals.go), the loop engine, transport, or the contract-fixer revise behavior. Acceptance: the contract reviewer finding carries material_impact/fix_direction/uncertainty/state; renderContractReviewerPrompt states the material-defect-only blocking rule and recall-first framing and requires material_impact for blocking findings; the parser downgrades a blocking finding lacking material_impact to advisory with a recorded reason; a wording/completeness-preference finding cannot remain blocking; tests cover the downgrade path and the prompt rule; make check passes.",
  "queries": [
    "internal/app/contract_review.go",
    "schema/prompt/parser",
    "/",
    "e.g",
    "style/completeness",
    "message/evidence/severity/category/blocking",
    "completeness/thoroughness",
    "parser/validation"
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
