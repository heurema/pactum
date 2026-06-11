# Review Fixer Context

## Run
- Run id: run_20260611_110706
- Run status: contract_approved

## Approved contract
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

## Current review findings
- Summary: findings=4 open=4 resolved=0 blocking_open=2
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_002 severity=medium category=correctness blocking=true status=open: SECURITY.md says `pactum execute plan` lets users inspect the exact command Pactum would launch, but under the default ACP transport the plan records the CLI descriptor while `execute run` launches the external `npx` ACP adapter.
    location: SECURITY.md:45
  - f_004 severity=medium category=validation blocking=true status=open: The SECURITY.md concept assertions do not pin the required safe-usage guidance to review, pin, or control external ACP/CLI tooling in restricted environments.
    location: internal/docs/docs_test.go:134
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_001 severity=low category=quality blocking=false status=open: No test pins the README security claims this contract exists to protect: the exact executor command `codex exec --json --dangerously-bypass-approvals-and-sandbox`, the ACP adapter path `npx -y @zed-industries/codex-acp@latest`, and the 'unsandboxed' warning appear in README.md:54-77 but in neither requiredDocMentions nor requiredSecurityPolicyMentions, and plain `codex exec` cannot be a forbidden phrase (substring of the full command). The goal's stated problem — README drifting back to plain `codex exec` while docs/agents.md stays correct — would not be caught by any test. Adding the full command string to requiredDocMentions would close most of the gap.
    location: internal/docs/docs_test.go:66
  - f_003 severity=low category=correctness blocking=false status=open: README.md says the generated default registers an unpinned `claude` entry, but the default config pins `model: claude-opus-4-8` and docs/agents.md states every registry entry pins its model.
    location: README.md:64

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
