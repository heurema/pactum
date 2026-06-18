# Contract Drafter Prompt

This prompt is prepared for a read-only contract drafter agent subprocess.
Pactum will parse the structured proposal into a pending draft proposal; it will not apply it until a human runs the accept command.

## Objective
Propose missing contract scope, acceptance, validation, and assumption entries from the contract goal, answered clarifications, and repository context.

## Inputs
- Drafter context: .heurema/pactum/runs/run_20260618_054400/contract/drafter-context.md
- Contract draft: .heurema/pactum/runs/run_20260618_054400/contract/contract.json
- Clarification answers: .heurema/pactum/runs/run_20260618_054400/clarify/answers.jsonl

## Boundaries
- Do not change the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Do not run commands that write to the repository.
- Propose additions only; Pactum will append accepted entries through contract revision.
- Use concrete, observable acceptance criteria and runnable validation commands.

## Structured proposal
Include a fenced JSON block exactly like:

```json
{
  "schema": "pactum.contract_draft_proposal.v1alpha1",
  "in_scope": ["Specific work to include."],
  "out_of_scope": ["Specific work to exclude."],
  "acceptance": ["Observable completion criterion."],
  "validation": ["command to run"],
  "assumptions": ["Assumption the human should review."]
}
```

Use empty arrays for fields that need no additions.
