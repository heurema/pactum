# Reviewer Context

## Run
- Run id: run_20260611_110706
- Run status: contract_approved

## Contract
- Goal: Tell the security truth in the user-facing docs and add a security policy. README's built-in agents section currently describes the codex executor as plain 'codex exec' while docs/agents.md documents the real command 'codex exec --dangerously-bypass-approvals-and-sandbox' — README must state the exact command and warn that real execution is unsandboxed with direct repository and runtime access, safe only in trusted repositories. Add SECURITY.md at the repo root with: the threat model (pactum is not a sandbox; the repository and runtime environment are the real boundary; execute run / review run / clarify run / contract draft launch external agent tooling), safe-usage guidance (trusted repositories only, prefer execute plan before execute run, review the contract path scope before execution, avoid exposing long-lived credentials in the environment), private vulnerability reporting to the maintainer before any public issue, and supported versions (main only, until tagged releases exist). SECURITY.md and README must stay consistent with docs/agents.md as the detailed reference. Wire SECURITY.md into the internal/docs test file lists that pin user-facing docs (existence and forbidden stale phrases) so it cannot drift silently.
- In scope:
  - Update README.md built-in agent/security text to name the CLI codex executor descriptor as `codex exec --json --dangerously-bypass-approvals-and-sandbox` and to describe the default ACP adapter path such as `npx -y @zed-industries/codex-acp@latest` as external tooling with repository and environment access.
  - Add root SECURITY.md covering Pactum's threat model, all current agent-launching commands, safe-use guidance, private vulnerability reporting, and supported versions.
  - Update docs/agents.md only if needed to keep README.md, SECURITY.md, and docs/agents.md consistent.
  - Update internal/docs tests so SECURITY.md is a required user-facing doc, is included in stale-phrase scanning, and has focused positive assertions for required security-policy concepts.
- Out of scope:
  - Do not update docs/backlog.md for this contract.
  - Do not add sandboxing, credential filtering, command gating, transport changes, or other runtime security controls.
  - Do not run real agent execution commands such as `pactum execute run`, `pactum review run`, `pactum clarify run`, `pactum clarify suggest`, `pactum contract draft`, `pactum review fix run`, or `pactum review loop`.
- Acceptance criteria:
  - README.md no longer describes the codex executor as plain `codex exec` and instead documents `codex exec --json --dangerously-bypass-approvals-and-sandbox` for CLI transport plus the default ACP adapter path.
  - README.md warns that real agent execution is unsandboxed direct external tooling with repository, runtime, and inherited environment access, and is appropriate only for a trusted repository/task context.
  - SECURITY.md states that Pactum is not a sandbox and that the repository plus runtime environment are the real security boundary.
  - SECURITY.md explicitly covers agent-launching commands including `pactum clarify suggest`, `pactum clarify run`, `pactum contract draft`, `pactum execute run`, `pactum review run`, `pactum review fix run`, and `pactum review loop`.
  - SECURITY.md warns that read-only/reviewer/drafter/clarifier stages can still expose inherited environment variables and recommends the smallest practical environment with no long-lived credentials.
  - SECURITY.md recommends trusted repositories only, `pactum execute plan` before `pactum execute run`, reviewing contract path scope before execution, and reviewing/pinning/controlling external ACP or CLI tooling in restricted environments.
  - SECURITY.md directs private vulnerability reports to `https://github.com/heurema/pactum/security/advisories/new` and says to avoid public issues until a private maintainer channel is established if GitHub private reporting is unavailable.
  - SECURITY.md states supported versions are `main` only until tagged releases exist.
  - internal/docs tests require SECURITY.md, scan it for forbidden stale phrases, and assert the required security-policy concepts.
- Validation commands:
  - go test ./internal/docs
  - make check

## Accepted memory
- Memory context: context/memory-context.md
- Selected items: 3
- Fresh: 1
- Stale: 2
- Unknown: 0
- Stale memory may be outdated and must be verified.

## Gate report
- Gate status: needs_review
- Execution attempt id: attempt_001
- Execution exit code: 0
- Validation command results:
  - command_001: go test ./internal/docs (exit 0, timed out: false, result: gate/validation/command_001/result.json)
  - command_002: make check (exit 0, timed out: false, result: gate/validation/command_002/result.json)
- Change summary:
  - changed files:
    - README.md
    - docs/agents.md
    - internal/docs/docs_test.go
  - new files:
    - SECURITY.md
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
