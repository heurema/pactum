# Project memory

Pactum can capture reusable project knowledge from reviewed runs and feed it back
into future runs. Memory is deliberately simple and predictable:

- **Deterministic** — a memory candidate is derived mechanically from a run's
  contract, gate report, and review. The same inputs always produce the same
  candidate. The candidate's source is recorded as `deterministic`.
- **Manually accepted** — nothing enters project memory until a human runs
  `pactum memory accept`. Proposing a candidate never adds to memory by itself.
- **Lexical** — selection and search match query tokens against item titles,
  summaries, tags, and file paths with fixed weights. There are no embeddings or
  vector search.
- **File-hash freshness tracked** — each accepted item records the files it
  touched and their SHA-256 hashes at acceptance time, so Pactum can later detect
  when those files changed.

There is **no LLM summarization** of memory and **no semantic stale detection**:
freshness is computed purely from file hashes.

## Propose, show, accept

`pactum memory propose <run_id>` builds a deterministic memory candidate for a
reviewed run and writes it to `runs/<run_id>/memory/memory-candidate.json` (with
a `memory-candidate.md` preview and a pending `memory-acceptance.json`). It
requires the run to be fully closed out: the contract approved, a gate report
present, the review approved, and no pending reviewer proposals.

`pactum memory show <run_id>` prints the candidate preview.

`pactum memory accept <run_id> --by manual` appends the candidate as an accepted
item (`mem_NNN`) to `memory/items.jsonl`, regenerates the human-readable
`memory/project-memory.md`, and records the touched files' hashes as the item's
baseline freshness.

```sh
pactum memory propose <run_id>
pactum memory show <run_id>
pactum memory accept <run_id> --by manual
```

## Search

`pactum memory search "<query>"` searches accepted memory lexically. Query tokens
are matched against each item's title, summary, tags, and files using fixed
weights (title highest, files lowest); stale items are penalized so fresh
knowledge ranks first. Results are read-only and capped by `--limit` (default 5).

```sh
pactum memory search "search index" --limit 5
```

## Refresh and stale

Accepted memory can drift as the repository evolves. Freshness has three states:

- `fresh` — every tracked file is unchanged from its accepted hash.
- `stale` — a tracked file changed or is missing.
- `unknown` — the item has no tracked files, or a file could not be read.

`pactum memory refresh` re-hashes the tracked files of every accepted item,
records a refresh entry in `memory/refreshes.jsonl`, and rewrites
`memory/project-memory.md` with the updated statuses. `pactum memory stale` shows
the stale and unknown items (read-only), using the latest refresh when one
exists.

```sh
pactum memory refresh
pactum memory stale
```

## Memory prompt boundary

When you run `pactum prompt build <run_id>`, Pactum selects the accepted memory
relevant to the contract (the same lexical scoring used by search) and writes it
into the run as `context/memory-context.md` and `context/memory-selection.json`.
The executor prompt lists the selected items with their freshness, and instructs
the agent to treat memory as context — not as semantic truth — and to verify
stale items against current source before relying on them.

To keep this boundary honest, `prompt build` records hashes in
`contract/prompt-manifest.json` for both the run-local memory artifacts and the
global accepted-memory source files (`memory/items.jsonl` and
`memory/refreshes.jsonl`). Before `pactum execute dry-run` or `pactum execute
run` proceeds, it re-verifies those hashes: if the run's memory context changed,
or if accepted memory changed after the prompt was built, execution refuses until
you rebuild the prompt. Memory that is merely **stale** (but unchanged since
prompt build) does not block execution — it is surfaced in the prompt so the
agent verifies it.
