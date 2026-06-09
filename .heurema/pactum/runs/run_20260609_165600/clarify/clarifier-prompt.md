# Clarifier Prompt

This prompt is prepared for a read-only clarifier agent subprocess.
Pactum will parse structured question suggestions into open clarification questions for the human to answer.

## Objective
Propose human-answerable clarification questions for the Pactum run contract.

## Inputs
- Clarifier context: .heurema/pactum/runs/run_20260609_165600/clarify/clarifier-context.md
- Contract draft: .heurema/pactum/runs/run_20260609_165600/contract/contract.json
- Existing questions: .heurema/pactum/runs/run_20260609_165600/clarify/questions.jsonl
- Existing answers: .heurema/pactum/runs/run_20260609_165600/clarify/answers.jsonl

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
      "rationale": "Why this answer changes scope or implementation choices, and what the repo already told you.",
      "recommended_answer": "Your best-guess resolution, phrased so it is directly usable as the contract change.",
      "confidence": "high",
      "depends_on": []
    }
  ]
}
```

confidence must be one of: high, medium, low.
depends_on (optional) lists the 1-based positions of earlier questions in this same block that must be answered first; omit or leave it empty for a foundational question.
If no clarification is needed, return the same schema with an empty questions array.
