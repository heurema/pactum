# Token-efficiency research — distilled

Status: **research distilled**. Sources: a web deep-research pass (5 search angles,
adversarial 3-vote verification) + a parallel gpt-5.5 reasoning pass, plus an
**empirical measurement of pactum's own usage ledger**. The empirical finding
overturns the headline priority, so read it first.

## The question

How should a contract-first autonomous coding-agent orchestrator minimise token
spend for a given output quality, across (A) how it **decomposes** the spec it hands
the executor (one flat contract the model self-decomposes vs a DAG of small
self-contained tasks) and (B) how it **executes** each unit (prompt caching, context
scoping, model routing, fresh-vs-shared context)?

## Bottom line

1. **Prompt caching is the top *billable* lever in the literature — but pactum has
   already captured almost all of it (measured 79–97% cache-read), so it is NOT
   where our remaining savings are.** This is the surprise (see *Empirical finding*).
2. **Context selection is the top *raw-token* lever** and is where pactum's residual
   spend lives (fresh, non-cached input + the executor re-exploring instead of being
   handed evidence).
3. **A task DAG helps quality/convergence and keeps per-task fresh context small —
   but only when granularity is gated on complexity; blanket micro-decomposition is
   not Pareto-optimal.** The scary "decomposition costs 4–10× more tokens" claim did
   **not** survive adversarial verification.

## Empirical finding (pactum-specific — this reorders the priorities)

The research's #1 open question was: *does pactum's fresh-ACP-session-per-attempt
pattern even get prompt-cache reuse, or does each fresh session start cold?* We
measured it from the captured usage ledger (`cache_read_tokens` per attempt):

| run | provider | input | cache_read | cache hit |
|---|---|---|---|---|
| usage rework | anthropic | 6.77M | 6.37M | **94%** |
| usage rework | codex | 3.71M | 2.94M | **79%** |
| config rework | anthropic | 17.8M | 17.3M | **97%** |

- **Caching works across fresh ACP sessions.** The same `cache_read` value recurs
  across *separate* reviewer attempts — cross-session reuse, not cold starts. The
  underlying agents (Claude Code, codex) cache aggressively, and pactum's prompt
  prefixes are stable enough within a stage/round.
- **The input-heavy profile is misleading.** "6.77M input" *looks* expensive, but 94%
  is cheap cache-read (Anthropic bills reads at 0.1× base input). Effective input ≈
  `0.4M fresh + 0.1×6.4M ≈ 1.0M` units vs 6.77M raw — we already save ~85%.
- **Therefore caching has little headroom for us** (you can't beat 94–97%), and the
  caching×DAG "crux" the research worried about is **moot for pactum** — fresh
  context per task does not cost us cache reuse.
- **The blind spot (now fixed): `pactum usage` used to show only raw input, not the
  cache-adjusted cost.** The `cache_read` field was captured but not surfaced — we
  were measuring the scary-but-cheap number instead of the true one. The command now
  surfaces `cache_read_ratio` and `effective_units` (per group + total), so the real
  cost is legible (e.g. 10.5M raw input → 89% cache-read → ~3.78M effective units).

## Sub-question A — task-DAG decomposition

- **[high] Small per-task context wins; decomposition is task-dependent; granularity
  is a tunable knob over an externalised task-DAG.** Input tokens dominate agentic
  coding cost, runs vary up to ~30×, and accuracy peaks at *intermediate* spend — more
  tokens do not reliably mean better results (SWE-agent / SWE-Effi cost studies).
- **[high] Real tools converge on `requirements → design → tasks → implement`**
  (GitHub Spec Kit: `/specify` `/plan` `/tasks` `/implement`; AWS Kiro:
  `requirements.md`/`design.md`/`tasks.md`; Task Master as a task layer). The reason
  is **context, not ceremony** — Cognition reports agents spend >60% of the first turn
  retrieving context, and agentic search forces attention over large irrelevant token
  masses; their retrieval subagent returns file **line-ranges**, not summaries.
- **[refuted 0-3] "Explicit decomposition (plan-and-execute DAG) costs ~4× more
  tokens, ReAct ~10×."** Did not survive verification — decomposition is **not**
  inherently prohibitively expensive.
- **[refuted 1-2] "Blanket adaptive Select-Then-Decompose is Pareto-optimal at ~25%
  cost."** Weakly refuted — the safe conclusion is **gate granularity on complexity**,
  not "always decompose."
- **[speculative, convergent] Optimal granularity = "one independently reviewable
  patch," not "one AST edit": 3–10 leaf tasks per feature**, each one cohesive
  behavioural change + a small file cluster + one validation check. Merge tasks that
  share coupled API/design decisions; split when relevant context exceeds a focused
  pack or when packages/pages/services validate independently.
- **Per-task context = evidence, not a map.** Relevant files / line-ranges +
  dependency decisions + exact acceptance criteria + validation command + contract
  constraints — *not* the whole repo, the whole transcript, or the whole mutable plan.
  (Reinforced by our own finding that the executor **ignores** the project map and
  re-explores with its own tools — see [token-efficiency observation in the backlog].)

## Sub-question B — execution levers, ranked by token impact

1. **Context selection / context budget — [high]** the biggest *raw-token* and
   quality lever. It *removes* tokens (not discounts them), cuts context pollution,
   and avoids exploration loops. This is pactum's highest-leverage residual.
2. **Prompt caching — [high]** the biggest *billable* lever in general (reads 0.1×,
   5-min writes 1.25×, 1-hr 2×; longest-exact-prefix match; OpenAI automatic from
   1024 tokens, up to ~90% input cost off) — **but already ~95% captured for pactum.**
3. **Workflow-shaped pipeline before autonomy — [high]** use deterministic/
   schema-bound workflows for clarify, contract review, task slicing, context packing,
   gate evaluation, review aggregation; reserve autonomy for the open-ended executor
   loop (Anthropic "Building Effective Agents": add agentic complexity only when
   simpler workflows fall short).
4. **Model routing — [high cost / medium quality]** cheap models for
   classification, linting, retrieval, task expansion, narrow review lenses; strong
   models for contract synthesis, high-risk planning, hard execution, arbitration.
   **Caveat:** do not switch models inside a warm session — each model has its own
   cache, so switching mid-session can cost more than staying. Route at **task
   boundaries**.
5. **Fresh vs shared context — [quality lever; conditional token lever]** fresh clean
   context helps reviewers and bounded child tasks (re-derive from the diff, avoid the
   coder's long trace); shared context helps coherent write decisions. Single-thread
   writes; let extra agents add review/retrieval/coordination.

### The caching × fresh-context crux — resolved

Fresh context per task does **not** inherently destroy cache reuse. It destroys reuse
only if the **prefix changes before the cache breakpoint** (different tools, model,
effort, cwd/system prompt, timestamps, git snapshot, task files, or a mutable ledger
placed too early). The winning shape:

```
[ stable: protocol + tools + project rules + immutable contract + base DAG ] —cache→
  + small volatile per-task suffix (task files, status, diff, timestamps)
```

For Anthropic's 5-min cache, repeated shared-prefix cost over n calls ≈
`1.25S + 0.1S(n−1)` instead of `nS` — break-even on the **2nd** reuse, approaching 90%
savings on the stable part. DAG + caching **wins** when the shared cached prefix is
large and reused across planner/executor/reviewers/fixer/gate while per-task suffixes
stay small; it **loses** when each micro-task carries a unique giant file bundle
*before* the breakpoint. *(Caveat: the published cache figures are from research
benchmarks, not coding specifically.)*

## Recommendations for pactum (re-prioritised after the empirical finding)

1. **Surface cache-adjusted cost in `pactum usage`** — ✅ **DONE.** The command now
   shows `cache_read_ratio` and `effective_units` (per group + total), so we measure
   the *true* cost (fresh + 0.1×cache-read), not the misleading raw input.
2. **Context-pack phase before execute** (the top raw-token lever). A deterministic
   step that emits relevant file paths + line-ranges + snippets + acceptance checks,
   handed to the executor as **evidence** — treat the executor's own repo exploration
   as a fallback, not the primary path (it ignores our map today).
3. **Plan-DAG with complexity-gated granularity** (3–10 leaf tasks; linear/simple work
   stays one contract; the DAG earns its place on real fan-in / independent
   validation). Per-task context = evidence pack, not the whole contract. Keeps
   per-task fresh input small (which compounds with caching).
4. **Keep cache keys stable within a unit; route models at task boundaries.** Don't
   switch model/effort/tool-set mid-session. Cheap models for retrieval/lint/narrow
   review; strong for synthesis/hard execution/arbitration.
5. **Stay workflow-shaped** for clarify / slice / pack / gate / aggregate; autonomy
   only inside the executor loop.
6. **Instrument the cost-quality frontier** (`pactum usage` + cache ratio + per-phase
   retries + files-read + task size + pass/fail). Token use is stochastic and weakly
   correlated with success, so pactum needs its own measured frontier before locking
   granularity. Caching is solved; measure what isn't.

## Primary sources (selected)

Anthropic *Building Effective Agents*, *Effective Context Engineering*, *Prompt
Caching* docs; OpenAI *Prompt Caching* guide; Cognition *Don't Build Multi-Agents*,
*SWE-grep / Fast Context*, *Devin can manage Devins*; GitHub Spec Kit; AWS Kiro
specs; Task Master; Aider repo-map; SWE-agent / SWE-Effi cost analyses (arXiv); Ralph
agentic-loop write-ups. Confidence tags above: `[high]` = multiple primary sources
agree; `[refuted N-M]` = killed/weakened by adversarial verification; `[speculative]`
= convergent reasoning, not a controlled study.
