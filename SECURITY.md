# Security Policy

## Threat model: Pactum is not a sandbox

Pactum prepares contracts and prompts, launches external agent tooling, and
records what happened. It does **not** sandbox, contain, or isolate anything it
launches: there is no container, virtual machine, or filesystem confinement.
The real security boundary is the **repository and the runtime environment you
run Pactum in** — an agent can do whatever your shell user can do there.

The agent-launching commands are:

- `pactum clarify run` (also run by `pactum task new --clarify`)
- `pactum contract draft`
- `pactum execute run`
- `pactum review run` (reviewer rounds, plus the write-enabled fixer unless
  `--no-fix` is set)
- `pactum review fix run`

Every one of these launches external agent tooling, either through the default
ACP transport — external npm adapter packages downloaded and run via `npx`,
such as `npx -y @zed-industries/codex-acp@latest` and
`npx -y @agentclientprotocol/claude-agent-acp@latest` — or through the CLI
transport (`PACTUM_AGENT_TRANSPORT=cli`), where the executor runs
`codex exec --json --dangerously-bypass-approvals-and-sandbox` or
`claude -p --output-format json --dangerously-skip-permissions`. Both paths run
in your repository, inherit your environment, and execute with your user's
permissions.

The read-only stages (`clarify run`, `contract draft`, and the reviewer rounds
of `review run`) constrain *file writes*, not the process: reviewer, drafter,
and clarifier agents still run as external tooling that inherits your
environment variables. Do not treat any agent-launching stage as safe for
secrets.

## Safe usage

- **Trusted repositories only.** Run agent-launching commands only in a
  repository and task context you are willing to expose to arbitrary external
  agent tooling — repository files, shell commands, and inherited environment
  variables. This is a human judgment about the repository; an agent CLI's own
  "trusted project" approval state (such as Codex's) is an agent-specific
  setting that can *increase* risk, not what makes a repository safe.
- **Plan before running.** Prefer `pactum execute plan` before
  `pactum execute run` to inspect the prompt, the boundary checks, and the
  resolved agent before anything launches. The plan's recorded command is the
  CLI-transport invocation; under the default ACP transport, `execute run`
  launches the external `npx` ACP adapter described above instead.
- **Review the contract path scope before execution.** Check the approved
  contract's in-scope and out-of-scope paths; the ACP write guard and the gate
  enforce that scope at the file-write boundary and after the fact, but shell
  commands the agent runs are not gated.
- **Minimize the environment.** Run Pactum with the smallest practical
  environment and no long-lived credentials (`GITHUB_TOKEN`, cloud keys, agent
  auth tokens): every agent-launching command may expose inherited environment
  variables to external tooling, including the read-only reviewer, drafter,
  and clarifier stages.
- **Control the external tooling.** In restricted or high-sensitivity
  environments, review, pin, or otherwise control the agent CLIs and ACP
  adapter packages outside Pactum before running agent-launching commands —
  the default ACP transport downloads and executes `@latest` npm adapters via
  `npx`.

See [docs/agents.md](docs/agents.md) for the detailed execution model,
transports, and per-stage write enforcement.

## Reporting a vulnerability

Report vulnerabilities privately through GitHub private vulnerability
reporting: <https://github.com/heurema/pactum/security/advisories/new>.

If GitHub private reporting is unavailable for this repository, contact the
maintainer privately and avoid filing a public issue until a private channel
is established.

## Supported versions

Only `main` is supported. There are no tagged releases yet; security fixes
land on `main`.
