<div align="center">

<pre>
██████╗  █████╗  ██████╗████████╗██╗   ██╗███╗   ███╗
██╔══██╗██╔══██╗██╔════╝╚══██╔══╝██║   ██║████╗ ████║
██████╔╝███████║██║        ██║   ██║   ██║██╔████╔██║
██╔═══╝ ██╔══██║██║        ██║   ██║   ██║██║╚██╔╝██║
██║     ██║  ██║╚██████╗   ██║   ╚██████╔╝██║ ╚═╝ ██║
╚═╝     ╚═╝  ╚═╝ ╚═════╝   ╚═╝    ╚═════╝ ╚═╝     ╚═╝
</pre>

<p>
  <a href="https://github.com/heurema/pactum/actions/workflows/ci.yml"><img src="https://github.com/heurema/pactum/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://www.npmjs.com/package/@heurema/pactum"><img src="https://img.shields.io/npm/v/@heurema/pactum" alt="npm"></a>
  <a href="https://goreportcard.com/report/github.com/heurema/pactum"><img src="https://goreportcard.com/badge/github.com/heurema/pactum" alt="Go Report Card"></a>
</p>

<h3>Contract-first orchestration for coding agents</h3>

<img src="assets/demo.gif" alt="pactum demo" width="760">

</div>

**Pactum turns a Claude Code or Codex run into an approved contract, a recorded
prompt boundary, and a review loop with explicit terminal states.** You drive the
agent through deterministic, auditable stages — each writes durable artifacts you
can inspect — so you always know what the agent was asked, what it changed, and how
it was checked.

## The problem

A normal agent run leaves too much implicit. You give Claude Code or Codex a
sentence, it edits your code, and you're left trusting three things you can't
actually see.

- **Did it do what you meant?** The task lived in one prose sentence; the agent's
  real interpretation of scope, edge cases, and "done" lives nowhere. Drift and
  silent scope-creep stay invisible until you read the diff.
- **Was it actually checked?** "I reviewed it, looks good" is not an auditable
  verification — and automated review is often worse: it tends to either
  **rubber-stamp** (a confident green that proves nothing) or **grind forever**
  (re-litigating findings until it burns your budget), and the two are
  indistinguishable until you inspect the artifacts.
- **Will it hold up at scale?** On a large task the agent's context fills, quality
  can degrade, and earlier decisions get forgotten — so the back half of a big
  change quietly contradicts the front half.

That's tolerable for a quick edit. It's hard to trust for anything that needs
review history, repeatability, or handoff.

## How pactum works

Pactum puts a **contract** and a chain of deterministic stages between you and the
agent.

- **Contract first.** Before any code is written, the task becomes an explicit
  contract — goal, in/out of scope, acceptance criteria, validation commands,
  assumptions — that you approve. Approval is pinned to a hash of the contract, so
  the spec the agent is held to cannot silently change underneath it.
- **Deterministic context.** A wiki-first project map and a SQLite full-text index
  (lexical, no embeddings) give reproducible context; the executor prompt is
  assembled from the contract, map, and accepted memory and recorded in a
  manifest — a prompt boundary that is itself part of the audit trail.
- **Auditable stages.** Every stage writes durable artifacts under
  `.heurema/pactum/` — the prompt the agent saw, the plan, the diff, the checks,
  the findings — so a run is inspectable and diffable after the fact.
- **Review with explicit terminal states.** The review loop ends `resolved`, or
  stops loud at `blockers_open` / `fixer_no_progress` — never a silent pass or an
  endless grind. A blocking finding clears only by a real fix or a recorded,
  attributable operator decision.
- **A safe stop.** `execute plan` (a dry run) is the default; real execution is a
  separate, explicit step. Pactum drives Claude Code and Codex over the Agent
  Client Protocol and installs as an Agent Skill, so the agent follows the
  workflow — every `--json` command carries a `next` array of legal moves and an
  `error.fix` remedy on failure.
- **Project memory.** Accepted, deterministic memory from reviewed runs feeds
  future prompts, with lexical search and file-hash freshness.

The point isn't that the agent is perfect — it's that an opaque run becomes
something you can read, review, and reproduce.

## The workflow

```
map → task → clarify → contract → prompt → execute(plan) → gate → review → memory
```

1. **Map** — build a deterministic project map + search index from the repo.
2. **Task** — turn a request into a contract-first run.
3. **Clarify** — surface and answer blocking questions before committing to a spec.
4. **Contract** — draft → revise → approve; approval is hash-pinned.
5. **Prompt** — assemble the deterministic executor prompt (the boundary).
6. **Execute** — `plan` is the safe stop; `run` is real, explicit execution.
7. **Gate** — detect changes and run the contract's validation commands.
8. **Review** — the loud loop (or a manual review); findings are fixed or resolved.
9. **Memory** — capture reusable, deterministic memory from the reviewed run.

Full detail in [docs/flow.md](docs/flow.md).

## Install

```sh
npm i -g @heurema/pactum              # macOS / Linux   (or: npx @heurema/pactum)
pactum skill install --agent claude   # or: --agent codex — drive pactum through your agent
```

Then tell your coding agent: **"use pactum for this task."** The skill package
(`assets/agent-skills/pactum`) is what teaches the agent the safe workflow — see
[docs/skill-install.md](docs/skill-install.md) and [docs/agent-skill.md](docs/agent-skill.md).

Supported: **macOS (arm64/x64)**, **Linux (amd64/arm64, glibc)**, and **Windows
(amd64)**. Windows on ARM and Alpine/musl Linux are not yet supported. Prebuilt
binaries are on
[Releases](https://github.com/heurema/pactum/releases); from source needs Go 1.26+. Details: [docs/install.md](docs/install.md).

## Safety & limits

Real execution is **unsandboxed**: pactum runs the agent through its ACP adapter —
`@heurema/codex-acp` for Codex, `@agentclientprotocol/claude-agent-acp` for
Claude — as a direct subprocess in your repository, with your environment and your
files. There is no container or isolation; pactum's value is the contract and the
boundaries it records, not a security boundary. Run it on a repository and task you
trust, and nothing launches an agent until you explicitly leave the `execute plan`
stop. Search and memory are lexical, not semantic. See [SECURITY.md](SECURITY.md).

---

[Workflow](docs/flow.md) · [Install](docs/install.md) · [Agents](docs/agents.md) · [Skill](docs/agent-skill.md) · [Memory](docs/memory.md) · [Security](SECURITY.md)
