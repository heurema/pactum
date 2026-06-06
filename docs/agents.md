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
<name>` on the review commands. When omitted, both default to `codex` unless
cross-model review is enabled for reviewer selection.

To opt into cross-model review, set `agents.cross_model_review: true` in
`.heurema/pactum/config.yaml`:

```yaml
agents:
  cross_model_review: true
```

The default is `false`, which preserves the existing reviewer selection. When
enabled and `--reviewer` is omitted, Pactum reads the latest execution attempt
and chooses the other built-in reviewer (`codex` execution -> `claude` review,
`claude` execution -> `codex` review). An explicit `--reviewer` always wins. If
the executor cannot be determined or is not one of the two built-ins, Pactum
falls back to the default built-in reviewer (`codex`) — in that case cross-model
review may not be achieved, so check the selected reviewer in the existing
`Resolved` block for `review dry-run` and `review run`.

To pin a per-stage model, set `agents.executor_model` for `pactum execute` or
`agents.reviewer_model` for `pactum review` in `.heurema/pactum/config.yaml` to
`model[:effort]`, for example `gpt-5:high`, `gpt-5`, or `:high`. When a field is
empty or omitted, Pactum does not pass model flags for that stage and the agent
CLI inherits its own configured defaults. For `codex`, Pactum emits
`-c model=...` and `-c model_reasoning_effort=...`; for `claude`, it emits
`--model ...` and `--effort ...`. Reviewer model flags are appended to the
read-only reviewer command (`codex exec --sandbox read-only`, or `claude -p`)
and do not add executor write-bypass flags.

The human output for `execute dry-run`, `execute run`, `review dry-run`, and
`review run` includes a `Resolved` block once per command. It shows the selected
agent plus the stage model and effort values Pactum applied: pinned values are
shown directly, and empty values are shown as `inherit` because Pactum does not
read the agent CLI's own config. A `pinning` summary reads `pinned` (model and
effort both set), `partial` (only one set), or `inherit` (neither).

Pactum does **not** install, bundle, configure, or authenticate these CLIs. You
must install and configure each agent CLI separately and make its command
available on your `PATH` before Pactum can run it — Pactum only invokes the
command it expects (`codex` or `claude`). Use `pactum agents doctor` to confirm
the CLI is present.

## `pactum agents doctor`

`pactum agents doctor` diagnoses the built-in agents **without launching them**.
It reports the default executor and reviewer, and for each agent its command,
input mode, resolved path on your `PATH`, and a status of `on_path` or
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

## Live output

`execute run`, `review run`, and `review fix` (and each per-round reviewer/fixer
sub-run inside `review loop`) stream the agent's stdout and stderr live to
**your terminal's stderr** as the process runs, so a multi-minute run is not a
silent black box. This is in addition to — not instead of — the per-attempt log
files, which are still written in full under the attempt directory.

The live stream goes to stderr on purpose: stdout stays the clean result channel
in every mode. The human summary (or, with `--json`, the machine-readable result
document) remains the only thing on stdout, so `--json` output stays parseable
and `review loop` can still parse its sub-command JSON. Redirect stderr (for
example `2>/dev/null`) to silence the live trace without affecting the result on
stdout.

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
  stdout/stderr logs — under `execute/attempts/`. This is real agent execution
  and runs **unsandboxed**: the agent runs directly in your repository with no
  container, VM, or filesystem confinement, exactly as described in the execution
  model above. It honors `--timeout` (default
  10 minutes). Because execution is **unsandboxed**, `execute run` asks for
  confirmation on an interactive terminal and **requires `--yes`** when stdin is
  not a terminal (CI/automation), so it never launches an agent unattended by
  accident. `execute dry-run` never needs `--yes`.

The two built-in executors use different output channels: codex streams its
reasoning/progress to **stderr** (with the final result on stdout), while claude
writes to **stdout** (stderr is often empty). Both streams are captured under
`execute/attempts/attempt_NNN/{stdout,stderr}.log` and surfaced by
`pactum execute show --logs`, so the meaningful trace may live in either file
depending on the agent.

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
  attempt under `review/reviewer-attempts/`. Like `execute run`, it is
  unsandboxed agent execution, so it asks for confirmation on an interactive
  terminal and **requires `--yes`** for non-interactive/automated use; `review
  dry-run` never needs `--yes`.

A reviewer can emit optional structured finding proposals as a fenced JSON block.
`pactum review propose-findings <run_id>` parses the captured reviewer stdout
into **pending proposals** — it does not create findings. In the manual flow, a
human then decides each one with `pactum review accept-proposal <run_id> p_001`
(which creates a real review finding) or `pactum review reject-proposal <run_id>
p_001 --reason "..."`. Outside the explicit `review loop` command, proposals
are inert until a person accepts them.

## Review fix

`pactum review fix <run_id> --agent codex --yes` launches a fresh
executor-role fixer against the run's current `review/findings.jsonl`. The
fixer prompt includes the approved contract goal/scope/acceptance criteria and
the current review findings, and instructs the agent to trace each finding to
code, fix valid findings in place, and explain a rebuttal for false positives.

This is write-enabled agent execution, not reviewer execution: `codex` uses
`codex exec --dangerously-bypass-approvals-and-sandbox`, and `claude` uses the
executor command with `--dangerously-skip-permissions`. The command honors the
executor model pin (`agents.executor_model`), prints the same `Resolved` block
as execution, captures request/result/stdout/stderr artifacts under
`review/fix/attempts/`, and writes `review/fix/fixer-prompt.md`,
`review/fix/fixer-context.md`, `review/fix/fixer-dry-run.json`, and
`review/fix/last-result.json`.

Like `execute run` and `review run`, `review fix` is unsandboxed and requires
`--yes` for non-interactive/automated use. It does not approve reviews, resolve
findings, or re-run the gate.

## Review loop

`pactum review loop <run_id> --reviewer codex --agent codex --yes` runs the
reviewer, parses structured finding proposals, accepts the proposals into review
findings, and runs the fixer when the current round creates open findings. After
each fixer attempt, Pactum re-runs the gate with the approved validation
commands, then starts another reviewer round.

The loop stops after the configured number of consecutive clean reviewer rounds,
after repeated no-change fixer rounds, or when `--max-rounds` is reached. If
the flags are omitted, Pactum reads `limits.review.max_rounds`,
`limits.review.clean_rounds`, and `limits.review.patience` from the workspace
config. The default clean-round requirement is 1, preserving the original "first
clean round converges" behavior. The default no-change patience is 2: when a
fixer runs but the source fingerprint is unchanged for two consecutive fixer
rounds, the loop terminates as `stalemate`. `--timeout` applies to each reviewer
or fixer subprocess, and `--json` prints the loop summary as JSON.

The loop writes `review/loop-summary.json` with the terminal reason and
per-round open-finding counts, clean streak, and unchanged-fingerprint streak.
It does not run `pactum review approve`; the human approval gate remains
explicit.
