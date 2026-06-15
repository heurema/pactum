# Contract plan DAG — design notes

Design input for making a pactum contract executable by a *weak* executor (a
cheaper model such as Sonnet rather than a frontier model) end to end, with the
two human gates unchanged. Sources: a deep-research pass over agent-planning
literature (planning surveys, ADaPT, Plan-and-Act, Divide-or-Conquer) and the
public practice of spec-driven and single-task loop runners. Ideas are absorbed
without attribution; this document is the reference for the contract-plan-DAG
backlog item.

## The problem

The contract today (`pactum.contract.v1`, `internal/app/run.go` `draftContract`)
is a flat declarative spec: a single `goal`, prose `scope.in/out`, optional
`paths_in_scope/out` globs, prose `acceptance_criteria`, post-hoc
`validation.commands`, and `assumptions`. `execute run` builds one prompt from
these fields (`renderApprovedPromptMD`, `internal/app/prompt.go`) and the
executor runs the whole task in a single shot. It must, unaided:

- **decompose** the goal into an ordered set of changes,
- **hold the whole plan in its head** across a long edit session, and
- **decide for itself when each part is done** (the only machine check is the
  final `validation.commands` gate, run once after execution).

A frontier model absorbs all three. A weak executor does not: the costly
operations for it are exactly self-decomposition and not-forgetting across a
long horizon. The contract gives it no scaffold for either.

## What the research established (and what it did not)

The honest version matters here, because the loud claims did not survive
adversarial verification.

### Confirmed
- **The loop, not the model, should pick the next unit of work.** Ordered,
  dependency-aware task lists with per-task file targets are the common shape
  across spec-driven tooling; they cut self-decomposed ordering and
  hallucinated paths.
- **Externalize state.** Long task sequences exceed the context window and the
  model forgets its own trajectory; persisting progress to disk and running one
  unit per fresh context is what keeps a weak executor on track. Plan-and-act
  scaffolds beat ReAct and plan-and-execute on the cited (non-code) benchmarks,
  and the right granularity tracks executor strength.

### Refuted / unproven
- **"A good enough plan lets a weak model do frontier work" is an open
  problem, not an established result** (refuted 0-3). The headline
  architect/editor win (a strong planner handing a weak editor a plan and
  beating the solo strong model) did not survive verification either (0-3).
- Per-step self-evaluation (reflection) measurably helping was refuted (1-2).
- All surviving *empirical* evidence is math-QA and web navigation, **not
  software engineering**; "weak" in those papers meant an un-finetuned 70B
  model, not a small/cheap one. Applying it to multi-file code is extrapolation.

### The load-bearing conclusion
**A plan substitutes for planning, not for solving.** Code resists compression:
a distilled *decomposer* can match its teacher, a distilled *solver* collapses.
Structure therefore helps a weak executor with **coordination and
not-forgetting**, and not with the act of solving a genuinely hard change. The
payoff of a better contract is (a) fewer failures *caused by the plan* and (b)
earlier, cleaner escalation on failures *caused by solving difficulty* — not a
weak model magically clearing a frontier bar. This bounds the whole design:
decompose finely enough that each unit's *code change* is routine for the
target executor, and keep a declarative definition of done to recover against.

## The model: contract = constitution + plan

Keep two layers in the one contract:

- **Constitution** (declarative, as today): `goal`, `scope`, `acceptance_criteria`,
  `validation.commands`. This is the definition of done and the final gate. It
  is the **recovery anchor**: if a planned task turns out wrong, what counts as
  success is still defined here, not by the plan.
- **Plan**: a DAG of self-contained tasks — the *strategy* that serves the
  constitution.

Do not collapse the contract into "just a plan." The constitution outliving any
individual task is what lets the system recover when a task is mis-specified.

## The self-contained task

A task is self-contained when the executor can complete it from a **fresh
context** given only: the constitution header, the task's own spec, and the
results of its completed dependencies (which are already in the working tree as
commits). Tasks communicate through **git and the ledger, not through context** —
each task starts cold, reads repository state (including prior tasks' commits),
and does its one unit. That is what makes "fresh context per task" real: the
memory lives outside the model.

Each task therefore carries:

- a local `title` (its unit of work),
- `depends_on[]` — the DAG edges,
- `acceptance[]` — its own observable done-criteria,
- `validation[]` — its own pass/fail command(s); the spine of self-checking,
- `context[]` — a retrieval scope (paths / `sym:` symbols) so the loop *packs
  context for the executor* instead of the executor hunting for it,
- `expected_files[]` — **advisory** expected touch points, not frozen truth.

The advisory/verifiable split is deliberate: precision goes into *verifiable*
fields (`acceptance`, `validation`), not *prescriptive* ones. Freezing exact
edits invites the executor to satisfy stale instructions instead of the goal.

## Schema (still v1 — evolve in place)

pactum has no users, so there is no version bump and no backward-compat path:
`pactum.contract.v1` gains a `plan` object; the drafter and prompt builder are
extended, not duplicated.

```jsonc
{
  "schema": "pactum.contract.v1",
  "goal": "...",
  "scope": { "in": ["..."], "out": ["..."] },
  "acceptance_criteria": ["..."],          // constitution: definition of done
  "validation": { "commands": ["go test ./..."] }, // constitution: final gate
  "assumptions": ["..."],
  "plan": {                                 // NEW — the DAG; inside the hash
    "tasks": [
      {
        "id": "t1",
        "title": "...",
        "depends_on": [],
        "context": ["internal/app/run.go", "sym:draftContract"],
        "expected_files": ["internal/app/run.go"],   // advisory
        "acceptance": ["..."],
        "validation": ["go test ./internal/app/ -run X"]
      },
      { "id": "t2", "title": "...", "depends_on": ["t1"], "...": "..." }
    ]
  }
}
```

Execution state is a **separate, unhashed** artifact (`execute/tasks-state.json`),
never part of the contract:

```jsonc
{
  "t1": { "status": "done",    "attempts": 1, "commit": "abc1234",
          "files_touched": ["internal/app/run.go"] },
  "t2": { "status": "blocked", "attempts": 3, "blocker": "...",
          "proposed": "split t2 into t2a/t2b" }
}
```

## Execution as a topological loop

`execute run` changes from single-shot to a loop:

```
loop:
  ready = tasks where every depends_on is done and status != done
  if ready is empty: break
  pick = first ready task (deterministic topological order)
  fresh context = constitution + pick spec + (repo state via git)
  run executor on pick
  run pick.validation
    pass  -> status=done, commit (code + state), record files_touched
    fail  -> attempts++; retry up to N; on exhaustion status=blocked (⚠️)
after loop:
  run constitution validation.commands  (unchanged final gate)
  hand off to gate / review (unchanged)
```

The loop — not the model — selects the next task, by readiness. Independent
branches are isolated: a blocked task only blocks its descendants.

## State split, gates, and freeze-vs-replan

- **Structure is frozen, state is external.** The DAG lives inside the hashed
  contract (so the two gates and the approval hash are untouched); execution
  state lives in the unhashed ledger. This is stricter and more auditable than
  mutating a plan file in place: the approved structure never changes under the
  human's feet.
- **Two gates preserved.** Human approves the contract (constitution + DAG)
  once; the loop runs unattended; human accepts the final result. No human in
  the middle.
- **Freeze-vs-replan, minimal form.** When a task wedges, the executor does
  **not** edit the frozen DAG. It writes `status=blocked`, a `blocker`, and a
  *proposed* amendment into the ledger, and that branch stops. The human
  decides. The DAG localizes the blast radius, so a wrong task does not
  invalidate the run — only its subtree waits.

## Failure policy and granularity

- **Bounded retries, then escalate** (do not subdivide forever): a task failing
  validation N times becomes `blocked` and is surfaced; escalation options are
  a stronger model *for that task only* or a human/amendment. This is the
  practical answer to "solving doesn't compress" — when a unit is intrinsically
  hard, fail it cleanly and early instead of looping.
- **Granularity is checkable.** If the drafter cannot write a single
  `validation` command for a task, the task is too large — split it. That is the
  concrete proxy for "sized to the executor's solving reach," with no
  auto-expansion machinery in the first cut.

## Optional plan review (agent-referenced)

The whole weak-executor bet rests on plan quality, so the seam to point a model
at the plan is built into the core from the start — **optional, off by default**,
and resolved through the existing agent registry rather than a bespoke role.

- **Where.** Between the drafter emitting `plan.tasks` and the human approval
  gate. The drafter is already a reviewer-role agent (`prepareContractDrafter`
  → `resolveReviewerEntry`, `internal/app/contract_draft.go`); plan review
  reuses the same resolution path. Reviewers are named by registry entry under a
  `plan.reviewers` array, and each entry's model and effort pins travel with the
  name; the cross-model rule gives a reviewer a model different from the
  drafter's.
- **What it does.** Each reviewer in the array reads the constitution + the DAG
  and returns structured findings; an optional fixer pass folds accepted
  findings back into the draft *before* it is hashed and approved. An empty (or
  absent) `reviewers` list skips the step and leaves the human gate as the only
  plan check — i.e. Phase 0/1 behaviour is unchanged. **One reviewer is the
  minimal form, several is the panel — the same code path, list length is the
  only difference**, so there is no separate "add a panel" milestone.
- **Lenses (multi-reviewer form).** A plan needs different lenses than code:
  completeness (does the DAG cover the goal and every acceptance criterion),
  dependency-correctness (no cycles, no missing edges), testability (is each
  `acceptance` expressible as a runnable `validation`), granularity (is each
  task within the executor's solving reach), scope-fidelity (does the DAG stay
  inside `scope.in` and clear of `scope.out`).
- **Why optional now.** It is the highest-leverage enrichment but not needed to
  validate the core loop, and Phase 0 must first show whether our weak-executor
  failures are plan-caused at all. An empty `reviewers` list keeps the cheap
  path open at zero cost.

## Config surface

Three additions to `pactum.config.v1`, all referencing agents from the existing
`agents:` registry by name:

```yaml
execute:
  executor: sonnet          # who runs the loop; weak executor under test
  max_attempts: 3           # retry-then-block threshold per task

plan:
  reviewers: [opus]         # [] or absent ⇒ no plan review (human gate only)

review:                     # code review — existing block, panel → reviewers
  max_rounds: 10
  patience: 2
  clean_rounds: 1
  reviewers: [fable, codex] # was: panel
```

One vocabulary for both stages: `reviewers` is an array of registry names, and
an empty array means nobody reviews. `panel` is renamed to `reviewers` (no users
→ rename in place, no compat path) so plan review and code review read the same.
The shared name covers only the *participant list*; the loop knobs differ —
`max_rounds`/`patience`/`clean_rounds` are code-review's multi-round convergence
and do not apply to the single-pass plan review.

`execute.executor` pins the loop's agent by name; drafter and fixer stay on
their own (strong) resolution, so a weak executor is not silently made the fixer
too. Deliberate semantic to keep in mind: `review.reviewers: []` disables
*automated code review* — only the `gate` (build/tests) and the human acceptance
gate remain. For plan review an empty list is harmless (the human still sees the
plan); for code review it removes a quality barrier, so it is a choice, not a
default to reach for.

## Deliberately deferred (seams left open)

To resist turning a contract runner into a workflow engine, the first cut is the
minimum: a sequential topological loop, structured task nodes, an external
ledger, retry-then-block, and the optional single-agent plan-review hook above.
Explicitly out of scope, with the seam noted:

- **Multi-round convergence for plan review.** The in-core `reviewers` array
  runs a single pass (findings + optional fixer); the multi-round
  rounds/patience/clean-rounds loop that code review uses is the later
  enrichment for plans. (Adding a second reviewer to the array is *not*
  deferred — it is just a longer list.)
- **Formal plan amendments** with their own hash and re-approval (beyond the
  ledger's proposed-amendment note).
- **Parallel execution** of independent DAG branches (pactum already spawns
  agent subprocesses; the DAG makes this safe later).
- **Per-slice review** of each landed diff instead of one final diff.
- **Auto-expansion** of tasks by estimated complexity.

## Rollout

- **Phase 0 — measure.** Run a weak executor (Sonnet) on real pactum contracts
  and classify failures: plan-caused (decomposition/ordering/missing context →
  the DAG + ledger pay off) vs solving-caused (the unit is just hard → finer
  `task new` or a stronger model, which the DAG does not fix). This is the
  cheapest, highest-information step and settles coordination-vs-solving on our
  own tasks rather than on extrapolated benchmarks.
- **Phase 1 — build the loop + optional plan-review hook.** Drafter emits
  `plan.tasks`; `execute run` becomes the topological loop; add the unhashed
  `tasks-state.json` ledger and retry-then-block. Wire the optional,
  registry-resolved plan-review step (off by default, single agent + findings +
  optional fixer) before the human gate. Dogfood it with Sonnet as executor —
  the build is the measurement vehicle.
- **Phase 2 — convergence, amendments, scheduling.** Add multi-round
  rounds/patience convergence to plan review (a second reviewer is just a longer
  `plan.reviewers` array, not part of this); add the formal plan-amendment
  artifact + bounded re-plan; later, parallel branches and per-slice review.

## Honest risks

- **The real work is `execute`-as-a-loop.** The drafter extension and the
  ledger are light; turning single-shot execution into a stateful topological
  loop (fresh context per task, per-task validation, commit-per-task) is the
  weight of the change.
- **Workflow-engine creep.** Dependency scheduling, amendments, blockers, and
  per-step gates are how a simple runner becomes a complex orchestrator. The
  deferral list above is the guardrail; each later phase must earn its place
  against a measured failure it removes.
- **The bet is unproven for code.** Every confirmed result is non-software, and
  the strongest pro-structure claims were refuted. Phase 0 exists so we commit
  to the loop only after seeing that our weak-executor failures are actually the
  coordination kind this design addresses.
