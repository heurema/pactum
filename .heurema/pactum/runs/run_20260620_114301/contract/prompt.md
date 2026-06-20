# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260620_114301
- Approval: approved
- Contract hash: 9a368a136d870b58cd5cacddb7102810932e41d6477c047e1f3be01d22ffd446

## Goal
De-hardcode `--agent codex` from pactum's lifecycle `next` affordances and human "Next:" output so the suggested execute-plan command respects the user's CONFIGURED executor instead of steering every user — including a claude-only user — to codex.

Background: the skill-install slice (#214) shipped a SKILL.md that correctly uses an `<agent>` placeholder and tells the agent to use the configured executor, but pactum's own machine/human surfaces still emit `pactum execute plan <run> --agent codex` literally. This was flagged by that slice's two non-blocking review findings (f_005: the lifecycle `next` affordance hardcodes codex; f_007: the human-facing output hardcodes it). Because an agent driving pactum runs the `next` array VERBATIM, a claude-only user is silently steered to `--agent codex`, contradicting the cross-agent guidance. The fix is to DROP the hardcoded `--agent codex` so the command defers to execute-plan's existing default-agent resolution (an omitted `--agent` already resolves the configured executor via `prepareExecution`). This is the pattern the codebase already uses elsewhere: `nextCommandForStatus` and `resolve.go`'s `nextCommandForStatus` sibling already emit the bare `pactum execute plan` form, and `execute plan` with no `--agent` is already runnable.

In scope (exactly three production affordance sites + their tests; change ONLY the emitted command string, not resolution logic):
1. internal/app/resolve.go — the `prompt_built` case in the `next`-affordance builder currently returns `[]string{"pactum execute plan " + runID + " --agent codex"}`. Drop ` --agent codex` so it returns `"pactum execute plan " + runID`.
2. internal/app/errors.go — `noExecutionAttemptError` sets `next: []string{"pactum execute plan " + runID + " --agent codex"}`. Drop ` --agent codex` likewise.
3. internal/app/prompt.go — the human "Next:" line in the prompt-build writer prints `"  pactum execute plan %s --agent codex\n"`. Drop ` --agent codex` so it prints `"  pactum execute plan %s\n"`.
4. Update the tests that assert the OLD string to assert the new bare form: internal/app/prompt_test.go (the `pactum execute plan <run> --agent codex` expectation) and internal/app/affordances_test.go (all three: the `wantNext` builder, the `assertNext` line, and the human-output `Next:` substring check).

The result: every lifecycle affordance for the execute-plan step emits `pactum execute plan <run>` (no `--agent`), so it respects `pipeline.execute.by` (the configured executor) for whoever is running — a claude user gets their claude executor, a codex user gets codex — instead of a hardcoded codex. This is consistent with the SKILL.md `<agent>` guidance and with the already-bare affordances elsewhere in resolve.go.

Out of scope (do NOT do here):
- Do NOT change execute-plan's default-agent RESOLUTION logic (`prepareExecution` / `ExecutePlan`); only the emitted affordance/human strings change. The omitted-`--agent` default already works.
- Do NOT touch internal/app/skill.go's "no agent targets detected; use --agent claude, --agent codex, or --agent all" help text — it legitimately lists all install targets and is not a lifecycle affordance.
- Do NOT introduce a new `--agent` placeholder token in the machine `next` array (an agent must be able to run it verbatim; a literal `<agent>` would not run). Dropping the flag is the correct machine-runnable form.
- No schema change, no new flag, no change to the contract-review "no nitpicks" calibration (a separate slice), no docs/SKILL.md changes (the skill is already correct).

Tests: the updated affordance/prompt tests assert the bare `pactum execute plan <run>` form (no `--agent codex`) in both the machine `next` arrays (resolve.go and errors.go paths) and the human "Next:" output (prompt.go). Verify no other test still asserts the hardcoded `--agent codex` for these lifecycle affordances (the skill_test.go references to `--agent codex` are for `skill install` and must stay).

Validation: go build ./..., go vet ./..., go test ./internal/..., go test ./..., make check.

## In scope
- Change internal/app/resolve.go so the prompt_built lifecycle next affordance emits pactum execute plan <runID> without --agent codex.
- Change internal/app/errors.go so noExecutionAttemptError emits pactum execute plan <runID> without --agent codex.
- Change internal/app/prompt.go so the human Next: line after prompt build prints pactum execute plan <runID> without --agent codex.
- Update internal/app/prompt_test.go and internal/app/affordances_test.go assertions for these lifecycle affordance and human-output surfaces to expect the bare execute-plan command.

## Out of scope
- Changing execute-plan default agent resolution, including prepareExecution or ExecutePlan behavior.
- Changing CLI schema, flags, prompt manifest structure, or adding a placeholder agent token to machine next arrays.
- Changing internal/app/skill.go install-target help text or unrelated skill-install tests that legitimately mention --agent codex.
- Changing docs, SKILL.md guidance, or contract-review calibration behavior.

## Acceptance criteria
- All lifecycle next arrays for the prompt-built execute-plan step emit exactly pactum execute plan <runID> with no --agent codex suffix.
- The no-execution-attempt error next array emits exactly pactum execute plan <runID> with no --agent codex suffix.
- The human prompt-build Next: output emits exactly pactum execute plan <runID> with no --agent codex suffix.
- Tests covering the affected affordance and prompt output paths assert the bare execute-plan command.
- No affected lifecycle affordance test or production affordance string still asserts or emits pactum execute plan <runID> --agent codex.
- Existing execute-plan default-executor behavior remains unchanged and covered by the existing execute tests.

## Validation commands
- go build ./...
- go vet ./...
- go test ./internal/...
- go test ./...
- sh -c 'if rg -n "pactum execute plan .*--agent codex" internal/app/resolve.go internal/app/errors.go internal/app/prompt.go internal/app/prompt_test.go internal/app/affordances_test.go; then exit 1; fi'
- make check

## Assumptions
- Omitting --agent from pactum execute plan already resolves to the configured executor through existing prepareExecution behavior.
- pipeline.execute.by remains the source of the configured executor for default execute-plan resolution.
- The --agent codex mentions in internal/app/skill.go and related skill-install tests are not lifecycle affordances and should remain unchanged.
- The existing SKILL.md guidance is already correct and does not need documentation changes for this slice.

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
- total: 5
- fresh: 5
- stale: 0
- unknown: 0

Items:
- mem_028 [fresh] score=62 — Make the pactum agent skill self-sufficient and one-command installable, so a...
- mem_007 [fresh] score=57 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_021 [fresh] score=55 — Make pactum's code-review loop never silently drop reviewer findings, and rec...
- mem_005 [fresh] score=55 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_020 [fresh] score=53 — Add the plan-DAG schema and structural validation to the contract — slice 1 o...

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

## House style
- Match the surrounding code: idiom, naming, comment density.
- Comment only where the code is not self-explanatory; do not narrate the obvious.
- Search for and reuse existing helpers before writing new ones.
- Keep the diff small and focused: change only what the contract requires.
- Simplicity first: no enterprise patterns for simple problems, question every new abstraction, no premature generalization or optimization.
- Over-engineering DON'Ts: wrappers that add nothing, factories or abstractions for a single case, unused extension points, dual implementations where the old path has no callers, silent fallbacks that hide failures.
- No dead code, no commented-out code, no unused parameters.
- Handle errors per the project's existing convention; no silent failures.
- Tests verify behavior, not implementation details, and cover error paths.
- Fake-test DON'Ts: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.
