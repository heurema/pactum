# Contract Drafter Context

## Run
- Run id: run_20260621_091056
- Run status: contract_draft

## Contract goal
Add a critic pass (recall→precision) to the code-review loop: after the reviewer panel proposes candidate findings, an independent READ-ONLY critic agent verifies each candidate, and ONLY critic-confirmed candidates are accepted as findings that can block convergence or drive the fixer. This is the precision filter that stops the fixer churning on hallucinated (false-positive) findings — the same thesis as the contract-review precision discipline, but applied to code-review findings via a separate agent pass. It consumes the candidate/confirmed state and the trigger/evidence/fix_direction/uncertainty fields already added to the reviewer findings schema by the anti-FP slice.

FIRST read the current code-review loop: internal/app/review_loop.go (the per-round flow, the accept seam `acceptReviewLoopProposal`, proposal→finding acceptance, terminal-reason mapping), internal/app/review_proposals.go (the reviewer findings schema pactum.reviewer_findings.v1alpha1 and its candidate/confirmed state + anti-FP fields), internal/app/review.go (renderReviewerPrompt, reviewFindingRecord), internal/app/config.go (the agent registry + cross-model reviewer default), and how reviewer attempts run read-only via runAgentAttemptLifecycle. Do not guess.

DESIGN:
1. New internal/app/review_critic.go with the critic round. Schemas: pactum.review_critic_request.v1alpha1, pactum.review_critic_result.v1alpha1, pactum.review_critic_verdicts.v1alpha1. The critic is a READ-ONLY agent attempt (ReadOnly: true) run through runAgentAttemptLifecycle, exactly like the reviewers.
2. Insert the critic between proposal generation and acceptance: after a round's NEW candidate proposals are parsed, run ONE critic pass over those new candidates (evaluate ALL candidates in a single prompt to cap cost), then accept into findings ONLY the candidates the critic CONFIRMED. The critic emits one verdict per proposal_id with verdict ∈ {confirmed, disputed, insufficient_evidence}. Verdicts are sorted by proposal_id for deterministic artifacts; a verdict for an unknown/hallucinated proposal_id is dropped with a warning.
3. The critic prompt (renderReviewCriticPrompt) frames an adversarial precision check: for each candidate, independently re-read the cited code and either confirm with concrete evidence, dispute with counter-evidence, or mark insufficient_evidence naming exactly what is missing. A false positive reaching the result is worse than dropping a marginal candidate; the critic must justify a confirm, never rubber-stamp.
4. Critic model selection: default the critic to a reviewer registry entry on a DIFFERENT engine than the primary reviewer when one is configured (mirror pactum's existing cross-model reviewer-default logic); fall back to the same model WITH a recorded warning when only one engine is available; allow an explicit pipeline.code_review.critic_by registry name override (validated as at most one registered ACP reviewer). The critic is read-only.
5. New loud terminal reasons in the review loop: `precision_rejected` (a round's blocking candidates were all disputed and no open blocking findings remain — approvable); `debate_no_consensus` (a candidate is insufficient_evidence leaving an evidence gap with no confirmed blocker — blocks approval); `critic_verdicts_unparsed` (critic output could not be parsed into verdicts after one corrective re-attempt — blocks approval). A critic failure must NEVER collapse to a clean round. Prior already-open blocking findings still drive the fixer even if all NEW candidates are rejected.
6. Record critic artifacts per round: the critic attempt, the verdicts (pactum.review_critic_verdicts.v1alpha1 JSONL), and round-summary fields (critic_attempt_id, precision_candidates, precision_confirmed, precision_rejected, precision_unresolved, critic_verdicts_artifact). The critic pass must be observable in review/loop-summary.json and the --json review output.

SCOPE: new internal/app/review_critic.go (+ tests); integration edits in internal/app/review_loop.go (accept seam + round flow + terminal reasons), internal/app/review.go / review_proposals.go as needed for the critic prompt/schema, internal/app/config.go for the optional critic_by; and tests. Do NOT change the contract-review path, the fixer's behavior, the transport/loop-engine internals, or the read-only/write boundary. Keep the reviewer panel and the anti-FP schema as-is — the critic consumes the candidate proposals they produce.

ACCEPTANCE: after the reviewer panel proposes candidates, a read-only critic pass runs over the NEW candidates and only critic-confirmed candidates become accepted (blocking-eligible) findings; a disputed blocking candidate does not become an open blocking finding and the round can converge `precision_rejected`; an insufficient_evidence candidate blocks approval as `debate_no_consensus`; unparseable critic output after one corrective re-attempt terminates `critic_verdicts_unparsed` (never a silent clean round); the critic defaults to a different-engine model when available and falls back to same-model with a recorded warning otherwise, overridable via pipeline.code_review.critic_by; prior open blocking findings still drive the fixer when new candidates are all rejected; the critic attempt + verdicts + round-summary precision fields are recorded as artifacts and exposed in the --json review output; make check passes.

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

Generated: 2026-06-21T09:10:56Z

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
  "query": "Add a critic pass (recall→precision) to the code-review loop: after the reviewer panel proposes candidate findings, an independent READ-ONLY critic agent verifies each candidate, and ONLY critic-confirmed candidates are accepted as findings that can block convergence or drive the fixer. This is the precision filter that stops the fixer churning on hallucinated (false-positive) findings — the same thesis as the contract-review precision discipline, but applied to code-review findings via a separate agent pass. It consumes the candidate/confirmed state and the trigger/evidence/fix_direction/uncertainty fields already added to the reviewer findings schema by the anti-FP slice.\n\nFIRST read the current code-review loop: internal/app/review_loop.go (the per-round flow, the accept seam `acceptReviewLoopProposal`, proposal→finding acceptance, terminal-reason mapping), internal/app/review_proposals.go (the reviewer findings schema pactum.reviewer_findings.v1alpha1 and its candidate/confirmed state + anti-FP fields), internal/app/review.go (renderReviewerPrompt, reviewFindingRecord), internal/app/config.go (the agent registry + cross-model reviewer default), and how reviewer attempts run read-only via runAgentAttemptLifecycle. Do not guess.\n\nDESIGN:\n1. New internal/app/review_critic.go with the critic round. Schemas: pactum.review_critic_request.v1alpha1, pactum.review_critic_result.v1alpha1, pactum.review_critic_verdicts.v1alpha1. The critic is a READ-ONLY agent attempt (ReadOnly: true) run through runAgentAttemptLifecycle, exactly like the reviewers.\n2. Insert the critic between proposal generation and acceptance: after a round's NEW candidate proposals are parsed, run ONE critic pass over those new candidates (evaluate ALL candidates in a single prompt to cap cost), then accept into findings ONLY the candidates the critic CONFIRMED. The critic emits one verdict per proposal_id with verdict ∈ {confirmed, disputed, insufficient_evidence}. Verdicts are sorted by proposal_id for deterministic artifacts; a verdict for an unknown/hallucinated proposal_id is dropped with a warning.\n3. The critic prompt (renderReviewCriticPrompt) frames an adversarial precision check: for each candidate, independently re-read the cited code and either confirm with concrete evidence, dispute with counter-evidence, or mark insufficient_evidence naming exactly what is missing. A false positive reaching the result is worse than dropping a marginal candidate; the critic must justify a confirm, never rubber-stamp.\n4. Critic model selection: default the critic to a reviewer registry entry on a DIFFERENT engine than the primary reviewer when one is configured (mirror pactum's existing cross-model reviewer-default logic); fall back to the same model WITH a recorded warning when only one engine is available; allow an explicit pipeline.code_review.critic_by registry name override (validated as at most one registered ACP reviewer). The critic is read-only.\n5. New loud terminal reasons in the review loop: `precision_rejected` (a round's blocking candidates were all disputed and no open blocking findings remain — approvable); `debate_no_consensus` (a candidate is insufficient_evidence leaving an evidence gap with no confirmed blocker — blocks approval); `critic_verdicts_unparsed` (critic output could not be parsed into verdicts after one corrective re-attempt — blocks approval). A critic failure must NEVER collapse to a clean round. Prior already-open blocking findings still drive the fixer even if all NEW candidates are rejected.\n6. Record critic artifacts per round: the critic attempt, the verdicts (pactum.review_critic_verdicts.v1alpha1 JSONL), and round-summary fields (critic_attempt_id, precision_candidates, precision_confirmed, precision_rejected, precision_unresolved, critic_verdicts_artifact). The critic pass must be observable in review/loop-summary.json and the --json review output.\n\nSCOPE: new internal/app/review_critic.go (+ tests); integration edits in internal/app/review_loop.go (accept seam + round flow + terminal reasons), internal/app/review.go / review_proposals.go as needed for the critic prompt/schema, internal/app/config.go for the optional critic_by; and tests. Do NOT change the contract-review path, the fixer's behavior, the transport/loop-engine internals, or the read-only/write boundary. Keep the reviewer panel and the anti-FP schema as-is — the critic consumes the candidate proposals they produce.\n\nACCEPTANCE: after the reviewer panel proposes candidates, a read-only critic pass runs over the NEW candidates and only critic-confirmed candidates become accepted (blocking-eligible) findings; a disputed blocking candidate does not become an open blocking finding and the round can converge `precision_rejected`; an insufficient_evidence candidate blocks approval as `debate_no_consensus`; unparseable critic output after one corrective re-attempt terminates `critic_verdicts_unparsed` (never a silent clean round); the critic defaults to a different-engine model when available and falls back to same-model with a recorded warning otherwise, overridable via pipeline.code_review.critic_by; prior open blocking findings still drive the fixer when new candidates are all rejected; the critic attempt + verdicts + round-summary precision fields are recorded as artifacts and exposed in the --json review output; make check passes.",
  "queries": [
    "candidate/confirmed",
    "trigger/evidence/fix_direction/uncertainty",
    "internal/app/review_loop.go",
    "internal/app/review_proposals.go",
    "internal/app/review.go",
    "internal/app/config.go",
    "internal/app/review_critic.go",
    "unknown/hallucinated"
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
