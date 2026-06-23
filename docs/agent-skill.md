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
`pactum execute plan` — it never runs a real, unsandboxed agent on its own.
The flow (full detail in
[`assets/agent-skills/pactum/references/workflow.md`](../assets/agent-skills/pactum/references/workflow.md)):

1. verify the CLI (`which pactum`)
2. `pactum status`, and `pactum init` only if needed
3. `pactum task new "<task>"` (sets the current run)
4. read relevant source files to understand the task context
5. clarify if needed (`clarify add`, then a typed `clarify answer` or the
   recommended-answer decision verbs `--recommended` / `--all-recommended`)
6. `pactum contract revise` (goal, in/out of scope, acceptance, validation)
7. `pactum contract approve --by manual`
8. `pactum prompt build` / `pactum prompt show`
9. `pactum execute plan --agent <agent>` (use the configured executor, or omit
   `--agent` to take the default)
10. report current run, relevant files, contract summary, plan command, next
    action

## Machine affordances

With `--json`, the CLI announces the legal moves so an agent reads `next` and
`error.fix` instead of memorizing the pipeline state machine: workflow commands
(and `pactum status` / `pactum task show`) return a top-level `next` array of
directly runnable pactum commands for the run's stage, and recognizable
precondition failures return a `pactum.error.v1alpha1` envelope with a stable
`error.code` and, when a single exact remedial command exists, `error.fix`.
Read-only not-ready responses (`pactum.not_ready.v1alpha1`) carry the remedial
command in `fix` and keep exit code 0. Commands that would launch a real agent
(`pactum execute run`) are never emitted as a `fix`. Decision verbs
(`clarify answer`, `contract accept`/`approve`, `review proposal
accept`/`reject`, `review approve`, `memory accept`) relay explicit human
decisions and record the principal from `--by`.

## Current-run usage

After `pactum task new` (or `pactum task use`), the run is **current**, so the
staged commands (`clarify`, `contract`, `prompt`, `execute`, `gate`, `review`,
`memory`) need no run id.

## When real execution is allowed

`pactum execute run` and `pactum review run` launch real agents as direct,
**unsandboxed** subprocesses — and `review run` includes a write-enabled fixer
unless `--no-fix` is set. The skill does not run them by default: it runs
them only after the user explicitly approves direct agent execution, matching
the threat model in [SECURITY.md](../SECURITY.md). See
[`assets/agent-skills/pactum/references/safety.md`](../assets/agent-skills/pactum/references/safety.md).

## Source of truth

Pactum's memory is navigation and audit context — best-effort and incomplete by
design. Source files remain the source of truth.
