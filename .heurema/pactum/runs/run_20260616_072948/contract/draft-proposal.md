# Contract Draft Proposal

## Status
- Run id: run_20260616_072948
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-16T07:32:51Z

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

