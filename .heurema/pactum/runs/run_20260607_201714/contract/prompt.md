# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260607_201714
- Approval: approved
- Contract hash: 31b9ea7139cc95fae991ddd49e082196bd59e698a86121ec02fd3d767fdaa693

## Goal
Token budget stop for the autonomous review->fix loop (slice 4 of docs/cost-budget-design.md, brought before the cost slice). Add budget.max_tokens; when set, the loop halts once the run's cumulative CAPTURED token usage reaches the budget, terminating with a distinct terminal reason budget_exceeded that composes with the existing M11.8 gate_failed / stalemate terminals. budget.mode (block|warn) controls enforcement. Tokens are the unit; no cost/$ in this slice

## In scope
- Config: add budget.max_tokens (*int64; nil = off, no budget) to budgetConfig in internal/app/app.go; keep mode and max_usd. Change the default budget.mode from warn to block (enforce-when-a-budget-is-set, consistent with blocking path-scope; harmless when no budget is set). Validate mode is block|warn (empty -> block)
- Loop enforcement (internal/app/review_loop.go): when budget.max_tokens is set, at the start of each round (after the first) compute the run's cumulative captured token total by summing the run usage ledger (reuse the usage reader from internal/app/usage.go). If the total >= max_tokens: mode block -> stop the loop with summary.TerminalReason = budget_exceeded (clean terminal, ReviewLoop returns nil, like stalemate/gate_failed), recording the token total and the budget in the summary; mode warn -> record a warning and continue. Post-call accumulation is the source of truth (a single round may overshoot by its own calls; that is acceptable and documented)
- Surface: the loop summary records terminal_reason budget_exceeded plus the token total and the configured max_tokens; the human + JSON loop output make the budget stop clear. status / pactum usage already show the per-run tokens
- Tests (deterministic, fake-agent loop harness): with budget.max_tokens set low and captured usage exceeding it, the loop stops with terminal_reason budget_exceeded; mode warn -> the loop continues to its other terminal; no max_tokens -> the loop is unaffected; the budget sum counts only captured usage records
- Docs: update docs/flow.md (loop terminal reasons incl budget_exceeded) and docs/backlog.md / docs/cost-budget-design.md (budget-stop slice implemented)

## Out of scope
- Cost in dollars and max_usd enforcement (max_usd stays declared but unenforced; this slice is token-native)
- Pre-call budget estimation / gating (post-call accumulation only); per-call or one-shot execute budget (the loop is the target)
- Native LLM API or provider abstraction; editing generated .heurema run artifacts

## Paths in scope
- internal/app/**
- docs/**


## Acceptance criteria
- With budget.max_tokens set and the loop's cumulative captured usage reaching it, the loop stops with terminal_reason budget_exceeded (mode block, the default); mode warn makes it advisory (loop continues)
- With no budget.max_tokens, the loop behaves exactly as before
- budget_exceeded is a clean terminal (ReviewLoop returns nil; summary records the terminal reason, token total, and max_tokens); it composes with the existing terminals
- The budget total sums only captured usage records; uncaptured calls do not count (documented limitation)
- make check green (incl deadcode); go test -race ./... clean

## Validation commands
- make check

## Assumptions
TBD

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
