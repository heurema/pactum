# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260610_132459
- Approval: approved
- Contract hash: 3b484190bb15eb008d9d9f30af04722843c9e997572612b0c719e80e5069eb32

## Goal
Make the five review lenses the built-in default behavior, not a config knob: every review spawns all five specialist reviewers itself. The lens set is fixed in code (correctness, implementation, tests, over_engineering, docs — the M19.0 checklists). review run and every review-loop round expand each resolved reviewer (the explicit --reviewer or each review.panel member) into five concurrent lens attempts, each with a focused prompt: only its own lens checklist plus a panel-focus note, while the shared hardened sections (high-signal contract, verify-then-report, pre-existing policy, output ordering, confidence schema) stay identical across lenses. Nothing is configurable: no new config fields, the registry entry shape is untouched, and the combined all-five-lenses prompt disappears because every attempt is lensed. Cross-lens duplicate findings collapse through the existing fingerprint dedup with severity-max; the cost of five attempts per reviewer per round is a deliberate default.

## In scope
- Config: agentRegistryEntry gains an optional lens field (yaml lens,omitempty). Validation at readConfig: a non-empty lens must be one of correctness, implementation, tests, over_engineering, docs (clear error naming the entry and the allowed set); empty means no lens. The lens applies only to review stages — an entry used as executor/fixer/clarifier/drafter ignores it (documented), so no cross-role validation error.
- Prompt: renderReviewerPrompt takes the lens; with a lens, the Review lenses section renders ONLY that lens's checklist and adds a focus note ('You are the <lens> reviewer in a panel; other lenses are covered by other reviewers — report only findings within your lens; do not silently expand scope'); with no lens, the section renders all five exactly as today (the unlensed prompt text must remain unchanged). The shared sections (high-signal contract, verify-then-report, pre-existing policy, output ordering, output shape with confidence) are identical in both variants.
- Artifacts and concurrency: panel members run concurrently, so a lensed member's prompt is written to a per-member artifact path (derived from the registry name, e.g. review/reviewer-prompt-<name>.md) and its attempt/request records point at that path; an unlensed reviewer keeps today's shared review/reviewer-prompt.md path so existing artifacts and tests stay stable. review dry-run/run on a lensed --reviewer shows and uses the per-member path; the review loop writes each lensed member's prompt before launching the round.
- Tests: lens validation (valid set, invalid value error naming the entry); the lensed prompt contains only its lens heading plus the focus note while the other four lens headings are absent, and still contains the shared sections; the unlensed prompt is unchanged (pin: all five headings, no focus note); per-member prompt artifact path used for a lensed reviewer in dry-run and in the loop round (two-member panel, one lensed + one unlensed, each attempt pointing at its own prompt). Update existing tests only where the renderReviewerPrompt signature changes.
- Docs: agents.md — the registry entry table/description gains lens with the allowed set and the review-only semantics, plus a short panel-of-specialists example; docs/review-prompt-design.md marks the follow-up shipped; docs/backlog.md updates the prompt-hardening item (arc complete).
- A fixed in-code lens set (correctness, implementation, tests, over_engineering, docs). renderReviewerPrompt takes the lens and renders the focused variant: the Review lenses section holds only that lens's checklist plus a focus note ('You are the <lens> reviewer; other lenses are covered by other reviewers running in parallel — report only findings within your lens; do not silently expand scope'); all shared sections identical across lenses.
- Fan-out: review run and each review-loop round spawn, for every resolved reviewer (explicit --reviewer or each panel member), five concurrent lens attempts. Per-attempt prompt artifacts keyed by member and lens (e.g. review/reviewer-prompt-<member>-<lens>.md) written before the attempts launch, each attempt's request pointing at its own prompt — concurrent-safe because registry names are unique and the lens set is fixed. The attempt request/result records and the review round summary surface the lens; usage records keep the registry name as agent_name (five usage records per member per round is expected).
- Aggregation unchanged: the loop's existing finding fingerprint dedup and severity-upgrade-on-duplicate collapse cross-lens duplicates; in single review run the proposals from the five attempts accumulate for human triage as today. review dry-run shows the five lens attempts that would run.
- Tests: each lens prompt contains only its own lens heading plus the focus note and all shared sections (and not the other four headings); a loop round with a two-member panel spawns member-times-lens attempts, each pointing at its own prompt artifact, and cross-lens duplicate findings still collapse to one with severity-max; review dry-run lists the lens fan-out; update existing review/loop tests for the new attempt counts and prompt paths.
- Docs: agents.md describes the five built-in specialist reviewers every review spawns (with the focus-note semantics and the deliberate five-attempts cost); docs/review-prompt-design.md marks the lens follow-up shipped as built-in default behavior rather than configuration; docs/backlog.md marks the prompt-hardening arc complete.

## Out of scope
- The workspace config.yaml stays unchanged (the feature is opt-in; the current two-member panel remains full-coverage generalists — lens adoption for the dogfood panel is a separate decision); no changes to finding parsing/confidence/fingerprint, the fixer, clarifier, drafter, gate, or loop convergence; no lens-based finding filtering at parse time (the prompt instructs focus; the reviewer remains free-form within the schema).
- NO config or registry schema changes (no lens field — lenses are explicitly not configurable); no parse-time filtering of findings by lens (the prompt instructs focus, the high-signal contract governs); no changes to the fixer, clarifier, drafter, gate, convergence semantics, or the transports; the workspace config.yaml is untouched.

## Paths in scope
- internal/app/*.go
- docs/*.md
- internal/app/*.go
- docs/*.md


## Acceptance criteria
- A registry entry accepts lens from the allowed set with loud validation; a lensed reviewer's prompt carries only its lens checklist plus the focus note while shared sections stay intact; the unlensed prompt is byte-stable; lensed members get per-member prompt artifacts safe for concurrent panel rounds.
- go build ./..., go vet ./..., and the deadcode gate are clean; go test -race ./... passes; docs mark the prompt-hardening arc complete.
- Every review run and every loop round spawns five lens attempts per resolved reviewer by default, each with a focused per-lens prompt artifact and the lens visible in its records; no configuration exists for it; the shared prompt sections are identical across lenses.
- Cross-lens duplicate findings collapse through the existing fingerprint dedup with severity-max; review dry-run lists the fan-out; go build ./..., go vet ./..., the deadcode gate, and go test -race ./... are clean; docs mark the arc complete.

## Validation commands
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...
- go build ./...
- go test ./internal/app/...
- go vet ./...
- go test -race ./...

## Assumptions
- Lens names map one-to-one onto the M19.0 prompt lens checklists; focusing is prompt-level guidance (parse-time filtering by lens is deliberately not done — the high-signal contract already governs what gets reported).
- Per-member prompt artifacts keyed by registry name are concurrency-safe because registry names are unique (validated) and each panel member writes its own file before the round launches.
- Specialist fan-out is the right default because the panel previously ran identical generalists — five focused attempts per reviewer with the existing dedup/severity-max aggregation raise recall without any new knobs; the token cost is accepted deliberately.

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
