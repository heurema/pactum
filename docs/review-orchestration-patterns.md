# Review & orchestration improvements — council-converged design

Status: **design DONE** (2026-06-20). This supersedes the earlier candidate-evaluation
framing with the per-item designs the council converged on. Each item carries a
concrete Go design, the new schemas / terminal reasons, how it preserves pactum's
invariants, effort, and tests.

**Implementation status (2026-06-21) — most of this shipped, each built end-to-end by
dogfooding pactum on itself:** #2 anti-FP finding schema (#222), #4 safe read-only git
(#224), #3 retry classifier (#226), #1 critic pass (#233) — plus two dogfood-found
items: contract-review precision discipline (#229) and the git-history guard (#231) with
its linked-worktree fix (#234). Still open: the ACP permission tripwire (a bypassable
fast-fail follow-up to the guard), the shared in-flight limiter, #7 fixer loop-breaker,
#5 `pactum reflect`, and #6 seeded alloy. See the backlog for current priority.

**Method.** Synthesized from a survey of comparable agentic code-review tooling, then
hardened over a three-round council: (1) two external reasoning models (codex gpt-5.5
at xhigh effort, Gemini 3.1 Pro) each produced an implementation-grade design grounded
in pactum's source; (2) an adversarial reconciliation round; (3) a three-way
fork-resolution round including Claude. Provenance is deliberately generic — these are
pactum-owned designs, not a port of any one tool.

**Invariants every item below respects.** Single-shot execution (one contract = one
session; no per-task execution loop — the plan-DAG was reverted in #207). Loud loops
(explicit terminal reasons; convergence gated on **blocking** findings only; never a
silent pass). Determinism + durable schema-versioned artifacts under `runs/<id>/`;
anything stochastic is seed-pinned and recorded. ACP-only transport (codex + claude).

## Final ship order

1. **#2 Anti-FP finding schema** — the data contract the critic consumes.
2. **#4 Safe read-only git** — tiny, independent safety win.
3. **#3 Retry classifier (read-only v1)** — baseline reliability for the reviewer fan-out.
4. **#1 Critic pass** — depends on #2; the headline quality win.
5. **Shared in-flight limiter** — the salvaged nugget from the dropped #8; a latent fan-out cap bug.
6. **#7 Fixer loop-breaker (default-on)** — cures fixer "amnesia"; independent, ships default-on (operator decision).
7. **#5 `pactum reflect`** — deterministic telemetry; now *tunes* #7's payload rather than gating it.
8. **#6 Seeded alloy** — cheap but low marginal value (the panel already supports cross-model); ship last.

Dropped: **#8 concurrent investigation waves** (recursive/agent-spawn) — conflicts with
single-shot + the reverted plan-DAG; only the shared limiter is salvaged (item 5).

---

## #2 — Anti-false-positive finding schema

**Decision.** Make the finding schema fight false positives, and split reviewer output
into `candidate` vs `confirmed`. This is the data model the critic pass (#1) consumes.

**Go design.**
- Modify the existing schema `pactum.reviewer_findings.v1alpha1` **in place**
  (`internal/app/review_proposals.go:19`). This is a deliberate breaking change — the
  project has **no external users yet**, so there is no v1alpha2, no dual-version parse,
  and no on-disk migration. (Earlier drafts proposed a v1alpha2 + migration; dropped as
  unnecessary.)
- Add to the reviewer proposal input and `reviewProposalRecord` (`review_proposals.go:26`):
  `state` (`candidate|confirmed`), `trigger`, `evidence`, `fix_direction`, `uncertainty`,
  `current_code_only` (`*bool`).
- Carry the anti-FP fields onto `reviewFindingRecord` (`internal/app/review.go:196`).
  **Evidence is copied into the confirmed finding** (resolved fork — see below), with a
  **length cap of ~500–1000 chars** on copy so a large stack trace can't bloat the fixer
  context; the proposal remains the provenance/origin record.
- Validation in `proposalRecordFromReviewerInput` (`review_proposals.go:522`): for
  v1alpha2 require non-empty `trigger`/`evidence`/`fix_direction`/`uncertainty`, a valid
  `state`, and explicit `current_code_only=true` for any blocking finding.
  **`current_code_only=false` cannot be blocking** (skip/downgrade — kills "improvement
  narration" of changes the diff already made).
- Reviewer prompt: reframe `renderReviewerPrompt` from "certain-or-silent" to **recall-first**
  (report tightly-evidenced candidates, state uncertainty); precision is owned by the critic.

**Loud-loop integration.** Stricter parsing turns vague reviewer output into the existing
`reviewer_findings_unparsed` loud terminal instead of silent churn.

**Effort: S–M** (simpler without migration). Risk: the shared parser is also used by
contract-review (`contract_review.go` reuses `reviewerFindingsSchema`) — the change must
not break it (keep parsing tolerant, or enforce the new required fields only on the
code-review path). **Tests:** v1alpha1 finding missing `trigger` dropped with a specific
warning; blocking + `current_code_only=false` skipped; a confirmed finding carries
`trigger`/`evidence` into the fixer context; contract-review still parses.

**Resolved fork (evidence placement).** Copy evidence into the confirmed finding (not a
proposal→finding lookup join). Rationale: the fixer consumes findings, and a lookup path
introduces a silent "evidence lost on lookup miss" failure mode; a self-contained finding
is the lower-complexity, more robust artifact. The length cap addresses the only real cost.

---

## #4 — Safe-by-construction read-only git

**Decision.** Treat "read-only git" as something constructed, not asserted by subcommand
name. A small validated wrapper for all internal read-only git use.

**Go design.**
- New `internal/gitctx` package: `Output(ctx, root, args...)`, `ValidateReadOnly(args)`,
  `Command(ctx, root, args...)`. Always `exec.CommandContext` (no shell), always
  `GIT_OPTIONAL_LOCKS=0` (so even `git status` can't contend on the user's index lock).
- Allowlist initially: `ls-files`, `rev-parse --verify HEAD`, `show-ref`, `for-each-ref`,
  limited `status --porcelain`, limited `diff --name-only/--name-status`. **Drop `branch`/`tag`
  entirely** (read/write indistinguishable by args) in favor of plumbing with no write mode.
- Deny: `-c`, `--git-dir`, `--work-tree`, `--exec-path`, `--no-index`, `--contents`,
  `--ignore-revs-file`, `--output`/`-o*`, absolute paths, `..`, NULs, and pathspec magic
  until a real caller needs them.
- Migrate the production read-only git calls (`gitCandidateFiles` in `internal/projectmap/scan.go`,
  `reviewLoopGitHead` in `internal/app/review_loop.go`) to the wrapper. Test helpers that
  mutate fixtures stay raw.

**Integration.** Does **not** replace the ACP write guard or the gate — it makes internal
read-only git construction auditable. **Effort: S.** Risk: rejecting an obscure valid read —
keep the denylist tested and auditable. **Tests:** allow/deny matrix; assert built commands
carry `GIT_OPTIONAL_LOCKS=0`.

---

## #3 — Transport retry/error classifier (read-only v1)

**Decision.** Classify provider errors precisely and retry transient ones, in the agent
attempt lifecycle — not inside review convergence. Merges with the existing
"Executor resilience" backlog item as its decision function.

**Go design.**
- New `internal/agents/retry.go`: `TransportErrorClass{Retryable, Kind, Reason, Statuses}`
  and `ClassifyTransportError(error)`. Walk the **full** unwrap chain (not the top-level
  message), recognize an HTTP status only in genuine status context, give **5xx precedence
  over a 4xx nested in a body**, treat 429 / rate-limit / network / timeout as transient and
  auth / billing / quota as permanent. Also retry empty/no-op responses up to a small cap.
- Drive retries from `runAgentAttemptLifecycle` (`internal/app/agent_attempt.go:88`), which
  is the single call site wrapping `agentTransport().Run` (`agent_attempt.go:130`) — **not**
  `loop.Run`.
- Extend `agents.RunResult` with `WriteBoundaryCrossed`, `RetryClass`, `RetryReason`; persist
  them per attempt. Record a `pactum.agent_retry_decision.v1alpha1` JSONL line per attempt
  (stage, attempt_id, class, reason, write_boundary_crossed, delay_ms, seed, next_attempt_id).
- Write-boundary detection in the ACP client: set `WriteBoundaryCrossed` before the file
  write in `WriteTextFile` and on write-stage permission approval; **plus a working-tree
  fingerprint comparison** after a failed write-stage attempt, to catch native codex writes
  that bypass `WriteTextFile`.

**v1 scope (resolved fork).** v1 retries **read-only stages only** (reviewers, clarifier,
contract draft) — 100% safe and covers the bulk of transient drops in the fan-out.
**Write-stage retry is a follow-up**, gated by `WriteBoundaryCrossed=false` **and** an
unchanged working-tree fingerprint; an ambiguous boundary stops loud and preserves the
failed attempt (no silent duplicate writes).

**Effort: M.** Risk: duplicate writes from unobserved native agent behavior → the
write-boundary + fingerprint gate is mandatory before write-stage retry. **Tests:**
classifier table tests; ACP write-boundary tests; lifecycle tests (transient→success,
permanent→no retry, write-boundary→no retry, empty stdout→retry).

---

## #1 — Critic pass (recall→precision)

**Decision.** Between candidate findings and the findings that gate the fixer, run an
adversarial read-only critic that must confirm each candidate with evidence, dispute it, or
flag insufficient evidence. Only confirmed candidates become BLOCKING. This is the headline
quality win and the sharpest reinforcement of the loud-loop differentiator.

**Go design.**
- New `internal/app/review_critic.go`. Schemas: `pactum.review_critic_request.v1alpha1`,
  `pactum.review_critic_result.v1alpha1`, `pactum.review_critic_verdicts.v1alpha1`.
- Refactor the accept seam in `runReviewLoopReviewRound` (`review_loop.go:656`; the accept
  call is `acceptReviewLoopProposal` at `review_loop.go:290`/`:1113`) into:
  dedupe existing/rebutted proposals → run the critic over **new candidates only** →
  `acceptReviewLoopProposal` accepts only `verdict=confirmed`.
- `runReviewLoopCriticRound(...)` runs through `runAgentAttemptLifecycle` with
  `ReadOnly: true` (like reviewer attempts). One verdict per `proposal_id`, **sorted by
  proposal ID** for deterministic artifacts; a hallucinated/unknown ID is dropped.
  Verdict enum: `confirmed | disputed | insufficient_evidence`.
- New round-summary fields: `critic_attempt_id`, `precision_candidates`, `precision_confirmed`,
  `precision_rejected`, `precision_unresolved`, `critic_verdicts_artifact`.
- New loud terminal reasons:
  - `precision_rejected` — blocking candidates all explicitly disputed and no open blockers
    remain (approvable).
  - `debate_no_consensus` — `insufficient_evidence` leaves an evidence gap with no confirmed
    blocker (blocks approval).
  - `critic_verdicts_unparsed` — critic output unparseable after one corrective re-attempt
    (blocks approval). A critic failure must **never** collapse to a clean round.
- Prior open blockers still drive the fixer even if all new candidates are rejected.

**Critic model selection (resolved fork).** Default to a reviewer on a **different engine**
than the primary reviewer when one is configured (mirroring pactum's existing cross-model
reviewer default in `agent_resolve`); fall back to the same model **with a recorded warning**
when only one engine exists (the default single-claude config); allow explicit
`pipeline.code_review.critic_by` (max one registered ACP reviewer). Do **not** hard-enforce
cross-model — that would break single-engine setups. The critic evaluates **all candidates
in one prompt** to cap cost.

**Effort: M–L.** Risk: an over-conservative critic dropping real blockers → require the
critic to *name* missing evidence (not reject on vibes) and keep rejected candidates recorded
(advisory). **Tests:** verdict-block parser tests; confirmed→fixer; disputed blocker→
`precision_rejected`; missing critic block→`critic_verdicts_unparsed`; insufficient evidence→
`debate_no_consensus`; existing blocker + rejected new candidate still runs the fixer.

---

## #5-salvage — Shared in-flight agent limiter

**Decision.** Extract a real global cap on concurrent agent attempts. Today the reviewer
fan-out runs concurrently with no enforced global bound — a local per-call semaphore is
bypassed. Ship as its **own small hardening PR**, early, independent of any config feature.

**Go design.**
- A shared limiter (using `golang.org/x/sync/semaphore`, already a dependency) around the
  single transport call site `agentTransport().Run` (`internal/app/agent_attempt.go:130`),
  configured by a global `max_in_flight` setting.
- Attempt IDs are currently allocated by goroutine-arrival order under a mutex; to keep
  artifact names deterministic under concurrency, add an `AttemptIDOverride` (or fold results
  in index order) so scheduling can't perturb artifact names.

**Effort: S.** **Tests:** prove max concurrent attempts is bounded; a delayed fake transport
proves deterministic result/artifact ordering independent of completion order.

**Resolved fork:** own PR, not bundled with #6 alloy (a concurrency-correctness fix must not
be coupled to a config feature).

---

## #7 — Fixer loop-breaker (default-on)

**Decision.** Cure the fixer's "amnesia" between stateless rounds with a **deterministic**
context inject — no LLM compactor. **Default-on** (operator decision).

**Go design.**
- Knob `pipeline.code_review.loop_breaker` (named a loop-breaker, **not** "compaction" — it
  is deterministic, not LLM summarization). **Default-on.**
- In `runReviewLoopFixRound` (`internal/app/review_loop.go:1199`; mirror in contract-review):
  after one same-blocker no-progress round (`noProgressStreak >= 1`), inject into the next
  fixer prompt (`renderReviewFixPrompt`, `internal/app/review_fix.go:331`): the prior failed
  attempt's **raw diff + exact gate stderr + prior attempt IDs + open-blocker keys** + a
  "try a different approach" nudge. This is free — no extra model call, just a richer prompt.
- Structurally **bounded to one** loop-breaker attempt. If the same blocker key set remains
  after the nudge, stop with a new loud terminal **`fixer_loop_breaker_exhausted`** (distinct
  from `fixer_no_progress` so `pactum reflect` can see that the loop-breaker fired).
- Artifact `pactum.review_fix_loop_breaker.v1alpha1` per round (run_id, round, trigger,
  source_attempt_ids, source_sha256, open_blocking_keys, summary); round summary gains
  `fixer_loop_breaker_artifact`, `loop_breaker_armed`.

**Loud-loop / determinism.** The gate keeps judging real files/commands — never the injected
summary. Determinism comes from deterministic extraction (attempt IDs, stdout/result hashes,
blocker keys, working-tree fingerprints). `#5` telemetry **tunes the payload**, it no longer
gates activation.

**Effort: M.** Risk: behavioral churn in existing no-progress tests + a new terminal reason
operators must expect (one-time migration). **Tests:** same blockers → loop-breaker injected
→ `fixer_loop_breaker_exhausted` when exhausted; the gate still runs after the fixer; the
disable flag suppresses it.

---

## #6 — Seeded alloy (reviewer panel)

**Decision.** Optional seeded model rotation across the reviewer panel. Cheap, but **low
marginal value** (the panel already supports cross-model via `pipeline.code_review.by`), so
ship **last**.

**Go design.**
- Extend `agentRegistryEntry` (`internal/app/config.go:128`) with
  `AlloyGroup *alloyGroupConfig{Seed, Strategy, Members[]}`; `validateAgentRegistry`
  (`config.go:341`) requires **exactly one** of `model` or `alloy_group`. Each member still
  resolves via `InferAgentFromModel` (no new providers).
- Legal **only** in `pipeline.code_review.by` / `pipeline.contract_review.by`; rejected for
  execute / fix / clarify / draft / memory. The fixer stays a fixed agent.
- Selection per run/round/lens **before** fan-out:
  `seed = sha256("pactum.alloy.v1alpha1\0"+group+"\0"+groupSeed+"\0"+runID+"\0"+round)`,
  then `members[(start+lensIndex) % len(members)]`. Record
  `review/alloy-selection-round-NNN.json` (`pactum.alloy_selection.v1alpha1`).
- The stagger/cache-warming grouping must key off the **selected** model/effort, not the
  group name.

**Determinism.** `runID`/`round`/`lens` are immutable per attempt → same selection on re-run;
the chosen member is recorded. **Effort: S–M.** **Tests:** uniform selection distribution;
a delayed fake transport proves selection/ordering is completion-order independent.

**Resolved fork:** LOW priority, ship after #5; `#5` before `#6` because reflect's telemetry
should show whether diversity addresses an actually-observed failure mode.

---

## #5 — `pactum reflect` (deterministic)

**Decision.** A deterministic CLI analyzer over recorded run artifacts — **not** an
autonomous prompt mutator. Produces a human-read "orchestration lessons" artifact.

**Go design.**
- `pactum reflect [run_id]` → `App.Reflect` in new `internal/app/reflect.go`. Artifacts under
  `runs/<id>/orchestration/` (`pactum.orchestration_lessons.v1alpha1`: run_ids, created_at,
  terminal_reason, source_runs, input_sha256, coverage, signals, lessons, proposed_tweaks,
  warnings) — explicitly **not** under the memory paths, to stay separate from project memory.
- Reads the ledger (`events.jsonl`), code-review loop summaries, proposals/decisions, fixer
  outcomes, gate reports, and usage records. Also **persist the contract-review loop summary**
  as a run artifact (it is built today but not written).
- Default sampling = the **target run** (deterministic, bounded — the "why did this run stall?"
  case); `--last N` / explicit run-ids for cross-run aggregate; `--min-runs 5` before emitting
  recommendations; **error-terminal runs excluded** from prompt-tweak confidence unless
  `--include-errors`.
- Terminal reasons: `lessons_emitted`, `insufficient_sample`, `no_review_loop_data`,
  `input_corrupt`, `reflection_error`. It never calls `ContractRevise` / `ReviewApprove` /
  memory accept. Determinism: sorted run IDs, canonical-JSON input hashing, rule-based lessons
  (no LLM synthesizer in v1).

**Effort: M.** Risk: noisy "self-improvement" feeling authoritative → `--min-runs`, advisory
artifact only, never auto-edits prompts. **Tests:** synthetic runs for `fixer_no_progress`,
parse misses, gate failures, corrupt events; identical inputs → identical `input_sha256` and
lesson ordering.

---

## Resolved decisions (the six forks)

| Fork | Decision |
|---|---|
| #6 alloy priority / order | LOW priority, ship last; **#5 before #6** |
| Critic model | **Prefer cross-model**, not enforced; fallback + recorded warning on single-engine; `critic_by` override |
| #7 default | **Default-on** (operator decision); deterministic loop-breaker, bounded to 1 attempt |
| #5 sampling | Default **target-run**; `--last N` opt-in; error terminals excluded unless `--include-errors` |
| #2 evidence | **Copy into the confirmed finding**, length-capped ~500–1000 chars |
| Shared limiter | **Own PR**, early, independent of #6 |

The `#7` knob was renamed from "compaction" to a deterministic **loop-breaker** (it injects
prior diff + gate stderr + blocker keys; it is not LLM summarization).

## Out of scope — not solved by the surveyed tooling

These remain pactum-owned gaps; nothing in the evaluated space addresses them:
- **Budget enforcement / spend caps** — owned by [`cost-budget-design.md`](cost-budget-design.md).
- **Sandboxing / interactive execute-time approvals** — the strongest safety idea on offer is
  read-only-by-construction tools (#4); no container/VM or approval model to borrow.
- **External prompt templating/versioning** — the surveyed tooling also compiles prompts in as
  source constants, same posture as pactum's compiled-in Go prompts.
