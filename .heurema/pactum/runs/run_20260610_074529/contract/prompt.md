# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_074529
- Approval: approved
- Contract hash: 27b3a031cd6c783a3ba2b4825e230247cec0221b68d3b98b04401b6cf1c37afd

## Goal
Build the autonomous clarify loop (Phase 1 slice 1), composing the shipped grill-me pieces — per-question recommended_answer + confidence (M15.0) and the Converged/coverage signal (M15.5) — into the review-loop pattern: 'pactum clarify loop' rounds suggest -> auto-resolve high-confidence recommendations -> re-suggest, until the clarification converges (no open blocking questions), the loop can make no further progress without a human, or max rounds is reached. The human approval gate downstream is unchanged: the loop automates the question-and-answer churn, while 'contract approve' stays manual — that is the safety story for letting the clarifier's own high-confidence recommendations answer its questions.

## In scope
- New command 'pactum clarify loop [run_id]' (cli.go + a new internal/app/clarify_loop.go): flags --reviewer (explicit clarifier override, as clarify suggest), --max-rounds (defaults to the new clarify.max_rounds config), --timeout (per-attempt idle, default 10m, like review loop), --yes (required for non-interactive agent execution), --json.
- Round structure: (1) run the clarifier exactly as clarify suggest does (reuse its preparation/recording machinery — same prompt, artifacts under clarify/clarifier-attempts/, dedupe stays prompt-level via the existing-questions context); (2) auto-resolve: every OPEN question whose confidence is high and whose recommended_answer is non-empty gets that recommendation recorded as its answer — answer record Source 'auto_recommended', decision record Source 'clarify_loop_auto' (extract the answer+decision write from ClarifyAnswer into a shared helper taking the sources, so manual and loop answers share one code path); questions with medium/low confidence or no recommendation stay open for the human; (3) refresh clarification artifacts once after the round's auto-resolves and recompute the status.
- Stop conditions, checked after each round: Converged (status.BlockingOpen == 0) -> terminal 'converged'; a round that created no new questions AND auto-resolved nothing -> terminal 'needs_human' (automation is out of moves; open blocking questions await the human); rounds exhausted -> terminal 'max_rounds'. Write clarify/loop-summary.json (schema pactum.clarify_loop_summary.v1) with per-round counts (questions created, auto-resolved, open blocking after), the terminal reason, converged, and the final per-dimension coverage; human output mirrors the review loop's summary rendering and the JSON output returns the summary document.
- Config: add a 'clarify' section to the strict config schema with max_rounds (default 3, validated >= 1 like the review limits) — the Phase 1 cap returning per the loop-architecture design note; update defaultConfigFile and the workspace config docs accordingly.
- Ledger: append clarification_loop_started / clarification_loop_finished events around the loop (per-round suggest/answer events already exist). The approval-reset warning semantics (M15.7) flow through unchanged — surface approval_reset in the loop output when a round reset an approved run.
- Tests: loop converges when high-confidence answers clear all blocking questions (terminal converged, auto-resolved answers recorded with the auto sources); loop stops needs_human when open blocking questions are medium/low confidence and a round makes no progress; max_rounds cap honored; the shared answer helper keeps manual ClarifyAnswer behavior byte-identical (source manual); config clarify.max_rounds parsed/validated/defaulted. Use the existing helper-process clarifier test pattern from clarify_suggest_test.go.
- Docs: docs/agents.md gains a clarify loop section (round structure, auto-resolve rule, stop conditions, the manual-approval safety note); docs/backlog.md marks Phase 1 slice 1 shipped (the clarify-loop item) and notes the remaining Phase 1 slices; docs/loop-architecture-design.md's Phase 1 cap row updates (clarify.max_rounds is live again).

## Out of scope
- No auto-approval of the contract (contract approve stays manual); no changes to the clarifier prompt content, the review loop, the agents package, or the transports; no budget gating for the clarify loop this slice (token records still accrue per attempt); no semantic dedup of questions (prompt-level only); no auto-resolve of medium/low-confidence recommendations.

## Paths in scope
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- pactum clarify loop runs suggest/auto-resolve rounds and terminates with converged (no open blocking), needs_human (no progress without a human), or max_rounds; clarify/loop-summary.json records rounds, terminal reason, converged, and coverage; auto-resolved answers carry Source auto_recommended (answer) / clarify_loop_auto (decision) while manual answers stay byte-identical.
- Only open questions with confidence high and a non-empty recommended_answer are auto-resolved; medium/low or recommendation-less questions stay open.
- clarify.max_rounds exists in the strict config (default 3, validated); --max-rounds overrides it; go build ./..., go vet ./..., deadcode, and go test -race ./... are clean; docs updated.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Auto-resolving only high-confidence recommendations is safe because the contract approval downstream remains manual — the loop compresses the Q&A churn, not the human decision.
- The needs_human terminal (no new questions and no auto-resolves in a round) is the correct no-progress signal: anything further requires answers only the human can give.

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
