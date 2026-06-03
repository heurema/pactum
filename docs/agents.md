# Agents

Pactum delegates the actual coding and (optional) reviewing to agent CLIs. It
prepares a deterministic prompt, runs the agent, and captures what happened — but
it does not wrap the agent in any isolation.

## Built-in agents

Two agents are built in. There are **no custom agents in the MVP**: you select
one of these two by name.

| Name | Command run | Role |
| --- | --- | --- |
| `codex` | `codex exec --dangerously-bypass-approvals-and-sandbox` | default executor and default reviewer |
| `claude` | `claude -p` | executor / reviewer |

Both agents receive their prompt from a prompt file that Pactum prepares (the
built executor prompt for execution, or the reviewer prompt for review); Pactum
feeds that file to the agent process on standard input. Pick the executor with
`--agent <name>` on the execute commands and the reviewer with `--reviewer
<name>` on the review commands. When omitted, both default to `codex`.

Pactum does **not** install, bundle, configure, or authenticate these CLIs. You
must install and configure each agent CLI separately and make its command
available on your `PATH` before Pactum can run it — Pactum only invokes the
command it expects (`codex` or `claude`). Use `pactum agents doctor` to confirm
the CLI is present.

## `pactum agents doctor`

`pactum agents doctor` diagnoses the built-in agents **without launching them**.
It reports the default executor and reviewer, and for each agent its command,
input mode, resolved path on your `PATH`, and a status of `ready` or
`missing_command` (with the issue listed when the command is not found). Pass
`--agent <name>` to inspect a single agent.

Run it before executing to confirm the agent CLIs are installed and visible:

```sh
pactum agents doctor
pactum agents doctor --agent claude
```

## Execution model: direct subprocess, no isolation

When you run an agent, Pactum launches it as a **direct subprocess in your
repository**:

- The working directory is your repository root.
- The agent inherits your environment.
- The prepared prompt is piped to the agent's standard input.
- The agent's stdout and stderr are captured to attempt artifacts.

There is **no Pactum-managed isolation**: no container, no virtual machine, no
sandbox, and no filesystem confinement. The agent can read and write your
repository exactly as if you had launched it yourself — the default `codex`
invocation even bypasses Codex's own approval/sandbox prompts. Pactum's
guarantees are about the *contract* and the *deterministic checks* around the
agent (approved contract hash, fresh project map, recorded memory boundary, gate
report), not about constraining what the agent process can do.

There is also **no Docker support yet**.

## Execute: dry-run vs run

- `pactum execute dry-run <run_id> --agent codex` prepares and records the exact
  command Pactum would launch (`execute/dry-run.json`, including the resolved
  command and arguments) but does **not** start any process. Use it to confirm
  the boundaries pass and to see what would run.
- `pactum execute run <run_id> --agent codex` launches the agent subprocess and
  captures the attempt — request, result (exit code, timing, timeout flag), and
  stdout/stderr logs — under `execute/attempts/`. It honors `--timeout` (default
  10 minutes). Because execution is **unsandboxed**, `execute run` asks for
  confirmation on an interactive terminal and **requires `--yes`** when stdin is
  not a terminal (CI/automation), so it never launches an agent unattended by
  accident. `execute dry-run` never needs `--yes`.

Both first re-verify the prompt boundary: the contract is still approved and its
hash matches, the project map is fresh and matches the prompt manifest, and the
accepted-memory boundary recorded at prompt build is unchanged. If any check
fails, neither dry-run nor run proceeds.

After execution, gate the result with `pactum gate run <run_id>` (validation
commands run only with `--allow-commands`).

## Review: dry-run vs run

Reviewer agents are optional, and Pactum never trusts their output
automatically.

- `pactum review dry-run <run_id> --reviewer codex` prepares the reviewer prompt
  and context (`review/reviewer-prompt.md`, `review/reviewer-context.md`,
  `review/reviewer-dry-run.json`) without launching a reviewer.
- `pactum review run <run_id> --reviewer codex` launches the reviewer subprocess
  (same direct-subprocess model as execution, with `--timeout`) and captures its
  attempt under `review/reviewer-attempts/`.

A reviewer can emit optional structured finding proposals as a fenced JSON block.
`pactum review propose-findings <run_id>` parses the captured reviewer stdout
into **pending proposals** — it does not create findings. A human then decides
each one with `pactum review accept-proposal <run_id> p_001` (which creates a
real review finding) or `pactum review reject-proposal <run_id> p_001 --reason
"..."`. This is why there is no semantic trust of reviewer output: proposals are
inert until a person accepts them.
