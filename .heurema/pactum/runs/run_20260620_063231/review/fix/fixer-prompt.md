# Review Fix Prompt

This prompt is prepared for a write-enabled executor agent subprocess.
Pactum captures the fix attempt artifacts and may parse the required structured outcome block.

## Objective
Address the current run's review findings against the approved Pactum contract.

## Inputs
- Fixer context: .heurema/pactum/runs/run_20260620_063231/review/fix/fixer-context.md
- Contract: .heurema/pactum/runs/run_20260620_063231/contract/contract.json
- Review artifacts: .heurema/pactum/runs/run_20260620_063231/review/review.json, .heurema/pactum/runs/run_20260620_063231/review/findings.jsonl, .heurema/pactum/runs/run_20260620_063231/review/resolutions.jsonl

## Approved contract
- Goal: Make the pactum agent skill self-sufficient and one-command installable, so a stranger driving pactum through their coding agent (Claude Code or Codex) reliably follows the safe workflow. Two parts: (A) rewrite the skill content so an agent does not need to read the reference files first, and (B) add a `pactum skill install` command that go:embeds the skill and writes it to the correct per-agent directory.

Background (from a Claude+Codex+Gemini design review): agents reliably SKIP "read references/workflow.md before acting" — they see an inline skeleton and start running commands, or skip context entirely (the same way our executor ignored the repo map). So the skill's entry file must be self-contained for the safe path, with the reference files demoted to optional enrichment. Pactum's real strength is its machine affordances: every --json command carries a `next` array of legal next commands and failures carry error.code/error.fix. The safety boundary is already machine-enforced: `pactum execute run` is never advertised in any `next` array, so an agent that only runs commands from `next` cannot auto-cross into unsandboxed execution.

In scope:

A. Rewrite assets/agent-skills/pactum/SKILL.md into a self-sufficient, next-driven "agent card" for the safe happy path:
- Keep YAML frontmatter `name: pactum` + `description` only (stay on the Agent Skills common subset that BOTH Claude Code and Codex accept; do NOT add Claude-only fields like disable-model-invocation).
- Inline the COMPLETE safe command sequence with `--json` on every command (init/status, map refresh if stale, task new, search + read, clarify, contract show/revise/approve, prompt build, execute plan — the safe stop).
- State the core loop rule explicitly: after each command, read the `next` array and run ONLY commands it lists; if `next` is empty, STOP and report to the user; on failure, use error.fix.
- Keep the dangerous rules inline and repeat them at the boundary: never run `pactum execute run` or `pactum review run` by default; `pactum execute plan` is the stop point; and add: do NOT implement source changes by hand after `execute plan` — pactum stops there unless the user explicitly exits pactum or approves unsandboxed execution (otherwise the agent runs the plan then just edits code itself).
- Do NOT hardcode `--agent codex`; use a placeholder/`<agent>` and tell the agent to use the configured executor (detect codex vs claude or ask), since claude users must not be steered to codex.
- Make map/search non-optional where it is pactum's value: instruct the agent to use `pactum search`/read for required context discovery and not substitute raw rg/cat unless pactum directs otherwise.
- Fix the `.heurema` rule (the current "do not commit .heurema/" conflicts with pactum's own durable run-record story): say do not include `.heurema/pactum` churn in feature commits, never delete or revert it, and report it separately unless the user asks for an audit-record commit.
- Specify the exact final report format: run id, contract status (approved or not), the plan command, files likely touched, and an explicit "stopped at execute plan" statement.
- Update the SKILL.md pointers and the reference files so references/workflow.md, install.md, safety.md are clearly OPTIONAL enrichment/detail, not required reading before acting.

B. Add a `pactum skill install` command:
- go:embed the assets/agent-skills/pactum/ package into the binary (a new embed source) so the installed binary can write the skill out without the repo present. Add a test that the embedded copy is byte-identical to assets/agent-skills/pactum/ (anti-drift), extending the existing internal/docs skill sync test rather than duplicating it.
- `pactum skill install --agent <claude|codex|auto|all> --scope <user|repo>`:
  - claude → user: ~/.claude/skills/pactum/ ; repo: .claude/skills/pactum/
  - codex  → user: ~/.agents/skills/pactum/ ; repo: .agents/skills/pactum/
  - auto detects which agent skill dirs / CLIs are present; all installs to every known target.
  - DEFAULT scope is repo (.<agent>/skills) for alpha, so a global skill does not trigger on every repository the user opens.
  - Idempotent overwrite; print the installed path, the skill version (the pactum binary version), and a note to reload/restart the agent if the skill does not appear.
- Add a discovery check (`pactum skill doctor`, or `skill install --check`): verify the skill files exist at the expected path for the selected agent/scope and that SKILL.md frontmatter parses.
- JSON output (`--json`) consistent with the rest of the CLI (carry `next` and the error envelope where applicable).

C. Update docs/skill-install.md to lead with `pactum skill install` as the one-command path (per-agent, correct paths), keeping manual copy as a documented fallback.

Out of scope: any marketplace/plugin distribution; changing the staged workflow commands themselves; the binary release workflow (separate); Gemini/Antigravity-specific skill paths beyond a noted comment (alpha targets claude + codex).

Tests (helper/temp-dir; do not invoke real agents): embedded skill is byte-identical to the on-disk package; `skill install` writes the package to the correct directory for each --agent and --scope into a temp HOME/repo; idempotent re-install; doctor/check reports present vs absent correctly; SKILL.md frontmatter still parses and the existing skill sync test passes.

Validation: go test ./internal/..., go test ./..., go build ./..., make check.
- In scope:
  - Rewrite `assets/agent-skills/pactum/SKILL.md` as a self-contained safe-path agent card with only `name` and `description` frontmatter, the full safe workflow, `--json` on Pactum workflow commands, `next`/`error.fix` loop rules, Pactum map/search context discovery, executor selection via `<agent>`, and the execute-plan stop/report boundary.
  - Update `assets/agent-skills/pactum/references/workflow.md`, `install.md`, `safety.md`, and related skill docs so references are optional enrichment/detail, not required pre-reading, and so `.heurema/pactum` guidance matches the durable run-record policy.
  - Add a top-level `pactum skill` CLI surface with install and discovery-check functionality, including command grammar, app handling, help text, human output, JSON output, and tests.
  - Add a go:embedded copy of `assets/agent-skills/pactum/` used by `pactum skill install`, plus an anti-drift test proving the embedded package is byte-identical to the on-disk package.
  - Implement `pactum skill install --agent <claude|codex|auto|all> --scope <user|repo>` with default scope `repo`, Codex and Claude target path resolution, auto/all behavior, idempotent overwrite, installed path output, binary version output, and reload/restart guidance.
  - Implement and document one discovery check interface, either `pactum skill doctor` or `pactum skill install --check`, that verifies expected skill files and parses `SKILL.md` frontmatter for the selected agent/scope.
  - Update `docs/skill-install.md` to lead with `pactum skill install` as the one-command path while preserving manual copy instructions as fallback.
- Out of scope:
  - Marketplace, plugin, or external distribution packaging.
  - Binary release workflow or package-manager publishing.
  - Gemini, Antigravity, or other agent-specific install targets beyond a brief alpha-scope note.
  - Changing Pactum lifecycle stage semantics or the execute/review safety boundary except where needed for the new skill commands and their JSON affordances.
  - Invoking real coding agents from implementation or tests.
- Acceptance criteria:
  - `assets/agent-skills/pactum/SKILL.md` frontmatter parses and contains only portable `name` and `description` keys, with no Claude-only or Codex-only fields.
  - `SKILL.md` can be followed without opening reference files: it lists the safe path from CLI check/init/status/map/task/search/read/clarify/contract/prompt through `pactum execute plan`, uses `--json` on Pactum commands where supported or added, and instructs agents to run only commands from `next`, stop on empty `next`, and use `error.fix` on failures.
  - `SKILL.md` and optional references state that `pactum execute run` and `pactum review run` are never default actions, `pactum execute plan` is the default stop point, and source edits must not be made by hand after `execute plan` unless the user exits Pactum or explicitly approves unsandboxed execution.
  - The safe-path skill instructions no longer hardcode `--agent codex`; they require detecting, using, or asking for the configured `<agent>` executor.
  - The final report format is documented as run id, contract status, plan command, likely touched files, and an explicit stopped-at-execute-plan statement.
  - The `.heurema/pactum` guidance says durable run records must not be deleted or reverted, must not be mixed into feature commits, and should be reported separately unless the user asks for an audit-record commit.
  - `pactum skill install` writes the embedded package to `.agents/skills/pactum/`, `$HOME/.agents/skills/pactum/`, `.claude/skills/pactum/`, and `$HOME/.claude/skills/pactum/` for the corresponding agent/scope combinations; omitting `--scope` uses repo scope.
  - `--agent auto` uses detected Codex/Claude targets without launching agents and reports a clear no-target result when nothing is detected; `--agent all` targets every known alpha target.
  - Re-running install over an existing Pactum skill directory succeeds and leaves the installed files matching the embedded package.
  - Human output prints installed or checked path(s), the Pactum binary version, and reload/restart guidance; JSON output includes a top-level `next` field and failures use the existing `pactum.error.v1alpha1` envelope style with stable `error.code` and `error.fix` where applicable.
  - The discovery check reports present, missing, and invalid-frontmatter cases without invoking real agents.
  - Tests cover embedded-package anti-drift, install paths for each agent/scope using temp HOME/repo directories, idempotent reinstall, discovery check present/absent behavior, `SKILL.md` frontmatter parsing, and existing skill sync/doc expectations.
- Validation commands:
  - go test ./internal/...
  - go test ./...
  - go build ./...
  - make check

## Current review findings
- Summary: findings=7 open=7 resolved=0 blocking_open=5
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: The install JSON response always advertises `pactum skill install --check`, which drops the selected scope and agent. For a user-scoped install, following `next` checks the default repo scope instead of the `$HOME` path that was just written, so the machine-guided workflow can report the successful install as missing.
    location: internal/app/skill.go:91
  - f_002 severity=medium category=correctness blocking=true status=open: The discovery check does not actually parse YAML frontmatter. It only checks for delimiters and the substring `name: pactum`, so syntactically invalid frontmatter can be reported as `present` instead of `invalid`.
    location: internal/app/skill.go:308
  - f_003 severity=medium category=correctness blocking=true status=open: `pactum skill install --check` can report `present` for an incomplete skill package or syntactically invalid YAML frontmatter.
    location: internal/app/skill.go:300
  - f_004 severity=medium category=scope blocking=true status=open: Human `--check` output omits the required Pactum version and reload/restart guidance.
    location: internal/app/skill.go:150
  - f_006 severity=medium category=quality blocking=true status=open: The auto no-target test can pass without exercising the no-target behavior.
    location: internal/app/skill_test.go:245
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_005 severity=medium category=scope blocking=false status=open: The lifecycle `next` affordance still hardcodes Codex for the execute-plan step, so a next-driven Claude workflow can still be steered to `--agent codex`.
    location: internal/app/resolve.go:282
  - f_007 severity=medium category=quality blocking=false status=open: The human-facing agent skill overview still hardcodes the safe plan command as `pactum execute plan --agent codex`, which contradicts the updated cross-agent guidance to use the configured `<agent>` executor or ask when uncertain.
    location: docs/agent-skill.md:48

## Fix boundaries
- Trace each finding to the relevant code before acting.
- Fix valid findings in place.
- For false positives, explain a concrete rebuttal instead of changing code.
- Keep changes inside the approved contract and review-finding scope.
- Do not edit `.heurema` artifacts.
- Do not run `pactum review approve`, `pactum review finding resolve`, or `pactum review run`.

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
  "schema": "pactum.review_fix_outcomes.v1alpha1",
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
