# Reviewer and executor prompt hardening — design notes

Survey-driven design input for strengthening the built-in reviewer and executor
prompts. Sources: the review prompts shipped by the two built-in agent vendors
(their slash-command review flows and a production security-review pipeline)
and a leading open-source multi-agent review orchestrator. Ideas are absorbed
here without attribution; this document is the reference for the prompt
hardening slices.

## What the surveyed prompts do that ours do not

### 1. High-signal contract (anti-noise)

The strongest common theme. The surveyed review prompts are explicit that a
finding must be *certainly real* before it is reported:

- "If you are not certain an issue is real, do not flag it. False positives
  erode trust and waste reviewer time."
- Only flag issues above an explicit confidence bar (one pipeline: >80%
  confident of actual exploitability; report nothing below 0.7).
- Explicit NOT-to-flag lists: style/quality preferences, input-dependent
  hypotheticals, subjective suggestions, anything a linter/typechecker would
  catch (our gate already runs vet/deadcode — machine-catchable findings are
  noise), and broad "potential" concerns without a concrete failure path.
- Report problems only — no positive observations, no praise filler.

Pactum's current reviewer prompt has good bones (read the file before flagging,
check mitigations, prefer evidence, no style preferences) but no confidence
bar, no NOT-to-flag list, and no "certain or silent" framing.

### 2. Specialized review lenses with concrete checklists

The orchestrator survey runs five independent lenses, each with a concrete
checklist rather than a generic "review the code". The checklists are the
value; condensed:

- **Correctness/quality** — logic errors (off-by-one, wrong operators), edge
  cases (empty/nil/boundary/concurrent), error handling (no silent failures),
  resource cleanup/leaks, races/deadlocks, input validation, secret exposure.
- **Implementation-vs-goal** — does the diff actually achieve the stated
  requirement: requirement coverage, correctness of approach, *wiring and
  integration* (components registered, routes added, configs updated),
  completeness (missing pieces that prevent the feature from working), logic
  flow end to end. For pactum this lens maps directly onto the approved
  contract: goal, in-scope items, and acceptance criteria are the stated
  requirement.
- **Test quality + fake-test detection** — missing tests for new paths,
  untested error paths, and a dedicated catalog of tests that verify nothing:
  always-pass tests, hardcoded-value checks, asserting mock behavior instead
  of code under test, ignored errors, conditional assertions that always pass,
  commented-out failing cases.
- **Over-engineering catalog** — wrapper-adds-nothing, factory for a single
  implementation, layer-cake pass-throughs, premature generalization (generic
  machinery for one case), unused extension points, dual implementations where
  the old path has no callers, silent fallbacks that hide failures, premature
  optimization. (Pactum's own deadcode gate catches unreachable code; this
  lens catches *reachable but unnecessary* complexity.)
- **Documentation gaps** — user-visible changes that require README/docs
  updates vs internal changes that do not (with an explicit skip list).

### 3. Verify-then-report discipline

The orchestrator forces a verification pass over every candidate finding
before it is acted on: read the actual code at file:line, check 20–30 lines of
surrounding context, check for existing mitigations, then classify CONFIRMED
vs FALSE POSITIVE and discard the latter. One vendor pipeline goes further and
uses separate validation agents that filter unvalidated findings. Pactum's
prompt asks the reviewer to read context but never asks it to classify its own
candidates before emitting them.

### 4. Findings-first output and honest empties

The vendor review mindset: findings are the primary output, ordered by
severity with file/line references; open questions and assumptions after; a
change summary only as a secondary detail. An empty review must say so
explicitly *and* name residual risks and testing gaps — an honest "no issues,
but X is untested" beats silence.

### 5. Severity tied to demonstrable impact + per-finding confidence

Severity definitions anchored to impact (directly exploitable / wrong results
vs needs-specific-conditions vs defense-in-depth), and a confidence score per
finding with a hard floor below which nothing is reported. Pactum findings
carry severity/blocking but no confidence; clarify questions already carry
confidence (M15.0), so the schema precedent exists in-house.

### 6. Pre-existing issues policy

The surveyed prompts disagree: the orchestrator says fix pre-existing issues
too; the vendor review plugin says do not flag them. For pactum the contract
decides: pre-existing problems are out of the diff's scope, so they belong as
**non-blocking advisory findings** (visible, never driving the fix loop) —
consistent with the M12.9 severity-gated convergence model.

## Executor prompt: house style

The executor prompt today says "follow the contract / no out-of-scope /
search before creating". The hardening adds the house style the reviewer will
also enforce:

- Match the surrounding code: idiom, naming, comment density. Comments only
  where the code is not self-explanatory; no narration of the obvious.
- Reuse existing helpers before writing new ones; search first.
- Small, focused diffs — change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every
  new abstraction, no premature generalization or optimization (the
  over-engineering catalog above as DON'Ts).
- No dead code, no commented-out code, no unused parameters.
- Error handling per project convention; no silent failures.
- Tests verify behavior (not implementation details) and cover error paths;
  no fake tests (the detection catalog above as DON'Ts).

## Slicing proposal

1. **Slice 1 — reviewer prompt hardening** (shipped, M19.0): bake the
   high-signal contract (certain-or-silent, NOT-to-flag list, problems-only),
   the five lens checklists, verify-then-report (classify CONFIRMED/FALSE
   POSITIVE before emitting), findings-first ordering with honest empties, the
   pre-existing → advisory policy, and a `confidence` field on finding
   proposals (schema precedent: clarify questions) into
   `renderReviewerPrompt`. Prompt-content tests guard each section. A missing
   confidence defaults to medium (the kind-field compatibility lesson); an
   invalid one skips the proposal with a warning; confidence is recorded and
   displayed but gates nothing yet.
2. **Slice 2 — write-stage house style** (shipped, M19.1): one shared
   house-style section (`writeHouseStyleSection`) in both write-stage prompts —
   the executor prompt (`renderApprovedPromptMD`) and the review-fix fixer
   prompt (`renderReviewFixPrompt`, which adds a note that the reviewer
   re-checks fixes against the same ruleset) — mirroring what the reviewer
   enforces. Prompt-content tests pin the section's key lines and that both
   prompts contain the identical section text.
3. **Slice 3 — specialist review lenses** (shipped, M19.2). Shipped as
   built-in default behavior rather than configuration: the lens set is fixed
   in code, and every `review run` and every review-loop round expands each
   resolved reviewer (the explicit `--reviewer` or each `review.panel` member)
   into five concurrent lens attempts. Each attempt's prompt carries only its
   own lens checklist plus a panel focus note ("you are the `<lens>` reviewer;
   other lenses are covered by other reviewers running in parallel"), while
   the shared hardened sections from slice 1 stay identical across lenses.
   The combined all-five-lenses prompt no longer exists. Cross-lens duplicate
   findings collapse through the existing finding fingerprint dedup with
   severity-max; the cost of five attempts per reviewer per round is a
   deliberate default, not a knob.
