# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260608_130542
- Approval: approved
- Contract hash: 7dcd7badc4bb0f045502c0bfd1403e05b6d12d317aab7bfc80262999c369a24f

## Goal
Make the autonomous review loop converge by gating on BLOCKING findings (builds on M12.8 resolve-on-fix). Add a new terminal reason resolved: the loop terminates resolved when no open blocking findings remain (BlockingOpen == 0 — the same condition that makes a review approvable). Non-blocking findings are still accepted and recorded as advisory but do NOT keep the loop running or drive the fixer, which stops the low/subjective-finding churn that made the M12.6 dogfood run to max_rounds without converging.

## In scope
- Convergence terminal resolved in ReviewLoop (internal/app/review_loop.go): after a round accepts its proposals (and, when a fixer ran, after applying fix outcomes), compute the live count of OPEN BLOCKING findings — open findings with Blocking==true, the existing BlockingOpen from summarizeReview. When it is 0, set summary.TerminalReason = "resolved" and stop. This is the primary success terminal; keep the existing terminals (clean_round, stalemate, max_rounds, gate_failed, budget_exceeded, reviewer_findings_unparsed) intact for their cases.
- Fixer gated on blocking: the loop runs the fixer only when there are open blocking findings. Update the fixer prompt (renderReviewFixPrompt) to PARTITION the open findings into "BLOCKING — fix or rebut these (emit a fix-outcome for each)" and "advisory (non-blocking) — context only, do NOT edit code for them and do NOT emit outcomes for them". The M12.8 fix-outcomes apply step then resolves the blocking findings.
- Non-blocking (advisory) findings: still accepted and recorded (audit + dedup unchanged from M11.4/M12.8), stay open as advisory, are never fixed, and do not gate convergence. Add open_blocking_findings to the round summary (the BlockingOpen count after the round) alongside the existing open_findings.
- Reviewer prompt blocking guidance: update renderReviewerPrompt to instruct the reviewer/panel to set blocking:true for findings that must block a merge (correctness/security bugs, high or critical severity) and blocking:false for advisory/style/low findings — so the convergence gate is meaningful. This is a targeted blocking-guidance addition only.
- Tests: cover the resolved terminal (a round whose only blocking finding is rebutted converges resolved within a couple of rounds; a round whose proposals are all non-blocking converges resolved without running the fixer; open_blocking_findings reported). Update any pre-existing loop test that asserted the OLD convergence (clean_round/max_rounds/stalemate) for a scenario that now legitimately resolves, adjusting its stub findings blocking flags and/or expected terminal — do NOT mask a real regression. Docs: note the resolved terminal and the blocking gate in docs/flow.md.

## Out of scope
- The broad anti-meta-churn / house-style+best-practices reviewer-and-executor prompt rework (slice 3 and the separate prompt-quality backlog item) — only the targeted blocking-guidance sentence is added here.
- Severity-DERIVED blocking in code (auto-marking high/critical as blocking regardless of the flag) — convergence keys on the existing Blocking flag; the reviewer prompt is responsible for setting it.
- Changing the M12.8 resolve-on-fix mechanism, the cross-model panel, the gate, budget, or the fixer agent/model/least-privilege; native LLM API.

## Paths in scope
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- The loop terminates resolved when no open blocking findings remain; non-blocking findings may remain open (advisory) and are recorded; open_blocking_findings appears on the round summary.
- The fixer runs only when open blocking findings exist and is scoped to them; a round whose accepted proposals are all non-blocking converges resolved without invoking the fixer.
- A run with a single blocking finding that the fixer rebuts (false positive) converges resolved within a couple of rounds, not max_rounds.
- make check (incl. deadcode + git diff --check) and make test-race pass; updated loop tests reflect the new convergence and no real regression is masked.

## Validation commands
- make build
- make check
- make test-race

## Assumptions
- BlockingOpen (open findings with Blocking==true, from summarizeReview) already exists and is the same condition gating review approval; reuse it for the convergence gate.
- M12.8 resolve-on-fix is in place: fixer outcomes resolve blocking findings so BlockingOpen drops as they are fixed/rebutted.
- No backward-compatibility constraints; additive summary fields and a new terminal reason are free, and loop tests may be updated to the new convergence model.

## Clarifications
- None

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 0
- fresh: 0
- stale: 0
- unknown: 0

Items:
- none

Rules:
- Accepted memory is context, not semantic truth.
- Stale memory may be outdated; verify before using.
- Use `pactum search "<term>"` and inspect current source files before relying on memory.
- Do not implement from memory alone.

## Instructions for future executor
- Follow the approved contract.
- Do not implement out-of-scope work.
- Search before creating new code.
- Prefer existing code items when applicable.
- If the contract is ambiguous, stop and request clarification.
- Use the listed validation commands as expected checks.
- Pactum gate can run approved validation commands after execution.
