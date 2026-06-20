# Contract Review: Completeness

You are reviewing a software change contract through the **contract-completeness** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Make the pactum agent skill self-sufficient and one-command installable, so a stranger driving pactum through their coding agent (Claude Code or Codex) reliably follows the safe workflow. Two parts: (A) rewrite the skill content so an agent does not need to read the reference files first, and (B) add a `pactum skill install` command that go:embeds the skill and writes it to the correct per-agent directory.

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

**Scope in**:
  - Rewrite `assets/agent-skills/pactum/SKILL.md` as a self-contained safe-path agent card with only `name` and `description` frontmatter, the full safe workflow, `--json` on Pactum workflow commands, `next`/`error.fix` loop rules, Pactum map/search context discovery, executor selection via `<agent>`, and the execute-plan stop/report boundary.
  - Update `assets/agent-skills/pactum/references/workflow.md`, `install.md`, `safety.md`, and related skill docs so references are optional enrichment/detail, not required pre-reading, and so `.heurema/pactum` guidance matches the durable run-record policy.
  - Add a top-level `pactum skill` CLI surface with install and discovery-check functionality, including command grammar, app handling, help text, human output, JSON output, and tests.
  - Add a go:embedded copy of `assets/agent-skills/pactum/` used by `pactum skill install`, plus an anti-drift test proving the embedded package is byte-identical to the on-disk package.
  - Implement `pactum skill install --agent <claude|codex|auto|all> --scope <user|repo>` with default scope `repo`, Codex and Claude target path resolution, auto/all behavior, idempotent overwrite, installed path output, binary version output, and reload/restart guidance.
  - Implement and document one discovery check interface, either `pactum skill doctor` or `pactum skill install --check`, that verifies expected skill files and parses `SKILL.md` frontmatter for the selected agent/scope.
  - Update `docs/skill-install.md` to lead with `pactum skill install` as the one-command path while preserving manual copy instructions as fallback.

**Scope out**:
  - Marketplace, plugin, or external distribution packaging.
  - Binary release workflow or package-manager publishing.
  - Gemini, Antigravity, or other agent-specific install targets beyond a brief alpha-scope note.
  - Changing Pactum lifecycle stage semantics or the execute/review safety boundary except where needed for the new skill commands and their JSON affordances.
  - Invoking real coding agents from implementation or tests.

**Acceptance criteria**:
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

**Validation commands**:
  - go test ./internal/...
  - go test ./...
  - go build ./...
  - make check

**Assumptions**:
  - Repo scope resolves relative to the current repository/work directory and user scope resolves through HOME, with tests overriding both using temporary directories.
  - The skill version printed by install/check is the Pactum binary version from the existing version package.
  - The discovery check spelling may be either `pactum skill doctor` or `pactum skill install --check` if the chosen command is documented, tested, and has JSON output.
  - Auto-detection may use known skill directories and/or agent CLI presence, but must not start Claude, Codex, or any real agent process.

## Lens: Completeness

Checklist:
- Does the contract fully cover its goal? Are there gaps in scope or acceptance_criteria?
- Is every acceptance criterion specific and observable enough to verify?

## Output

State your analysis in prose. If you find issues, also include a structured block:

```json
{
  "schema": "pactum.reviewer_findings.v1alpha1",
  "findings": [
    {
      "message": "Describe the contract issue clearly.",
      "severity": "medium",
      "category": "quality",
      "blocking": true,
      "evidence": "Quote or cite the contract field that shows the issue."
    }
  ]
}
```

Rules:
- Use severity: low, medium, high, critical.
- Use category: correctness, scope, quality, validation, process, other.
- Omit file and line (not applicable for contract review).
- Set blocking=true for defects that should block approval: gaps that make the contract unexecutable or ungatable.
- Set blocking=false for advisory issues.
- If no issues, say so clearly. Do not include an empty findings block.
