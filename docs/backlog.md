# Backlog

Deferred work and known follow-ups, consolidated from dogfood reports and PR
reviews across M8–M10. Rough priority in parentheses.

## Execution / review UX

- **Agent registry in the config** (M18.0, shipped; simplified in M19.3). The
  per-stage agent/model entries are replaced by one required top-level `agents`
  registry that everything else references by name (`--agent`, `--reviewer`,
  and `review.panel`, now a plain list of names). M19.3 simplified an entry to
  `{name, model, effort}`: the `agent` key is gone — the engine is inferred
  solely from the model (`claude*` or the `opus`/`sonnet`/`haiku`/`fable`
  aliases → claude; `gpt*`, `codex*`, or `o<digit>*` → codex; anything else is
  a loud config error), which makes `model` required on every entry:

  ```yaml
  agents:                  # required, at least one entry
    - name: fable
      model: claude-fable-5  # infers the claude engine
    - name: codex
      model: gpt-5.5         # infers the codex engine
      effort: high
  review:
    panel: [fable, codex]  # registry names; empty = cross-model
  ```

  The `execute` section is gone — a name carries its model+effort everywhere,
  a name is decoupled from the engine (two claude-backed entries may share one
  panel), `writeDefaultConfigIfMissing` generates
  `agents: [{name: claude, model: claude-opus-4-8}]`, and an empty/missing
  registry is a loud error. An inherit-the-CLI-default entry can no longer
  exist (the deliberate consequence of model-only inference). References
  resolve only against the registry (bare engine names are not implicitly
  available); an omitted `--agent` defaults to the first entry, and the
  cross-model reviewer default compares INFERRED engines (first entry on a
  different engine, else the first entry). The usage ledger records the
  registry name (`agent_name`) alongside the engine. The registry is also the
  natural seed for custom agents later (command/args fields on an entry).
  Small follow-up: a registry-aware `agents doctor` view (list the registered
  entries with their inferred engines and pins, not just the built-ins).
- **Timeout follow-ups** (med). `--timeout` is now idle-based (M11.6),
  completion-aware (M20.0: an idle-killed attempt whose output carries the
  agent's successful terminal marker finalizes as completed-with-warning —
  exit 0, `timed_out: true` + `completed_despite_timeout: true`), and
  per-project configurable (M20.1: the `timeouts.idle` config key; resolution
  is explicit `--timeout` → `timeouts.idle` → built-in 25m since M20.2, which
  raised the built-in from 10m and stopped emitting the key into the generated
  config — set it only to deviate). Still open:
  (b) an optional **absolute total cap** (off by default) as a CI backstop for an
  agent that keeps emitting output yet never finishes (the idle timer never
  fires) — a sibling key in the `timeouts` section;
  (d, note: moving claude to stream-json would also require reworking the
  completion detector and parseClaudeUsage, which unmarshal the single terminal
  result envelope) **silent-agent idle-timeout gap** — the claude executor (`claude -p --output-format
  json`) emits nothing to stdout/stderr until its final result, so the idle watchdog
  (which appears to arm on first output) never fires: a claude run is silent for its whole
  duration and is neither idle-killed mid-work (good here) nor protected against a real
  hang (bad). Surfaced in the M12.9 dogfood (claude silent ~30 min, finished cleanly, never
  killed). Fix: run the claude executor with `--output-format stream-json` (streams events
  → resets the idle timer, like codex), and/or arm the idle watchdog from process start.
- **Agent tool-call trace + live progress feed** (med, research-backed —
  design in [agent-tool-trace-design.md](agent-tool-trace-design.md);
  absorbs the earlier "ACP tool-call progress in the live output" item).
  Both ACP adapters already deliver full tool-call payloads (command lines,
  ranged-read offsets, locations, outputs, status lifecycle) to
  `acpClient.SessionUpdate`, which ticks the watchdog and discards them.
  Record a per-attempt `tool-trace.jsonl` beside `stdout.log` (gitignored
  like `*.log`): ts, call/update, tool_call_id, kind, title, status,
  locations, input/output byte sizes, duration + exit code on the terminal
  update; content (`raw_input`/`raw_output`) only behind an explicit opt-in
  with truncation bounds, copying the sizes-by-default redaction model.
  Emit a one-line live feed per call (closes the "tool-heavy run looks
  frozen" gap). Trace counts are honest lower bounds (adapter event coverage
  is per-type and has had gaps). Follow-up rides the navigation arc: an
  adoption report over traces (ranged-read share, repo-CLI usage) replaces
  narration-grep as evidence.
- **Workspace config leaks across runs/branches** — structurally resolved by the
  M16.0 config redesign: model pins became per-agent entries, so a pin can no
  longer reach a different agent (the M10.2 failure mode — a leaked claude pin
  breaking a codex run — is impossible by construction), and `config.yaml` is
  version-controlled since M12.3 so it no longer silently persists across
  branches. Since M18.0 the pins live on the agents-registry entries, which
  keeps the same per-entry isolation.

## Review→fix loop (L3b and beyond)

- **Review-loop convergence** (high). **Slices 1-2 shipped (M12.8 resolve-on-fix,
  M12.9 severity-gated convergence)** — root causes (a) and (b) below are addressed; (c)
  the anti-meta-churn reviewer prompt remains (overlaps the prompt-quality item below).
  The first real dogfood of the autonomous `review
  loop` (cross-model panel on the M12.6 store-port) did NOT converge — it hit `max_rounds`
  with open findings accumulating monotonically (4→7→10→12, never a clean round). Root
  causes: (a) **findings are never resolved** — a fixed finding stays "open", so
  `open_findings` only grows and a 0-new-proposals clean round is the only path to
  convergence; a successful fix should mark its finding resolved (or a re-review should).
  (b) **No severity gate** — most findings were `low` design-observations; low/subjective
  findings drive fixer churn. Converge on blocking/critical+major only (the L2 pass) and
  defer low. (c) **Meta-churn** — by rounds 3-4 the panel began critiquing the FIXER'S OWN
  changes (a method it added, "the working tree changed", "validation wasn't re-run"),
  which never clears; the reviewer prompt should judge the contract's diff, not process
  meta-commentary, and avoid re-reviewing already-accepted unchanged regions. (d) The
  cross-model panel itself worked (disjoint real findings — it caught real mis-routing the
  manual pass missed — 0 bad merges) but doubled finding volume and amplified churn. Net:
  the loop is strong at FINDING but cannot yet CONVERGE on a refactor; today its output is
  a draft to curate, not an auto-merge.
- **Strengthen the executor + reviewer prompts with house style + best practices**
  (**arc complete, M19.0–M19.2**). The built-in prompt templates
  carried only generic guidance. A survey of leading agent review prompts is
  distilled in [`review-prompt-design.md`](review-prompt-design.md); the slices:
  (1) reviewer prompt hardening — **shipped (M19.0)**: the high-signal contract
  (certain-or-silent, explicit NOT-to-flag list, problems-only), five lens
  checklists (correctness, implementation-vs-contract, test quality +
  fake-test detection, over-engineering catalog, documentation gaps),
  verify-then-report (CONFIRMED/FALSE-POSITIVE classification before
  emitting), findings-first ordering with honest empties,
  pre-existing-issues-as-advisory policy, and a per-finding `confidence` field
  (schema precedent: clarify questions, M15.0; missing defaults to medium,
  invalid skips with a warning, recorded and displayed but gating nothing yet);
  (2) write-stage house style — **shipped (M19.1)**: one shared section in the
  executor and review-fix fixer prompts — match idiom/naming/comment density,
  reuse helpers, small focused diffs, simplicity-first with the
  over-engineering catalog as DON'Ts, no dead code, behavior-verifying tests
  with the fake-test catalog as DON'Ts — so the writer is told the rules the
  reviewer holds it to; (3) specialist review lenses — **shipped (M19.2)** as
  built-in default behavior rather than configuration: every `review run` and
  every loop round expands each resolved reviewer into five concurrent lens
  attempts (one per M19.0 checklist), each with a focused per-lens prompt;
  cross-lens duplicates collapse through the existing fingerprint dedup with
  severity-max.
- **Loop stop conditions.** Stalemate-by-fingerprint and K-consecutive-clean —
  **done (M10.3)**. The earlier `review.budget` token-stop plumbing gated nothing
  real and was removed (M25.1, including the `budget_exceeded` terminal); a
  designed budget stop, denominated in effective units, returns later per
  [`cost-budget-design.md`](cost-budget-design.md).
- **Cost/budget remaining slices** (see [`cost-budget-design.md`](cost-budget-design.md)).
  Slices 1-2 (write- and read-stage token accounting) done (M12.0, M12.1). The
  earlier `max_tokens` budget stop was removed (M25.1) as it gated nothing real;
  the budget feature returns later denominated in effective units, alongside cost
  ($) overlay and estimation. Also: harden the claude usage parser to tolerate
  incidental leading/trailing stdout (today any non-JSON stdout degrades claude
  capture to `captured=false`; safe but brittle).
- **Cross-run / workspace usage stats command** — static aggregate **done (M13.0)**.
  `pactum usage --all` scans every run's `usage.jsonl` and reports workspace total
  tokens with by-run / by-stage / by-agent / by-model breakdowns and the cache-read
  ratio, in human and `--json` form. Best-effort: a missing or corrupt ledger is
  skipped, an empty workspace reports zero. Derived view, never the source of truth.
  Remaining: trend-over-time series / charts (a separate slice; the static aggregate
  deliberately excludes it).
- **Rebuttal channel** (med). Feed the fixer's rebuttal of a false positive back to
  the reviewer on the next round so it re-judges instead of re-reporting.
- **Semantic review-finding dedup** (low). The autonomous review loop now dedups
  proposals against currently open findings only by the exact stored
  `(file, line, message)` tuple. Reworded messages, line-number drift, and other
  semantic duplicates are still treated as distinct findings; add fuzzy/semantic
  reconciliation only after the exact-match behavior has settled.
- **JSON sub-command contract** (low). The loop json-parses sub-command stdout; a
  mid-loop not-ready condition could inject human text into the parsed buffer. Make
  the contract explicit.

## Loop phases not yet built

- **L2 — severity by composition** (med). A broad fix pass, then a critical/major-
  only final gate (no per-finding severity schema).
- **Phase 1 — clarify loop** (high, large). Slices done: `clarify suggest` (agent
  proposes questions, M11.0), `contract draft` (agent drafts contract fields from
  answers → proposal → `accept-draft`, M11.1), and **slice 1 — the autonomous
  clarify loop driver** (M17.0, shipped): `pactum clarify loop` runs
  suggest → auto-resolve → re-suggest rounds, composing the grill-me pieces —
  per-question `recommended_answer` + `confidence` (M15.0) and the
  converged/coverage signal (M15.5) — into the review-loop pattern. Each round
  auto-resolves every open question whose confidence is `high` and whose
  recommendation is non-empty (answer source `auto_recommended`, decision source
  `clarify_loop_auto`; medium/low or recommendation-less questions stay open),
  refreshes artifacts once, and stops `converged` (no open blocking),
  `needs_human` (a round created nothing and resolved nothing), or `max_rounds`
  (the revived `clarify.max_rounds` cap, default 3). The loop writes
  `clarify/loop-summary.json` (`pactum.clarify_loop_summary.v1`); `contract
  approve` stays manual — that is the safety story for letting the clarifier's
  own high-confidence recommendations answer its questions. The **task-new
  integration** (M21.0, shipped) folds the loop into task creation: `pactum
  task new "<task>" --clarify` runs it right after the run is created and
  surfaces the remaining open blocking questions with their recommendations —
  one command from task to a pre-interrogated contract; the loop pass-throughs
  (`--reviewer`, `--max-rounds`, `--timeout`) ride along, a loop failure leaves
  the created run intact, and without `--clarify` the command is byte-identical
  to before. Remaining Phase 1
  slices: the **contract-refinement leg** (fold `contract draft` →
  `accept-draft` into the loop so answers refine the contract, not just the
  question set) and the **human-answer round-trip** (resume the loop after the
  human answers what `needs_human` left open). Open edges:
  re-proposed-question dedup is still prompt-level only (no semantic dedup);
  the clarifier/drafter fuse run+parse so a non-zero exit
  discards valid output (decouple like `review propose-findings`); an all-empty
  `contract draft` records a pending proposal that `accept-draft` then rejects
  (dead-end — report "no additions" instead); re-running `contract draft` after accept
  clobbers the accepted proposal's audit fields; `accept-draft` hardcodes
  `accepted_by:"manual"` (no `--by`); no budget gating for the clarify loop yet
  (token records still accrue per attempt).
- **Sharper clarify questioning** (med). **Arc complete — all five slices shipped
  (M15.0–M15.5).** Strengthened `clarify suggest` from a soft, surface-level
  checklist into a "grill the requester" interrogation, sliced after the grill-me
  principles so the resulting contract is precise rather than agreeable.
  - **Slice 1 — recommended answers + explore-first** (M15.0, shipped). Every
    proposed question now carries a concrete `recommended_answer` plus a
    `confidence` (high|medium|low), so the human confirms/adjusts a recommendation
    instead of authoring an answer from scratch; the prompt enforces explore-first
    discipline — resolve each candidate from the contract/repo/search first and fold
    repo-answerable findings into the rationale and recommended answer, escalating
    only questions that genuinely need a human decision. (Recommendation is captured
    and displayed only; confidence-gated auto-resolve is a later slice.)
  - **Slice 2 — dependency-ordered questioning** (M15.2, shipped). The clarifier
    now orders its questions foundational-first and declares `depends_on` for any
    question whose framing or answer hinges on an earlier one, referencing those
    earlier questions by their 1-based position in the emitted sequence. Pactum
    resolves the positions to the assigned question ids in a single forward pass —
    a forward/self/out-of-range/skipped reference is dropped with a warning while
    the question is still recorded — persists the resolved ids on each question, and
    marks an open question `blocked` when any prerequisite is still unanswered.
    `clarify status` and the clarifier-context list surface the `depends_on` ids and
    the blocked flag. Additive and display-only: the open/answered status values and
    the Open/Answered/BlockingOpen counters are unchanged (a blocked blocking
    question still counts as open/blocking), and this slice does not auto-defer or
    hide dependent questions in the loop (a later slice).
  - **Slice 3 — terminology / domain challenge** (M15.3, shipped). Every
    clarification question now carries a `kind` from a closed set —
    `terminology, scope, acceptance, edge_case, assumption, other` — validated when
    the suggestion is recorded (a missing/invalid kind skips that question with a
    clear warning while the others are still recorded), persisted on the question,
    and surfaced in `clarify status` and the clarifier-context list. The clarifier
    prompt gains a "challenge vague terminology" instruction: flag ambiguous or
    overloaded domain terms in the contract (goal/scope/acceptance), ask which
    concrete meaning is intended with the candidate interpretations named, anchored
    on the repository's actual concepts/identifiers, and tag such questions
    `kind=terminology`. Additive and behavior-compatible — no schema-version bump;
    the ambiguous term and its candidate meanings live in the question text and
    recommended answer (only the kind tag is structured). The kind field is the
    structural basis for the remaining slices.
  - **Slice 4 — edge-case probing** (M15.4, shipped). The clarifier prompt gains a
    dedicated "Probe edge cases" section: rather than abstractly "consider edge
    cases", it now instructs the clarifier to invent CONCRETE boundary and failure
    scenarios for each in-scope behavior and acceptance criterion — empty/missing/
    zero/duplicate/extreme inputs, error and failure paths, partial or interrupted
    operations, concurrency and ordering, resource/size limits, and other "what
    about X" cases the contract is silent on — name the specific scenario in the
    question text, recommend how the contract should behave there, and tag the
    question `kind=edge_case` (the category from slice 3). The clarifier is told to
    prefer the scenarios most likely to change scope, acceptance, or implementation
    and to skip what the contract or repository already settles. Prompt-quality
    change only — no schema, validation, or enum change; a prompt-content test
    asserts the section and its `kind=edge_case` tagging instruction cannot be
    silently dropped.
  - **Slice 5 — coverage / convergence signal** (M15.5, shipped). `clarify status`
    now reports, per contract dimension (the `kind` set from slice 3), how many
    questions are open vs answered (and open-and-blocking), and surfaces an overall
    `converged` flag (true iff no open **blocking** questions remain — mirroring the
    review-loop `resolved` condition). The coverage breakdown always lists the five
    canonical dimensions — `terminology, scope, acceptance, edge_case, assumption` —
    in fixed order even at zero, so an unprobed dimension is visible rather than
    hidden behind a flat open count; `other` appears only when some question (an
    explicit `other` kind or a kind-less manual question) falls outside the canonical
    set. The per-kind tallies sum back to the overall Total/Answered/Open/BlockingOpen.
    Both `clarify status` and the clarifier context surface the breakdown, and the
    clarifier prompt gains a "Cover the material dimensions" instruction: consider each
    material dimension before concluding, but do not manufacture questions for
    dimensions the contract/repository already settles (explore-first still applies).
    Additive and signal-only — no schema-version bump; `Converged`/`Coverage` are
    computed and surfaced for the human and a future autonomous clarify loop to
    consume, with no auto-stop / convergence-driven control flow built this slice.
    This completes the "grill the requester" clarification arc (slices 1-5).

## Agent-first CLI polish arc

The CLI's primary consumer is an orchestrating AI agent driving pactum through
a skill, not a human typing commands. That flips the design goal from
memorability to **derivability**: the best agent CLI is the one whose complete
description fits in a few sentences, with everything else inferable from a
regular grammar. Every irregularity costs skill-doc tokens and invites
hallucinated commands. Principles: a uniform verb set per stage (`run`,
`show`, plus `approve/accept/reject` for decisions and `add/resolve/answer`
for collections); one name per action (no aliases or duplicates); zero
interactivity (confirmation happens upstream, in the human's conversation
with their agent); the CLI announces legal moves instead of making the agent
guess (structured `next` affordances and error envelopes carrying a `reason`
code plus the remedial `fix` command); stdout is the agent's context, so
human output stays terse and artifacts travel by path. Decision commands
carry an optional `--by <principal>` (default `manual`) recording whose
decision the agent relayed — attribution without ceremony.

- **Slice 1 — grammar normalization** (M23.0, shipped): renames, nested
  subcommands instead of hyphenated ones, duplicate/alias removal.
- **Slice 2 — confirmation model** (M23.1, shipped): interactive confirms,
  `--yes`, and `gate run --allow-commands` removed; the optional
  `--by`-with-default extended to all decision verbs (`contract accept`,
  `clarify answer`, `review proposal accept|reject`), recorded as
  `decided_by`/`accepted_by`; automatic loop decisions carry only their
  `source` (review-loop auto-accepts now record `review_loop`, not `manual`).
- **Slice 3 — affordances** (med): structured error envelopes
  (`reason` + `fix`), a `next` array in every mutating command's `--json`
  output and in `pactum status`, so the skill never encodes the stage state
  machine.
- **Slice 4 — pipeline smoothing + skill rewrite** (M24.1, shipped):
  `review run` absorbed the loop and the implicit review scaffold (prepare and
  the loop spelling removed), `prompt build` self-heals a stale map
  (`map_refresh`), the clarifier round folded into
  `clarify run --no-auto --max-rounds 1`, recommended-answer decision verbs
  (`clarify answer --recommended` / `--all-recommended`), and the skill
  rewritten against the final grammar.

## Agent file-navigation arc (research-backed)

Agents (executor, reviewer lenses, clarifier, drafter) read large source files
through their own grep/read tools; on big files (`internal/app/review.go` is
~1.5k lines) the dominant pattern is a 3-hop loop — grep, read a window,
re-read a wider window — multiplied across ten review lens attempts per
round. A survey of 2024–2026 localization research (signatures-only file
skeletons are sufficient for localization; symbol-boundary chunks beat line
windows; symbol-level navigation is the highest-value primitive) maps onto
machinery pactum already has: the tree-sitter code-items index carries
`Signature`/`StartLine`/`EndLine` per symbol — the agents just never see it.
Explicit non-goals, by the same survey: no embeddings/vector index (staleness,
nondeterminism, infra cost vs marginal gain over FTS5 + symbols), no LSP
runtime dependency, no model-based context compression. The survey
distillation lives in
[agent-file-navigation-design.md](agent-file-navigation-design.md).

- **Symbol-grade search results** (small): plumb `StartLine`/`EndLine`/
  `Signature` from the code-items index into `search.Result` for `code_item`
  hits and render them in executor-context as `path:start-end signature`, so
  the first Read is a ranged read; add a `--symbol <name>` filter to
  `pactum search`.
- **`pactum outline <path>`** (small-med): deterministic per-file skeleton
  (signatures + line ranges + doc comments) from the code-items index,
  content-hash-stamped; executor-prompt convention: for files over ~400
  lines, outline first, then Read by range (line numbers valid until the
  first edit of that file).
- **Contract-scoped skeletons in executor-context** (med): at prompt build,
  inline outlines of every file in the contract path scope under a token
  budget (exported symbols first).
- **Review hunk→symbol annotation** (med): annotate each diff hunk in the
  review input with its enclosing symbol and full range, so verify-then-report
  reads the whole enclosing function once instead of windowing around the
  hunk — targets the multi-million-token review leg directly.

## Contract plan DAG arc (research-backed)

The flat contract (`goal` + prose `scope`/`acceptance_criteria` + post-hoc
`validation.commands`) forces the executor to self-decompose and hold the whole
plan in its head — the two costliest operations for a weak/cheap executor
(Sonnet rather than a frontier model). A deep-research pass over agent-planning
literature distilled a load-bearing, honestly-bounded conclusion: structure
helps a weak executor with **coordination and not-forgetting, not with
solving** (a plan substitutes for planning, not for solving), and every
confirmed empirical result is non-software, so this is a bet to validate, not a
settled fact. The fix is two layers in the one contract — a declarative
*constitution* (the recovery anchor + final gate, as today) plus a *plan* DAG of
self-contained tasks the loop (not the model) steps through. Schema stays
`pactum.contract.v1` (no users → evolve in place, no version bump). A
three-agent design panel (codex gpt-5.5 + two opus, distinct lenses) hardened
the plan: freeze the *checks* (not just the structure), replan via contract
revisioning (not ledger edits), single-writer ledger, usage accounting as core
infra, and "do not build the loop before Phase 0." Distillation in
[contract-plan-dag-design.md](contract-plan-dag-design.md). The panel's ordering
is deliberate — measure, account, freeze, *then* build:

- **Phase 0 — pre-registered go/no-go baseline** (small): run a Sonnet executor
  on real pactum contracts in **two arms** (monolithic + decomposed cold-start)
  and classify every failure into **three buckets** — planning/coordination,
  solving, and **handoff/context-loss** (a convention lost across a cold node
  boundary — a failure the DAG itself can manufacture, invisible to a monolithic
  run). Measure token cost by role. **Commit the threshold before seeing
  results** (build only if coordination+handoff clear a bar on ≥10 contracts
  with real fan-in) or it is confirmation theater. No production code.
- **Phase 0.5 — usage schema + rollups** (small-med): design before the loop or
  the economics are unmeasurable. One append-only `usage.jsonl` row per
  invocation (drafter / reviewer / fixer / executor-per-task / replan / retry /
  escalation, failures too) carrying facts only — `run_id, contract_hash,
  contract_revision, phase, role, agent, model, task_id, attempt_no, trigger,
  status` + token fields. No stored `tier` (a derived classification, redundant
  with `role`+`model`); the weak-vs-strong split is derived at rollup by grouping
  on `role` (executor-loop total vs planning-role totals), exact cost from
  `model` × pricing.
- **Phase 1a — drafter emits the plan DAG** (med): extend `draftContract`
  (`internal/app/run.go`) with `plan.tasks[]` — each `{id, title, depends_on[],
  context[], expected_files[] (advisory), acceptance[], validation[]}`.
  Validation lives **inside the hashed contract**, non-vacuous (references
  `expected_files`, can fail) + baseline-red; the executor runs but never
  authors/weakens it. Granularity rule: one *falsifiable* validation per task;
  the DAG earns its place only with real fan-in (in-degree > 1) — linear work
  stays a smaller `task new` contract.
- **Phase 1b — ledger state machine** (med): single-writer lease,
  commit-per-task, validation-required completion (every actor), no concurrent
  manual/auto. The unhashed `execute/tasks-state.json` holds only state
  (`status, by, attempts, commit, files_touched, blocker`); structure stays in
  the hash.
- **Phase 1c — minimal topological executor** (large; the real work, gated on
  Phase 0): `execute run` → loop over ready nodes, fresh context per task,
  per-task validation, retry-then-block (branch stops, independent branches keep
  running). Dogfood with Sonnet. Optional `plan.reviewers[]` hook here (findings-
  only reviewers; fixer = drafter role as a distinct cold pass) + the
  `review.panel` → `review.reviewers` rename (one vocabulary; `[]` = off).
- **Phase 2 — revisioning, delegation, convergence** (deferred): contract
  revisioning (hash N→N+1, delta re-approval) + one bounded blocked-node
  re-expansion; human `--task <id>` + scoped `--by <agent>` (delegate a node to
  a chosen agent / escalate a blocked one); multi-round plan-review convergence;
  parallel branches; per-slice review. Not built: unbounded auto-replan, in-place
  unhashed amendments, auto-expansion-by-complexity, broad per-task routing.

## Hardening / cleanup

- **`gofmt` is not in the gate** (small). `make check` runs test/vet/deadcode +
  `git diff --check`, but not `gofmt -l`, so formatting drift accumulates
  uncaught (e.g. const-block alignment in `internal/app/clarify_round.go` and
  `review.go` drifted after the M25.1 budget-const removal — vet-clean, builds
  fine, but unformatted). Add `gofmt -l` (fail on non-empty) to `make check`
  and `gofmt -w` the existing drift in the same slice.

- **Config + usage polish slice** (M25.1, shipped; one combined contract):
  (1) **Hid the unfinished budget surface.** `review.budget`
  (`mode`/`max_tokens`) gated nothing real (warn-mode plumbing only): removed
  from the config surface entirely — not generated, not accepted (loud
  leftover-key error, like the old `agent:` key), and the warn-mode review-loop
  code path deleted. Token accounting in the usage ledger stays untouched.
  Budget enforcement returns later as a designed feature
  ([cost-budget-design.md](cost-budget-design.md) is its home).
  (2) **Usage display polish.** `pactum usage --all` leads with the workspace
  summary, sorts runs by total tokens descending, and gained `--top N`.
  Uncaptured calls are marked explicitly (usage not reported by the agent)
  instead of zero-valued rows that mud the aggregates, plus an `effective_units`
  cost proxy and per-run `by_attempt` breakdown. Optional stretch (deferred):
  per-lens breakdown of the review stage.
  (3) **Map staleness pin narrowed to the `map:` section.** The map manifest
  previously pinned the SHA-256 of the whole `config.yaml`, so editing `agents:`
  or `review.panel` — which cannot affect map output — falsely invalidated the
  map (bit us live when swapping the review panel). It now pins a hash of the
  canonicalized `map:` section: map-parameter changes still invalidate,
  agent/review/panel edits do not; a legacy whole-file manifest is stale once.

- **Gate validation command parsing + negative-match semantics** (med). The
  gate runner tokenizes validation commands with `strings.Fields` — quote
  blind, so a quoted pattern argument shatters into garbage args (the M24.1
  dogfood hit this live: a quoted `rg` command exited 2 as a malformed
  invocation). And there is no way to express the common "this pattern must
  NOT appear" validation: `rg`/`grep` exit 1 on a clean tree, which the gate
  reads as failure. Fix both: shell-style quote-aware tokenization, plus a
  validation form for expected-no-match commands (or document that such
  checks belong in test suites the gate already runs).
- **`contract revise` removal parity** (small). Revise is append-only: a
  broken validation command (or stale scope entry) cannot be removed through
  the CLI — the M24.1 dogfood had to hand-edit `contract.json` and re-approve
  to drop an unrunnable validation command, which also orphaned the completed
  execution attempt (the approval SHA changed) and forced a re-execution.
  Add `--remove-validation`/`--remove-*` (by index or exact text) so contract
  repair stays inside the recorded pipeline.

- **Security truth in docs + SECURITY.md** (small, P0). README still describes
  the codex builtin as plain `codex exec` while `docs/agents.md` documents the
  real `codex exec --dangerously-bypass-approvals-and-sandbox` — the README
  must be honest exactly where a user first learns how agents run. Add a short
  `SECURITY.md`: threat model (pactum is not a sandbox; the repo and runtime
  environment are the boundary), safe-usage guidance (trusted repos, plan
  before run, path scope review), private reporting, supported = `main` until
  tagged releases exist.
- **govulncheck in CI** (M24.3, shipped). Pinned via the go.mod tool
  directive (mirroring deadcode), `make vuln`, and a separate blocking CI job
  so a slow vulndb fetch never delays the main check loop.
- **Committed run-record hygiene gate** (M24.3, shipped). `make
  heurema-hygiene` (`cmd/heurema-hygiene`) deterministically scans tracked +
  staged `.heurema` index content — git index, not the worktree — for
  absolute home-directory paths and credential-shaped strings, fails listing
  file:line + detector + redacted preview, and runs in the main CI check job.
- **`internal/app` decomposition arc** (med, after the CLI polish arc). The
  stage orchestration files have outgrown one package (`review.go` ~1.5k
  lines, `gate.go` ~0.7k, `contract_draft.go` ~0.7k): extract per-stage domain
  services (start with review), leaving `internal/app` as CLI binding + JSON
  envelopes. Pays off twice: human navigation and agent reads — smaller files
  sharpen contract path scopes, grep precision, outline quality, and cache
  locality (see the file-navigation arc above; splitting is its cheapest
  durable item).

- **Lens fan-out test flakiness under full-suite race load** (small). Three
  different review tests (`TestReviewRunStoresCrossReviewerAttempts`,
  `TestReviewLoopDedupsReproposedOpenFindingAcrossRounds`,
  `TestReviewRunCreatesIncrementingAttempts`) have each flaked once during a
  full `go test -race ./...` run while passing repeatedly in isolation
  (`-count=3 -race`) and on the package re-run. The M19.2 lens fan-out
  multiplied concurrent helper subprocesses per test, so under full-suite load
  timing assumptions occasionally slip. Diagnose the shared cause
  (helper-process sequencing or output-collection timing) rather than
  retry-masking it.

- **ACP shell-command / tool-call scope gating on write stages** (med). The
  M13.5 real-time write scope guard enforces the contract path-scope only at the
  `WriteTextFile` boundary (see [`agents.md`](agents.md)); an agent that writes
  through a *shell command* or other tool call it runs bypasses the guard, and
  such changes are still caught only by the post-hoc gate. M16.1 narrowed the
  remaining surface to the write stages (`execute run`, `review fix`): the two
  gaps that blocked making ACP the default are closed — model pins now reach the
  ACP adapters (codex `-c` overrides, claude env vars), and read-only stages
  (review, clarify suggest, contract draft) are enforced per leg: claude via the
  ACP client (write denial + permission rejection — claude routes writes through
  the client), codex via `-c sandbox_mode="read-only"` pinned on the adapter
  (codex applies patches natively, so client-side denials cannot stop it).
  M16.2 then flipped the default: ACP is now the hardwired default transport and
  `PACTUM_AGENT_TRANSPORT=cli` is the debug escape hatch, completing the ACP arc
  (M13.x→M16.2). This item is what remains of it. Closing it is deeper — it
  means intercepting/gating the adapter's command and tool-call requests on
  write stages, not just file writes — so it stays a documented limitation for
  now.
- **Consolidated ACP design note** (low, optional). The ACP transport, its
  real-time write scope guard, usage normalization, and cross-platform
  process-group reaping are currently described across `agents.md` and the M13.x
  milestone history; a single `docs/acp-transport-design.md` could collect the
  rationale and the known limitations (shell-command writes, no isolation) in one
  place. Nice-to-have, not blocking.
- **Clarify `answer` resets approval** (low). The silent-regression bug is fixed
  (M15.7): `clarify ask` / `answer` / `suggest` on an already-approved run now
  **warn** that they reset approval to pending (surfaced via `approval_reset` and
  a printed warning) instead of regressing to `clarifying` silently. Residual open
  question: `clarify answer` *resolves* a question rather than adding one, yet it
  still resets approval through the shared `refreshClarificationArtifacts` /
  `resetApprovalIfApproved` path — arguably answering an open (non-blocking)
  question should not regress an approved run. Decide whether to special-case
  `answer` (skip the reset when no new open question is introduced).
- **Committed `.heurema` run-record growth** (low, investigate). The durable run
  record is versioned and grows linearly with the number of dogfood runs. Not a
  problem today, but worth a proper look before it does. Baseline measured
  2026-06-09: tracked `.heurema` ≈ 774 KB for 36 runs (~18 KB/run, 15 files);
  the append-only ledger `events.jsonl` adds ~1.75 KB/run; the whole repo
  (89 PRs of code + 36 runs) packs to **1.0 MiB** because the near-identical JSON
  records delta-compress extremely well. Key finding: ~⅔ of a run is the contract
  stored three times — `contract.json` + `contract.md` + `prompt.md` (~12 of ~18
  KB), and `contract.md`/`prompt.md` are regenerable from `contract.json`.
  Mitigations to evaluate, by leverage: (A) stop committing the regenerable
  `contract.md` + `prompt.md` (selective `.gitignore`), cutting ~⅔ per run with
  no real loss; (B) prune old `runs/` dirs while keeping the ledger + memory as
  the live queryable summary (history retains the detail); (C) ledger-only (don't
  commit run dirs); (D) move the durable record to an external store (the
  SQLite/REST idea). Verdict: revisit when tracked `.heurema` crosses ~5 MB or a
  few hundred runs; (A) is the cheap proactive win if we want it sooner. Relates
  to the run-record batching policy (feature PRs stay code-only; run-records are
  committed in periodic `audit:` batches).

## Ideas (exploratory — parked to think about)

- **Pactum for DevOps / operations changes.** The contract-first model maps
  naturally onto operational work: the contract becomes a change plan whose
  in-scope items are the EXACT commands to apply (and the environments they
  target), acceptance criteria are post-state checks, and validation commands
  double as verification/rollback probes. Execution would then be allowed to
  apply only the planned commands — a command-level scope guard, the ops
  analogue of the path-scope write guard: instead of "which files may change",
  "which commands may run" (exact or templated allowlist), with anything
  outside the plan denied in real time and the durable `.heurema` record
  serving as the change-management audit trail (plan -> approve -> apply within
  boundaries -> verify). Open questions to think through before slicing:
  command matching (exact string vs templated args vs semantic equivalence),
  plan/apply parity (the terraform-plan analogy — how to dry-run a command
  plan), side effects outside the repo (cloud APIs) that the gate cannot diff —
  post-state checks as validation commands would have to carry the
  verification weight, and how this composes with the ACP shell-gating gap
  (the same mechanism that would gate agent shell commands on write stages
  could enforce a command allowlist).

## Resolved (for reference)

- Review-loop convergence slice 2 — severity-gated convergence (M12.9) — the loop now
  terminates with a new `resolved` terminal reason when no open **blocking** findings
  remain (`BlockingOpen == 0` — the same condition that makes a review approvable). The
  fixer is gated to run only when open blocking findings exist and its prompt partitions
  the findings into blocking (fix/rebut, emit an outcome) vs advisory (context only).
  Non-blocking findings are still accepted, deduped, and recorded but stay open as advisory
  — they never drive the fixer or keep the loop running, which kills the low/subjective
  churn from the M12.6 dogfood (a round whose proposals are all non-blocking converges
  `resolved` without invoking the fixer). The round summary gains `open_blocking_findings`;
  the reviewer prompt now instructs setting `blocking` meaningfully. Existing terminals
  (`clean_round`, `stalemate`, `max_rounds`, `gate_failed`, `budget_exceeded`,
  `reviewer_findings_unparsed`) are intact (verified: stub-finding tests still drive their
  terminals, not silently resolving). First dogfood executed with the **claude** executor.
  Slice 3 (anti-meta-churn reviewer prompt) remains. Open seam: the M12.4 panel severity
  upgrade bumps `severity` but not `blocking`, so a finding upgraded low→critical stays
  advisory — revisit whether high/critical severity should imply blocking.
- Review-loop convergence slice 1 — resolve-on-fix (M12.8) — the fixer now emits a
  structured `pactum.review_fix_outcomes.v1` fenced-JSON block (one outcome per finding:
  `fixed` / `rebutted` / `blocked`), parsed decoupled and best-effort (mirroring `review
  propose-findings`) by the new `review apply-fix-outcomes [run_id] [fixer_attempt_id]`.
  `fixed` and `rebutted` findings become resolved (with a new `outcome` field on the
  resolution record); `blocked` stays open. The autonomous loop applies outcomes after
  each fix round, so `open_findings` shrinks as work completes and the existing terminals
  (`clean_round` / `stalemate`) finally fire — a fully-fixed round lets the next reviewer
  round converge instead of churning to `max_rounds`. A finding resolved as `rebutted` (a
  false positive) is suppressed if a later round re-proposes the same `(file, line,
  message)`; `fixed`/manual resolutions stay re-acceptable (a re-raise may mean the fix
  did not hold). Best-effort: a missing/malformed block warns and never errors, so the
  loop still tolerates a fixer that emits no outcomes. Slices 2 (severity-gated
  convergence) and 3 (anti-meta-churn reviewer prompt) remain.
- Storage port (M12.6) — new leaf package `internal/store` (`Store` interface + an `FS`
  filesystem implementation byte-for-byte equivalent to the prior `os.*` calls). A
  package-level `activeStore` in `internal/app` routes the workspace durable-record I/O
  through the port: the JSON/JSONL/YAML primitives, the workspace `os.*` calls, the
  prompt-manifest removal (`store.Remove`), the run-dir reservation (`store.Mkdir`, kept
  non-recursive/fail-if-exists for the atomic claim), and all `ledger.Append` sites
  (`ledger.Append` gained a `store.Store` parameter). A map-backed in-memory `Store`
  swapped in a test proves the backend is swappable. Direct `os.*` is intentionally kept
  for non-record I/O: workspace discovery (`findUp`, `os.Getwd`, `filepath.Abs`), repo-tree
  reads, temp files, the project-map files (`search.sqlite`, `map/`), and transcript `*.log`
  (agent + gate-validation stdout/stderr, which stream live). Surfaced by review: the first
  manual adversarial pass + the dogfooded `gate` → cross-model `review loop` together caught
  inconsistent routing (run-local memory hashes → `storeFileSHA256`, attempt-dir checks →
  `storeDirExists`, prompt-manifest delete → `store.Remove`, map-file existence → direct
  `filesystemRegularFile`, run reservation → store) — all fixed. Opens the door (per the
  storage research) to a future regenerable, gitignored SQLite index for cross-run queries;
  no binary DB in git, no API server (YAGNI). Not ported: `internal/agents` transcripts and
  `internal/projectmap` (regenerable/transcript, out of scope).
- Cross-model review panel (M12.4) — `agents.review_panel` lists two or more reviewer
  agents; each autonomous review-loop round runs them CONCURRENTLY against the same
  diff/contract (goroutine fan-out, sequential per-attempt proposal parsing in panel
  order), then the existing `(file, line, message)` fingerprint dedup merges cross-
  reviewer duplicates (first accepted, rest recorded as `duplicate` decisions, so
  corroboration stays in the audit). Severity is reconciled to the max: a duplicate
  proposal that outranks the open finding upgrades it (`low<medium<high<critical`) and
  emits a `review_finding_severity_upgraded` event. An explicit `--reviewer` disables
  the panel; empty/absent `review_panel` is byte-for-byte the single-reviewer path.
  Concurrency is race-clean: a package-level mutex guards only the shared lifecycle
  sections (attempt-id allocation + mkdir, the events/usage ledger appends, the shared
  last-result write) so the agent subprocess still runs lock-free; concurrent reviewers
  share a synchronized live-output writer. Validated under `go test -race`. The N×
  per-round token cost is already bounded by the M12.2 budget stop. Deferred: an
  agreement-count field and severity-threshold gating (require K reviewers). Per-panel-
  member model pins shipped later with the M16.0 config redesign (`review.panel`
  entries carry their own `model`/`effort`).
- Token accounting — slices 1-2 (M12.0, M12.1) — executor/fixer agents and read-stage
  reviewer/clarifier/drafter agents now run with structured output (`codex exec --json`
  / `claude -p --output-format json`, with read-stage Codex kept read-only); the
  runner parses token usage per agent (best-effort, never fatal), normalized per
  [`cost-budget-design.md`](cost-budget-design.md), recorded as a `UsageRecord` to the
  per-run `ledger/usage.jsonl` via the shared lifecycle, and surfaced as tokens-per-task
  in `status` and a new `pactum usage` command. Tokens are the unit; cost/budget/
  estimation are later slices. (codex `--json` usage validated against real CLI output.)
- Blocking path-scope enforcement (M11.11) — `gate.scope_enforcement` defaults to
  `block`, so changed/new files that are undeclared by `paths_in_scope` or matched
  by `paths_out_of_scope` now produce `scope.status: blocked` and an overall
  failed gate. `gate.scope_enforcement: warn` preserves the M11.5 advisory warning
  behavior. In the autonomous review loop this composes with M11.8: the failed
  gate stops the loop with terminal reason `gate_failed` and records the gate
  report artifact for escalation.
- Project map honors `.gitignore` (M11.10) — in a git repo the scan enumerates files
  via `git ls-files --cached --others --exclude-standard`, so the repo's ignore rules
  are respected and build artifacts (`__pycache__/*.pyc`, `dist/`, …) are no longer
  indexed or churning the map; `.heurema` is always excluded; non-git dirs fall back to
  the filesystem walk. Fixes the real root cause of "spurious project-map staleness"
  on non-Go repos (found by the foreign-repo generality test; verified end-to-end:
  regenerating a `.pyc` no longer makes the map stale).
- `.heurema` version-control policy (M11.9) — `init` now writes a `.gitignore` that
  versions the durable run record (contracts, decisions, the ledger/audit timeline,
  gate verdicts, review findings, memory) and ignores only regenerable artifacts
  (`map/`, `cache/`, `runs/*/context/`) and raw `*.log` transcripts. The outcome lives
  in git history and the learnings in memory, so the bulky agent transcripts are not
  committed. Found via the foreign-repo generality test, which showed the old split
  committed generated context + gate logs while ignoring the ledger.
- `-race` in CI (M11.7) — a `make test-race` target (`go test -race ./...`) plus a
  dedicated `race` CI job run the race detector on every PR, catching the data-race
  class (e.g. the M10.2 live-output race) that a non-race `make check` misses. Local
  `make check` stays fast; the race run is CI/pre-merge only.

- Gate-failure-in-loop policy (M11.8) — a fixer-induced failed gate now stops the
  autonomous review loop with terminal reason `gate_failed`, records the failed
  gate status and report artifact in the round summary, and returns cleanly for
  escalation. Infrastructure gate errors still terminate as `error`.

- Idle agent timeout (M11.6) — `--timeout` is now an idle (no-output) safety
  timeout: a subprocess is killed only after the window passes with no stdout/stderr
  (default 10m then, 25m since M20.2), so long actively-streaming runs are never cut mid-work; the fixed
  total wall-clock cap is removed. Channel-based watchdog, race-clean.
- Legacy `default_executor` / `default_reviewer` / `adapters` config removed (#44) —
  intentionally ignored, not a bug.
- `agents doctor` status `ready` → `on_path`, `clarify list` alias, log-channel docs (#38).
- Per-stage `model[:effort]` + resolved-config header (#39, #41, #42); cross-model
  review (#43); fix stage (#46); review loop driver (#47); live agent output (#49);
  loop stop conditions — stalemate + K-consecutive-clean (M10.3); clarify suggest
  (M11.0); contract drafting (M11.1).
- Lifecycle dedup (#53) — the five agent-run commands (`execute run`, `review run`,
  `review fix`, `clarify suggest`, `contract draft`) now share one attempt-lifecycle
  helper (`runAgentAttemptLifecycle`); behavior-preserving, net −137 LOC (M11.2).
- Dead-code gate (M11.3) — `make check` now runs `go tool deadcode` (golang.org/x/
  tools, pinned via the go.mod `tool` directive), which flags unused package-level
  functions including production code reachable only from tests — the class `go vet`
  misses (and how the M11.2 refactor left orphaned wrappers). Removed the 7 dead funcs
  it found; tree is clean and the gate keeps it that way.
- Review-loop correctness (M11.4) — round-summary `open_findings` now reports the live
  open-finding count (was the per-round accept count); redundant `total_open_findings`
  removed. The loop dedups a re-proposed currently-open finding by exact
  `(file, line, message)` instead of minting a duplicate finding — it records a
  `duplicate` proposal-decision + one `review_proposal_duplicate` event. Resolved/
  rejected re-proposals are not suppressed; the M10.1 unparsed-findings guard is intact.
- Mechanical path-scope warnings (M11.5) — contracts can now carry
  `paths_in_scope` / `paths_out_of_scope` slash globs, `contract revise` can append
  them, and the gate reports advisory warnings for changed/new files that are
  undeclared or explicitly out of scope. Hard-fail/blocking behavior is intentionally
  left as a separate follow-up.
