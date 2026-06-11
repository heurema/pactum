# Contract Draft Proposal

## Status
- Run id: run_20260611_110706
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-11T11:13:53Z

## In scope
- Update README.md built-in agent/security text to name the CLI codex executor descriptor as `codex exec --json --dangerously-bypass-approvals-and-sandbox` and to describe the default ACP adapter path such as `npx -y @zed-industries/codex-acp@latest` as external tooling with repository and environment access.
- Add root SECURITY.md covering Pactum's threat model, all current agent-launching commands, safe-use guidance, private vulnerability reporting, and supported versions.
- Update docs/agents.md only if needed to keep README.md, SECURITY.md, and docs/agents.md consistent.
- Update internal/docs tests so SECURITY.md is a required user-facing doc, is included in stale-phrase scanning, and has focused positive assertions for required security-policy concepts.

## Out of scope
- Do not update docs/backlog.md for this contract.
- Do not add sandboxing, credential filtering, command gating, transport changes, or other runtime security controls.
- Do not run real agent execution commands such as `pactum execute run`, `pactum review run`, `pactum clarify run`, `pactum clarify suggest`, `pactum contract draft`, `pactum review fix run`, or `pactum review loop`.

## Acceptance criteria
- README.md no longer describes the codex executor as plain `codex exec` and instead documents `codex exec --json --dangerously-bypass-approvals-and-sandbox` for CLI transport plus the default ACP adapter path.
- README.md warns that real agent execution is unsandboxed direct external tooling with repository, runtime, and inherited environment access, and is appropriate only for a trusted repository/task context.
- SECURITY.md states that Pactum is not a sandbox and that the repository plus runtime environment are the real security boundary.
- SECURITY.md explicitly covers agent-launching commands including `pactum clarify suggest`, `pactum clarify run`, `pactum contract draft`, `pactum execute run`, `pactum review run`, `pactum review fix run`, and `pactum review loop`.
- SECURITY.md warns that read-only/reviewer/drafter/clarifier stages can still expose inherited environment variables and recommends the smallest practical environment with no long-lived credentials.
- SECURITY.md recommends trusted repositories only, `pactum execute plan` before `pactum execute run`, reviewing contract path scope before execution, and reviewing/pinning/controlling external ACP or CLI tooling in restricted environments.
- SECURITY.md directs private vulnerability reports to `https://github.com/heurema/pactum/security/advisories/new` and says to avoid public issues until a private maintainer channel is established if GitHub private reporting is unavailable.
- SECURITY.md states supported versions are `main` only until tagged releases exist.
- internal/docs tests require SECURITY.md, scan it for forbidden stale phrases, and assert the required security-policy concepts.

## Validation commands
- go test ./internal/docs
- make check

## Assumptions
- GitHub private vulnerability reporting for `github.com/heurema/pactum` is the preferred private reporting channel; SECURITY.md should include fallback private-contact guidance if that GitHub advisory flow is not enabled.

