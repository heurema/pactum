# Memory Candidate

## Run
- Run id: run_20260620_063231
- Source: deterministic

## Contract
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

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - assets/agent-skills/pactum/SKILL.md
  - assets/agent-skills/pactum/references/install.md
  - assets/agent-skills/pactum/references/safety.md
  - assets/agent-skills/pactum/references/workflow.md
  - docs/skill-install.md
  - internal/app/cli.go
  - internal/app/commands.go
  - internal/docs/skill_test.go
- New files:
  - assets/embed.go
  - internal/app/skill.go
  - internal/app/skill_test.go
- Missing files: none

## Clarifications
- None

## Review Decisions
- f_001 [medium] resolved internal/app/skill.go:91: The install JSON response always advertises `pactum skill install --check`, which drops the selected scope and agent. For a user-scoped install, following `next` checks the default repo scope instead of the `$HOME` path that was just written, so the machine-guided workflow can report the successful install as missing.
  Resolution: skill.go:91 — Next now uses fmt.Sprintf to embed the actual agent and scope flags: `pactum skill install --check --agent %s --scope %s`. TestSkillInstallJSONOutput updated to assert the exact next value for claude/repo.
- f_002 [medium] resolved internal/app/skill.go:308: The discovery check does not actually parse YAML frontmatter. It only checks for delimiters and the substring `name: pactum`, so syntactically invalid frontmatter can be reported as `present` instead of `invalid`.
  Resolution: checkSkillDir (skill.go ~320-340) now calls yaml.Unmarshal on the extracted frontmatter block and asserts parsedFM.Name == "pactum" instead of using strings.Contains. Syntactically invalid YAML now returns status="invalid" with the parse error detail.
- f_003 [medium] resolved internal/app/skill.go:300: `pactum skill install --check` can report `present` for an incomplete skill package or syntactically invalid YAML frontmatter.
  Resolution: checkSkillDir now calls embeddedSkillFiles() (new helper that walks assets.SkillFS) and stat-checks every expected relative path under destDir after the frontmatter check. Any missing files produce status="invalid" with a list of what is absent. TestSkillCheckIncomplete added to exercise this path.
- f_004 [medium] resolved internal/app/skill.go:150: Human `--check` output omits the required Pactum version and reload/restart guidance.
  Resolution: SkillCheck human output (skill.go ~150-165) now prints a `pactum skill check (<version>)` header using version.Current().Version, and appends `Reload or restart your coding agent if the skill does not appear.` after the per-target lines.
- f_005 [medium] open internal/app/resolve.go:282: The lifecycle `next` affordance still hardcodes Codex for the execute-plan step, so a next-driven Claude workflow can still be steered to `--agent codex`.
- f_006 [medium] resolved internal/app/skill_test.go:245: The auto no-target test can pass without exercising the no-target behavior.
  Resolution: TestSkillAutoDetectNone now sets PATH to an empty temp dir via t.Setenv so no agent CLI can be found. The `if err == nil { return }` bail-out is removed; the test unconditionally asserts a preconditionError with code skill_no_targets.
- f_007 [medium] open docs/agent-skill.md:48: The human-facing agent skill overview still hardcodes the safe plan command as `pactum execute plan --agent codex`, which contradicts the updated cross-agent guidance to use the configured `<agent>` executor or ask when uncertain.
- Proposal summary: pending=0 accepted=7 rejected=0

## Reusable Project Knowledge
- scope: in scope: Rewrite `assets/agent-skills/pactum/SKILL.md` as a self-contained safe-path agent card with only `name` and `description` frontmatter, the full safe workflow, `--json` on Pactum workflow commands, `next`/`error.fix` loop rules, Pactum map/search context discovery, executor selection via `<agent>`, and the execute-plan stop/report boundary.
- scope: in scope: Update `assets/agent-skills/pactum/references/workflow.md`, `install.md`, `safety.md`, and related skill docs so references are optional enrichment/detail, not required pre-reading, and so `.heurema/pactum` guidance matches the durable run-record policy.
- scope: in scope: Add a top-level `pactum skill` CLI surface with install and discovery-check functionality, including command grammar, app handling, help text, human output, JSON output, and tests.
- scope: in scope: Add a go:embedded copy of `assets/agent-skills/pactum/` used by `pactum skill install`, plus an anti-drift test proving the embedded package is byte-identical to the on-disk package.
- scope: in scope: Implement `pactum skill install --agent <claude|codex|auto|all> --scope <user|repo>` with default scope `repo`, Codex and Claude target path resolution, auto/all behavior, idempotent overwrite, installed path output, binary version output, and reload/restart guidance.
- scope: in scope: Implement and document one discovery check interface, either `pactum skill doctor` or `pactum skill install --check`, that verifies expected skill files and parses `SKILL.md` frontmatter for the selected agent/scope.
- scope: in scope: Update `docs/skill-install.md` to lead with `pactum skill install` as the one-command path while preserving manual copy instructions as fallback.
- scope: out of scope: Marketplace, plugin, or external distribution packaging.
- scope: out of scope: Binary release workflow or package-manager publishing.
- scope: out of scope: Gemini, Antigravity, or other agent-specific install targets beyond a brief alpha-scope note.
- scope: out of scope: Changing Pactum lifecycle stage semantics or the execute/review safety boundary except where needed for the new skill commands and their JSON affordances.
- scope: out of scope: Invoking real coding agents from implementation or tests.
- review_resolution: f_001 resolved: The install JSON response always advertises `pactum skill install --check`, which drops the selected scope and agent. For a user-scoped install, following `next` checks the default repo scope instead of the `$HOME` path that was just written, so the machine-guided workflow can report the successful install as missing.; resolution: skill.go:91 — Next now uses fmt.Sprintf to embed the actual agent and scope flags: `pactum skill install --check --agent %s --scope %s`. TestSkillInstallJSONOutput updated to assert the exact next value for claude/repo.
- review_resolution: f_002 resolved: The discovery check does not actually parse YAML frontmatter. It only checks for delimiters and the substring `name: pactum`, so syntactically invalid frontmatter can be reported as `present` instead of `invalid`.; resolution: checkSkillDir (skill.go ~320-340) now calls yaml.Unmarshal on the extracted frontmatter block and asserts parsedFM.Name == "pactum" instead of using strings.Contains. Syntactically invalid YAML now returns status="invalid" with the parse error detail.
- review_resolution: f_003 resolved: `pactum skill install --check` can report `present` for an incomplete skill package or syntactically invalid YAML frontmatter.; resolution: checkSkillDir now calls embeddedSkillFiles() (new helper that walks assets.SkillFS) and stat-checks every expected relative path under destDir after the frontmatter check. Any missing files produce status="invalid" with a list of what is absent. TestSkillCheckIncomplete added to exercise this path.
- review_resolution: f_004 resolved: Human `--check` output omits the required Pactum version and reload/restart guidance.; resolution: SkillCheck human output (skill.go ~150-165) now prints a `pactum skill check (<version>)` header using version.Current().Version, and appends `Reload or restart your coding agent if the skill does not appear.` after the per-target lines.
- review_resolution: f_006 resolved: The auto no-target test can pass without exercising the no-target behavior.; resolution: TestSkillAutoDetectNone now sets PATH to an empty temp dir via t.Setenv so no agent CLI can be found. The `if err == nil { return }` bail-out is removed; the test unconditionally asserts a preconditionError with code skill_no_targets.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- review_resolution: proposal p_007 accepted as f_007
- validation: go test ./internal/... passed
- validation: go test ./... passed
- validation: go build ./... passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
