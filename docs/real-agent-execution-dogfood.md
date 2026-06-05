# Real agent execution dogfood (M8.0)

**Date:** 2026-06-05
**Branch:** `dogfood/m8.0`
**Isolation:** none — by deliberate decision. Execution ran directly in this
repository's working tree (a throwaway branch, not a separate clone/VM/container).
Both built-in agents run **unsandboxed** with the current user's credentials.
**Run id:** `run_20260605_093754`

This report records the first controlled run of Pactum's real `execute run` path
through the external agent CLIs (codex, claude), end to end:
`contract → prompt → execute run → gate → review`.

LLM execution uses external agent CLIs only. No native OpenAI/Anthropic API
calls, no provider abstraction, no in-process agent runtime were added.

## Environment

| Agent | Binary | Version | Auth | `agents doctor` |
|-------|--------|---------|------|-----------------|
| codex | `codex` | codex-cli 0.134.0 | ChatGPT login | `ready` |
| claude | `claude` | 2.1.161 (Claude Code) | macOS keychain | `ready` |

Both authenticated and on `PATH`, so the dogfood was not blocked on availability.
`pactum version`: 0.1.0 (commit of the dogfood build includes the claude fix below).

## Task (tiny, reversible, docs-only)

> Add one sentence to `docs/agents.md` clarifying that real agent execution
> (`pactum execute run`) runs unsandboxed.

Contract scope: in — "tiny bounded docs-only change"; out — large refactor,
dependency changes, touching generated `.heurema` artifacts. Acceptance — "small
and reviewable". Validation — `make check`.

## Executor fix shipped in this PR

The claude built-in executor previously ran `claude -p` with **no permission
flag**. In headless mode (no TTY) Claude Code's default permission system denies
edit/write tool calls, so `execute run --agent claude` would exit 0 with an
**empty diff** — a success-looking no-op. codex already ran
`--dangerously-bypass-approvals-and-sandbox`; the fix adds the symmetric
`--dangerously-skip-permissions` to claude so both user-selectable executors can
actually mutate the working tree. (`internal/agents/config.go`, plus test
assertions.)

## Commands run

```
make build
pactum version
pactum init
pactum map refresh
pactum agents doctor
pactum task new "<task>"
pactum clarify ask "Should this real execution dogfood keep the change tiny and reversible?" --blocking
pactum clarify answer q_001 "Yes. ..."
pactum contract revise --goal ... --add-in-scope ... --add-out-of-scope ... --add-acceptance ... --add-validation "make check"
pactum contract approve --by manual
pactum prompt build
pactum execute dry-run --agent codex
pactum execute run   --agent codex  --yes       # attempt_001
pactum execute status / show --logs
pactum gate run --allow-commands / gate show
pactum review prepare / review status / review show
git checkout -- docs/agents.md                  # reset before symmetry check
pactum execute run   --agent claude --yes        # attempt_002 (post-fix)
pactum gate run --allow-commands                 # gate claude's change
```

## Execution results

| Attempt | Agent | Exit | Duration | Files changed | In scope? |
|---------|-------|------|----------|---------------|-----------|
| attempt_001 | codex | 0 | 71.2 s | `docs/agents.md` only | yes |
| attempt_002 | claude | 0 | 50.5 s | `docs/agents.md` only | yes |

Both agents made exactly the requested one-sentence docs change, touched only
`docs/agents.md`, and **independently ran `make check` themselves** during
execution before reporting done. No code, dependency, or `.heurema` changes.

The retained change in this PR is claude's wording (attempt_002), which was
gate-validated after the fix; codex's attempt_001 went through the full
gate→review loop and produced an equivalent in-scope sentence.

### Artifact observations

- Per-attempt artifacts written under
  `execute/attempts/attempt_NNN/{request,result,stdout,stderr}.log` plus
  `execute/last-result.json`. `result.json` (`pactum.execution_result.v1`)
  records `started_at`/`finished_at`/`duration_ms`/`exit_code`/`timed_out`.
- **Log channel differs per agent:** codex writes its reasoning/progress trace to
  `stderr` (~23 KB here), with `stdout` carrying the result; claude writes its
  output to `stdout` and left `stderr` empty. `execute show --logs` surfaces both
  (truncating to the last ~80 lines / 8 KB), but a reader should know the
  meaningful content lives in different channels depending on the agent.
- `execute show --logs` output is verbose (full agent reasoning + repeated
  diffs). Fine for debugging; noisy for a quick glance.

## Scope adherence / gate enforcement (the core thing being tested)

The point of the dogfood was whether the `contract → gate` loop actually holds a
real, non-deterministic agent inside its boundaries.

- Both agents **stayed within scope** on their own — single-file, docs-only,
  no out-of-scope edits, no `.heurema` writes. So scope enforcement was not
  adversarially stressed this round (the agents simply complied).
- `gate run --allow-commands` correctly: detected exactly 1 changed file, ran the
  approved validation command (`make check`) and reported `passed: 1 / failed: 0`,
  and set gate status to `needs_review` (not auto-pass). Gate ran cleanly for
  both attempt_001 (codex) and attempt_002 (claude).
- **Not yet validated:** what gate does when an agent *overshoots* scope (extra
  files, `.heurema` writes, failing validation). That needs a deliberately
  out-of-scope task in a future round — see recommendations.

## Gate / review results

- **Gate:** `needs_review`, validation `make check` passed, 1 changed file.
- **Review:** `review prepare` created the review artifact in `pending` state
  with 0 findings. The reviewer agent was **not** run and review was **not**
  auto-approved (per plan — no memory created). The handoff
  `gate → review prepare → review status/show` worked without friction.

## UX gaps observed

1. **`agents doctor` "ready" overpromises.** It only does `exec.LookPath` (PATH
   presence) — it does not verify auth or edit-capability. "ready" should read
   more like "on PATH". (Note: `ready` is an enum value in the
   `pactum.agents_doctor.v1` schema, so changing it is a schema change, not a
   cosmetic tweak — deferred, see below.)
2. **No way to list clarification questions.** `pactum clarify list` errors with
   "unexpected argument list". The question id is shown by `clarify ask`, but
   there is no obvious command to re-list open question ids later.
3. **Log channel asymmetry** between codex (stderr) and claude (stdout), as above
   — not a bug, but undocumented.

## Is `execute run` ready for normal use?

Yes, for its intended model: the full loop
(`contract → prompt build → execute run → gate → review`) works end to end with
both real agent CLIs, artifacts are complete and well-structured, prompt/contract/
map/memory boundaries are re-verified before launch, and `--yes` is required for
non-interactive runs. The understood constraints stand: execution is
**unsandboxed**, uses external CLIs only, and there is no Docker/native-API path.

## Recommended next fixes

- **Clarify `agents doctor` status** so "ready" doesn't imply auth/edit-capability
  (handle the schema enum change deliberately — separate small PR).
- **Add `clarify list`/`clarify show`** (or document that ids come from
  `clarify ask`) so open question ids are discoverable.
- **Exercise scope overshoot:** run a deliberately out-of-scope task to confirm
  the gate flags extra/`.heurema`/failing changes rather than passing them.
- **Cross-review prep:** the agent descriptor is shared between executor and
  reviewer roles. With `--dangerously-skip-permissions`, claude-as-reviewer could
  technically write files. Before enabling codex⇄claude cross-review, split
  executor vs reviewer args so the reviewer runs without write-bypass.

No secrets, private local paths, or agent auth details are included in this report.
