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
built executor prompt for execution, the clarifier prompt for `clarify suggest`,
the contract drafter prompt for `contract draft`, or the reviewer prompt for
review); Pactum feeds that file to the agent process on standard input. Pick the
executor with `--agent <name>` on the execute commands and the
reviewer/clarifier/drafter with `--reviewer <name>` on the clarify, contract
draft, and review commands.

Reviewer selection is cross-model by default: when `--reviewer` is omitted,
Pactum reads the latest execution attempt and chooses the other built-in agent
(`codex` execution -> `claude` review, `claude` execution -> `codex` review).
The same rule is used by `clarify suggest` and `contract draft`; before any
execution attempt exists they fall back to the default built-in reviewer. An
explicit `--reviewer` always wins. If the executor cannot be determined or is
not one of the two built-ins, Pactum falls back to the default built-in reviewer
(`codex`) — in that case cross-model review may not be achieved, so check the
selected reviewer in the `Resolved` block for `clarify suggest`,
`contract draft`, `review dry-run`, and `review run`.

Agents and their models are configured per stage in
`.heurema/pactum/config.yaml`. Both `execute.models` and `review.panel` are
lists of the same entry shape — `agent` (required, a built-in name), `model`
and `effort` (optional pins):

```yaml
execute:
  models:               # pins for whichever agent --agent invokes
    - agent: claude
      model: claude-opus-4-8
      effort: high

review:
  panel:                # review roster + reviewer-role model registry
    - agent: claude
      model: claude-fable-5
    - agent: codex
```

`execute.models` is a pure lookup: when `pactum execute run --agent <name>`
invokes an agent that has an entry, its `model`/`effort` are applied; an agent
without an entry inherits the agent CLI's own defaults. A model pin can never
reach a different agent. The fixer (`review fix`) writes code like the executor
and uses the same `execute.models` lookup.

`review.panel` is both the review-loop roster and the reviewer-role model
registry. When `pactum review loop` runs without `--reviewer`, each review round
runs all panel entries concurrently against the same reviewer prompt, then
parses their finding proposals in the configured order; each member runs with
its own entry's `model`/`effort`. Duplicate proposals still collapse through the
normal finding fingerprint, and duplicate findings keep the maximum severity. An
explicit `--reviewer <name>` disables the roster for that invocation and runs
only that reviewer, taking pins from its panel entry when one exists. When the
panel is empty or absent, review falls back to the cross-model single-reviewer
selection above. `clarify suggest` and `contract draft` resolve their reviewer
the same way and take pins from that agent's panel entry when present.

Entry validation is strict: an unknown agent name, a duplicate agent within one
list, or a `model` containing `:` (use the separate `effort` key) are
configuration errors. For `codex`, pins emit `-c model=...` and
`-c model_reasoning_effort=...`; for `claude`, `--model ...` and `--effort ...`.
Reviewer pins are appended to the read-only reviewer command and do not add
executor write-bypass flags.

The human output for `clarify suggest`, `contract draft`, `execute dry-run`,
`execute run`, `review dry-run`, and `review run` includes a `Resolved` block
once per command. It shows the selected agent plus the stage model and effort
values Pactum applied: pinned values are shown directly, and empty values are
shown as `inherit` because Pactum does not read the agent CLI's own config. A
`pinning` summary reads `pinned` (model and effort both set), `partial` (only
one set), or `inherit` (neither).

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

### Transport: CLI (default) or ACP

How the agent is *reached* is a swappable transport behind one seam, so the loop,
gate, and attempt lifecycle are unaware of it:

- **`cli`** (default) — the one-shot agent CLI described above (`codex exec`,
  `claude -p`), with the prompt piped to stdin.
- **`acp`** — the agent is driven over the [Agent Client
  Protocol](https://agentclientprotocol.com) via its server adapter
  (`claude-agent-acp` / `codex-acp`, launched with `npx`) using a JSON-RPC client.
  The agent edits the working tree through client-serviced file writes, its text
  streams to the attempt log as it works, and the turn's token usage comes from
  the protocol. The protocol's `Usage` is normalized to the same OTel-inclusive
  convention the CLI parsers use (`InputTokens` includes cache read+write,
  `OutputTokens` includes reasoning; see
  [`cost-budget-design.md`](cost-budget-design.md)), so ACP and CLI usage records
  are directly comparable. The same `RunResult` and attempt artifacts are
  produced either way.

Select it with the `PACTUM_AGENT_TRANSPORT` env var (`acp` or `cli`; the default
is `cli`). The transport is intentionally not a config key — it is an execution
mechanism, not workspace state, and ACP is planned to become the hardwired
default once its remaining gaps (model pins over ACP, a write guard for
read-only stages) are closed, leaving the env var as a debug escape hatch.

The ACP adapters are external npm packages and inherit the agent's auth from the
environment. `cli` remains the default.

#### Real-time write scope guard (ACP only)

Because the ACP transport services the agent's file writes itself, it can enforce
the contract path-scope *in real time*, at the file-write boundary. On the write
stages (`execute run` and `review fix`), each `WriteTextFile` is checked against
the approved contract's `paths_in_scope` / `paths_out_of_scope`: a write whose
repo-relative path is out of scope (or escapes the repo) is denied — the agent
receives a write failure and nothing touches disk. This is the architectural
payoff of ACP: a live guard, instead of relying only on the post-hoc gate to
catch out-of-scope changes after the fact.

The guard has two deliberate limits:

- It gates **only the file-write boundary** (`WriteTextFile`). An agent that
  writes through a *shell command* it runs bypasses the guard; such changes are
  still caught only by the post-hoc gate.
- It applies **only to the ACP transport**. The CLI transport is unchanged and
  continues to rely entirely on the gate. Read-only stages (reviewer, clarify
  suggest, contract draft) are not scope-restricted, and when a contract
  declares no path-scope every in-repo write is allowed.

## Live output

`clarify suggest`, `contract draft`, `execute run`, `review run`, and
`review fix` (and each per-round reviewer/fixer sub-run inside `review loop`)
stream the agent's stdout and stderr live to **your terminal's stderr** as the
process runs, so a
multi-minute run is not a silent black box. This is in addition to — not instead
of — the per-attempt log files, which are still written in full under the
attempt directory.

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
  model above. It honors `--timeout` as an idle safety timeout: the process is
  cancelled only after the configured duration passes with no stdout or stderr
  output (default 10 minutes). Because execution is **unsandboxed**, `execute
  run` asks for confirmation on an interactive terminal and **requires `--yes`**
  when stdin is not a terminal (CI/automation), so it never launches an agent
  unattended by accident. `execute dry-run` never needs `--yes`.

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

## Clarify suggest

`pactum clarify suggest <run_id> --reviewer codex --yes` launches a read-only
reviewer-role agent to propose clarification questions from the contract goal,
repository context, first-pass search results, and existing clarifications. It
writes `clarify/clarifier-prompt.md`, `clarify/clarifier-context.md`, captures
the attempt under `clarify/clarifier-attempts/`, and stores the latest result at
`clarify/clarifier-last-result.json`.

The clarifier must emit a fenced JSON block with
`schema: "pactum.clarification_suggestions.v1"` and a `questions` array. Each
question object carries:

- `text` — the question for the human (required).
- `blocking` — whether execution should not continue without the answer (required).
- `rationale` — why the answer changes scope or implementation, and what the repo
  already settled (required).
- `recommended_answer` — the best-guess resolution, phrased so the human can apply
  it directly (required).
- `confidence` — one of `high`, `medium`, `low` (required).
- `kind` — one of `terminology`, `scope`, `acceptance`, `edge_case`, `assumption`,
  `other`; an omitted or empty `kind` defaults to `other` (v1-compatible), while a
  non-empty value outside the set is rejected.
- `depends_on` — optional array of 1-based positions of earlier questions **in the
  same block** whose answers this one hinges on; positions are numbered per block,
  so a reference never resolves across into another block's questions.

Pactum parses those suggestions directly into **open clarification questions** in
`clarify/questions.jsonl` for a human to answer later with `pactum clarify answer`;
the agent never answers questions, revises the contract, or edits code. Like other
agent-running commands, it asks for confirmation on an interactive terminal and
**requires `--yes`** for non-interactive/automated use.

## Contract draft

`pactum contract draft <run_id> --reviewer codex --yes` launches a read-only
reviewer-role agent to propose missing contract fields from the contract goal,
answered clarifications, repository context, and first-pass search results. It
writes `contract/drafter-prompt.md`, `contract/drafter-context.md`, captures
the attempt under `contract/drafter-attempts/`, and records the latest pending
proposal at `contract/draft-proposal.json` with a Markdown preview at
`contract/draft-proposal.md`.

The drafter must emit a fenced JSON block with
`schema: "pactum.contract_draft_proposal.v1"` and the proposal fields
`in_scope`, `out_of_scope`, `acceptance`, `validation`, and `assumptions`.
Pactum does **not** apply this output automatically: `pactum contract
show-draft <run_id>` shows the pending proposal, and a human must run `pactum
contract accept-draft <run_id>` to append the proposed fields through the normal
contract revision path. Accepting the draft resets contract approval like any
other revision; the human still approves separately with `pactum contract
approve`.

The drafter never answers clarification questions, changes the contract goal, or
edits code. Like other agent-running commands, `contract draft` streams live
agent output to stderr, honors `--timeout` as an idle no-output timeout, supports
`--json`, and **requires `--yes`** for non-interactive/automated use.

## Review: dry-run vs run

Reviewer agents are optional, and Pactum never trusts their output
automatically.

- `pactum review dry-run <run_id> --reviewer codex` prepares the reviewer prompt
  and context (`review/reviewer-prompt.md`, `review/reviewer-context.md`,
  `review/reviewer-dry-run.json`) without launching a reviewer.
- `pactum review run <run_id> --reviewer codex` launches the reviewer subprocess
  (same direct-subprocess model as execution, with the idle `--timeout`) and
  captures its attempt under `review/reviewer-attempts/`. Like `execute run`, it is
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
executor model pin (the fixer's agent entry in `execute.models`), prints the same `Resolved` block
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
the flags are omitted, Pactum reads `review.max_rounds`,
`review.clean_rounds`, and `review.patience` from the workspace
config. The default clean-round requirement is 1, preserving the original "first
clean round converges" behavior. The default no-change patience is 2: when a
fixer runs but the source fingerprint is unchanged for two consecutive fixer
rounds, the loop terminates as `stalemate`. The idle `--timeout` applies to each
reviewer or fixer subprocess, and `--json` prints the loop summary as JSON.

The loop writes `review/loop-summary.json` with the terminal reason and
per-round open-finding counts, clean streak, and unchanged-fingerprint streak.
It does not run `pactum review approve`; the human approval gate remains
explicit.
