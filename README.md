# Pactum

Pactum is a contract-first CLI for agentic software work.

Instead of handing a task straight to a coding agent, Pactum turns the task into
an explicit, reviewable **contract** and drives the work through deterministic,
auditable stages. Every stage writes durable artifacts under a workspace
directory, so you can see exactly what an agent was asked to do, what it did, and
how it was checked — before any change is trusted.

Pactum invokes agents (`codex`, `claude`) directly as subprocesses — by default
through each agent's Agent Client Protocol (ACP) adapter, which streams output
live and lets Pactum guard file writes in real time; setting
`PACTUM_AGENT_TRANSPORT=cli` is the debug escape hatch that invokes the one-shot
agent CLIs instead. It does **not** sandbox or isolate execution: an agent runs
with your shell environment and your repository, just as if you launched it
yourself. Pactum's value is the contract, the boundaries it records, and the
deterministic checks around the agent — not a security boundary.

## What Pactum does

- Creates a workspace at `.heurema/pactum/` inside your repository.
- Builds a **deterministic, wiki-first project map and search index** (a
  generated map wiki, file inventory, repo map, and a SQLite full-text search
  index). The map wiki is generated from deterministic facts (file inventory and
  manifests); tree-sitter code items are kept only as best-effort symbol hints
  and are incomplete by design — source files remain the source of truth.
  Unsupported languages and framework files may have no code items but still
  appear in the wiki and file inventory. No embeddings or vector search are
  involved — search is lexical and reproducible.
- Creates **contract-first runs**: each task becomes a run with a draft contract
  (goal, in/out of scope, acceptance criteria, validation commands, assumptions).
- Supports the full staged workflow:
  - **Clarification** — record blocking/non-blocking questions and answers.
  - **Contract approval** — revise and approve the contract; approval is pinned
    to a contract hash.
  - **Prompt boundary** — build a deterministic executor prompt from the
    approved contract, project map, and accepted memory, recorded in a manifest.
  - **Execution** — run a built-in agent against the prepared prompt and capture
    its attempt artifacts.
  - **Gate** — deterministically detect repository changes and, with explicit
    opt-in, run the contract's validation commands.
  - **Review** — prepare a manual review, add/resolve findings, and approve.
  - **Reviewer proposals** — optionally run a reviewer agent and parse its
    structured output into *pending* proposals that a human accepts or rejects.
  - **Memory** — propose, inspect, and accept deterministic project memory from
    reviewed runs, with lexical search and file-hash freshness tracking.

## Built-in agents

Pactum ships two built-in agents:

- `codex` — over the default ACP transport, launches the external adapter
  `npx -y @zed-industries/codex-acp@latest`; over the CLI transport, the
  executor runs `codex exec --json --dangerously-bypass-approvals-and-sandbox`.
- `claude` — over the default ACP transport, launches the external adapter
  `npx -y @agentclientprotocol/claude-agent-acp@latest`; over the CLI
  transport, the executor runs
  `claude -p --output-format json --dangerously-skip-permissions`.

Agents are invoked through the required `agents` registry in
`.heurema/pactum/config.yaml`: each entry is `{name, model, effort}` — the
engine is inferred from the required model — and `--agent`/`--reviewer`
accept those registry names. The generated default registers a single
`claude` entry pinned to a current model, and an omitted `--agent` runs the
first registry entry. See [docs/agents.md](docs/agents.md) for the registry
and selection rules.

Both built-ins read their prompt from a prompt file that Pactum prepares. Check
that the agent CLIs are installed and visible on your `PATH` with
`pactum doctor`.

Pactum runs these agents as **direct subprocesses in your repository**. There is
no Pactum-managed isolation, container, or virtual machine around them. Both the
ACP adapters and the agent CLIs are external tooling with direct access to your
repository, your runtime, and your inherited environment variables — real agent
execution is **unsandboxed** and appropriate only for a repository and task
context you trust with that access. See [SECURITY.md](SECURITY.md) for the
threat model and safe usage.

## Not in the MVP

The following are intentionally **not** part of the current MVP:

- Docker / container execution.
- A web UI.
- Embeddings or vector search (search and memory are lexical and deterministic).
- LLM summarization of artifacts or memory.
- Semantic trust of reviewer output (reviewer findings are always proposals a
  human must accept).
- Custom agent adapters (only the built-in `codex` and `claude` agents exist).

## Install

Pactum is a Go module (`github.com/heurema/pactum`, Go 1.26+). There is no
packaged release yet; build, run, or install it from source. See
[docs/install.md](docs/install.md) for the full guide.

**1. Run from source** (no binary):

```sh
go run ./cmd/pactum --help
```

**2. Build a local binary** with the Makefile:

```sh
make build
./bin/pactum --help
```

**3. Install into your Go bin:**

```sh
go install ./cmd/pactum
pactum --help
```

This installs `pactum` into `go env GOBIN` (or `$(go env GOPATH)/bin`); make
sure that directory is on your `PATH`. Once releases are tagged, you will also
be able to `go install github.com/heurema/pactum/cmd/pactum@latest`.

**Smoke test** the safe command surface in a throwaway repo:

```sh
scripts/smoke.sh
```

The examples below use `pactum <command>`; substitute `go run ./cmd/pactum
<command>` if you have not built or installed a binary.

## Quick start

Pactum tracks a **current run**, so after `pactum task new` you can omit the run
id from the staged commands below — they default to the current run (override
with an explicit id or `pactum task use <run_id>`).

```sh
# 1. Initialize the workspace and build the project map.
pactum init
pactum status

# 2. Create a contract-first run for a task (this becomes the current run).
pactum task new "add feature X"

# Inspect runs at any time (the current run is marked with *):
pactum task list

# 3. Clarify open questions before approving the contract.
pactum clarify add "Question?" --blocking
pactum clarify answer q_001 "Answer"

# 4. Shape and approve the contract.
pactum contract revise \
  --goal "..." \
  --add-in-scope "..." \
  --add-acceptance "..." \
  --add-validation "go test ./..."
pactum contract approve

# 5. Build the deterministic executor prompt boundary.
pactum prompt build

# 6. Inspect the planned execution, then run the agent. `execute run` runs the
#    agent directly in your repository. An omitted --agent runs the first
#    entry of the config agents registry; --agent <name> picks another entry.
pactum execute plan
pactum execute run

# 7. Gate the result: it runs the contract's validation commands.
pactum gate run

# 8. Review the run. `review run`, `review finding add`, and `review approve`
#    self-scaffold the review record once the gate report exists;
#    `pactum review run` would instead drive autonomous reviewer/fixer rounds.
pactum review finding add "..." --blocking --severity medium --category quality
pactum review finding resolve f_001 --note "Fixed"
pactum review approve

# 9. Capture reusable project memory from the reviewed run.
pactum memory propose
pactum memory accept

# At any point, see where you are and what to run next:
pactum status

# Print the version:
pactum version
```

> Every staged command still accepts an explicit run id (for example
> `pactum contract approve run_20260603_120000`) and the secondary-id commands
> accept either form, e.g. `pactum review finding resolve f_001` or
> `pactum review finding resolve <run_id> f_001`.

## Documentation

- [docs/install.md](docs/install.md) — prerequisites, building, installing, and
  smoke-testing Pactum from source.
- [docs/flow.md](docs/flow.md) — the workflow stage by stage, with the artifacts
  each stage produces and whether it mutates state.
- [docs/workspace.md](docs/workspace.md) — the `.heurema/pactum/` layout, which
  parts are generated vs durable, and what to commit.
- [docs/agents.md](docs/agents.md) — the built-in agents, `pactum doctor`,
  plan vs run, and the direct-subprocess execution model.
- [SECURITY.md](SECURITY.md) — the threat model, the agent-launching commands,
  safe usage, and private vulnerability reporting.
- [docs/memory.md](docs/memory.md) — deterministic project memory: propose,
  accept, search, refresh/stale, and the prompt boundary.
- [docs/agent-skill.md](docs/agent-skill.md) — the repo-local, cross-agent
  Pactum skill package (`assets/agent-skills/pactum/`) for Codex and Claude
  Code, and its safe default workflow.
- [docs/skill-install.md](docs/skill-install.md) — installing the CLI and the
  skill package today (marketplace packaging is deferred).
- [CHANGELOG.md](CHANGELOG.md) — notable changes (everything is currently
  **Unreleased**; there are no packaged releases yet).
- [docs/dogfood-second-repo.md](docs/dogfood-second-repo.md) — findings from
  running Pactum's safe surface on a second, non-Go repository.

## Continuous integration

Every pull request and push to `main` runs GitHub Actions
([`.github/workflows/ci.yml`](.github/workflows/ci.yml)), which executes the same
local checks shipped in this repo: `make check` (tests, vet, deadcode, and
`git diff --check`), `make heurema-hygiene` (the committed run-record leak
gate), `make build`, and `scripts/smoke.sh`, plus separate `make test-race`
and `make vuln` (govulncheck) jobs. CI does not require `codex`/`claude` to be
installed and never runs a real agent.
