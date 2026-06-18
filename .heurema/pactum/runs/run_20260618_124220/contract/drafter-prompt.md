# Contract Drafter Prompt

This prompt is prepared for a read-only contract drafter agent subprocess.
Pactum will parse the structured proposal into a pending draft proposal; it will not apply it until a human runs the accept command.

## Objective
Propose missing contract scope, acceptance, validation, and assumption entries from the contract goal, answered clarifications, and repository context.

## Inputs
- Drafter context: .heurema/pactum/runs/run_20260618_124220/contract/drafter-context.md
- Contract draft: .heurema/pactum/runs/run_20260618_124220/contract/contract.json
- Clarification answers: .heurema/pactum/runs/run_20260618_124220/clarify/answers.jsonl

## Boundaries
- Do not change the contract goal.
- Do not answer clarification questions.
- Do not edit files.
- Do not run commands that write to the repository.
- Propose additions only; Pactum will append accepted entries through contract revision.
- Use concrete, observable acceptance criteria and runnable validation commands.

## Optional plan
Include plan.tasks[] only when the work has real intra-contract fan-in (a task with more
than one dependency) or independently-validatable surfaces. Target 3-10 leaf tasks.
A leaf task is one independently reviewable patch, not one edit.
Each task requires one falsifiable validation referencing its expected_files.
Linear or simple work: omit plan entirely (no "plan" field in the JSON block).

Task fields:
- id (required, unique within plan.tasks)
- title (optional short label)
- depends_on (ids of other tasks this one depends on)
- context (evidence selectors: each entry has path with optional lines and/or symbol, plus why)
- expected_files (advisory; paths in paths_in_scope)
- acceptance (required, non-empty; observable completion criterion)
- validation (required, non-empty; falsifiable command referencing expected_files)

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

Use empty arrays for fields that need no additions. Omit "plan" entirely for linear work.
