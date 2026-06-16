# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260616_072948
- Approval: approved
- Contract hash: 22ff486063ecb3726fc492c8ea66a9a1fa327e5239f94ced2f04612be1058605

## Goal
Sanitize absolute home paths in the recorded would_run command so committed dry-run artifacts never leak a home path. When PACTUM_CLAUDE_ACP_COMMAND or PACTUM_CODEX_ACP_COMMAND is set to an absolute path under the user's home (e.g. ~/<user>/repos/.../codex-acp), acpAdapterCommand returns it verbatim and BuildACPWouldRun / BuildDryRunPlan (internal/agents/dryrun.go) record it in the would_run.command of request.json artifacts. Those artifacts are committed under .heurema and then fail 'make heurema-hygiene' with a home-path leak (this actually happened for run_20260615_222003's codex reviewer attempts). Fix: when building the would_run DryRunCommand, replace the user's home-directory prefix with '~' in Command (and Args/Env, where an absolute home path can also appear), so the recorded representation has no absolute home path. Use os.UserHomeDir() to get the prefix; if it cannot be determined, or the value is not under the home directory, leave it unchanged. The ACTUAL executed adapter command must stay byte-for-byte unchanged — only the recorded would_run representation is sanitized (ACPTransport.Run still uses the real path). Add a unit test for BuildACPWouldRun that an absolute home-path override is recorded as '~/...' while a non-home value (e.g. 'npx') is left as-is. Keep claude and codex behavior identical; do not change acpAdapterCommand's actual command resolution.

## In scope
- Sanitize the recorded DryRunCommand produced by BuildACPWouldRun and BuildDryRunPlan by replacing an os.UserHomeDir() prefix in Command, Args, and Env with '~'.
- Add focused unit coverage for home-path overrides and non-home command values for both codex and claude ACP would_run recording.

## Out of scope
- Changing acpAdapterCommand resolution semantics or the PACTUM_CLAUDE_ACP_COMMAND and PACTUM_CODEX_ACP_COMMAND override format.
- Changing the actual ACPTransport.Run process launch command, args, or env.
- Weakening cmd/heurema-hygiene home-path detection.
- Rewriting historical .heurema run records as part of this feature change.

## Acceptance criteria
- When PACTUM_CODEX_ACP_COMMAND or PACTUM_CLAUDE_ACP_COMMAND is set to an absolute path under os.UserHomeDir(), BuildACPWouldRun records would_run.command as '~/...' with no absolute home prefix.
- BuildDryRunPlan records sanitized Command, Args, and Env values whenever those recorded values contain a path under os.UserHomeDir().
- Non-home values such as 'npx', 'codex-acp', and relative override paths are recorded unchanged.
- Values are left unchanged when os.UserHomeDir() cannot be determined or when the value is not equal to or below the home directory.
- Existing acpAdapterCommand override tests still pass, demonstrating the actual adapter command resolution remains unchanged.

## Validation commands
- go test ./internal/agents -run TestBuildACPWouldRun
- make check
- make heurema-hygiene

## Assumptions
- Textual prefix replacement using os.UserHomeDir() is sufficient; symlink, case-folding, and path canonicalization are not required.
- The '~' substitution is only for recorded artifacts and is never reused as an executable command path.
- Env values are recorded as KEY=value strings, so sanitizing the recorded string representation is sufficient.

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
- fresh: 4
- stale: 1
- unknown: 0

Items:
- mem_013 [fresh] score=44 — Dogfood Pactum with a local codex-acp adapter that returns official ACP Promp...
- mem_012 [fresh] score=30 — Capture Codex token usage from ACP usage_update metadata and add per-engine A...
- mem_005 [fresh] score=30 — Make the CLI announce legal moves so an agent never guesses the pipeline stat...
- mem_007 [fresh] score=26 — Fix three valid external review findings. (1) pactum export must preserve its...
- mem_002 [stale] score=26 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go

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
