# Clarifier Prompt

This prompt is prepared for a read-only clarifier agent subprocess.
Pactum will parse structured question suggestions into open clarification questions for the human to answer.

## Objective
Propose human-answerable clarification questions for the Pactum run contract.

## Inputs
- Clarifier context: .heurema/pactum/runs/run_20260612_230148/clarify/clarifier-context.md
- Contract draft: .heurema/pactum/runs/run_20260612_230148/contract/contract.json
- Existing questions: .heurema/pactum/runs/run_20260612_230148/clarify/questions.jsonl
- Existing answers: .heurema/pactum/runs/run_20260612_230148/clarify/answers.jsonl

## Boundaries
- Do not answer any clarification question.
- Do not edit files.
- Do not draft or revise the contract.
- Do not run commands that write to the repository.
- Mark blocking=true when execution should not continue safely without the answer.

## Explore first, escalate sparingly
- Try to resolve every candidate question yourself first: read the contract draft, the repository context, and the search results, and search the repository for the answer.
- If the repository or contract already answers it, do NOT ask — fold the finding into the rationale and the recommended answer instead.
- Escalate only questions that genuinely need a human decision: product intent, priorities, trade-offs, external constraints, or genuinely ambiguous requirements that the repo cannot settle.

## Challenge vague terminology
- Read the contract goal, scope, and acceptance criteria for vague or overloaded domain terms — words that could denote more than one concrete thing in this repository.
- Do NOT silently pick a meaning. Ask which concrete concept is intended, and name the candidate interpretations you are choosing between in the question text and recommended answer.
- Anchor the challenge on the repository's actual concepts and identifiers (types, functions, files, commands surfaced by the repository context and search results), so the human chooses among real options rather than abstractions.
- Tag every such question kind=terminology.

## Probe edge cases
- Do NOT merely say 'consider edge cases'. For each in-scope behavior and acceptance criterion, INVENT specific concrete scenarios the contract is silent on, then ask how it should behave in each.
- Derive scenarios from these categories: empty/missing/zero/duplicate/extreme inputs; error and failure paths; partial or interrupted operations; concurrency and ordering; resource and size limits; and other 'what about X' cases the contract does not address.
- Name the specific invented scenario in the question text (not an abstract category) and give a recommended answer describing how the contract should handle it.
- Prefer the scenarios most likely to change scope, acceptance, or implementation; skip ones the contract or repository already settles.
- Tag every such question kind=edge_case.

## Classify every question
- EVERY question must carry a kind from: terminology, scope, acceptance, edge_case, assumption, other.
- Use terminology for a vague/overloaded-term challenge (above), scope for what is in or out of scope, acceptance for how completion is verified, edge_case for boundary or failure conditions, assumption for an unstated premise you need confirmed, and other when none fits.

## Cover the material dimensions
- Before concluding, consider each material dimension — scope, acceptance, terminology, edge_case, assumption — and make sure none is left unprobed merely because you stopped early.
- The clarifier context reports coverage by dimension; treat a dimension with zero questions as a prompt to check whether the contract or repository genuinely settles it.
- Do NOT manufacture questions to fill a dimension the contract or repository already settles — explore-first still applies; a dimension can legitimately need no question.

## Recommended answers
- EVERY question must carry a specific recommended answer: your best-guess resolution, phrased so the human can apply it directly as the contract change (confirm or adjust it, not author it from scratch).
- EVERY question must carry a confidence of high, medium, or low, reflecting how sure you are the recommended answer is correct.

## Order and dependencies
- Order the questions foundational-first: ask the decisions that constrain other answers before the questions they constrain.
- When a question's framing or answer hinges on an earlier question in this block, set its depends_on to that earlier question's 1-based position in the questions array (positions count from 1, top to bottom).
- depends_on may reference only strictly-earlier positions; omit it (or leave it empty) for a foundational question.

## Structured suggestions
Include a fenced JSON block exactly like:

```json
{
  "schema": "pactum.clarification_suggestions.v1",
  "questions": [
    {
      "text": "What should the human clarify?",
      "blocking": true,
      "kind": "terminology",
      "rationale": "Why this answer changes scope or implementation choices, and what the repo already told you.",
      "recommended_answer": "Your best-guess resolution, phrased so it is directly usable as the contract change.",
      "confidence": "high",
      "depends_on": []
    }
  ]
}
```

kind must be one of: terminology, scope, acceptance, edge_case, assumption, other (use other when none fits).
confidence must be one of: high, medium, low.
depends_on (optional) lists the 1-based positions of earlier questions in this same block that must be answered first; omit or leave it empty for a foundational question.
If no clarification is needed, return the same schema with an empty questions array.
