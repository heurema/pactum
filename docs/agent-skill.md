# Pactum agent skill

Pactum ships a repo-local **agent skill** so coding agents (Codex, Claude Code,
and any agent that supports `SKILL.md`) can drive its contract-first workflow as
a reusable capability — not just as commands a human types.

The skill is a single portable package; the agent files are the source of
truth, and this page is the human overview.

## Where it lives

```
assets/agent-skills/pactum/
  SKILL.md                # the portable skill (name + description frontmatter)
  references/
    workflow.md           # full command-by-command workflow
    install.md            # installing the CLI and the skill
    safety.md             # rules for real (unsandboxed) execution
```

It is **cross-agent**: both Codex (`.agents/skills/`) and Claude Code
(`.claude/skills/`) load a skill from its `SKILL.md`. `AGENTS.md` is a separate,
project-instruction layer — not the skill package.

See [docs/skill-install.md](skill-install.md) for how to install the CLI and the
skill today.

## Safe default flow

The skill is procedural and conservative. Its default stop point is
`pactum execute dry-run` — it never runs a real, unsandboxed agent on its own.
The flow (full detail in
[`assets/agent-skills/pactum/references/workflow.md`](../assets/agent-skills/pactum/references/workflow.md)):

1. verify the CLI (`which pactum`)
2. `pactum status`, and `pactum init` only if needed
3. `pactum map refresh` if the map is stale
4. `pactum task new "<task>"` (sets the current run)
5. targeted `pactum search` (identifiers, paths, domain terms; `--kind wiki`,
   `--kind code_item`, `--kind import`) and read the relevant `map/wiki/` pages
   and source files
6. clarify if needed
7. `pactum contract revise` (goal, in/out of scope, acceptance, validation)
8. `pactum contract approve --by manual`
9. `pactum prompt build` / `pactum prompt show`
10. `pactum execute dry-run --agent codex`
11. report current run, relevant files, contract summary, dry-run command, next
    action

## Current-run usage

After `pactum task new` (or `pactum task use`), the run is **current**, so the
staged commands (`clarify`, `contract`, `prompt`, `execute`) need no run id.

## When real execution is allowed

`pactum execute run` and `pactum review run` launch a real agent as a direct,
**unsandboxed** subprocess. The skill does not run them by default and does not
pass `--yes` unless the user explicitly approves direct agent execution. See
[`assets/agent-skills/pactum/references/safety.md`](../assets/agent-skills/pactum/references/safety.md).

## Source of truth

Pactum's map, wiki, code-items, and memory are navigation and audit context —
best-effort and incomplete by design. Source files remain the source of truth;
the skill verifies against them before relying on the map.
