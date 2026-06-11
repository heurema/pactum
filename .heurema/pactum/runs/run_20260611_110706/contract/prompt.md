# Executor Prompt

This prompt is prepared from an approved Pactum contract.
This prompt is prepared for the selected built-in agent when `pactum execute run` is used.
Pactum records execution artifacts and validates contract, map, and memory boundaries before execution.

## Contract status
- Run: run_20260611_110706
- Approval: approved
- Contract hash: 302efa19660ecad6417f87b2c2384a601b644b4b54fb10654653ec65bbeaee1d

## Goal
Tell the security truth in the user-facing docs and add a security policy. README's built-in agents section currently describes the codex executor as plain 'codex exec' while docs/agents.md documents the real command 'codex exec --dangerously-bypass-approvals-and-sandbox' — README must state the exact command and warn that real execution is unsandboxed with direct repository and runtime access, safe only in trusted repositories. Add SECURITY.md at the repo root with: the threat model (pactum is not a sandbox; the repository and runtime environment are the real boundary; execute run / review run / clarify run / contract draft launch external agent tooling), safe-usage guidance (trusted repositories only, prefer execute plan before execute run, review the contract path scope before execution, avoid exposing long-lived credentials in the environment), private vulnerability reporting to the maintainer before any public issue, and supported versions (main only, until tagged releases exist). SECURITY.md and README must stay consistent with docs/agents.md as the detailed reference. Wire SECURITY.md into the internal/docs test file lists that pin user-facing docs (existence and forbidden stale phrases) so it cannot drift silently.

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

## Clarifications
- q_001 Should SECURITY.md describe only the four commands named in the draft (`pactum execute run`, `pactum review run`, `pactum clarify run`, `pactum contract draft`), or all current commands that can launch external agent tooling, including `pactum clarify suggest`, `pactum review fix run`, and `pactum review loop`?
  Rationale: The draft lists four commands, but `docs/agents.md` documents additional agent-running paths. This affects whether the security policy fully matches the repository's actual execution surface.
  Decision: SECURITY.md should describe all current agent-launching commands, using the four named commands as examples but explicitly including clarify suggest/clarify run, contract draft, execute run, review run, review fix run, and review loop where applicable.
- q_002 Concrete edge case: if a user runs a read-only stage such as `pactum clarify run` or `pactum contract draft` while their shell contains long-lived credentials like `GITHUB_TOKEN`, cloud keys, or agent auth tokens, should SECURITY.md warn that those credentials are still exposed to external agent tooling?
  Rationale: The draft says to avoid exposing long-lived credentials, and `docs/agents.md` says agent subprocesses/adapters inherit the environment even for read-only stages. The policy should avoid implying that read-only stages are safe for secrets.
  Decision: Warn that every agent-launching command may expose inherited environment variables to external tooling, including read-only/reviewer/drafter/clarifier stages, and recommend running Pactum with the smallest practical environment and no long-lived credentials.
- q_003 [blocking] What private vulnerability reporting channel should SECURITY.md name: GitHub private vulnerability reporting for `github.com/heurema/pactum`, a maintainer email/address, or a generic instruction to request a private maintainer channel before filing a public issue?
  Rationale: The repo identifies the module and GitHub owner path, but no maintainer email or enabled private-reporting channel is documented. A security policy needs a usable private path; inventing a contact would be unsafe.
  Decision: Use GitHub private vulnerability reporting at `https://github.com/heurema/pactum/security/advisories/new`; if that is not enabled, instruct reporters to contact the maintainer privately and avoid public issues until a private channel is established.
- q_004 For test coverage, should `internal/docs` only add `SECURITY.md` to the existing required user-facing doc/stale-phrase lists, or should it also assert that SECURITY.md contains the required security-policy concepts?
  Rationale: `internal/docs/docs_test.go` already pins user-facing docs by explicit file list and stale phrase checks. The draft says SECURITY.md should not drift silently, but existence plus forbidden phrases would not catch deletion of the threat model, reporting channel, or supported-version policy.
  Decision: Add `SECURITY.md` to the required user-facing docs and stale-phrase scan, and add focused positive assertions that it mentions Pactum is not a sandbox, trusted repositories, planning before running, path-scope review, credential exposure, private vulnerability reporting, and support for `main` only until tagged releases exist.
- q_005 [blocking] When the contract says README must state the exact `codex` command, should it name the CLI executor descriptor (`codex exec --json --dangerously-bypass-approvals-and-sandbox`) while also explaining that the default transport is ACP via `npx -y @zed-industries/codex-acp@latest`, or should README only use the shorter `docs/agents.md` wording (`codex exec --dangerously-bypass-approvals-and-sandbox`)?
  Rationale: The draft uses 'exact command' and points at `docs/agents.md`, but the repository currently has two concrete command surfaces: `internal/agents/config.go` defines the CLI executor descriptor with `--json`, while `internal/app/app.go` makes ACP the default and `internal/agents/acp_transport.go` launches external npm ACP adapters via `npx`. The README already mentions ACP as default, so silently treating `codex exec ...` as the only exact command could keep the security docs technically incomplete.
  Decision: README should state the CLI executor descriptor as `codex exec --json --dangerously-bypass-approvals-and-sandbox` when describing the CLI transport / executor descriptor, and also say the default ACP transport launches external ACP adapters such as `npx -y @zed-industries/codex-acp@latest`; both paths are direct external tooling with repository and environment access. Keep `docs/agents.md` as the detailed reference and update it only if needed for consistency.
- q_006 What should 'trusted repositories' mean in SECURITY.md and README: a human-vetted repository/task boundary, Codex CLI's trusted-project approval state, or both?
  Rationale: The contract says real execution is safe only in trusted repositories. In `docs/agents.md`, 'trusted repo' also appears in the specific Codex approval-policy sense where Codex may ask no permission. Those are different security concepts, and collapsing them could make the guidance ambiguous.
  Decision: Use 'trusted repository' to mean a repository and task context the human is willing to expose to arbitrary external agent tooling, including repository files, shell commands, and inherited environment variables. If Codex's trusted-project state is mentioned, distinguish it as an agent-specific approval setting that can increase risk; it is not what makes a repository safe.
- q_007 Concrete edge case: if a user runs the default ACP transport for the first time and `npx -y @zed-industries/codex-acp@latest` or `npx -y @agentclientprotocol/claude-agent-acp@latest` downloads and executes an external adapter package, should SECURITY.md explicitly warn about that external package/runtime dependency in addition to warning about agent CLIs?
  Rationale: `docs/agents.md` and `internal/agents/acp_transport.go` show ACP is the default and runs adapter packages through `npx`; q_002 already covers inherited credentials for agent-launching commands, but not the separate supply-chain/runtime implication of executing latest external npm adapters. This affects whether the security policy fully describes the default execution surface.
  Decision: Yes. SECURITY.md should say Pactum may launch external agent tooling either through the default ACP adapters (`npx` packages) or the CLI transport, and both inherit the repository/runtime environment. Users in restricted or high-sensitivity environments should review/pin/control those tools outside Pactum before running agent-launching commands.
- q_008 Should this contract update `docs/backlog.md` to remove or mark complete the existing P0 item that says README still describes `codex` as plain `codex exec`, or is `docs/backlog.md` out of scope because the contract targets user-facing docs and `internal/docs` pins only README/install/flow/workspace/agents/memory plus the new SECURITY.md?
  Rationale: `docs/backlog.md` contains the exact security-docs task text and will become stale after the README/SECURITY.md change, but it is not part of `internal/docs/docs_test.go`'s required user-facing doc set. This changes whether the implementation touches only the user-facing docs/tests named in the contract or also updates project planning documentation.
  Decision: Keep `docs/backlog.md` out of scope for this contract; update README.md, add SECURITY.md, adjust docs/agents.md only if needed for consistency, and wire SECURITY.md into `internal/docs` tests. Treat backlog pruning or completion marking as a separate cleanup.

## Project context
- Executor context: context/executor-context.md
- Repo map: .heurema/pactum/map/repo-map.md
- Search results: context/search-results.json
- Accepted memory context: context/memory-context.md

## Accepted memory

Memory context:
- context/memory-context.md

Selected memory:
- total: 3
- fresh: 1
- stale: 2
- unknown: 0

Items:
- mem_002 [stale] score=56 — Normalize the CLI command grammar for agent-first use: every stage exposes a ...
  reason: missing file internal/app/agents_doctor.go
  reason: missing file internal/app/agents_doctor_test.go
- mem_003 [stale] score=36 — Remove the interactive confirmation layer from the CLI: the consumer is an AI...
  reason: missing file internal/app/confirm.go
- mem_001 [fresh] score=24 — Add an export command that dumps a run's full record as a single archive

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
