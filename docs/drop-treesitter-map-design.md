# Dropping tree-sitter + the persisted project map (pure-Go simplification)

Status: **research/design DONE (2026-06-21).** Conclusion of a deep web-research
pass plus a two-round Claude + Codex + Gemini council. Recommends removing
pactum's tree-sitter code-index and the persisted global project map, going
pure-Go, and replacing the map with a run-local repo snapshot + on-demand git
search. No implementation has started.

## Why (the problem)

pactum is a contract-first orchestrator that **drives an external coding agent**
(Claude Code / Codex over ACP) which already has its own Read/Grep/Bash tools.
It currently builds, per run: a deterministic wiki-first project **map**
(file inventory + generated wiki + `repo-map.md` + `llms.txt`), a **SQLite FTS**
lexical index, and a **tree-sitter code-items** index (symbol hints), via
`go-tree-sitter` + 5 grammars.

Footprint and findings:

- **tree-sitter is pactum's ONLY CGO dependency.** `internal/codeindex` ≈ 1083
  LOC (treesitter.go ~21 KB) + 6 grammar deps. Dropping it makes pactum
  **pure-Go**.
- **The executor ignores the map.** Measured: `run_20260617_115334`'s execute
  attempt had **0 references** to `pactum search` / `repo-map` / `llms.txt` /
  `search.sqlite` — it re-explored with its own tools. pactum's own backlog:
  "the map earns its keep for *our pipeline* more than for the executor's
  navigation." The README calls the code-items "best-effort symbol hints,
  incomplete by design."
- `internal/projectmap` ≈ 3136 LOC; the map is woven into prompt build, gate
  scope, status, run, execute. The tree-sitter index feeds the map's symbol
  layer.

## Evidence (external deep-research, 2024–2026 sources)

- **Dominant pattern is agentic on-demand search (ripgrep + read), NOT a
  pre-built map/AST/embedding index.** Claude Code, Codex CLI, Cline, OpenHands
  are on-demand / anti-index (Codex's own prompt: *"rather than relying on
  external indexing or embeddings"*); Anthropic/Cline/Cognition argue dynamic
  agentic search beats static retrieval. Cursor (embeddings) is an IDE-inline
  outlier, a different product shape.
- **Aider's tree-sitter repo-map** is the canonical pre-built artifact, but it
  publishes no with/without benchmark, **disables the map for weaker models**
  ("overwhelmed/confused"), and is a *context-injection* design for models that
  lack a Read/Grep loop — pactum's executor has one.
- **Localization research:** AST graphs win on fine-grained localization
  *accuracy* (LocAgent ~94% file-acc vs BM25 ~62% vs embeddings ~80–85%), **but
  embeddings lose to plain LLM navigation** (Agentless 78.7% vs 67.7%), and the
  end-to-end *resolve* lift of a graph index on a **strong** agent is small
  (RepoGraph +2.34 Agentless / +2.00 SWE-agent; the "99.63%" was off a weak
  baseline, only 8.56% off the strong one). What moves the needle is search
  *output* quality (summarized/ranked/truncated beats raw), not index-vs-grep.
- **tree-sitter in a cross-platform Go binary is a high, citable tax:** CGO
  required; cross-compile disabled by default (needs a C cross-toolchain per
  target); documented Windows/macOS pain; multi-grammar size/go.sum pollution;
  the CGO-free WASM path is pre-release + ~10× slower.
- **A redundant search tool is net-negative** for a grep-equipped agent
  (Anthropic: overlapping tools distract; return high-signal bounded output).
  Lexical FTS over `rg`/`git grep` only pays with ranking + bounded output;
  Sourcegraph dropped embeddings for keyword/BM25.

## Decision: B-minus + run-local baseline snapshot (council unanimous)

Drop tree-sitter, drop the persisted global map/wiki/FTS, and stop advertising
map context to the executor. pactum's deterministic boundary becomes a **small
run-local repository snapshot** (files + content hashes) captured at prompt
build; inventory comes from `git ls-files`, code search from `git grep` on
demand.

- **Contract scoping** needs only a path list (`git ls-files`).
- **Gate out-of-scope** is path-glob based — file-level only; no symbols, no map.
- **Gate change-detection / audit / no-drift** use the run-local snapshot
  (files+hashes captured at prompt build; execute verifies no drift since then;
  gate diffs current files against the baseline).
- **Reviewer / search context** is a lazy `git grep` at the moment of need,
  sorted by path, capped, recorded into run context — not a persisted index.
  Rationale: the latency bottleneck is the model (~10 s), not search (~20–50 ms
  cold `git grep`), so a built + cache-invalidated FTS index earns nothing; and
  BM25 is prose-tuned and butchers exact code symbols.

## The pure-Go cascade (the high-leverage win)

Removing CGO collapses the entire release/distribution complexity:

- The GitHub Actions build matrix becomes **one Linux runner + `GOOS/GOARCH`**
  cross-compilation — no native-runner matrix, no Windows-mingw build, no
  `zig-cc` hacks, no macOS runners.
- **musl/Alpine, Windows-ARM, and FreeBSD come for free**; smaller binary.
- The npm launcher still ships per-platform binaries, but they are now trivially
  built (and musl becomes possible).
- Caveat: it shifts a dependency from CGO to the **host** — pactum hard-requires
  `git` (and `git grep`/ripgrep) on `PATH`, which it already does as a git tool.

This directly retires the friction of the native-runner + Windows-release work.

## What happens to the map (before → after)

- **Before:** per-run file inventory + generated wiki + `repo-map.md` +
  `llms.txt` + SQLite FTS + tree-sitter code-items — all built, kept fresh, and
  largely ignored by the executor.
- **After:** a **run-local snapshot** (files + hashes) for the gate / scoping /
  audit, plus **on-demand `git grep`** for pactum-internal context. The map is
  **not injected into the executor prompt** (it navigates with its own tools).
  The persisted wiki / `repo-map.md` / FTS index are removed.

## Migration sequence (phased)

1. **Drop `internal/codeindex` / tree-sitter → pure-Go.** Emit zero code-items
   (or a deprecated `--symbol` error) for one slice; remove the 6 grammar deps +
   CGO; remove `--symbol`/code-item prompt guidance. Touch points:
   `status.go` (requires `code-items.jsonl`/`repo-map.md`/`llms.txt`/
   `search.sqlite`), `map.go` (writes them together), `internal/search/types.go`
   (imports `codeindex.Item`), config (`map.code_index`), prompt text, tests,
   docs. **Acceptance:** `CGO_ENABLED=0 go test ./...` passes; no `runtime/cgo`
   or tree-sitter in deps; smoke flow intact; `make check` green.
2. **Flip release.yml to pure-Go cross-compile** (single runner + `GOOS/GOARCH`;
   delete native runners + mingw; drop the musl rejection; keep per-platform
   checksum assets). Update the npm launcher's platform mappings/tests.
3. **Stop executor map injection** — remove repo-map/search guidance from the
   executor prompt/context; keep contract + memory + clarifications + validation.
4. **Replace the global map with a run-local baseline + lazy search** — a
   repository-snapshot package; prompt build captures the baseline; execute prep
   verifies no drift; gate diffs current files against the baseline; status
   computes inventory on demand.
5. **Delete the persisted map/FTS surface + docs** — `map refresh` becomes a
   deprecated no-op/alias; update README, install docs, workflow docs, the agent
   skill, and tests.

## Irreversible loss / honest case to NOT do this

The only thing not cheap to re-add is the public promise of **`pactum search
--symbol`** (exact, multi-language, line-addressed symbol results). Mitigation:
**preserve the `pactum search` command shape + result schema** so a future
content-FTS or `go/parser`-based symbol layer can slot back in — just stop
backing it with tree-sitter/CGO. More broadly, this commits pactum to an
**LLM-heavy** architecture: if it later wants cheap *non-LLM* static analysis,
deterministic refactoring, or semantic chunking, re-adding tree-sitter/CGO is
painful. Because the artifacts are generated and source files are the truth, the
capability can return later as an optional, benchmark-backed plugin.

## What is NOT affected

pactum's differentiator — **contract-first + durable ledger + loud review
loops** — does not depend on the map or tree-sitter. This simplification removes
navigation scaffolding that does not earn its keep; it does not touch identity.

## Highest-leverage first slice

**Drop `internal/codeindex` / tree-sitter → pure-Go** (migration step 1). It is
the biggest single simplification in the codebase, it lands the release-pipeline
cascade, and it is low-risk (the symbol layer is "best-effort" and ignored).

## Sources

Aider repo-map (docs + `aider/repomap.py` + 2023 blog); OpenAI Codex CLI system
prompt; Cline (no-RAG post, 2025-05-27); Anthropic (multi-agent research system,
2025-06-13; writing-tools-for-agents, 2025-09-11); Cognition SWE-grep
(2025-10-16); LocAgent (arXiv 2503.09089); Agentless (arXiv 2407.01489);
RepoGraph (arXiv 2410.14684); SWE-agent (arXiv 2405.15793); Go `cmd/cgo`
cross-compilation docs; `go-tree-sitter` cross-compile issues; Sourcegraph Cody
keyword/BM25 pivot.
