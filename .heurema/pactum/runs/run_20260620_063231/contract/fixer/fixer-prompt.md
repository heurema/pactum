# Contract Review Fixer Prompt

You are fixing a software change contract to address blocking review findings.

Current contract version: 042718ed03d646fe4cdaa8a23bad52d76f11504cced9dc44e14b755fcbd76c62

## Current Contract

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

## Blocking Findings to Address

1. [opus/completeness] Part C (update docs/skill-install.md to lead with 'pactum skill install') is a named deliverable in both the goal and Scope in, but no acceptance criterion references docs/skill-install.md. The docs rewrite is unverifiable at the gate and could be silently dropped while all acceptance criteria still pass. Add an acceptance criterion asserting docs/skill-install.md leads with the one-command path and retains manual copy as a fallback.
   Evidence: Goal/Scope in: 'Update docs/skill-install.md to lead with `pactum skill install` as the one-command path... keeping manual copy as a documented fallback.' No acceptance criterion mentions docs/skill-install.md.
2. [opus/testability] The Part A (SKILL.md rewrite) acceptance criteria are prose content-claims ('can be followed without opening reference files', 'state that…', 'is documented as…', 'guidance says…') with no runnable validation. The Tests/Validation sections only cover frontmatter parsing and the embedded-byte-identical skill-sync test, so none of criteria 2-6 are gated by the listed commands: the SKILL.md content rewrite — the contract's headline goal — passes validation as long as bytes match and frontmatter parses. Add a SKILL.md content-lint test asserting presence of the required tokens (full safe command sequence, --json, next/empty-stop/error.fix rules, execute-plan-stop and dangerous-command boundary, report-format fields, .heurema phrases) and absence of '--agent codex', and explicitly mark the irreducibly-prose 'followable without references' property as human-review acceptance.
   Evidence: Acceptance criterion: 'SKILL.md can be followed without opening reference files…'; criteria 3/5/6 use 'state that', 'is documented as', 'guidance says'. Tests section only lists 'SKILL.md frontmatter still parses and the existing skill sync test passes' — no content assertions over the rewritten body.
3. [opus/validation-soundness] Validation commands run an existing test that forbids the feature being added. internal/docs/skill_test.go:TestSkillDocsAvoidStaleAndPrematureClaims lists the literal string "pactum skill install" (and "pactum install") in its forbidden set and scans docs/skill-install.md, SKILL.md, and the references. Part B/C require introducing exactly that command and leading docs/skill-install.md with it. Therefore `go test ./...` and `go test ./internal/...` will FAIL on a correct, complete implementation. The contract only authorizes 'extending the existing skill sync test' for the anti-drift check and elsewhere requires 'existing skill sync/doc expectations' to pass — these are mutually contradictory. The contract must explicitly scope updating/removing the obsolete forbidden entries so the gate is satisfiable.
   Evidence: internal/docs/skill_test.go line 198 forbids "pactum skill install" (scanned files include docs/skill-install.md, SKILL.md at lines 177-185); contract Part C: 'Update docs/skill-install.md to lead with `pactum skill install` as the one-command path'; acceptance: 'Tests cover ... existing skill sync/doc expectations.'
4. [opus/validation-soundness] Existing skill-content tests encode the old 'references are required reading' philosophy that this contract inverts, and will likely fail under the rewrite without being brought into scope. TestSkillInlinesSafetyAndStop requires SKILL.md to contain 'source of truth' and frames references/workflow.md as required reading; TestSkillInstallReference requires references/install.md to contain 'go install ./cmd/pactum', 'make build', npm, and uvx. The contract demotes references to optional enrichment and rewrites install.md, but does not authorize updating these assertions. The gate (go test) is contradictory with the contract unless internal/docs/skill_test.go is explicitly updated.
   Evidence: internal/docs/skill_test.go TestSkillInlinesSafetyAndStop (requires "source of truth", line 72 'explicitly tells the agent to read references') and TestSkillInstallReference (requires "go install ./cmd/pactum", "make build", "npm", "uvx"); contract: 'references ... are clearly OPTIONAL enrichment/detail, not required reading before acting.'
5. [opus/assumptions-surfaced] The SKILL.md next-driven loop implicitly assumes every safe-path command already emits `--json` with a populated `next` array and `error.code`/`error.fix`. This is buried under 'where supported or added' and conflicts with scope-out 'changing the staged workflow commands themselves'. If commands like search/read/map lack `next`, the executor faces a contradiction (add it = change commands, out of scope; or ship a loop that dead-ends), and a reviewer cannot gate missing-`next` as defect-or-acceptable. Make the affordance-exists assumption explicit, or scope in adding it.
   Evidence: Acceptance: 'uses `--json` on Pactum commands where supported or added, and instructs agents to run only commands from `next`, stop on empty `next`'. Scope out: 'Changing Pactum lifecycle stage semantics... except where needed' and 'changing the staged workflow commands themselves'.
6. [codex-xhigh/testability] The central SKILL.md self-sufficiency requirements are accepted mostly by prose review, not by an explicit runnable validation. Add a test or golden/content assertion that verifies the required safe-path commands, --json usage, next/error.fix loop rule, absence of hardcoded --agent codex, execute-plan stop language, .heurema/pactum guidance, and final report format.
   Evidence: Acceptance criteria include extensive SKILL.md content requirements, but Tests only mention: "SKILL.md frontmatter still parses and the existing skill sync test passes."

## Fixer Instructions

- Address each blocking finding by updating the relevant contract field.
- Do NOT change the goal field — it is out of scope for the fixer.
- Only include the contract fields you are changing in the output.
- base_version must exactly match the version shown above.

## Output

Output your reasoning, then a single JSON block with the revise payload:

```json
{
  "schema": "pactum.contract_revise.v1alpha1",
  "base_version": "042718ed03d646fe4cdaa8a23bad52d76f11504cced9dc44e14b755fcbd76c62",
  "contract": {
    "acceptance_criteria": ["...updated criteria..."],
    "validation": {"commands": ["...updated commands..."]}
  }
}
```

Omit any contract field you are not changing. Do not include the goal field.
