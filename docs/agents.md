# Agents

Pactum delegates the actual coding and (optional) reviewing to agent CLIs. It
prepares a deterministic prompt, runs the agent, and captures what happened — but
it does not wrap the agent in any isolation.

## Built-in agents

Two agents are built in. There are **no custom agents in the MVP**.

| Name | Command run | Role |
| --- | --- | --- |
| `codex` | `codex exec --dangerously-bypass-approvals-and-sandbox` | executor / reviewer |
| `claude` | `claude -p` | executor / reviewer |

Both agents receive their prompt from a prompt file that Pactum prepares (the
built executor prompt for execution, the clarifier prompt for `clarify suggest`,
the contract drafter prompt for `contract draft`, or the reviewer prompt for
review); Pactum feeds that file to the agent process on standard input.

## The agents registry

The top-level `agents` list in `.heurema/pactum/config.yaml` is the config's
source of truth for agents: every reference — `--agent`, `--reviewer`, and
`review.panel` — resolves a registry **name**, and a name carries its
model+effort wherever it is used. Each entry has:

- `name` (required) — how the entry is referenced everywhere; a free,
  path-safe label.
- `model` (required) — the model the entry runs; the underlying engine is
  inferred solely from it (see below).
- `effort` (optional) — a reasoning-effort pin applied wherever the name is
  invoked.

```yaml
agents:
  - name: fable
    model: claude-fable-5   # infers the claude engine
  - name: codex
    model: gpt-5.5          # infers the codex engine
    effort: high

review:
  panel: [fable, codex]   # registry names; empty = cross-model single reviewer
```

There is no `agent` key: the engine is inferred from the model alone, by
prefix-distinct vendor families:

- a model starting with `claude`, or equal to one of the claude aliases
  (`opus`, `sonnet`, `haiku`, `fable`), runs on **claude**;
- a model starting with `gpt` or `codex`, or starting with `o` followed by a
  digit (`o3`, `o4-mini`, ...), runs on **codex**;
- anything else fails loudly at config read with an error naming the entry and
  the recognized forms.

Matching is case-insensitive on the trimmed model; the aliases are exact
matches. Pactum does not validate that the model exists — a recognizable but
wrong model id still fails at the provider, as before.

The registry is required: an empty or missing `agents` list is a loud error
("at least one agent must be registered"), and `pactum init` generates a
single claude entry pinned to a current model (`agents: [{name: claude,
model: claude-opus-4-8}]`). Because the engine is inferred from the model, an
entry that inherits the agent CLI's own default model cannot exist — every
entry pins its model. Bare engine names are not implicitly available —
referencing an unregistered name (including an engine that is not registered)
is an error. Validation is strict: blank or duplicate names, a missing model,
a model the engine cannot be inferred from, a `model` containing `:` (use the
separate `effort` key), a leftover `agent:` key from the previous config
shape, or a panel name that is not registered are configuration errors.

A name is decoupled from the engine it runs on, so two entries can run the
same engine with different models — for example an `opus` writer and a
`fable` reviewer, or a panel holding two claude-backed entries that run as
separate panel members.

## Name resolution

Pick the executor with `--agent <name>` on the execute commands (the fixer in
`review fix` and `review loop` resolves the same way) and the
reviewer/clarifier/drafter with `--reviewer <name>` on the clarify, contract
draft, and review commands. Both flags accept registry names only.

When `--agent` is omitted, the **first registry entry** is the executor. When
`--reviewer` is omitted, selection is cross-model against **inferred
engines**: Pactum reads the latest execution attempt's engine and picks the
first registry entry whose inferred engine differs, falling back to the first
registry entry when every entry runs on the executor's engine. Before any
execution attempt exists (`clarify suggest`, `contract draft`), the would-be
executor is the first registry entry, so the same rule applies against it. An
explicit `--reviewer` always wins. Check the selected entry in the `Resolved`
block for `clarify suggest`, `contract draft`, `review dry-run`, and
`review run`.

`review.panel` is the review-loop roster: a list of registry names. When
`pactum review loop` runs without `--reviewer`, each review round runs all
panel members concurrently — every member expanding into the five built-in
lens attempts described under [Review: dry-run vs run](#review-dry-run-vs-run),
so a round spawns members × lenses attempts — then parses their finding
proposals in the configured order; each member runs with its own
entry's `model`/`effort`, and two names backed by the same engine run as
separate members. Duplicate proposals still collapse through the normal finding
fingerprint, and duplicate findings keep the maximum severity. An explicit
`--reviewer <name>` disables the roster for that invocation and runs only that
entry (still as five lens attempts). When the panel is empty or absent, review
falls back to the cross-model single-reviewer selection above.

For `codex`, pins emit `-c model=...` and `-c model_reasoning_effort=...`; for
`claude`, `--model ...` and `--effort ...`. Reviewer pins are appended to the
read-only reviewer command and do not add executor write-bypass flags. The
usage ledger records both the registry name (`agent_name`) and the inferred
engine (`agent`); execution and attempt artifacts keep recording the engine,
so cross-model comparison semantics are unchanged.

The human output for `clarify suggest`, `contract draft`, `execute dry-run`,
`execute run`, `review dry-run`, and `review run` includes a `Resolved` block
once per command. It shows the selected registry name plus the model and
effort values Pactum applied from its entry: pinned values are shown directly,
and an empty effort is shown as `inherit` because Pactum does not read the
agent CLI's own config. A `pinning` summary reads `pinned` (model and effort
both set) or `partial` (model only — the model is always pinned now that it is
required). The engine appears in the Agent/Attempt sections below it.

Pactum does **not** install, bundle, configure, or authenticate these CLIs. You
must install and configure each agent CLI separately and make its command
available on your `PATH` before Pactum can run it — Pactum only invokes the
command it expects (`codex` or `claude`). Use `pactum agents doctor` to confirm
the CLI is present.

## `pactum agents doctor`

`pactum agents doctor` diagnoses the built-in agent CLIs **without launching
them**: for each built-in it reports the command, input mode, resolved path on
your `PATH`, and a status of `on_path` or `missing_command` (with the issue
listed when the command is not found). Pass `--agent <name>` to inspect a
single built-in. Note: doctor predates the agent registry — it inspects the
underlying built-in CLIs by their built-in names (not registry names), and the
default executor/reviewer lines it prints reflect the built-in fallbacks, not
the registry's first entry; a registry-aware doctor view is a recorded
follow-up.

Run it before executing to confirm the agent CLIs are installed and visible:

```sh
pactum agents doctor
pactum agents doctor --agent claude
```

## Execution model: direct subprocess, no isolation

When you run an agent, Pactum launches it as a **direct subprocess in your
repository** (by default that subprocess is the agent's ACP adapter — see the
transport section below):

- The working directory is your repository root.
- The agent inherits your environment.
- The prepared prompt is piped to the agent's standard input.
- The agent's stdout and stderr are captured to attempt artifacts.

### Transport: ACP (default) or CLI

How the agent is *reached* is a swappable transport behind one seam, so the loop,
gate, and attempt lifecycle are unaware of it:

- **`acp`** (default) — the agent is driven over the [Agent Client
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
- **`cli`** — the one-shot agent CLI described above (`codex exec`,
  `claude -p`), with the prompt piped to stdin.

ACP is the default; `PACTUM_AGENT_TRANSPORT=cli` is the debug escape hatch that
selects the one-shot CLI transport (the value is trimmed and case-insensitive;
empty, `acp`, or any other value keeps the ACP default). The transport is
intentionally not a config key — it is an execution mechanism, not workspace
state, and the env var exists for debugging, not configuration. The two gaps
that blocked making ACP the default — model pins over ACP and a write guard for
read-only stages — are closed (see below); what remains is the documented
shell-command gating limitation for write stages.

The ACP adapters are external npm packages and inherit the agent's auth from the
environment.

Over ACP the idle `--timeout` is reset by **any agent protocol activity** —
streamed text, tool calls and tool-call updates, thoughts, plans, permission
requests, and client-serviced file reads/writes — not only by visible output.
An agent that works silently through tool calls for minutes (reading the repo,
running tools) keeps the watchdog fed and is not killed as idle; the timeout
fires only when the protocol goes truly quiet. The attempt log is unaffected:
only the agent's streamed message text is written to `stdout.log`, the
liveness ticks carry no content. A prompt response recorded before the kill
counts as the agent's completion signal for the completion-aware finalize
described under [Execute: dry-run vs run](#execute-dry-run-vs-run).

#### Model pins over ACP

The per-entry model pins from the agents registry reach the agent over ACP the
same way they reach the agent CLIs. The resolved pin travels with the run
request, and the transport threads it the way each adapter accepts it:

- **codex** — `codex-acp` accepts the same `-c` config overrides as the codex
  CLI, so the adapter is launched with `-c model="<model>"` (TOML-quoted) and
  `-c model_reasoning_effort=<effort>`.
- **claude** — `claude-agent-acp` launches Claude Code, which honors the
  `ANTHROPIC_MODEL` and `CLAUDE_CODE_EFFORT_LEVEL` env vars for the launched
  session, so the adapter subprocess gets them in its environment.

An unpinned run adds neither; the `Resolved` block shown before a run reflects
the same pin regardless of transport. One claude caveat: `claude-agent-acp`
resolves `ANTHROPIC_MODEL` against its known-model list and silently keeps the
default when nothing matches — a mistyped pin runs the default model while the
usage ledger records the pinned name (the CLI transport's `--model` fails loudly
instead).

#### Read-only stages over ACP

The read-only stages (`review`, `clarify suggest`, `contract draft`) are marked
read-only on the run request, and enforcement follows how each agent actually
performs writes:

- **claude** — `claude-agent-acp` routes the agent's file edits and permission
  requests through the ACP client, so the read-only client refuses them: every
  `WriteTextFile` is denied with a clear `read-only stage` error before touching
  disk (regardless of path scope), and permission requests are answered with a
  reject option (or cancelled when the agent offers none) instead of
  auto-approved. File reads keep working. The client still advertises the write
  capability — the agent must route writes through the client where they are
  denied, not fall back to native writes that would bypass it.
- **codex** — codex applies patches natively in-process and consults its own
  approval policy (a trusted repo asks no permission at all), so client-side
  denials cannot stop it. The adapter is instead launched with
  `-c sandbox_mode="read-only"`, pinning the same sandbox the CLI reviewer uses
  (`codex --sandbox read-only`) regardless of the operator's codex config.

Write stages (`execute run`, `review fix`) keep auto-approval and the
scope-guarded writes described below.

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

- It gates **only the file-write boundary** (`WriteTextFile`) and, on write
  stages, auto-approves permission requests. An agent that writes through a
  *shell command* it runs bypasses the guard; such changes are still caught
  only by the post-hoc gate.
- It applies **only to the ACP transport**. The CLI transport is unchanged and
  continues to rely entirely on the gate (and, for reviewers, on the agent's
  own sandbox flags). On write stages, when a contract declares no path-scope
  every in-repo write is allowed; read-only stages deny every write outright,
  as described above.

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
  output. The idle window resolves in order: an explicit `--timeout`, then the
  `timeouts.idle` workspace config key (a duration string, e.g. `15m`), then
  the built-in 10 minutes — the same resolution applies to every agent-running
  command that honors `--timeout`. Because execution is **unsandboxed**, `execute
  run` asks for confirmation on an interactive terminal and **requires `--yes`**
  when stdin is not a terminal (CI/automation), so it never launches an agent
  unattended by accident. `execute dry-run` never needs `--yes`.

The idle timeout is **completion-aware**: when the watchdog fires but the
captured output already carries the agent's successful terminal marker —
claude's final result envelope with `is_error: false`, codex's terminal
`turn.completed` event, or (over ACP) a prompt response recorded before the
kill — the attempt is finalized as **completed with a warning** instead of
failed. The exit code becomes 0, the attempt proceeds through the normal
success path, and the result document records the honest pair
`timed_out: true` + `completed_despite_timeout: true`, with a visible warning
in the attempt stderr, the live output, and the human summary. Partial or
error-terminal output keeps the plain timed-out failure. The kill itself still
happens — completion awareness finalizes honestly, it does not extend the
deadline. This applies to every stage that honors `--timeout`, not just
`execute run`.

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

Both write-stage prompts — the built executor prompt and the review-fix fixer
prompt — carry a shared **house style** section: match the surrounding idiom,
reuse existing helpers before writing new ones, small focused diffs,
simplicity-first with the over-engineering catalog as DON'Ts, no dead code,
and behavior-verifying tests with the fake-test catalog as DON'Ts. It is the
write-side mirror of the discipline rules the built-in reviewer checks (style rules keep the codebase consistent; the reviewer does not flag pure style) (see
[`review-prompt-design.md`](review-prompt-design.md)), so the writer is told
the rules it will be reviewed against.

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

## Clarify loop

`pactum clarify loop <run_id> --yes` composes `clarify suggest` and the
per-question recommendation into autonomous clarification rounds. Each round:

1. **Suggest** — runs the clarifier exactly as `clarify suggest` does (same
   prompt, same artifacts under `clarify/clarifier-attempts/`, same prompt-level
   dedupe via the existing-questions context).
2. **Auto-resolve** — every open question whose `confidence` is `high`, whose
   `recommended_answer` is non-empty, and whose `depends_on` prerequisites are
   all answered gets that recommendation recorded as its answer (answer source
   `auto_recommended`, decision source `clarify_loop_auto`). Questions with
   `medium`/`low` confidence, no recommendation, or an unanswered prerequisite
   stay open — a recommendation formed without the answer its `depends_on`
   declares it needs is not committed by automation. Questions resolve
   foundational-first, so a prerequisite auto-resolved earlier in the same
   round unblocks its dependents.
3. **Refresh** — after a round that auto-resolved something the clarification
   artifacts are refreshed once; a resolve-less round only recomputes the
   status (suggest already refreshed when it recorded questions).

The loop stops at the first of three terminals, checked after each round (a
round that errors finalizes the summary with the defensive `error` terminal
instead):

- `converged` — no open blocking questions remain (the same condition as the
  `clarify status` converged flag).
- `needs_human` — a round created no new questions and auto-resolved nothing:
  automation is out of moves, and the open blocking questions await the human.
- `max_rounds` — the round cap was reached. `--max-rounds` overrides the
  `clarify.max_rounds` workspace config key (default 3).

The loop writes `clarify/loop-summary.json`
(`pactum.clarify_loop_summary.v1`) with per-round counts (questions created,
auto-resolved, open blocking after), the terminal reason, the converged flag,
and the final per-dimension coverage; `--json` prints the same document.
`--reviewer` selects the clarifier explicitly (same resolution as `clarify
suggest`), the idle `--timeout` applies to each clarifier attempt, and `--yes`
is required because the loop runs clarifier agents directly. Running the loop
on an approved run surfaces the same approval-reset warning as `clarify
suggest`.

**The safety story is the downstream gate:** the loop lets the clarifier's own
high-confidence recommendations answer its questions, which is acceptable only
because `pactum contract approve` stays manual — the loop automates the
question-and-answer churn, never the human decision to approve the contract.

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

Every review spawns five built-in specialist reviewers — one per review lens:
`correctness`, `implementation`, `tests`, `over_engineering`, `docs`. The lens
set is fixed in code and deliberately not configurable. Each resolved reviewer
(the explicit `--reviewer` or each panel member) expands into five concurrent
lens attempts, each reading its own per-member, per-lens prompt
(`review/reviewer-prompt-<name>-<lens>.md`). A lens prompt carries only that
lens's checklist plus a focus note — the attempt is told it is the `<lens>`
reviewer, that the other lenses are covered by reviewers running in parallel,
and to report only findings within its lens without silently expanding scope.
Five attempts per reviewer per round cost five subprocess runs and five usage
records (all under the reviewer's registry name as `agent_name`); that cost is
a deliberate default in exchange for focused, higher-recall reviews.
Cross-lens duplicate findings collapse through the normal finding fingerprint
dedup, keeping the maximum severity.

- `pactum review dry-run <run_id> --reviewer codex` prepares the reviewer
  context and the five per-lens prompts (`review/reviewer-context.md`,
  `review/reviewer-prompt-<name>-<lens>.md`, `review/reviewer-dry-run.json`)
  without launching a reviewer; its output lists the five lens attempts that
  would run.
- `pactum review run <run_id> --reviewer codex` launches the five lens
  attempts concurrently (same direct-subprocess model as execution, with the
  idle `--timeout` per attempt) and captures each attempt under
  `review/reviewer-attempts/`, with the lens recorded in the attempt's
  request and result. Like `execute run`, it is
  unsandboxed agent execution, so it asks for confirmation on an interactive
  terminal and **requires `--yes`** for non-interactive/automated use; `review
  dry-run` never needs `--yes`. All lens attempts run to completion, but if any
  attempt fails the command (and a review-loop round) fails as a whole — the
  completed lenses' output stays on disk in their attempt artifacts.

Beyond the lens checklist, every lens prompt shares the same hardened review
methodology: findings must be certain-or-silent (with an explicit NOT-to-flag
list), a verify-then-report pass that emits only
CONFIRMED candidates, findings-first output with honest empties, pre-existing
issues as non-blocking advisories, and a per-finding `confidence`
(high/medium/low — recorded and displayed, not yet gating anything). The
design sources are condensed in
[`review-prompt-design.md`](review-prompt-design.md).

A reviewer can emit optional structured finding proposals as a fenced JSON block.
`pactum review propose-findings <run_id>` parses the captured reviewer stdout of
**every completed reviewer attempt** (all lenses) into **pending proposals** — it
does not create findings; pass `--attempt <id>` to parse a single attempt, and
with several attempts each warning is prefixed with its attempt id. In the manual flow, a
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
fixer's registry-entry pins (an omitted `--agent` defaults to the first
registry entry), prints the same `Resolved` block
as execution, captures request/result/stdout/stderr artifacts under
`review/fix/attempts/`, and writes `review/fix/fixer-prompt.md`,
`review/fix/fixer-context.md`, `review/fix/fixer-dry-run.json`, and
`review/fix/last-result.json`.

Like `execute run` and `review run`, `review fix` is unsandboxed and requires
`--yes` for non-interactive/automated use. It does not approve reviews, resolve
findings, or re-run the gate.

## Review loop

`pactum review loop <run_id> --reviewer codex --agent codex --yes` runs the
reviewer round — five concurrent lens attempts per resolved reviewer, with each
member's per-lens prompts written before the round launches and the lens
surfaced per attempt in the round summary — parses structured finding
proposals, accepts the proposals into review
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
