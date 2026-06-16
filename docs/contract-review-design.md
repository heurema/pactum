# Contract review — design notes

Cross-review of the **contract** before the human approval gate. Today only the
*code* is cross-reviewed (the `review run` panel runs after execution); the
contract itself goes drafter → human with no adversarial check. This closes that
gap. Reference for the contract-review backlog item.

## The gap

```
task → clarify → contract draft (ONE agent) → accept → approve (HUMAN) → … → execute → review run (PANEL on the code)
```

The contract's quality rests on the strong drafter and the single human at
`approve`. A one-agent draft + one-human gate misses what an adversarial panel
catches — and the dogfood proved this live: a drafter wrote a `validation`
command the gate couldn't run, and another wrote an over-broad grep-guard that
conflicted with a legitimate test. A panel reviewing the contract would have
flagged both before approval.

## The design

An **optional** panel reviews the contract between `accept` and `approve`,
reusing the existing review-loop machinery (`internal/app/review_loop.go`):
configured reviewers run contract lenses → structured findings → an optional
fixer applies accepted findings **via the new declarative `contract revise
--from`** → re-review → converge → the human approves a hardened contract.

Off by default: `contract.reviewers: []` (or absent) skips the step, so current
behaviour is unchanged. One name = minimal, several = panel (same path), mirroring
the code-review reviewers.

## Lenses (for the current flat contract)

Different from code lenses; tuned to what makes a contract executable and safe:

- **completeness** — does the contract cover its `goal`? gaps in `scope` or
  `acceptance_criteria`?
- **testability** — is each `acceptance_criteria` entry backed by / expressible
  as a runnable `validation` command (not just prose)?
- **validation-soundness** — are the `validation.commands` actually runnable and
  meaningful: gate-executable (no un-runnable shell forms), non-vacuous (would
  fail on wrong output), and not self-contradictory with the tests? (This lens
  directly targets the two live failures above.)
- **scope-fidelity** — is `scope.in`/`scope.out` coherent, non-contradictory,
  and neither over- nor under-broad for the goal?
- **assumptions-surfaced** — are risky assumptions called out for the human
  rather than buried?

(When the plan DAG lands — see [contract-plan-dag-design.md](contract-plan-dag-design.md) —
dependency-correctness and granularity lenses are added; this is the non-DAG
version of that doc's optional plan-review.)

## Config

```yaml
contract:
  reviewers: [opus]   # [] or absent ⇒ no contract review (human gate only)
```

Registry names, resolved like the code-review panel; the cross-model rule gives a
reviewer a model different from the drafter's. Naming note: this is `reviewers`
(array), consistent with the `review.panel → review.reviewers` rename the
plan-DAG arc proposes for code review.

## Fixer uses declarative revise

When the fixer accepts a finding it edits the contract through `contract revise
--from -` (partial-replace, version-guarded) — the exact primitive built for an
agent operator. So the contract-review fixer is just an agent doing a
read→modify→write on the contract, with the version guard preventing it from
clobbering a concurrent change. No bespoke mutation path.

## First slice (minimal cut)

`contract review <run>` runs the configured `contract.reviewers` panel on the
contract with the lenses above and emits **structured findings** (human-readable
+ `--json`), surfaced before `approve`. Off by default. **No auto-fixer in slice
1** — the human reads the findings and revises (now easy via `revise --from`),
then approves. Reuse the reviewer fan-out + lens machinery from the code review
loop; review the contract document, not a diff.

Deferred to slice 2: the fixer-applies-via-revise convergence loop, and the full
multi-round rounds/patience convergence.

## Honest scope

This adds adversarial review to the *planning* side, where the whole
weak-executor bet concentrates its risk (plan quality is the one thing the
deep-research pass could not prove a weak executor recovers from). It does not
make the executor smarter — it manufactures contract quality so the executor is
handed a better-specified, gate-runnable, non-contradictory contract.
