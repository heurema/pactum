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
  **frozen inside the hashed contract** (see below),
- `context[]` — a retrieval scope (paths / `sym:` symbols) so the loop *packs
  context for the executor* instead of the executor hunting for it,
- `expected_files[]` — **advisory** expected touch points, not frozen truth.

The advisory/verifiable split is deliberate: precision goes into *verifiable*
fields (`acceptance`, `validation`), not *prescriptive* ones. Freezing exact
edits invites the executor to satisfy stale instructions instead of the goal.

**`validation` is the contract's immune system — and the most exploitable seam.**
A weak executor under retry pressure has a cheap path to green: weaken the check
(make a test assert nothing, skip the new case) rather than do the work. The
node then marks done on a lie, its subtree unblocks on a poisoned tree, and the
failure surfaces only at the final gate — several nodes deep. Three rules close
this, all consensus from the design panel:

- **Frozen, un-weakenable.** Per-task `validation` lives inside the hashed
  contract, authored by the drafter at plan time. The executor may *run* a
  validation but never author or weaken one. A node whose `validation` was
  edited in the same commit as its implementation is auto-`blocked` for review.
  No downstream actor (executor, human, auto-replan) changes a validation
  without a new contract revision and re-approval.
- **Non-vacuous.** A validation that can't fail proves nothing. Plan review
  rejects a command that doesn't reference the task's `expected_files` or that a
  global no-op (`go build ./...`) would satisfy regardless of the work.
- **Baseline-red.** Where feasible the validation must *fail on the pre-change
  tree* (or an empty diff): a check already green before the task starts is not
  a check.

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
  "t1": { "status": "done",    "attempts": 1, "by": "sonnet",
          "commit": "abc1234", "files_touched": ["internal/app/run.go"] },
  "t2": { "status": "blocked", "attempts": 3, "by": "sonnet",
          "blocker": "...", "proposed": "split t2 into t2a/t2b" }
}
```

`by` records which registry agent (or human) ran each node. `proposed` is a
*suggestion to the human only* — it is never auto-applied to the frozen DAG; a
real structure change goes through a contract revision (below).

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
- **Single-writer ledger.** The ledger is written by exactly one actor at a
  time, held by an explicit lease. The auto-loop and a manual `--task` run are
  **mutually exclusive** — never two writers committing per-task concurrently
  (that races on `tasks-state.json` and silently drops a node's status).
- **Replan is a contract revision, not a ledger edit.** A blocked task does
  **not** mutate the frozen DAG, and a structure change does **not** live in the
  unhashed ledger — a structure-shaped object in the state file would let the
  running DAG drift from its hash and silently break the two gates. Instead,
  changing the plan produces a **new contract revision** (hash N → N+1) with a
  cheap delta re-approval. Bounded auto-replan = the drafter re-expands one
  blocked node, in-scope, *once*, emitting that new revision; beyond that it
  stops for the human.
- **Independent branches keep running under a block.** A blocked node stalls
  only its own subtree; every node not downstream of it continues. One mid-tree
  block must not idle the whole plan ("stranded subtree"), and parallel progress
  is what preserves the automated-middle promise while a human deliberates.

## Failure policy and granularity

- **Bounded retries, then escalate** (do not subdivide forever): a task failing
  validation N times becomes `blocked` and is surfaced. Escalation options: a
  stronger agent *for that node only* (`--by`, below), a human-directed re-run,
  or one bounded drafter re-expansion (a new revision). This is the practical
  answer to "solving doesn't compress" — when a unit is intrinsically hard, fail
  it cleanly and early instead of looping.
- **Granularity is checkable, but "one command" is not the rule.** A single
  `validation` command is *gameable* — a vacuous `go build ./...` certifies an
  under-sized node. The real rule: each task carries **one falsifiable
  validation that references its `expected_files`** (it must be able to fail on
  the wrong output). And the DAG itself earns its place only when there is real
  intra-contract fan-in — a node with in-degree > 1. Linear work with no joins
  is better served by a smaller `task new` contract (same coordination benefit,
  zero new machinery); the DAG is for genuine dependency joins. No
  auto-expansion-by-complexity in the first cut.

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
- **The plan fixer.** Reviewers emit *findings only*, never authoritative edits
  — a reviewer that writes the fix is no longer an independent check on it. The
  fixer is the **drafter role**, re-invoked as a *distinct cold pass* that sees
  the accepted findings + the current plan + repo context but **not its own
  prior reasoning trace** (a drafter that continues its own thinking rationalizes
  the flaw it introduced instead of restructuring). Symmetric to code review
  (fixer = executor role), with one asymmetry that matters: a plan fix re-freezes
  into a new hash and every node inherits it, so it is costlier than a code fix —
  hence a bounded fix loop, then a human. No new config knob; the fixer reuses
  the drafter resolution.
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

## Human-directed task delegation

The operator must be able to point a chosen agent at a chosen node — *"have
opus do t2"* — not only let the auto-loop run everything on the weak executor.
This is **delegation, not hand-coding**: a human picks the task and the agent; a
*model* still does the work, so validation applies exactly as in the auto-loop.

- `pactum execute run --task <id>` runs **one** node instead of the whole loop.
- `--by <agent>` routes that node to a named registry agent (opus on a hard
  node, sonnet on a routine one) — the same flag is the escalation path for a
  blocked node.
- Invariants that do **not** bend for manual runs: readiness is hard (a node
  whose `depends_on` aren't `done` is not runnable — forcing it breaks the
  cold-start-through-git contract and corrupts downstream state); the ledger
  lease makes manual and auto **mutually exclusive**; and `validation` is
  **mandatory regardless of who ran it**. The only bypass is an explicit
  `--force-done` that writes a loud `unvalidated` flag the final gate must
  surface — a rare edge, never the normal path.
- The ledger's `by` field records the agent per node, which is exactly what the
  token rollup (below) needs to answer "what did the weak loop cost vs. the
  strong escalations."

In v1 `--by` is scoped to explicit `--task` runs and blocked-node escalation —
not a casual per-node routing policy. Broad per-task agent routing is deferred.

## Token usage accounting

Capturing token statistics is a hard requirement (pactum already records
per-attempt usage to `ledger/usage.jsonl`; we just shipped and fixed a
regression there). The DAG multiplies the invocation sites — the drafter, plan
reviewers (1..N), the plan fixer, and the executor **once per task**, plus
auto-replans, retries, and per-node escalations — so usage capture is **core
infrastructure, not plumbing**, and must be designed before the loop or the
economics are unmeasurable.

- **One row per invocation, append-only.** Every drafter / reviewer / fixer /
  executor-task-attempt / replan / retry / escalation writes one accounting row,
  including failed calls (with `usage_missing` when the provider returns none).
  The row carries: `run_id`, `contract_hash`, `contract_revision`, `phase`,
  `role`, `agent`, `model`, `tier` (`weak`|`strong`), `task_id` (when
  applicable), `attempt_no`, `trigger`, `status`, and the token fields
  (input / output / cache-read / cache-write / reasoning). Rows are tied to the
  contract revision and the task transition that produced them, never inferred
  later from mutable state.
- **Rollups are derived, never hand-maintained.** Summaries group by `phase`,
  `task_id`, `role`, `agent`, `model`, `tier`, and `contract_revision`. Direct
  per-task cost is reported separately from contract-level planning overhead,
  with an optional amortized "fully loaded" view.
- **Weak-vs-strong is first-class.** `weak_execute_total`, `strong_plan_total`,
  `strong_review_total`, `strong_fix_total`, `strong_escalation_total` — this is
  the direct answer to the economic question (does a strong-authored plan + a
  cheap loop actually cost less than running the strong model throughout).

## The operator-facing state machine is the product

A half-finished DAG run is a lot of moving state: per-task status, retries,
commits, blocked subtrees, contract revisions, the ledger lease, validation
outcomes, and the usage rows. The likeliest v1 failure is not model economics or
context loss — it is that **a human cannot reason about that state**. So the
ledger semantics, the lease/locking, contract revisioning, usage accounting, and
*inspectable summaries* (`pactum plan show <run>` rendering the DAG with
per-node status, actor, attempts, and cost) are treated as first-class product
surface, not as plumbing behind the model workflow.

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
- **Contract revisioning + bounded replan implementation.** The model is
  decided (replan = a new revision, hash N→N+1, delta re-approval; one scoped
  re-expansion per blocked node), but building it comes *after* the frozen-DAG
  loop. Until then a blocked node simply stops for the human.
- **Unbounded / multi-revision auto-replan** (re-expanding a re-expanded node) —
  never; one bounded re-expansion, then a human.
- **Parallel execution** of independent DAG branches (pactum already spawns
  agent subprocesses; the DAG makes this safe later — until then independent
  branches run sequentially in topological order).
- **Per-slice review** of each landed diff instead of one final diff.
- **Auto-expansion** of tasks by estimated complexity, and broad per-task agent
  routing (`--by` stays scoped to explicit runs + escalation in v1).

## Rollout

The design panel (three independent reviews) converged on one ordering: **do not
build the loop first.** Measure, design the accounting, freeze the checks, then
build.

1. **Phase 0 — pre-registered go/no-go.** Run a weak executor (Sonnet) on real
   pactum contracts in **two arms** — monolithic (whole contract in one context)
   *and* decomposed cold-start (one task per fresh context) — and classify every
   failure into **three buckets**: planning/coordination, solving, and
   **handoff/context-loss** (a convention or decision lost across a cold node
   boundary — a failure the DAG's own design can *manufacture*, which a
   monolithic run never reveals). Measure token cost by role. **Commit the
   threshold before seeing results** (e.g. build the DAG only if coordination +
   handoff failures clear some bar on ≥10 contracts with non-trivial fan-in) —
   otherwise Phase 0 is confirmation theater.
2. **Define the threshold** — minimum task count, real fan-in (in-degree > 1),
   coordination/handoff failure rate, acceptable strong-token overhead.
3. **Design the usage schema + rollups** (the accounting above) — *before* the
   loop, or the economics are unmeasurable.
4. **Freeze per-task validation in the hash** with non-vacuity + baseline-red
   checks and reviewer scrutiny.
5. **Specify the ledger state machine** — single-writer lease, commit-per-task,
   validation-required completion, no concurrent manual/auto.
6. **If Phase 0 passes — build the minimal topological executor** for a frozen
   DAG (drafter emits `plan.tasks`; `execute run` becomes the loop;
   `tasks-state.json` ledger; retry-then-block). Dogfood with Sonnet — the build
   is the measurement vehicle. Wire the optional `plan.reviewers` hook here.
7. **Contract revisioning + bounded blocked-node replan.**
8. **Human `--task` + scoped `--by`** once the lease and validation semantics
   are solid.

Do **not** build yet: unbounded auto-replan, in-place unhashed amendments, a
multi-reviewer plan panel as the default, auto-expansion-by-complexity, broad
per-task agent routing, or the DAG loop before Phase 0 justifies it.

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
- **The validation seam is the most exploitable one.** A weak executor under
  retry pressure weakens the check rather than does the work; if validations
  aren't frozen in the hash and non-vacuous, every other mechanism inherits a
  corruptible foundation. This is why "freeze the checks, not just the
  structure" is a v1 requirement, not a later hardening.
- **Operator legibility may be the real v1 failure.** Not economics or context
  loss, but a human unable to reason about a half-finished DAG (retries,
  commits, blocked subtrees, revisions, leases, usage rows). The state machine
  and its inspectable summaries are product, not plumbing — see above.
