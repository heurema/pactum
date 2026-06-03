# Second-repo dogfood report

A record of running Pactum's safe (no-agent) command surface against a second,
real repository — one that is **not** Pactum itself. The goal was to see how the
project map, search, and the contract → prompt boundary behave on an unfamiliar
codebase in a different ecosystem.

No agent was launched: the run stops at `prompt build` and `execute dry-run`.
No code changes were applied to the second repo, and its working tree was left
untouched (the `.heurema/` workspace created by `pactum init` was removed
afterward).

## The repository

- **What it is (anonymized):** a private internal **TypeScript + Vue 3 (Vite)**
  web application. Repo name and path are intentionally omitted.
- **Ecosystem:** Node / Vite, Vue single-file components (`.vue`), TypeScript,
  a small Express-style server entry, Tailwind.
- **Size:** 62 tracked files; 61 indexed by Pactum (one file was deleted in the
  working tree at dogfood time).
- **Why chosen:** small, real, and deliberately **not Go** — Pactum is itself a
  Go project, so this exercises the TS/JS tree-sitter path and a framework
  (Vue) Pactum has no dedicated grammar for.

## Commands run

```sh
# from the second repo, with $PK = <pactum>/bin/pactum
$PK init
$PK status
$PK search "main" --kind code_item
$PK search "config"
$PK memory search "test"

$PK task new "understand repository structure and identify a safe small improvement"
$PK clarify ask "Should this dogfood avoid real agent execution?" --blocking
$PK clarify answer q_001 "Yes. This dogfood should stop at prompt build and dry-run only."
$PK contract revise --goal "..." --add-in-scope "..." --add-out-of-scope "..." \
  --add-acceptance "..." --add-validation "echo dogfood-only"
$PK contract approve --by manual
$PK prompt build
$PK prompt show
$PK execute dry-run --agent codex
$PK agents doctor
$PK status
```

Not run: `execute run`, `review run`, `gate run` (validation was a no-op
`echo`, and running it would have added no signal).

## What worked

- **`init` mapped the repo cleanly:** 61 files indexed, 143 code items, search
  index `ready`, status `fresh` — on the first try, no configuration.
- **TypeScript extraction is real:** code items break down as 77 `ts_import`,
  44 `ts_func`, 13 `ts_interface`, 6 `ts_export`, plus 3 JS items. Functions and
  interfaces are surfaced, not just files.
- **The full staged flow ran with zero run ids typed.** After `task new`, every
  subsequent command (`clarify`, `contract revise/approve`, `prompt build`,
  `execute dry-run`) resolved the current run automatically. The blocking
  clarification correctly gated approval until answered.
- **Prompt boundary is honest and auditable.** `prompt show` rendered the
  approved contract (goal, in/out of scope, acceptance, validation) with the
  pinned contract hash; `execute dry-run` printed the exact `codex exec … <
  prompt.md` it *would* run plus boundary checks (`contract hash: ok`,
  `project map: fresh`) — without launching anything.
- **`agents doctor` worked as a pure PATH check** and exited 0.

## What was confusing / rough edges

- **Code-item search is noisy because imports dominate.** `search "main"
  --kind code_item` returned `ts_import` entries from `src/client/main.ts`
  (e.g. `./router`, `pinia`) at the top — not the entry-point logic a human
  means by "main". Imports are 54% of all code items (77/143), so they crowd
  out functions/interfaces in code-item results.
- **No "next step" hint after `prompt build` in its own output.** You learn the
  next command from `pactum status` (which did show
  `next: pactum execute dry-run --agent codex`), not from `prompt build`
  itself. Minor, but the discoverability lives in one place.

## Search usefulness

Mixed, and honestly weakest where this repo is densest:

- `search "config"` was **useful**: it surfaced `postcss.config.js`,
  `tailwind.config.js`, `vite.config.ts`, and a `config`-related import in
  `src/server/index.ts` — a reasonable first map of where configuration lives.
- `search "main" --kind code_item` was **less useful** (imports first, see
  above).
- Because search is lexical with fixed weights, it is predictable and fast, but
  it does not rank "the important symbol" above "a symbol that merely mentions
  the token." For a newcomer that means search is a good *locator*, not a good
  *prioritizer*.

## Map usefulness

- Good as an inventory and for TS structure. The repo map + 143 code items give
  a quick, deterministic sense of the TS surface.
- **Biggest gap: Vue single-file components are opaque.** 17 `.vue` files are
  indexed as **files**, but produce **0 code items** — Pactum has tree-sitter
  grammars for TS/JS but none for `.vue`, so the `<script>`/component logic that
  makes up much of a Vue app is invisible to code-item search. On this repo that
  is a large blind spot (Vue SFCs are ~28% of files). This is expected given the
  MVP grammar set, but it materially limits map usefulness for Vue/Svelte/etc.
  front-ends.

## Memory behavior

- `memory search "test"` correctly returned "No accepted memory items matched"
  on a fresh workspace. Memory only accrues after a reviewed run is closed out
  and `memory accept` is run, so an empty result here is the right answer, not a
  failure. Memory was not exercised further because this dogfood deliberately
  stopped before review.

## Current-run behavior

- Excellent. This was the single biggest ergonomic win on an unfamiliar repo:
  `task new` set the current run, and nothing downstream needed the
  `run_2026…` id. `pactum status` consistently reported `latest`, `current`, and
  a runnable `next` command, so "where am I / what next" was always one command
  away.

## Prompt boundary behavior

- `prompt build` recorded the boundary (contract hash, map freshness) and
  `prompt show` reproduced the contract deterministically. `execute dry-run`
  re-verified the boundary before printing the planned command. Nothing executed.
  This is the part that most clearly delivers on the "explicit, auditable gates"
  premise, and it behaved identically to the home repo.

## Agent doctor result

- `agents doctor` reported both built-in agents (`codex`, `claude`) with the
  default executor/reviewer = `codex`, as a PATH check only, exit 0. On a
  machine without those CLIs it would report `missing_command` and still exit 0.

## Known limitations observed

- `.vue` (and by extension other non-TS/JS framework SFC formats) yield no code
  items — only file-level indexing.
- Code-item search is dominated by imports; no symbol-importance ranking.
- Lexical search locates but does not prioritize.
- Memory is empty until a run is reviewed and accepted (by design).

## Concrete follow-up candidates

1. **De-emphasize or filter imports in code-item search** (e.g. a `--kind`
   value or default weighting that ranks `ts_func`/`ts_interface` above
   `ts_import`). Highest-value, smallest-scope improvement observed.
2. **Vue/SFC awareness** — at minimum extract the `<script>` block of `.vue`
   files through the existing TS grammar so component logic becomes code items.
   (Larger; explicitly out of MVP, but the clearest map gap.)
3. **Optional: a `Next:` hint on `prompt build` output**, mirroring `task new`
   and `status`, so each step is self-documenting.

None of these are blockers; Pactum's safe surface worked end-to-end on a real,
unfamiliar, non-Go repository.
