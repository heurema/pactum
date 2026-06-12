# Review Fixer Context

## Run
- Run id: run_20260612_175035
- Run status: contract_approved

## Approved contract
- Goal: Stagger the cold start of same-model reviewer groups in the review panel fan-out to stop paying duplicate prompt-cache write premiums. Background (verified research, recorded in docs/cost-budget-design.md): Anthropic prompt-cache entries become usable only after the first response begins, parallel Claude Code sessions in the same directory read each other's cache, and the model is part of the effective cache key; today every review round launches all member-by-lens attempts simultaneously, so five concurrent claude-engine lens attempts each pay the 1.25x cache-write premium on the same shared prefix (system + tools + CLAUDE.md, roughly 25k tokens) — a staggered launch (1 write + 4 reads) instead of 5 writes saves about 74 percent of the prefix cost per claude round. Behavior: when the review fan-out spawns lens attempts, group them by the resolved registry entry's (inferred engine, model, effort); for groups whose inferred engine is claude and whose size exceeds one, launch exactly one attempt first and hold the rest; release the held attempts concurrently as soon as the first attempt's first streamed output chunk arrives (over ACP that is the first agent message text written to the attempt log — the existing transport already observes it), or immediately if the first attempt terminates before producing output, or after a hold timeout of 60 seconds so a silent first attempt can never serialize the panel. Codex-engine groups launch unchanged (no benefit: codex sets a per-thread prompt_cache_key; no cost: OpenAI charges no write premium). Single-attempt groups and the fixer are unaffected. This is built-in default behavior like the lens fan-out itself — no config knob; the live output prints one line when a group is held and when it is released so a watching operator understands the pause. The hold must not change attempt artifact naming, ordering of recorded attempts, or proposal collection semantics. Tests pin: claude-model groups launch one-then-rest on first output; the timeout and the early-termination releases both work; codex groups and single-attempt groups launch immediately; recorded attempt artifacts and review semantics are byte-compatible with the unstaggered path.
- In scope:
  - Implement `review run` reviewer lens scheduling after roster resolution for explicit reviewers, configured panels, and empty-panel cross-model fallback.
  - Group reviewer lens attempts across the whole review round by normalized `(engine, model, effort)`, independent of registry name.
  - For Claude groups with more than one attempt, launch exactly one lead attempt, hold the rest, and release held attempts concurrently on first visible output, lead completion before output, or a 60 second hold timeout.
  - Add a transport-agnostic first-visible-output callback: ACP fires on the first non-empty agent message chunk written to `stdout.log`; CLI fires on the first non-empty stdout or stderr write.
  - Emit live output lines when a Claude group is held and when it is released.
  - Add tests covering Claude first-output release, timeout release, completion-before-output release, cross-registry grouping by normalized model and effort, Codex immediate launch, and single-attempt immediate launch.
  - Update `docs/agents.md` and `docs/cost-budget-design.md` to describe the implemented same-model Claude review stagger behavior.
- Out of scope:
  - `review plan`, proposal collection commands, proposal accept/reject commands, fixer execution, execute stages, clarify stages, and contract-draft stages.
  - Adding a config knob, environment flag, or user-facing option for enabling or disabling staggered review launches.
  - Changing prompt contents, attempt artifact naming, attempt ID allocation order, reviewer lens set, model resolution rules, or Codex prompt cache key behavior.
  - Running real `pactum review run` agent subprocesses as validation without explicit human approval.
- Acceptance criteria:
  - A `review run` with a multi-attempt Claude group starts exactly one transport invocation for that normalized `(engine, model, effort)` group before any held attempts start.
  - Held Claude attempts are not invoked until the lead attempt produces first visible output, exits before visible output, or the 60 second timeout elapses.
  - When release is triggered, all held attempts in the Claude group are launched without intentional serialization.
  - Two different reviewer registry names resolving to the same Claude model and effort share one stagger group with one lead attempt.
  - Codex groups, non-Claude groups, and single-attempt groups launch immediately with no stagger hold.
  - Artifact schemas, artifact paths, attempt ID ordering, request prompt references, round summary ordering, proposal parsing, and proposal decision semantics remain compatible with the unstaggered path; timestamps, durations, usage values, scheduling order, and new live-output hold/release lines may differ.
  - `docs/agents.md` no longer describes all review lens attempts as always launching concurrently without qualification, and `docs/cost-budget-design.md` describes the Claude stagger as implemented rather than only planned.
- Validation commands:
  - go test ./internal/app ./internal/agents
  - make check

## Current review findings
- Summary: findings=5 open=5 resolved=0 blocking_open=2
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_004 severity=medium category=quality blocking=true status=open: The stagger tests do not verify that held attempts are released concurrently; a serialized release loop would still pass because held transport calls return immediately and the test only waits for the final launch count.
    location: internal/app/review_stagger_test.go:132
  - f_005 severity=medium category=quality blocking=true status=open: docs/agents.md still says review run uses five concurrent lens attempts per resolved reviewer, but same-model Claude groups now launch only one lead and hold the remaining attempts until first output, completion, or timeout.
    location: docs/agents.md:583
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=low category=quality blocking=false status=open: The rewritten round-level error collection (per-(reviewer,lens) error grid filled by concurrent stagger groups, scanned and wrapped at runReviewLoopReviewRound) has no test in which a reviewer lens attempt actually fails. All stagger tests and pre-existing review-round tests run the grid with all-nil errors, so the claimed 'same failure surface as the unstaggered path' (first error in (reviewer,lens) order, 'reviewer %s lens %s' wrap) is unverified, including the case where the failing attempt is a staggered group's lead. The equivalent wrap was also untested before this change, so this is a carried-forward gap on rewritten plumbing rather than a new bug.
    location: internal/app/review_loop.go:561
  - f_002 severity=low category=quality blocking=false status=open: The 'Review run rounds' section still describes the reviewer round as 'five concurrent lens attempts per resolved reviewer' with no stagger qualification. The acceptance criterion required docs/agents.md to no longer describe all review lens attempts as always launching concurrently without qualification; the two earlier mentions were qualified but this one, in the section dedicated to `review run`, was missed.
    location: docs/agents.md:584
  - f_003 severity=low category=quality blocking=false status=open: The new 'Implemented (M25.2)' paragraph claims 'attempt artifacts, IDs, request prompts, and proposal semantics are byte-compatible with the unstaggered path', but attempt result artifacts record started_at, duration_ms, and usage values that necessarily differ. Contract clarification q_003 explicitly scoped byte-compatibility to schemas, paths, ID ordering, request prompt references, summary ordering, and proposal semantics, allowing timestamps/durations/usage to differ; the doc reintroduces the unscoped phrasing.
    location: docs/cost-budget-design.md:224

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
