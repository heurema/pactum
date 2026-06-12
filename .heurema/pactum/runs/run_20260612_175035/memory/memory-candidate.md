# Memory Candidate

## Run
- Run id: run_20260612_175035
- Source: deterministic

## Contract
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

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - docs/agents.md
  - docs/cost-budget-design.md
  - internal/agents/acp_transport.go
  - internal/agents/acp_transport_test.go
  - internal/agents/executor_test.go
  - internal/agents/runner.go
  - internal/agents/types.go
  - internal/app/agent_attempt.go
  - internal/app/app.go
  - internal/app/review_loop.go
- New files:
  - internal/app/review_stagger_test.go
- Missing files: none

## Clarifications
- q_001: When the contract says "review panel fan-out," should the stagger apply to every `review run` reviewer lens fan-out whose grouped Claude attempts exceed one, including an explicit single Claude reviewer that expands into the five lenses, or only to configured multi-member `review.panel` runs?
  Answer: Apply the stagger to all `review run` reviewer lens attempts after roster resolution: explicit reviewer, configured panel, and empty-panel cross-model fallback. Keep `review plan`, proposal collection commands, and the fixer out of scope.
- q_002: If two different registry names resolve to the same Claude model and effort, should their member-times-lens attempts form one shared stagger group, or should each registry name stagger independently?
  Answer: Group across the whole review round by normalized `(engine, model, effort)` only, not by registry name. Two names with the same Claude model and effort should share one group with one lead attempt and all other attempts held.
- q_003: What should "recorded attempt artifacts and review semantics are byte-compatible" mean, given that staggered launches necessarily change start times, durations, and add live-output hold/release lines?
  Answer: Require byte-compatible schemas, artifact paths, attempt ID ordering, request prompt references, round summary ordering, proposal parsing, and proposal decision semantics. Allow timestamps, durations, usage values, scheduling order, and new live-output hold/release lines to differ.
- q_004: Should the first-output release trigger be implemented for the CLI transport debug path as well as the default ACP transport?
  Answer: Implement the stagger through a transport-agnostic first-visible-output callback. ACP should fire it on the first non-empty agent message chunk written to `stdout.log`; CLI should fire it on the first non-empty stdout or stderr write. Completion-before-output and the fixed 60-second hold timeout remain fallback releases.
- q_005: Should the contract explicitly include documentation updates for the changed reviewer launch behavior?
  Answer: Include docs updates in scope: update `docs/agents.md` to describe same-model Claude stagger behavior and update `docs/cost-budget-design.md` from planned slice to implemented behavior.

## Review Decisions
- f_001 [low] resolved internal/app/review_loop.go:561: The rewritten round-level error collection (per-(reviewer,lens) error grid filled by concurrent stagger groups, scanned and wrapped at runReviewLoopReviewRound) has no test in which a reviewer lens attempt actually fails. All stagger tests and pre-existing review-round tests run the grid with all-nil errors, so the claimed 'same failure surface as the unstaggered path' (first error in (reviewer,lens) order, 'reviewer %s lens %s' wrap) is unverified, including the case where the failing attempt is a staggered group's lead. The equivalent wrap was also untested before this change, so this is a carried-forward gap on rewritten plumbing rather than a new bug.
- f_002 [low] resolved docs/agents.md:584: The 'Review run rounds' section still describes the reviewer round as 'five concurrent lens attempts per resolved reviewer' with no stagger qualification. The acceptance criterion required docs/agents.md to no longer describe all review lens attempts as always launching concurrently without qualification; the two earlier mentions were qualified but this one, in the section dedicated to `review run`, was missed.
- f_003 [low] resolved docs/cost-budget-design.md:224: The new 'Implemented (M25.2)' paragraph claims 'attempt artifacts, IDs, request prompts, and proposal semantics are byte-compatible with the unstaggered path', but attempt result artifacts record started_at, duration_ms, and usage values that necessarily differ. Contract clarification q_003 explicitly scoped byte-compatibility to schemas, paths, ID ordering, request prompt references, summary ordering, and proposal semantics, allowing timestamps/durations/usage to differ; the doc reintroduces the unscoped phrasing.
- f_004 [medium] resolved internal/app/review_stagger_test.go:132: The stagger tests do not verify that held attempts are released concurrently; a serialized release loop would still pass because held transport calls return immediately and the test only waits for the final launch count.
  Resolution: Added TestReviewRunReleasesStaggeredClaudeGroupConcurrently in internal/app/review_stagger_test.go. Each held attempt blocks inside the transport on an all-in-flight barrier (allHeldInFlight) that opens only when all len(reviewLenses)-1 held attempts are simultaneously present; a serialized one-at-a-time release deadlocks the first held attempt and the test fails as 'release was serialized'. Verified it fails (5.1s timeout) when launchReviewerLensTasks is temporarily serialized, and passes -race -count=3 against the concurrent implementation.
- f_005 [medium] resolved docs/agents.md:583: docs/agents.md still says review run uses five concurrent lens attempts per resolved reviewer, but same-model Claude groups now launch only one lead and hold the remaining attempts until first output, completion, or timeout.
  Resolution: Edited docs/agents.md 'Review run rounds' section: replaced 'five concurrent lens attempts per resolved reviewer' with 'five lens attempts per resolved reviewer, launched concurrently except that same-model Claude groups are staggered (one lead warms the prompt cache before the rest)', matching the qualified phrasing used in the other two mentions.
- Proposal summary: pending=0 accepted=5 rejected=0

## Reusable Project Knowledge
- scope: in scope: Implement `review run` reviewer lens scheduling after roster resolution for explicit reviewers, configured panels, and empty-panel cross-model fallback.
- scope: in scope: Group reviewer lens attempts across the whole review round by normalized `(engine, model, effort)`, independent of registry name.
- scope: in scope: For Claude groups with more than one attempt, launch exactly one lead attempt, hold the rest, and release held attempts concurrently on first visible output, lead completion before output, or a 60 second hold timeout.
- scope: in scope: Add a transport-agnostic first-visible-output callback: ACP fires on the first non-empty agent message chunk written to `stdout.log`; CLI fires on the first non-empty stdout or stderr write.
- scope: in scope: Emit live output lines when a Claude group is held and when it is released.
- scope: in scope: Add tests covering Claude first-output release, timeout release, completion-before-output release, cross-registry grouping by normalized model and effort, Codex immediate launch, and single-attempt immediate launch.
- scope: in scope: Update `docs/agents.md` and `docs/cost-budget-design.md` to describe the implemented same-model Claude review stagger behavior.
- scope: out of scope: `review plan`, proposal collection commands, proposal accept/reject commands, fixer execution, execute stages, clarify stages, and contract-draft stages.
- scope: out of scope: Adding a config knob, environment flag, or user-facing option for enabling or disabling staggered review launches.
- scope: out of scope: Changing prompt contents, attempt artifact naming, attempt ID allocation order, reviewer lens set, model resolution rules, or Codex prompt cache key behavior.
- scope: out of scope: Running real `pactum review run` agent subprocesses as validation without explicit human approval.
- clarification: q_001: When the contract says "review panel fan-out," should the stagger apply to every `review run` reviewer lens fan-out whose grouped Claude attempts exceed one, including an explicit single Claude reviewer that expands into the five lenses, or only to configured multi-member `review.panel` runs? Answer: Apply the stagger to all `review run` reviewer lens attempts after roster resolution: explicit reviewer, configured panel, and empty-panel cross-model fallback. Keep `review plan`, proposal collection commands, and the fixer out of scope.
- clarification: q_002: If two different registry names resolve to the same Claude model and effort, should their member-times-lens attempts form one shared stagger group, or should each registry name stagger independently? Answer: Group across the whole review round by normalized `(engine, model, effort)` only, not by registry name. Two names with the same Claude model and effort should share one group with one lead attempt and all other attempts held.
- clarification: q_003: What should "recorded attempt artifacts and review semantics are byte-compatible" mean, given that staggered launches necessarily change start times, durations, and add live-output hold/release lines? Answer: Require byte-compatible schemas, artifact paths, attempt ID ordering, request prompt references, round summary ordering, proposal parsing, and proposal decision semantics. Allow timestamps, durations, usage values, scheduling order, and new live-output hold/release lines to differ.
- clarification: q_004: Should the first-output release trigger be implemented for the CLI transport debug path as well as the default ACP transport? Answer: Implement the stagger through a transport-agnostic first-visible-output callback. ACP should fire it on the first non-empty agent message chunk written to `stdout.log`; CLI should fire it on the first non-empty stdout or stderr write. Completion-before-output and the fixed 60-second hold timeout remain fallback releases.
- clarification: q_005: Should the contract explicitly include documentation updates for the changed reviewer launch behavior? Answer: Include docs updates in scope: update `docs/agents.md` to describe same-model Claude stagger behavior and update `docs/cost-budget-design.md` from planned slice to implemented behavior.
- review_resolution: f_001 resolved: The rewritten round-level error collection (per-(reviewer,lens) error grid filled by concurrent stagger groups, scanned and wrapped at runReviewLoopReviewRound) has no test in which a reviewer lens attempt actually fails. All stagger tests and pre-existing review-round tests run the grid with all-nil errors, so the claimed 'same failure surface as the unstaggered path' (first error in (reviewer,lens) order, 'reviewer %s lens %s' wrap) is unverified, including the case where the failing attempt is a staggered group's lead. The equivalent wrap was also untested before this change, so this is a carried-forward gap on rewritten plumbing rather than a new bug.
- review_resolution: f_002 resolved: The 'Review run rounds' section still describes the reviewer round as 'five concurrent lens attempts per resolved reviewer' with no stagger qualification. The acceptance criterion required docs/agents.md to no longer describe all review lens attempts as always launching concurrently without qualification; the two earlier mentions were qualified but this one, in the section dedicated to `review run`, was missed.
- review_resolution: f_003 resolved: The new 'Implemented (M25.2)' paragraph claims 'attempt artifacts, IDs, request prompts, and proposal semantics are byte-compatible with the unstaggered path', but attempt result artifacts record started_at, duration_ms, and usage values that necessarily differ. Contract clarification q_003 explicitly scoped byte-compatibility to schemas, paths, ID ordering, request prompt references, summary ordering, and proposal semantics, allowing timestamps/durations/usage to differ; the doc reintroduces the unscoped phrasing.
- review_resolution: f_004 resolved: The stagger tests do not verify that held attempts are released concurrently; a serialized release loop would still pass because held transport calls return immediately and the test only waits for the final launch count.; resolution: Added TestReviewRunReleasesStaggeredClaudeGroupConcurrently in internal/app/review_stagger_test.go. Each held attempt blocks inside the transport on an all-in-flight barrier (allHeldInFlight) that opens only when all len(reviewLenses)-1 held attempts are simultaneously present; a serialized one-at-a-time release deadlocks the first held attempt and the test fails as 'release was serialized'. Verified it fails (5.1s timeout) when launchReviewerLensTasks is temporarily serialized, and passes -race -count=3 against the concurrent implementation.
- review_resolution: f_005 resolved: docs/agents.md still says review run uses five concurrent lens attempts per resolved reviewer, but same-model Claude groups now launch only one lead and hold the remaining attempts until first output, completion, or timeout.; resolution: Edited docs/agents.md 'Review run rounds' section: replaced 'five concurrent lens attempts per resolved reviewer' with 'five lens attempts per resolved reviewer, launched concurrently except that same-model Claude groups are staggered (one lead warms the prompt cache before the rest)', matching the qualified phrasing used in the other two mentions.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- validation: go test ./internal/app ./internal/agents passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
