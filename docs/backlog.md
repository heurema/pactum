# Backlog

Deferred work and known follow-ups, consolidated from dogfood reports and PR
reviews across M8–M10. Rough priority in parentheses.

> **Plan-DAG reverted (2026-06-19).** The plan-DAG arc (a contract carrying
> `plan.tasks[]`, executed node-by-node in a topological loop) was built (slices
> #194/#198/#200/#203/#205) then **removed in full (#207)** after deep research +
> the GO/NO-GO run showed the cold per-task fresh-context loop is a documented
> anti-pattern (it *manufactures* cross-task incoherence; caching already solved
> cost; single-shot built every slice). **Execution is now single-shot: one
> contract = one coherent session.** Wherever an item below proposes or relies on
> the plan-DAG as a cure (notably the token-efficiency "#3 plan-DAG" lever and the
> contract-review convergence item), treat that framing as obsolete — the
> decomposition lever now lives at the **contract level** (the drafter emitting
> sequential contracts for oversized work, a future separately-designed
> capability). See `docs/contract-plan-dag-design.md` (REVERTED header) and the
> deep-research synthesis. The reviewer-findings-capture fix (#196) was kept.

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
  narration-grep as evidence. **Reproduced live 2026-06-19:** asked whether the
  executor actually uses the provided map / `pactum search`, the only per-attempt
  artifact was the agent's PROSE narration in `stdout.log` ("read X, read Y") —
  no structured record of which files it read, whether it ranged-read, or whether
  it ever invoked `pactum search`. Narration-grep could not answer it. This gap
  now blocks a question we actively care about (map/search adoption by the
  executor and reviewer), so it is a prerequisite for measuring whether the map
  we generate is exercised at all — strong candidate to pull forward in priority.
- **Publish the codex-acp fork to npm under `@heurema` (keep it until upstream
  ships per-turn usage)** (med — distribution; decided 2026-06-19, Option C).
  Codex token-count capture needs the breakdown (`input`/`output`/`cached`/
  `reasoning`), which pactum reads from `PromptResponse.Usage`. Upstream codex-acp
  0.16.0 does NOT populate that yet (proven live: a `contract draft` via upstream
  `npx @zed-industries/codex-acp@latest` → `pactum usage` `captured_records: 0`);
  its `usage_update` session notification (`SessionUsageUpdate`) carries only a
  **context-window gauge** (`used`/`size`), NOT the per-turn breakdown — so
  reading `usage_update` does NOT replace the fork for cost. The
  `heurema/codex-acp` fork (`feat/acp-prompt-usage`, "Forward Codex token usage in
  ACP prompt responses") forward-ports the populate-`PromptResponse.Usage` patch
  (upstream PR #210 / issue #165), which pactum already reads. **Decision:** keep
  the fork and **publish it to npm under `@heurema`** so alpha users install it as
  easily as upstream (`npx -y @heurema/codex-acp`). The fork already carries the
  full upstream npm toolchain — `release.yml` (8-target Rust cross-compile matrix
  + npm publish, manual `workflow_dispatch`), the `bin/codex-acp.js` platform
  shim, `npm/template/package.json`, and `npm/publish/*.sh`. Publish work (in the
  fork repo, not pactum): (1) rebrand `@zed-industries` → `@heurema` across
  `npm/package.json` (name + 6 platform `optionalDependencies`), the shim, the
  template, the publish scripts, and `release.yml`; (2) create the `@heurema` npm
  scope + an automation `NPM_TOKEN` GitHub secret; (3) for alpha, **skip the
  Apple/Azure code-signing steps** (upstream's certs we don't have) — unsigned
  binaries are acceptable for the trusted dev circle (note the macOS Gatekeeper
  prompt); (4) `workflow_dispatch` → builds the platform binaries + publishes the
  7 packages. Then wire pactum: document/default `PACTUM_CODEX_ACP_COMMAND="npx
  -y @heurema/codex-acp"`. **Retire trigger:** when upstream ships
  `PromptResponse.Usage` in a published release, drop the fork by bumping the
  codex adapter back to upstream — zero pactum code (pactum already reads it);
  update [[codex-usage-fork-adapter]] then. Separately, reading `usage_update`
  as a context-window/budget gauge is its own optional feature (not fork-related;
  see `cost-budget-design.md`).
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
  `clarify/loop-summary.json` (`pactum.clarify_loop_summary.v1alpha1`); `contract
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

## Loop engine + uniform pipeline + effort (panel-designed)

The config (`pactum.config.v1alpha1`) was a flat mush — registry, map, gate, and
two *different* review loops (`review.panel` vs `contract.reviewers`) sitting flat
— and underneath it the code has **no shared loop abstraction**: three hand-rolled
`for round := …` loops (`clarify_loop.go`; `review_loop.go` 1182 lines;
`contract_review.go` 1176 lines), where `review_loop`/`contract_review` are
near-duplicates sharing only data types — ~1200 lines of copy-pasted orchestration.
Three three-voice design panels (codex gpt-5.5/xhigh + two opus, distinct lenses)
converged on one unification: **every stage is a bounded loop over a set of
performers**; the config is that abstraction's reflection; sane defaults live in
code. Stay on `v1alpha1`.

### The loop engine (the missing abstraction)

One tiny shared engine owns *only* the round counter and the stop machine; all
domain work lives in each stage's `step`. It is blind to performers and to the
domain:

```go
type Limits struct { Max, Patience, Settle int }   // per-stage defaults in code

type HumanExit struct { Reason string }
type RoundResult struct {
    Clean    bool        // this round met its convergence predicate (no findings)
    Progress bool        // durable state changed (a fix applied)
    Human    *HumanExit  // exit to a human now
    Summary  string      // ledger text only; not read by the stop machine
}
type Outcome struct { Reason string; Rounds int; Last RoundResult } // settled|stalemate|max|human

type Step func(ctx, RoundContext) (RoundResult, error)
func Run(ctx, Limits, Step) (Outcome, error)  // Clean→clean-streak (settle);
                                              // !Clean&&!Progress→stale-streak (patience);
                                              // a Clean round leaves the stale-streak UNCHANGED
                                              // (streaks decoupled); Human→exit; Max→ceiling
```

Boundary: the engine reads three signals and counts streaks — nothing else. `step`
owns fan-out to performers, MERGE/SELECT combine, fixer, lenses, domain work. The
two streaks are **decoupled**: a `Clean` round grows the clean-streak and leaves the
stale-streak *unchanged* (the stale-streak resets only on `Progress`, increments only
on `!Clean && !Progress`) — the semantics the contract-review panel caught and
corrected before this shipped. A single-shot stage (`contract_draft`, `memory`) is
just a `step` run once via `Run{Max:1}` — uniform stamping path, no ceremony. Porting
`review_loop`+`contract_review` onto `Run` deletes the ~1200-line duplication.

**Shipped (#172):** `internal/loop` (the engine + `TestDepsIsolation` stdlib-only
guard) and the `contract_review` port (incl. the previously-missing
`contract_review_loop_started`/`_finished` ledger events). Dogfooded through pactum;
the executor (Sonnet) ran through a mid-task context compaction and still passed the
deterministic gate. **Remaining in this first slice:** the `review_loop` port (the
dup-collapse) and the new `pipeline`/`by`/`loop` config + resolver. Two slices:

- **Config restructure — the `pipeline`** (med). Rename the version field
  `schema:` → `version:` and drop the type prefix (`version: v1alpha1`, not
  `schema: pactum.config.v1alpha1`) — the kind is obvious from the filename. Same
  for the standalone **contract** file. Do **not** flatten it on streamed records
  (ledger rows, the 60+ `pactum.<kind>.<ver>` artifacts): there the qualified value
  is a type discriminator *and* each kind versions independently (`pactum.map.v2`
  is already v2) — keep `schema:` there. Top level: `version`, `agents` (the
  registry), `map`, **`out_of_scope: block|warn`** (the former
  `gate.scope_enforcement` — drop the `gate:` wrapper; the name says what it does:
  out-of-scope edits are a hard fail or a warning), and `pipeline`. No `settings:`
  wrapper. An optional flat **`timeout`** override may sit top-level (default idle
  25m lives in code — omit from the shipped config).

  `pipeline` is a **stage→`{by, loop?}`** map naming every agent-invoking stage
  (`clarify, contract_draft, contract_review, execute, code_review, memory`).
  **`by:`** is who performs the stage — scalar sugar or list, normalized to
  `[]string`; one uniform field, no `agent:`/`reviewers:`/`panel:` split (this kills
  the old scalar/list/object polymorphism — "просто списки does not feel good").
  **`loop:`** `{max, patience, settle}` is the *only* per-step settings block,
  optional (omitted → per-stage code defaults). No `enabled:` (stages mandatory),
  no per-stage `fixer`/`lenses` (derived/hardcoded: `code_review` fixer = `execute`
  agent, `contract_review` fixer = `contract_draft` agent). `gate` is **not** a
  `pipeline` stage — it runs validation commands + the scope check, invokes no
  agent (the honest exception to "every stage has a performer set"). `pipeline`
  **order stays code-owned** — it is NOT a user-editable DAG (that is the separate
  *Contract plan DAG* arc).

  **Multi-model on any step, two strategies — intrinsic in code, never a config
  field.** `by: [a, b]` is uniform *syntax* everywhere, but its *meaning* is the
  stage's intrinsic strategy: **MERGE** (clarify, both reviews, memory — performers
  fan out, outputs union; additive, like a review panel) or **SELECT** (`execute` —
  N candidates, best-of-N, one survivor; multiplicative N× cost). Do **not** call
  SELECT "like review": MERGE keeps all N, SELECT discards N−1. Build the one cheap
  honest selector now — **`execute` best-of-N by the gate it already runs**: N
  candidates each in their own worktree, winner = the **first to pass the gate in
  `by:` declaration order** (declaration order is the deterministic, reproducible
  tiebreak, not wall-clock); zero pass → the round fails (the stop machine handles
  it, no fallback); the first cut runs candidates sequentially with short-circuit on
  first pass. `contract_draft` stays **single** (no oracle to rank drafts —
  multi-`by` is a load error until a draft selector exists). No `selector` /
  `max_parallel` / `must_pass` config (each would leak `strategy`); parallelism is
  code-owned until proven needed.

  Validation (minimal — no per-stage knob whitelist): normalize `by:` → `[]string`,
  reject **unregistered agents**; one code-owned predicate `stageHasSelector(kind)`
  makes `len(by) > 1` a **load error** only where no multi-performer impl exists
  (MERGE: always ok; `execute`: ok via gate; `contract_draft`: error). Reject
  **incoherent `loop:` values** (`patience` on a `Max:1` stage, `settle > max`,
  negatives) — a coherence check on the block itself, not a type whitelist. Empty
  `by:` is **invalid**, not "disabled" (explicit stage-disabling is designed later).
  The engine is unchanged whether `by:` is length 1 or N — fan-out + combine live in
  `step`, so SELECT needs zero engine changes. **Run-record stamping** (in the
  caller — the engine is performer-blind): resolved performers + `Limits` +
  `Outcome` per stage; for a SELECT `execute` round, additionally **all N candidate
  attempts + the winner + why it won**, or the run is not reproducible. `backoff`/
  retry is a *different* state machine (error-driven, not convergence-driven) — it
  stays out of `Limits`, inside `step` or a separate `retry.Do`, when
  `execute`-retry lands (see *Executor resilience*).
- **Effort resolution + validation + stamping** (small-med). Resolve effort as a
  two-link chain: `agent.effort` (if set) → **code per-engine default**
  (`claude=high`, `codex=high`) → send a concrete value. There is **no
  config-level `inherit`**; send-nothing survives only as an internal adapter
  detail for an engine with no effort knob at all. Per-engine defaults live in
  **code** (not config — a single cross-provider scalar cannot exist: claude
  vocabulary is {low,medium,high,xhigh,max}; codex has no `max`). Validate any
  non-empty effort against the resolved engine's vocabulary **at config load**
  (reject e.g. `max` on a codex agent) — do **not** invent a normalized pactum
  vocabulary (lossy), and do **not** pass through blindly (turns a
  load-time-catchable typo into a late execute-time failure). Stamp
  `{model, effort, effort_source}` on every agent attempt; `effort_source ∈
  {agent, engine_default}` is the whole audit story. The run record also stamps
  the **resolved** lenses/rounds/timeout that actually ran, so the ledger stays
  self-describing when the code constants later change. Defer: a `pactum config
  resolve` effective-config dump, and per-*model* (not just per-engine) effort
  capability tables.

Target shape — common case (every stage is `{by}`; `loop:` omitted → code defaults):

```yaml
version: v1alpha1
agents:
  - {name: sonnet,      model: claude-sonnet-4-6, effort: max}
  - {name: codex-xhigh, model: gpt-5.5,           effort: xhigh}
  - {name: opus,        model: claude-opus-4-8}          # no effort → code default
  - {name: codex,       model: gpt-5.5,           effort: high}
map: {max_file_bytes: 500000, code_index: auto}
out_of_scope: block
pipeline:
  clarify:         {by: opus}
  contract_draft:  {by: codex-xhigh}              # SELECT, single only
  contract_review: {by: [opus, codex-xhigh]}      # MERGE panel
  execute:         {by: sonnet}
  code_review:     {by: [codex-xhigh]}            # MERGE panel
  memory:          {by: opus}
```

Tuned — a review with loop overrides, and `execute` as a best-of-N panel (the new
non-review array: N× cost, gate-selected, first-pass-in-`by`-order wins):

```yaml
pipeline:
  code_review:
    by: [codex-xhigh]
    loop: {max: 12, patience: 3, settle: 2}
  execute:
    by: [sonnet, codex]
```

Implementation note: the per-stage `by:` agents must reproduce today's resolved
role-resolution assignments — verify against `run.go` before hardcoding, so the
restructure changes config *shape* without changing *which agent runs where*.

**Decision (MVP scope): the first slice ships the config rework AND the loop-engine
extraction together** — not a config-only cut first. A config-only MVP (rework the
shape, keep the three hand-rolled loops as-is) was considered and rejected: the
engine *is* the point — it collapses the ~1200-line `review_loop`/`contract_review`
duplication and fixes `contract_review`'s missing ledger events, which a config-only
cut would leave on the table. So the config reflects a real abstraction from day one,
not a reshaped façade over the old loops.

First slice: extract `internal/loop` (`Run`/`Limits`/`RoundResult`/`Outcome` +
table tests for settled/stalemate/max/human/error); port `review_loop` **and**
`contract_review` onto it (behaviour unchanged — and **add the ledger events
`contract_review` currently lacks**); add the new config (`version` / `agents` /
`map` / `out_of_scope` / `pipeline.<stage>.{by, loop?}`, `schema → version` rename)
and resolver, exercised by the two reviews; stamp resolved performers + limits +
outcome in the caller. `loop:` is valid only on the loop stages
(`clarify`/`contract_review`/`code_review`); `by:` lists only where a panel exists
today (the two reviews) — no new multi-model capability in this slice. Then
`execute` best-of-N-by-gate (worktree isolation, declaration-order tiebreak, stamp
all attempts + winner). **Deferred, config-ready-but-unbuilt:** `contract_draft`
multi-model, `execute`-retry/`backoff`, `clarify`/`memory` migration onto `Run`,
explicit stage-disabling, and a `pactum config resolve` effective-config dump.

## Judge selector — best-of-N where no oracle (panel-designed; DEFERRED, gated on measurement)

A three-voice panel (codex gpt-5.5/xhigh + two opus) evaluated "use select-best-of-N
*everywhere* — a stage could merge or select, whichever is better." Verdict: the
instinct points at a real abstraction but "everywhere" is wrong, and the one
valuable narrow cut should **not** be built now. Recorded so the analysis isn't lost.

**The abstraction (when/if built).** A SELECT stage owns an intrinsic, in-code
`Selector` (never a config field): `Select(candidates, evidence) → Selection`. Two
kinds: **OracleSelector** — objective, deterministic, dominates where available
(`execute` → the gate; winner = first gate-passer in `by:` order; already covered by
the loop-engine slice above). **JudgeSelector** — a code-resolved LLM role ranks
candidates for SELECT stages with **no** oracle (`contract_draft`); isolated from the
`by:` performers (never grades its own work), blind to candidate identity where
feasible, given the stage rubric + all candidates; returns winner + ranking + rejects
+ confidence + rationale. A **single** judge, not a panel (cost-on-cost; "who judges
the judges"; a downstream review panel already converges quality). `execute` may
later add **judge-refine among gate-passers** (gate filters to *correct* diffs first,
then a judge ranks for quality — gate-filter-first is non-negotiable and is why
execute's judge is safe while draft's has no floor).

**Why it is mostly NOT worth it.** Best-of-N buys only a better *starting point*, and
each whole-artifact stage already has a downstream convergence loop whose job is to
erase starting-point variance (`contract_draft` → `contract_review`; `execute` →
`code_review`). It pays only when candidate variance is high **and** the loop is
weak/short/expensive **and** a reliable selector exists. Asymmetry: a wrong contract
*skeleton* is load-bearing (the review fixer converges within a draft's frame, rarely
re-decomposes it), so `contract_draft` is high-leverage — but it has no oracle, so a
judge can confidently pick a **worse** draft, paying N× to possibly demote a good one
(net-negative). `execute` has the gate (objective, free, deterministic) so its
best-of-N is the only clearly-worth-it case, and it is already in the engine slice.

**Determinism caveat (must advertise, not hide).** An OracleSelector run is
*replayable* (declaration-order tiebreak); a JudgeSelector pick is *nondeterministic*
— stamping all candidates + selector kind + admissibility/gate evidence + judge
role/rubric-version + decision + ranking + rationale + fallback gives
**auditability, not replayability**. Tie / judge-refusal / all-bad → deterministic
fallback to `by:` order (judge refusal may retry once with a narrower prompt, then
fall back); all-invalid → fail the stage.

**Merge stays merge.** For composable outputs (reviews, clarify, memory) MERGE
strictly dominates — select drops a real finding (A finds bug X, B finds bug Y → union
{X,Y}; select keeps one). The right enrichment for MERGE stages is **dedup +
severity/confidence ranking + contradiction handling *within* the merged set**, never
select. (This dedup/rank-within-merge is itself a small, independent, buildable
improvement to the review combine.)

**Config impact: none.** `by: [a, b]` stays the only opt-in; selector kind, judge
identity, and refine-on/off are all intrinsic in code. No `strategy`/`selector`/
`judge`/`refine` field ("the first knob is the leak"). Unlocking multi-`by` on
`contract_draft` is *removing* its load-error, not adding a knob. Run records expand,
not config.

**Recommendation.** **Do not build judge-selection in Phase-0** — it optimizes a
baseline that does not yet exist (still validating a weak executor works at all), and
the downstream loops should carry quality meanwhile. Keep `execute` oracle best-of-N
(already in the engine slice). Smallest *future* cut, gated on measured evidence that
single-draft + review wastes real rounds or yields worse contracts: `contract_draft`
multi-`by` with one code-resolved judge + admissibility filtering + deterministic tie
fallback + full run-record stamping. **Never:** universal select-best, configurable
selectors, judge panels by default, select on any MERGE stage.

## Token-efficiency optimization (research DONE → [token-efficiency-research.md](token-efficiency-research.md))

**The research has been run** (web deep-research + gpt-5.5 reasoning + an empirical
measurement of pactum's own usage ledger) — distilled in
[`token-efficiency-research.md`](token-efficiency-research.md). The empirical finding
**overturned the headline priority**: pactum already gets **79–97% cache-read**
across its fresh ACP sessions (measured), so prompt caching — the literature's top
billable lever — has almost no headroom for us, and the "fresh-session kills caching"
worry is empirically false. The input-heavy profile is *misleading*: ~94% of input is
cheap cache-read (0.1×), so effective cost is ~85% below the raw token count. **Revised
top priorities:** (1) **surface cache-adjusted cost in `pactum usage`** (`cache_read_ratio`
+ `effective_units`, already captured but not shown — we have been reading the
scary-but-cheap raw number); (2) **context-pack phase before execute** — hand the
executor file/line-range *evidence* (it ignores the map and re-explores), the top
*raw-token* lever; (3) **plan-DAG with complexity-gated granularity** (3–10 leaf tasks,
not microtasks) — valued for quality/convergence + small per-task fresh input, NOT as
a cache-cost fix (caching is solved); (4) model routing at task boundaries; (5)
workflow-shape the deterministic stages. The scary "decomposition costs 4–10× tokens"
claim was *refuted* in verification. The remaining recorded angles + sources are in the
doc. **Original framing (kept for the record):**
**Goal: minimize tokens spent for a given output quality, across BOTH contract
composition and task execution.** Prereq: `pactum usage` + codex capture fixed, so
optimizations are measurable.

Research angles (to investigate with primary sources when run):
1. **Prompt caching** (Anthropic `cache_control` / ephemeral cache): structure the
   stable prefix (system + contract + code context) so it is cached across the many
   sub-agent calls — reviewers per lens, fixer, every round — that currently re-pay
   full input each time. Likely the **single biggest lever** given the input-heavy
   profile. Quantify savings, TTL (5-min cache window), and the prompt-structuring
   constraints (stable prefix first, volatile suffix last).
2. **Context selection / minimal-sufficient context**: how much code to feed vs
   retrieval/relevance; the cost of over-context; repo-map/code-index targeting; the
   trade between fewer input tokens and weak-model success.
3. **Workflows vs autonomous agents** (Anthropic "Building Effective Agents"):
   prescriptive workflows are cheaper/deterministic than open-ended agents; where
   pactum's pipeline can be more workflow-shaped (less re-deciding) to cut tokens,
   and when autonomy actually earns its cost.
4. **Model routing / right-sizing per stage**: a cheap/small model for cheap
   subtasks (clarify, memory, trivial fixes), the strong model only where it pays;
   the economics of which stage needs which tier.
5. **Reducing redundant re-reads**: fresh-session-per-actor re-reads context
   (reproducible but expensive) — can caching / scoped handoff / a shared cached
   prefix cut it without losing the read-only↔write boundary; Batch API for the
   parallelizable reviewer fan-out.
6. **Token-efficient contract composition**: convey the spec in the fewest tokens
   while staying executable by a weak model (ties to *contract-sizing* + the plan-DAG
   arc); structured vs prose; compression without losing precision.
7. **Empirical cost-vs-quality**: known token-cost/quality Pareto data (SWE-bench
   cost-per-solve and similar); budget-bounded agents; where diminishing returns hit.
8. **The API-metered future**: cost as a first-class metric, budget caps, the
   industry optimization trend — and what that implies for pactum's defaults.

Once run, the research feeds concrete pactum changes (cache the contract/context
prefix, scope context, route models per stage, workflow-shape the pipeline) — each
measurable against `pactum usage`. **First step: write the deep-research brief from
these angles; do not start optimizing blind.**

**Concrete observation (data, not hypothesis) — the executor ignores the project
map.** We feed the executor a *lean* `executor-context.md` (~47 lines): the map as
**pointers + retrieval guidance** (`repo-map.md` / `llms.txt` / `search.sqlite` paths
+ "use `pactum search` before adding code"), not the map dumped inline — good token
hygiene. But the executor (Claude Code over ACP, an **external agent with its own
`Read`/`Grep`/`Bash`**) treats "use `pactum search`" as a weak suggestion and
**ignores it**: in `run_20260617_115334`'s execute attempt, **0 references** to
`pactum search` / `repo-map` / `llms.txt` / `search.sqlite` — it re-explores the repo
from scratch with its own tools. So the map's indexing cost (build + `map refresh`)
is **not leveraged at execute time**; the executor re-reads, which is a chunk of the
input-heavy profile. Implication for the research: the map earns its keep for *our
pipeline* (search-context derivation, reviewer context) more than for the *executor's*
navigation — so executor-side token savings likely come from **prompt-caching the
stable prefix** (angle 1) and **scoping context** (angle 2), NOT from harder-selling
the map to an agent that won't use it. Quantify: how much of `execute` input is
re-exploration the map could have served vs genuinely-needed file content.

## Hardening / cleanup

- **Executor resilience: auto-retry on transient failure + resume (not reset)**
  (med). Today a model drop (rate_limit / network / 5xx) or idle-timeout during
  `execute` (or any agent stage) is recorded as a failed attempt and the operator
  must re-run by hand; a partial execution leaves a half-edited working tree that
  the gate/review catch but nobody auto-cleans. Two parts:
  - **Auto-retry transient failures** with backoff — distinguish *transient*
    (rate_limit/network/timeout → retry) from *terminal* (the agent ran fine but
    produced wrong code → not retry; that is review's job). Seen live: codex
    rate-limit on execute and fable rate-limit on review (the latter now handled
    by graceful degradation #154; the executor path is not).
  - **Resume, not auto-reset.** When an attempt fails partway, the next attempt
    should *continue from the partial working tree* rather than redo from scratch
    (which throws away real work + burned tokens). The pieces already exist: the
    partial working tree persists (git never auto-resets it), `git diff` is the
    "what's already done" signal, the frozen contract is the target, and the
    prior attempt's `stdout.log` is the trajectory. A resume attempt feeds the
    agent the contract + the current diff ("partial progress is already on disk;
    finish the remainder, don't redo done work") and the per-task `validation`
    tells it when it's complete. Offer an explicit `--fresh` to discard the
    partial tree when it is garbage (resume by default, reset on request — the
    safe-default/opt-in polarity).
  - **The plan DAG makes resume principled** (see
    [contract-plan-dag-design.md](contract-plan-dag-design.md)): commit-per-task
    + the unhashed `tasks-state.json` ledger mean re-running the loop picks up at
    the first not-done task automatically (completed tasks' commits are in git).
    So fine-grained resume is another argument for the DAG; the single-shot
    version above is the coarse interim.
  - **Context-compaction policy (research-backed).** A deep-research pass (Liu
    lost-in-the-middle; NoLiMa — 11/13 models <50% at 32K; Chroma "context rot";
    Anthropic & Cognition context-engineering) established with *high* confidence
    that long/summarized context measurably degrades coding agents and a *fresh
    focused* window beats a long/compacted one holding the same info; compaction is
    lossy and irreversible. No source A/B-tested compacted-vs-fresh for *coding*
    specifically, so the policy is well-motivated inference (*medium* confidence),
    not a measurement. The policy: **(1)** let the executor finish even through a
    compaction; **(2)** judge by the deterministic gate **and** the code-review
    panel — the panel encodes the contract beyond what `build/test/lint` check, and
    `build/test/lint` alone miss *silent spec-drift*; **(3)** on gate/review failure
    **OR** ≥2 compactions **OR** observed forgetting-signs, don't blind-continue and
    don't cold-wipe — spawn a **fresh** executor that *resumes from disk + the
    re-readable contract* (a fresh focused window, the resume above); **(4)** the
    verifier must encode the acceptance criteria so the gate can catch spec-drift,
    not just compilation. Validated live in #172: a Sonnet executor ran through a
    mid-task compaction and shipped a correct, in-scope engine *because* the
    two-layer verification (gate + the contract-encoding review panel, which caught
    two spec-drift bugs the tests missed) held — not because compaction was harmless.
  - **`PACTUM_CODEX_ACP_COMMAND` in the gate environment fails the adapter tests
    deterministically** (small — was mis-filed as "flaky gate"). Originally
    observed in `run_20260617_115334` (the `pactum usage` slice) and again during
    plan-DAG slice 1: the in-loop gate hit `gate_failed` while the full suite was
    green when run by hand. **Root cause found — it is not flaky.** When
    `PACTUM_CODEX_ACP_COMMAND` is exported (the codex-usage fork override), the
    adapter resolves the codex command to the fork path, so the tests that assert
    the *default* `npx @zed-industries/codex-acp` invocation fail every time:
    `TestExecutePlanAppliesExecutorModelConfigToCodex`, `TestExecutePlanExplicitCodex`,
    `TestExecutePlanJSONOutput`, `TestExecutePlanPinAppliesOnlyToMatchingAgent`,
    `TestAgentsDoctorSelectedBuiltIn`. The "green before and after" was simply runs
    *without* the env. **Operational fix (now in practice): never set the fork env
    on gate / `go test` / `review run` steps** — it is only for live codex execution
    where usage capture matters. **Code fix (do): make those tests env-robust** —
    either clear `PACTUM_CODEX_ACP_COMMAND` inside the test (t.Setenv to empty) or
    assert against the resolved adapter command rather than hard-coding the default,
    so a developer with the fork env exported in their shell still gets a green
    suite. Separately the loop could re-run the gate once on `gate_failed` and only
    terminate on a *reproducible* failure, but the adapter-test case is a real
    deterministic failure, not noise — fix the tests, don't paper over with retries.

- **codex reviewers inconsistently emit the structured findings envelope → the
  review loop captures 0 findings while real ones exist** (RESOLVED #196 —
  force-the-format: the reviewer block is mandatory + always emitted, a missing
  block triggers a corrective retry then a loud `reviewer_findings_unparsed` stop,
  per-lens; kept after the plan-DAG revert as it's independent of the DAG).
  Observed live in plan-DAG slice 1's code-review (`run_20260617_182429`): all five
  codex lenses (correctness, implementation, tests, docs, over_engineering)
  reasoned out **real, confirmed** findings in their stdout — a validation-gating
  hole, a lossy plain-text renderer, three test-quality gaps, and a docs miss — but
  only the `docs` and `tests` lenses wrapped them in the
  `pactum.reviewer_findings.v1alpha1` JSON block. The others reported findings as
  prose only, so the loop's structured sink aggregated **0 findings** and
  `findings.jsonl` was empty, leaving the review trivially "clean" and approvable
  despite four real issues. An operator had to read the raw stdout to recover them.
  This is a silent false-negative in the review pipeline — the inverse of a flaky
  gate. Options: (a) make the reviewer prompt's "emit exactly one JSON block"
  instruction load-bearing and **reject/retry an attempt that produces prose
  findings with no JSON envelope** (detect "Findings:" prose + no parsed block);
  (b) add a cheap extraction fallback that parses the prose findings when the JSON
  block is absent; (c) at minimum, **warn loudly** when a reviewer attempt exits 0
  but yields no parsed findings *and* its stdout contains finding-like prose, so the
  gap is never silent. Ties to the codex-adapter usage gap — codex-over-ACP is the
  weak link in structured-output fidelity.

- **Proactive fresh-context restart (pre-compaction), configurable** (feature —
  record only, not yet designed/built). The *reactive* policy above resumes after a
  failure/compaction; this is its *proactive* complement: don't let the executor
  reach auto-compaction at all. Watch the agent's context fill level and, **before**
  it crosses a configurable threshold, checkpoint and **restart the agent with a
  fresh window** — resuming from the externalized state (on-disk partial work + the
  re-readable contract), exactly the resume path above. Motivation is the same
  research: a fresh focused window beats a long/compacted one, and compaction is
  lossy/irreversible — so avoiding the compaction entirely (rather than recovering
  from it) is strictly better when the state is externalized. **Configurable:** a
  threshold (e.g. fraction of the context window, absolute token budget, or turn
  count) plus on/off, so an operator can tune how aggressively to recycle the
  window — or disable it. Open mechanism question (for the design pass, not now):
  how pactum *observes and controls* the agent's context level over ACP — Claude
  Code auto-compacts internally, so proactive restart means either pactum kills the
  ACP session at the threshold and opens a fresh one with a resume prompt, or the
  underlying agent is configured to surface fill level / suppress auto-compact and
  signal pactum. Depends on the resume-from-disk capability landing first.

- **`operator` field in the trace** (small). Record *who drove* each agent
  invocation — the principal that ran the pactum command — distinct from `agent`
  (the model that did the work) and `role` (the stage). In the human → agent →
  pactum → agent model this captures the middle layer (which human / orchestrating
  agent / CI driver initiated the step), which the records don't show today. Reuse
  the existing `--by <principal>` concept (already recorded on approve/accept,
  default `manual`) rather than a parallel mechanism: thread it through every
  agent-invoking command (draft / execute / review / contract review) into
  `usage.jsonl` and the per-attempt records as `operator`. It folds into the
  usage-accounting schema in [contract-plan-dag-design.md](contract-plan-dag-design.md)
  (the row already carries `role`/`agent`/`model`) and is orthogonal to the DAG
  ledger's `by` (which is the task's *executor*, not its *driver*). Honest scope:
  the operator is **self-reported** (pactum cannot infer its caller) — attribution
  for audit, not a security boundary; document that so the field is not mistaken
  for a trust signal. In the same pass, **rename the trace `role` field to
  `stage`** — `executor`/`drafter`/`reviewer`/`fixer` read more naturally as
  pipeline stages. Caveat now that contract review exists: `fixer` and `reviewer`
  occur in *two* loops (code review and contract review), so either give `stage`
  disambiguated values (`code-review` vs `contract-review`, e.g.
  `contract-review-fix`) — preferred, one field — or keep the plan-DAG usage
  schema's separate `phase` (where) + function axes.

- **ACP-only transport — DONE; permanent directive + residual cleanup** (small).
  All agent invocation goes through ACP (`internal/agents/acp_transport.go`); the
  `claude -p` / `codex exec` CLI paths and the CLITransport were removed (#152/#155)
  — there is **no CLI invocation left in the code** (verified: no CLI descriptor, no
  `claude -p`, no `codex exec`). **Directive (permanent): ACP is the ONLY transport
  and the default everywhere; no CLI fallback may be reintroduced, and anything new
  — new engines, usage capture, effort, timeouts — must go through ACP, never a CLI
  shim.** Residual cleanup (small): (a) remove the 4 stale `// The CLI transport
  ignores it` comments (`agent_attempt.go:48,52`, `acp_transport.go:234`,
  `types.go:83`) — they reference a transport that no longer exists; (b) reconcile
  docs that still describe `claude -p` / `codex exec` invocation as current
  (`cost-budget-design.md`, `real-agent-execution-dogfood.md`) — keep them as history
  only. This connects to the codex-usage item: *ACP by default everywhere* means
  codex usage must capture via the **default** ACP adapter (upstream the
  `codex/token_usage` meta), not via a fork / `PACTUM_CODEX_ACP_COMMAND` workaround
  that re-smells the CLI era.
- **Graceful reviewer degradation** (small-med). A reviewer model that is
  unavailable / rate-limited (or whose process dies) currently aborts the whole
  review loop with a non-zero exit — observed when `claude-fable-5` returned
  `rate_limit` ("Claude Fable 5 is currently unavailable") and the loop had to
  be re-run with fable removed from the panel. Skip/retry the failed lens with a
  recorded warning and converge on the available reviewers instead of failing
  the run.
- **Reviewer attempt can HANG indefinitely on a large diff (codex), no idle
  timeout fires, no fallback** (med — reliability; new pain 2026-06-19). During
  the (since-reverted) plan-DAG slice-4b code-review, all five `codex-xhigh`
  reviewer attempts **stuck mid-generation on the large diff for ~5 hours** — they
  trickled 1–3 KB then stalled, so the 25-min idle timeout never fired, the loop
  sat on round 1, and ~17 codex-acp child processes leaked. Distinct from graceful
  degradation above (that's death/rate-limit; this is a live-but-stuck session
  the idle watchdog misses). Workaround that worked: kill it and switch
  `code_review.by` to **opus** (a reliable emitter that routes cleanly through ACP
  and completed the review). Fixes: (a) a real **idle/total cap on a reviewer
  attempt** that fires even when output trickles; (b) on hang/timeout after a
  bounded retry, **fall back to a reliable emitter (claude/opus)** — the design's
  "default the reviewer role to the reliable emitter where a cross-model reviewer
  exists"; (c) reap leaked adapter child processes on attempt kill. codex-over-ACP
  is the weak emitter on big diffs; opus/sonnet reviewers don't exhibit this.
- **Contract-review: bound rounds + halt on unresolved blockers, separate
  blocking from advisory** (med). Observed live in run_20260617_060708 (the
  `review_loop` port): on a long contract (15 enumerated sub-tests) the
  contract-review loop **ran the full 10 rounds, hit `max_rounds`, and never
  converged** — the panel kept piling on *advisory precision nits* (each round the
  contract grew), and the fixer **never landed the one real blocking finding**
  (a wrong arithmetic assertion: "reviewer-attempt events == max_rounds", but each
  round fans out across the fixed 5 lenses → `5 × max_rounds × panelSize`). It then
  **silently terminated and handed back a contract with the blocking finding still
  open** — the operator had to spot it and hand-fix the criterion. Three fixes:
  (a) **`max_rounds` with any unresolved *blocking* finding must HALT for the
  operator** (a distinct non-approvable terminal), not silently pass — a contract
  with an open blocker should never reach `approve`; (b) **separate blocking from
  advisory** so advisory nits never drive non-convergence (converge once blockers
  are clear; advisory findings are recorded, not loop-extending); (c) a
  **fixer-no-progress escalation** — if the fixer fails to change the blocking
  finding's status for K consecutive rounds, stop and escalate rather than burn the
  remaining rounds rewording around it. **Keep `max_rounds` default at 10** — the
  ~90-min run was bounded (for the later config slice) by a *manual one-off* cap,
  not by lowering the default; the round count is not the real lever.

  **The root cause is contract SIZE, not rounds.** A too-large contract (the
  15-sub-test monster) hands the panel an unbounded supply of advisory nits, so it
  never converges regardless of the cap; a focused contract (the config slice's ~10
  criteria) converges fast. So the real fix is **right-sizing the contract** — keep
  each contract small enough that draft + review converges quickly. *Open how to
  enforce it:* a soft size budget on the contract, a "too big → decompose" signal at
  draft time, or splitting the work before drafting. **(Updated 2026-06-19: the
  plan-DAG — once proposed as the structural cure here — was reverted (#207). The
  size cure now lives at the *contract level*: keep contracts small, and for
  oversized work have the drafter emit sequential contracts. So the (a)/(b)/(c)
  fixes above are now the PRIMARY defense, more important than before since no
  structural DAG cure is coming.)** Live mitigation that works today: temporarily
  cap `contract_review.loop.max` (e.g. 4) for a known-large contract, or kill a
  grinding run and operator-approve from the heavily-fixed state. Net: the
  recursion correctly *finds* blockers; it must not *pretend to have resolved*
  them — and the deeper fix is to never hand it an oversized contract.
- **Codex usage needs the forked codex-acp adapter** (small). *(Reframed — the old
  "non-fork CLI path" framing is stale: the `codex exec --json` CLI path was removed
  by the ACP-only change; everything is ACP now.)* Codex usage is read from the
  `_meta["codex/token_usage"]` notification (`acp_transport.go:378`), which the
  **forked** `codex-acp` (`~/repos/personal/codex-acp/target/release/codex-acp`)
  emits but the **default** adapter (`@zed-industries/codex-acp@latest`) does **not**.
  So unless `PACTUM_CODEX_ACP_COMMAND` points at the fork, every codex
  drafter/reviewer/fixer attempt records `captured=false` — observed across the
  loop-engine dogfood runs (~56 uncaptured codex records/run, the entire
  drafter + code-review panel invisible in cost stats). Fixes: (a) **operationally**,
  export `PACTUM_CODEX_ACP_COMMAND=<fork>` for dogfood runs (the fork is built
  locally — capture works immediately); (b) **durably**, upstream the
  `codex/token_usage` meta into the default adapter (or document the fork as
  required) so codex cost isn't silently dropped. Until one lands, all
  cost-accounting numbers are a lower bound missing every codex stage.
- **`contract revise` cannot remove fields** (small). Only `--add-*` flags
  exist, so a bad/over-specified validation command (or scope/acceptance entry)
  can't be dropped via CLI — forcing a hand-edit of `contract.json` that orphans
  the execution attempt by SHA. Add `--remove-*` parity.
- **`gofmt` is not in the gate** (shipped, #150). `make check` runs test/vet/deadcode +
  `git diff --check`, but not `gofmt -l`, so formatting drift accumulates
  uncaught (e.g. const-block alignment in `internal/app/clarify_round.go` and
  `review.go` drifted after the M25.1 budget-const removal — vet-clean, builds
  fine, but unformatted). Add `gofmt -l` (fail on non-empty) to `make check`
  and `gofmt -w` the existing drift in the same slice. **Reproduced live in #172:**
  the dogfood executor left `internal/loop/loop_test.go` unformatted; the local
  gate's `make check` passed (gofmt not in it) yet CI's `check` *failed* on it,
  forcing a follow-up format commit. So an executor's output can clear the local
  gate but fail CI — the gate should run `gofmt -l` so unformatted-but-otherwise-
  correct output is caught before it reaches CI.

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
  The ACP arc is now complete (M13.x→M26): ACP is the only transport, the CLI
  path is removed. This item is what remains of the shell-command gating gap.
  Closing it is deeper — it means intercepting/gating the adapter's command and
  tool-call requests on write stages, not just file writes — so it stays a
  documented limitation for now.
- **Consolidated ACP design note** (low, optional). The ACP transport, its
  real-time write scope guard, and usage normalization are currently described
  across `agents.md` and the M13.x milestone history; a single
  `docs/acp-transport-design.md` could collect the rationale and the known
  limitations (shell-command writes, no isolation) in one place.
  Nice-to-have, not blocking.
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

- **Add Gemini as a third ACP engine (native `gemini --acp`)** (candidate;
  researched 2026-06-19). Gemini CLI ships **native** Agent Client Protocol
  support — it is the reference implementation of ACP (Google + Zed), launched
  with `gemini --acp` (JSON-RPC over stdio, `--acp --debug` for diagnostics), so
  **no fork/adapter is needed** (unlike codex, which requires `codex-acp` — see
  the usage-fork note in the contract-review/agents lessons). Pactum's ACP-only
  transport already spawns an arbitrary adapter command and infers the engine
  from the model id, so wiring a `gemini-*` agent is mostly: register the agent
  in the `agents:` registry, infer the engine, and spawn `gemini --acp` as the
  adapter. Payoff: a third cross-model voice for the contract-review/code-review
  panels, and a candidate **reliable fallback emitter** for the reviewer-hang
  fallback work (the reliable-emitter half of "Reviewer attempt can HANG
  indefinitely"). **Verify before committing to it:** (a) does native
  `gemini --acp` emit token/usage data over ACP, or will usage capture need a
  fork the way codex did; (b) is Gemini-over-ACP a reliable streaming/structured
  emitter on large diffs (codex-over-ACP is the weak emitter that stalls — a
  reviewer/fallback role needs clean ACP routing); (c) auth model in a headless
  dogfood run (standard Gemini CLI auth). Sources: geminicli.com/docs/cli/acp-mode,
  zed.dev/acp.

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
  structured `pactum.review_fix_outcomes.v1alpha1` fenced-JSON block (one outcome per finding:
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
