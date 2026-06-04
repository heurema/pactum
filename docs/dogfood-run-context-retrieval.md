# Dogfood: run-context retrieval

A real-task dry-run dogfood on a large TypeScript monorepo surfaced a bug in the
run prompt boundary. This note records the finding and the fix.

## M6.0 finding

The map and search themselves worked well. From zero knowledge of the repo,
manual `pactum search "format"` / `pactum search "formatCurrency"` found the
target files immediately. But the run's auto-assembled context was empty:

- `context/search-results.json` had `results: []`.
- `context/executor-context.md` showed `Relevant search results: None`.

So an agent relying on the prepared prompt got no first-pass file pointers, even
though the index clearly contained the right files.

## Root cause

The run context search ran the **entire task sentence as one FTS query**:

```
add a formatPercent helper to apps/admin/src/lib/format.ts following formatCurrency
```

FTS5 token matching effectively ANDs all tokens, so it required a single
document containing *add, formatPercent, helper, apps, admin, src, lib, format,
following, …* — which nothing satisfies. Even a two-word query like
`format helper` returned nothing.

## M6.1 fix

Run-context retrieval no longer queries the whole sentence. Instead it:

1. Extracts a small, ordered set of **targeted queries** from the task (and, at
   `prompt build`, from the approved contract): path-like strings first, then
   code identifiers, then split identifier / domain terms, then plain words —
   deterministic, deduped, capped at 8.
2. Runs each query through the existing local index and **combines** the hits,
   deduping by kind/path/title/code-kind, earlier queries first, capped at the
   result limit.
3. Refreshes the context at `prompt build` from the contract, since
   clarification and revision usually make the work more precise than the
   initial sentence.

The example above now extracts queries like:

```
apps/admin/src/lib/format.ts
formatPercent
formatCurrency
format
percent
currency
```

`executor-context.md` shows the query source, the targeted queries, and the top
results.

## Before / after

| | Before M6.1 | After M6.1 |
| --- | --- | --- |
| Run-context results for a realistic task sentence | 0 | non-empty, includes the target file and its test |
| Pactum self-dogfood (`fix run-context retrieval …`) | 0 | 10 |

## Scope and limitations

This is deterministic, lexical first-pass retrieval — not semantic relevance
ranking. There is no embeddings/vector search, no file-content indexing, and no
change to the global `pactum search` semantics. Agents are still instructed to
search and read source files before editing; source files remain the source of
truth.
