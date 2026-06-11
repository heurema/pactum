# Agent file navigation — design notes

Survey-driven design input for reducing what agents spend reading large
source files. Sources: published localization research (2024–2026), the
repo-map machinery of a leading open-source pair-programming tool, the
symbol-navigation toolset of an LSP-backed agent server, and the context
engineering guidance published by the agent-platform vendors. Ideas are
absorbed here without attribution; this document is the reference for the
agent file-navigation backlog arc.

## The problem, measured on pactum itself

Executor, reviewer lenses, clarifier, and drafter read source files through
their own grep/read tools. The executor prompt inlines only the contract plus
`context/executor-context.md` (map excerpts and search results) — raw files
never travel in the prompt — so the cost shows up *during* the run: on a
large file the dominant pattern is a three-hop loop (grep for the identifier,
read a window, re-read a wider window to understand couplings), multiplied by
ten review lens attempts per round. A representative dogfood run burned
~2.6M input tokens on the execute leg and ~2.7M on review, ~92% of it
cache-read — healthy caching, but a 1.5k-line file read early stays in every
later turn's bill. `internal/app/review.go` is ~1.5k lines; `gate.go` and
`contract_draft.go` are ~0.7k each.

## What the survey established

### 1. Signatures-only skeletons are sufficient for localization

The strongest result. A staged localization pipeline — pick suspicious files
from the repo tree, pick symbols from *skeletons* of those files (signatures,
class fields, declarations, module comments — no bodies), then read full
content only for the chosen elements — was strong enough on the standard
repair benchmark that a major vendor adopted it as benchmark infrastructure.
Cost per issue was cents, with no embeddings anywhere. Full-file reads are
only needed at the final edit step; everything before that works from
outlines.

### 2. A token-budgeted signature map beats inlining files

The repo-map approach: extract definitions and references per file with
tree-sitter, rank symbols by a reference-graph walk, and render the top
signatures into a fixed token budget (~1k by default). The model learns "how
to use the API exported from a module" from signatures and asks for full
files only when needed. Pactum's map wiki is area-level, not symbol-level —
the per-file outline layer is the missing piece, and pactum's code-items
index already stores per-symbol `Signature`/`StartLine`/`EndLine`/`Exported`,
so rendering outlines is serialization, not new parsing.

### 3. Symbol-boundary chunks beat line windows

AST-aware chunking research: splitting retrieval units along syntax-tree
boundaries (then merging small siblings) improved both retrieval recall and
end-task pass rates over fixed line windows, across languages. For pactum the
actionable reading is not a new chunker — symbol granularity ≈ what the
code-items index already models — but that FTS5 documents could index symbol
*bodies* so a lexical hit inside a function body returns the enclosing
symbol's exact range.

### 4. Symbol-level navigation is the highest-value agent primitive

LSP-backed agent toolsets expose "symbols overview of a file" and "read/edit
symbol by name"; field reports consistently name the per-file symbols
overview as the biggest token saver. The lesson transfers without the LSP
runtime: the agent's own ranged-read tool is fine — what it lacks is the
*address*. Supplying `path:start-end` turns the first read into the right
read.

### 5. References over payloads (context engineering)

Vendor guidance for long-horizon agents: maintain lightweight identifiers
(paths, stored queries) and load data just-in-time rather than inlining
everything up front; retrieve *some* data up front for speed and let the
agent explore for the rest. `executor-context.md` is pactum's "some data up
front"; the upgrade path is making its references symbol-precise instead of
file-precise.

### 6. The null hypothesis: simpler navigation over well-factored code

The team behind one of the major coding agents removed vector retrieval
entirely — agentic search "generally works better... simpler and doesn't
have the same issues around security, privacy, staleness, and reliability."
Every derived index is a liability; pactum already pays the tree-sitter
index cost, so exposing it better is the cheapest position on that curve —
and refactoring the oversized files (the decomposition arc) is the fix no
tooling substitutes for.

## Explicit non-goals

- **No embeddings / vector index.** Nondeterministic across model versions,
  stale by construction, real infra cost; marginal gain over FTS5 + symbol
  ranges for a repo that already has lexical search with rerank.
- **No LSP runtime dependency.** Language-server processes are stateful and
  flaky in headless runs; the two valuable verbs (file outline, symbol
  address) are reproducible from the deterministic code-items index.
- **No model-based context compression.** Perplexity-ranked pruning needs a
  model in the loop — incompatible with reproducible artifacts.

## Staleness and determinism rules

Any derived outline lies the moment the agent edits the file mid-leg.
Outlines are generated at prompt-build time from the working tree and
stamped with the source file's content hash; the prompt convention says line
numbers are valid until the first edit of that file (then fall back to
grep). Tree-sitter extraction, FTS5/BM25 ranking, and skeleton rendering are
deterministic given tree state — compatible with pactum's
reproducible-artifact requirement.

## Slicing (mirrors the backlog arc)

1. **Symbol-grade search results** (small) — plumb
   `StartLine`/`EndLine`/`Signature` into `code_item` search results and the
   executor-context rendering (`path:start-end signature`); add
   `pactum search --symbol <name>`.
2. **`pactum outline <path>`** (small-med) — deterministic per-file skeleton
   command + the executor-prompt convention (outline first for files over
   ~400 lines, then read by range).
3. **Contract-scoped skeletons in executor-context** (med) — inline outlines
   of contract-path-scope files at prompt build under a token budget,
   exported symbols first.
4. **Review hunk→symbol annotation** (med) — annotate diff hunks with the
   enclosing symbol and full range so verify-then-report reads the whole
   function once; targets the review leg directly.
5. **Decomposition of `internal/app`** (tracked separately under hardening) —
   smaller files sharpen grep precision, outline quality, contract path
   scopes, and cache locality at once.
