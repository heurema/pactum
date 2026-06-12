# Reviewer Context

## Run
- Run id: run_20260612_175035
- Run status: contract_approved

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

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 5
- Fresh: 4
- Stale: 1
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/app ./internal/agents (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
- Change summary:
  - changed files:
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
  - new files:
    - internal/app/review_stagger_test.go
  - missing files:
    - none

## Existing manual review
- Review status: pending
- Current findings summary: findings=0 open=0 resolved=0 blocking_open=0
- Existing findings:
  - none
- Existing resolutions:
  - none
- Proposal summary: pending=0 accepted=0 rejected=0
- Existing proposals:
  - none

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
- Execution result: execute/last-result.json

## Reviewer guidance
- This context is not complete semantic truth.
- Use `pactum search "<term>"` and inspect files before proposing findings.
- Do not invent changes.
- Do not approve automatically.
- If you are not certain an issue is real after verification, do not flag it.
