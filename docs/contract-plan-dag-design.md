# Contract plan-DAG — design (reconciled)

Status: **approved direction, build in slices**. This reconciles the original
plan-DAG design notes with what has since shipped (the `internal/loop.Run` engine,
the `version/agents/map/out_of_scope/pipeline` config, schema `v1alpha1`, `pactum
usage` cost visibility) and the token-efficiency research
([token-efficiency-research.md](token-efficiency-research.md)). Settled by two design
panels (codex gpt-5.5/xhigh + two opus, distinct lenses).

## The problem

The flat contract (`goal` + prose `scope`/`acceptance` + a post-hoc `validation`
gate) makes the executor self-decompose and hold the whole plan in its head — the
two costliest operations for a weak/cheap model. The research confirmed the
load-bearing point: **structure helps a weak executor with coordination and
not-forgetting, not with solving** (a plan substitutes for planning, not for
solving), and decomposition's value is *context*, not ceremony (agents spend the
majority of a turn retrieving context; ours empirically *ignores* the project map
and re-explores). The "decomposition costs 4–10× tokens" alarm was **refuted** in
verification. So: split the contract into a DAG of small self-contained tasks the
loop steps through, one at a time, in fresh focused context.

## The model: contract = constitution + plan

Two layers in one contract:

- **Constitution** (declarative, as today): `goal`, `scope`, `acceptance_criteria`,
  `validation.commands`. The definition of done and the final gate — the **recovery
  anchor**: if a planned task is mis-specified, what counts as success is still
  defined here, not by the plan.
- **Plan**: a DAG of self-contained tasks — the *strategy* that serves the
  constitution.

**No backward compatibility** (no users): there is no flat-contract-without-plan
path. Every contract carries a `plan`; `execute` is *always* the topological loop. A
simple task is a plan of **one** node; a linear feature is a short chain. The DAG
"earns its place" rule (below) is about *granularity* (how many nodes), not about
whether a plan exists.

## The self-contained task

A task is self-contained when the executor can complete it from a **fresh context**
given only: the constitution header, the task's own spec, the packed evidence for its
`context[]`, and the results of its completed dependencies (already in the tree as
commits). **Tasks communicate through git and the ledger, not through model context** —
each task starts cold, reads repository state, does its one unit, commits. The memory
lives *outside* the model; that is what makes "fresh context per task" real.

Each task carries:
- `id`, `title`;
- `depends_on[]` — the DAG edges (and, see *Forward-compat*, the parallelism map);
- `context[]` — **structured evidence selectors** (`{path, lines, why}` or
  `{symbol, why}`), resolved to file slices the loop *packs* for the executor (not a
  repo map it ignores);
- `expected_files[]` — **advisory** touch points, not frozen truth;
- `acceptance[]` — local observable done-criteria;
- `validation[]` — its own pass/fail command(s), **frozen inside the hashed contract**.

Precision goes into *verifiable* fields (`acceptance`, `validation`), not
*prescriptive* ones — freezing exact edits invites satisfying stale instructions
instead of the goal.

### Validation is the immune system (and the most exploitable seam)

A weak executor under retry pressure has a cheap path to green: weaken the check
(assert nothing, skip the new case) rather than do the work. The node marks done on a
lie, its subtree unblocks on a poisoned tree, and the failure surfaces only at the
final gate, several nodes deep. Three rules, enforced in code **before any unattended
run**:
- **Frozen, un-weakenable.** Per-task `validation` lives inside the hashed contract,
  authored by the drafter at plan time. The executor may *run* a validation, never
  author or weaken one. A node whose `validation` was edited in the same commit as its
  implementation is auto-`blocked`. No downstream actor changes a validation without a
  new contract revision + re-approval.
- **Non-vacuous.** Reject a validation that doesn't reference the task's
  `expected_files` or that a global no-op (`go build ./...`) satisfies regardless.
- **Baseline-red.** Where feasible the validation must *fail on the pre-change tree*:
  a check already green before the task starts is not a check. This is the real teeth;
  it costs a build per task and must land before the loop runs unattended.

## Schema (`v1alpha1`)

The contract gains a `plan` object **inside the hash**; the drafter and prompt builder
are extended, not duplicated. Execution state is a **separate, unhashed** artifact.

```jsonc
// contract — pactum.contract.v1alpha1 (hashed)
{
  "schema": "pactum.contract.v1alpha1",
  "goal": "...",
  "scope": { "in": ["..."], "out": ["..."] },
  "acceptance_criteria": ["..."],                  // constitution: definition of done
  "validation": { "commands": ["make check"] },    // constitution: final gate
  "assumptions": ["..."],
  "plan": {                                         // NEW — inside the hash
    "tasks": [
      {
        "id": "t1",
        "title": "...",
        "depends_on": [],
        "context": [
          { "path": "internal/app/run.go", "lines": "60-100", "why": "contract shape" },
          { "symbol": "draftContract", "why": "schema owner" }
        ],
        "expected_files": ["internal/app/run.go"],            // advisory
        "acceptance": ["..."],
        "validation": ["go test ./internal/app -run TestX"]   // FROZEN, non-vacuous, baseline-red
      }
    ]
  }
}
```

```jsonc
// execute/tasks-state.json — pactum.execute_tasks_state.v1alpha1 (UNHASHED, never structure-shaped)
{
  "schema": "pactum.execute_tasks_state.v1alpha1",
  "contract_sha256": "...",                        // ties state to the hash it ran against
  "tasks": {
    "t1": { "status": "done",    "attempts": 1, "by": "sonnet",
            "base_head": "abc123", "commit": "def456",
            "files_touched": ["internal/app/run.go"],
            "context_pack": { "path": "execute/context/t1.md", "sha256": "..." },
            "validation": { "status": "passed", "attempt_id": "attempt_001" } },
    "t2": { "status": "blocked", "attempts": 3, "by": "sonnet",
            "blocker": { "reason": "...", "proposed": "split t2 into t2a/t2b" } }
  }
}
```

**The non-negotiable invariant:** anything structure-shaped (a node, an edge, a
validation) lives **only** in the hashed `plan`. The state file carries
status/attempts/commit and the revision hash it ran against — `proposed` is a *string,
human-only*, never auto-applied; the state schema is *incapable* of expressing
structure, so the running DAG can never drift from its hash and break the two gates.

## Execution: a topological scheduler over `internal/loop.Run`

`execute` becomes two nested layers — reusing the loop engine, not rebuilding it:

```
outer topological scheduler  (NEW, app-level — a graph-drain, NOT loop.Run)
  load plan + tasks-state (one writer)
  loop:
    ready = nodes where every depends_on is done && status not done/blocked-upstream
    if ready empty: break
    pick = first ready in deterministic topological order
    pack = resolve(pick.context[]) -> file slices (evidence; the context-pack)
    outcome = loop.Run(ctx, Limits{Max: execute.loop.max, Settle: 1, Patience: 0}, step)
       step = run executor(constitution + pick spec + pack + git base) ;
              run pick.validation ;
              RoundResult{ Clean: validation passed, Progress: tree changed }
    outcome.Reason == "settled" -> status=done, commit (code + state)
    outcome.Reason == "max"     -> status=blocked (contain failed diff), stall subtree only
    outcome.Reason == "human"   -> escalate (validation tamper / unsafe state)
  after drain: run constitution validation.commands  (unchanged final gate)
              -> gate -> code-review                  (unchanged)
```

- The **per-node retry is `loop.Run{Max:N}`** — identical to the review-loop `Step`
  pattern we already trust (sentinel demux, settle/stale). `Patience:0` disables
  stalemate (every failed attempt is "no progress"; `Max` is the only floor — exactly
  retry-then-block). No new loop primitive.
- The **outer scheduler is hand-written** (~80 lines): it terminates on a *structural*
  condition ("no ready nodes"), not on rounds/settle — forcing it into `loop.Run`
  would be a category error. The loop, not the model, selects the next task by
  readiness; a blocked node stalls only its own subtree.

## Config fit (the `pipeline` shape)

```yaml
pipeline:
  contract_review: { by: [opus, codex-xhigh] }
  plan_review:     { by: [opus] }                       # NEW stage, single-pass; [] / absent => human gate only
  execute:         { by: sonnet, loop: { max: 3 } }     # the per-task retry-then-block cap (existing Loop slot)
  code_review:     { by: [codex-xhigh] }
```

`plan_review` is a new `pipelineStage` field mirroring `contract_review` (reuses
`stageBy`); `execute.loop.max` fills the existing-but-unused `Loop` slot. Config names
*who* and *how many attempts* — never *how to decompose* or *how to route* (strategy
stays in code). `execute.loop` rejects non-default `patience`/`settle` (meaningless
for a binary node) rather than silently accept dead config.

## Granularity

The DAG earns extra nodes only on real intra-contract **fan-in** (a node with
in-degree > 1) or independently-validatable surfaces; target **3–10 leaves**; a leaf is
"one independently reviewable patch," not one AST edit; **one falsifiable validation
per task that references its `expected_files`**. No auto-expansion-by-complexity. The
rule is enforced as a **plan-review lens** (granularity / dependency-correctness /
testability / non-vacuity / scope-fidelity), not as code that auto-splits.

## State split, gates, safety

- **Two gates preserved.** Human approves the contract (constitution + DAG) once; the
  loop runs unattended; human accepts the final result. No human in the middle.
- **Single-writer ledger.** `tasks-state.json` is a whole-file read-modify-write; two
  concurrent writers lost-update a node's status silently. **The lease does not exist
  yet** (the ledger is `O_APPEND` only). So the first loop is **auto-only, with no
  `--task` path** — structurally one writer. The lease becomes blocking the moment
  `--task` (a second writer) ships, not before.
- **Replan is a contract revision, not a ledger edit.** A blocked task never mutates
  the frozen DAG; a structure change is a **new revision** (hash N→N+1, cheap delta
  re-approval). Bounded auto-replan = the drafter re-expands *one* blocked node, once,
  emitting that revision; beyond that it stops for the human.
- **Independent branches keep running under a block** — a blocked node stalls only its
  subtree; parallel progress preserves the automated-middle promise.

## Phase 0: build the minimal loop as the measurement vehicle

The original rollout said "measure before building." Two of its premises are now
resolved — the accounting prerequisite shipped (`pactum usage` surfaces
`effective_units`/`cache_read_ratio`) and the cost objection was refuted — and the one
risk it really guarded (the **handoff/context-loss** failure the DAG *manufactures*)
cannot be measured without a cold-start loop to manufacture it. So the minimal
executor **is** the measurement vehicle. Phase 0 does not disappear; it moves onto the
loop:
- The pre-registered **go/no-go threshold** (coordination + handoff failures clear a
  bar on ≥10 real fan-in contracts, two arms — monolithic vs decomposed cold-start)
  is a **hard gate on the *superstructure*** (lease/`--task`, revisioning, parallel),
  **not** on building the loop. A no-go is an allowed outcome: the loop stays a
  documented experiment and the superstructure is not built.
- **The handoff failure to watch:** t1 commits an implicit convention (a naming
  choice, an error-wrapping style); t5 starts in fresh context, reads the tree, and
  re-derives a *different* convention. Both pass their own frozen validation; the diff
  is internally incoherent in a way a monolithic frontier run never produces. Per-node
  green makes the run *look* healthy — so failures must be classified into
  planning / solving / **handoff** from run #1, or we read poison as success.

## The build plan (each slice = one dogfood PR; code + docs, run-records batched)

1. **Plan schema + structural validation.** `plan.tasks[]` on the hashed contract;
   reject at load/revise: duplicate ids, cycles, unresolved `depends_on`,
   `expected_files` outside `paths_in_scope`, empty `acceptance`/`validation`. Tests
   for valid + each rejection. *Defer:* drafter emission, `plan show`, any execution
   change. *Smallest foundational unit — converges fast.*
2. **Drafter emits the plan + `pactum plan show`.** Drafter (extended, not duplicated)
   produces `plan.tasks[]`; `plan show` renders the static DAG. *Defer:* execution.
3. **Validation freeze (the immune system) + `plan_review` hook.** Frozen / non-vacuous
   / baseline-red enforced in code; `pipeline.plan_review.{by}` single-pass with the
   lenses. **Blocking prerequisite before any unattended loop.**
4. **Minimal topological `execute` loop (auto-only).** Outer scheduler + per-node
   `loop.Run{Max}`; deterministic context-pack (resolve `context[]` → file slices);
   commit-on-pass / contain-on-block (workspace boundary); per-node frozen validation;
   constitution gate after drain. **No `--task` (one writer).** Handoff/context-loss
   classification wired from run #1. *This slice is the Phase-0 instrument — on
   probation.*
   **— hard pre-registered GO/NO-GO gate here → gates everything below —**
5. **Usage rows gain `task_id` / `role` / `attempt_no` / `contract_revision`** (facts
   only; weak-vs-strong derived at rollup, no stored `tier`).
6. **Single-writer lease + `--task <id>` / `--by <agent>`.** `--task` is the first
   second-writer → the lease lands here. Validation mandatory for any runner;
   `--force-done` writes a loud `unvalidated` flag.
7. **Contract revisioning + bounded blocked-node replan.**

**Never:** unbounded/multi-revision auto-replan, in-place unhashed amendments,
multi-round plan-review convergence, parallel branch execution, auto-expansion-by-
complexity, broad per-task routing.

## Forward-compat: parallel worktrees (deferred, but the model already supports it)

"Tasks read git" generalises cleanly to future parallelism because a task reads **not
global HEAD but its own base = the commits of its dependencies**, and `depends_on` is
the parallelism map. Sequentially (first cut) the base *is* HEAD (one shared tree,
topological order). In a parallel future each task runs in its **own worktree off the
merge of its dependencies' commits**: nodes not connected by edges run concurrently;
fan-in (in-degree > 1) is the merge point; the final result merges the leaf branches.
**The contract and state schemas do not change** — `depends_on` and per-node `commit`/
`base_head` already carry it; only the *scheduler* gains worktree-creation + merge (and
merge-conflict handling at fan-in is exactly why it is deferred, not a re-architecture).

## Honest risks

- **The validation seam is the foundation and is currently un-enforced.** Frozen is a
  diff check; non-vacuous is a heuristic; **baseline-red is the real teeth and the most
  expensive/fragile** (a build per task, "where feasible"). It must land in slice 3,
  before slice 4 runs unattended — the one ordering that must not slip.
- **The single-writer lease does not exist** (`O_APPEND` only) — so slice 4 is auto-only
  and `--task` must not exist as a flag until the lease ships (slice 6).
- **Replan-as-revision is easy to violate in code** — make structure *unrepresentable*
  in the unhashed state; don't rely on discipline.
- **Workspace contamination** — a blocked node's failed diff must not leak into
  independent branches (commit-on-pass / contain-on-block); the riskiest single line of
  the loop slice.
- **Operator legibility is the likeliest v1 failure** — a half-run DAG is a lot of
  state; `plan show` (static in slice 2, live in slice 4) is product, not plumbing, and
  must lead so we never run a DAG we cannot inspect.
- **Workflow-engine creep** — each deferred item will feel "almost free now that the
  loop exists"; the pre-registered go/no-go, not convenience, gates the superstructure.
