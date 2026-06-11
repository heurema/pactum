# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260611_110706/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260611_110706/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260611_110706/review/review.json, .heurema/pactum/runs/run_20260611_110706/review/findings.jsonl, .heurema/pactum/runs/run_20260611_110706/review/resolutions.jsonl

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

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or any review loop command.

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

The reviewer will re-check your fixes against the discipline rules above.

## Output shape
Your final output MUST include exactly one fenced `json` block with this shape:

```json
{
  "schema": "pactum.review_fix_outcomes.v1",
  "outcomes": [
    {
      "finding_id": "f_001",
      "outcome": "fixed",
      "note": "What changed and where, or the concrete rebuttal/blocker."
    }
  ]
}
```

Rules:
- Include exactly one outcome entry for every blocking finding listed above with status open.
- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.
- Use outcome fixed when you changed code to address a valid blocking finding.
- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.
- Use outcome blocked when concrete missing information or state prevents a fix.
- Do not include advisory or resolved findings in the outcomes list.
