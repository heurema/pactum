# Project map evaluation (post-wiki, post-CommonJS)

This report records the state of Pactum's deterministic, wiki-first project map
after M5.6–M5.8 and recommends what to do next. It is an evaluation document —
it changes no product behavior.

- **Date:** 2026-06-04
- **Pactum baseline:** `main` after M5.6 (wiki-first map, #29), M5.7 (map
  quality + search polish, #30), and M5.8 (CommonJS symbol hints, #31).
- **Method:** for each repository, a shallow clone was evaluated read-only with
  `pactum init` + `pactum map refresh` + `pactum search`. No agents were run, no
  repository was modified, and every generated `.heurema/` workspace was removed
  afterward. Nothing from the evaluated repos is committed here.

## Summary

The map is **useful enough across every ecosystem tested** to proceed toward
broader dogfood / release work rather than more language depth.

- Ecosystem detection fired correctly for all six repos (Go, Node.js, Rust,
  Java/Maven, Python, frontend) using deterministic manifest facts.
- Candidate entrypoints were found wherever a conventional entrypoint exists
  (Go `cmd/*/main.go`, JS `index.js`, Rust `src/main.rs`, Vue `src/main.ts`,
  Python `__main__` files); library repos correctly showed none invented.
- Command hints (`make`, `go`, `npm`, `pytest`, `mvn`/`gradle`, CI `run:`) were
  surfaced from manifests only — no guessed commands.
- Import-like entries never leaked into `--kind code_item` in any repo.
- Unsupported languages (Rust, Java) produced **0 code items** but still got a
  complete, useful wiki (ecosystem, areas, entrypoints, commands, config).
- CommonJS support (M5.8) turned `expressjs/express` from **0 → 447** code
  items.

The largest remaining blind spot is **file search being metadata-only** (path /
language / kind), which weakens content search in repos whose language has no
code items (e.g. Rust, Java). Symbol extraction for Rust/Java and `.vue`
`<script>` blocks remains deferred and, based on this evaluation, should stay
deferred for now.

## Repo matrix

| Repo | Language | Files | Code items | Ecosystem detected | Candidate entrypoints | Commands surfaced |
| --- | --- | ---: | ---: | --- | --- | --- |
| Pactum | Go | 81 | 980 | Go | `cmd/pactum/main.go` | Make 7, Go 3, CI 4 |
| expressjs/express | JavaScript (CommonJS) | 213 | 447 | Node.js / JavaScript | `index.js` | npm 6, CI 6 |
| sharkdp/fd | Rust | 56 | 0 | Rust | `src/main.rs` | Make 9, CI 6 |
| google/gson | Java (Maven) | 310 | 0 | Java (Maven) | none (library) | JVM 2, CI 4 |
| psf/requests | Python | 121 | 895 | Python | `src/requests/certs.py`, `help.py` | Make 7, Python 2, CI 2 |
| antfu/vitesse | TS / Vue / Vite | 79 | 69 | Node.js + frontend | `src/main.ts`, `vite.config.ts` | npm 12, CI 4 |

Code-item composition (definition vs. import-like) on the repos that produce
items:

| Repo | Definitions | Import-like |
| --- | --- | --- |
| Pactum | go_func 401, go_method 50, go_type 32, go_main 1 | go_import 430, go_package 66 |
| express | js_export 62 | js_import 385 |
| requests | py_method 435, py_func 94, py_class 70, py_main 2 | py_import 257, py_module 37 |
| vitesse | ts_func 10, ts_export 10, ts_type 1 (+1 js_export) | ts_import 46 (+1 js_import) |

Search behavior, per repo (representative query):

| Repo | Query | Default top result | `--kind code_item` import leak? |
| --- | --- | --- | --- |
| Pactum | `prompt` | `file internal/app/prompt.go` | none (10 results) |
| express | `router` | `code_item lib/express.js` | none (3 results) |
| fd | `search` | `repo_map` (no code items) | n/a (0 results) |
| gson | `json` | `file …/reflect-config.json` | n/a (0 results) |
| requests | `session` | `code_item …/sessions.py` | none (10 results) |
| vitesse | `vite` | `wiki map/wiki/entrypoints.md` | none (1 result) |

## What improved after the wiki-first map (M5.6 + M5.7)

- **Orientation no longer depends on Tree-sitter.** Even with 0 code items
  (Rust `fd`, Java `gson`), the wiki gives a real map: detected ecosystem,
  per-area pages with evidence-backed roles, candidate entrypoints, and
  manifest-derived commands. This is the central payoff — the map degrades
  gracefully on unsupported languages instead of going blank.
- **Build-system coverage broadened (M5.7).** `.NET` (`.csproj`/`.sln`/
  `global.json`), Java/Maven (`pom.xml`), and JVM/Gradle (`build.gradle`,
  `gradlew`) are now detected as ecosystems, listed in config, and produce
  command hints (`dotnet`, `mvn`, `gradle`/`./gradlew`). `gson` went from "no
  ecosystem detected" to "Java (Maven)" with `mvn test`/`mvn package`.
- **Rust entrypoint conventions (M5.7).** `fd` now lists `src/main.rs` as a
  candidate Rust binary entrypoint with Cargo.toml evidence.
- **Search ranking polish (M5.7).** Import-like entries are penalized in
  unfiltered search and the entrypoints/commands/config wiki pages are boosted;
  `vitesse`'s `vite` search leads with `wiki/entrypoints.md`, and definitions
  lead over imports elsewhere. No import-like entry leaked into `code_item` in
  any repo.
- **Friendlier roles / tighter frontend (M5.7).** Config/doc directories read as
  "configuration"/"documentation" instead of "likely JSON code"; `vitesse` is
  correctly "frontend" (Vue + `src/main.ts` + Vite config), whereas a library
  that merely uses Vite as a devDependency is not.

## What improved after CommonJS hints (M5.8)

- **`expressjs/express`: 0 → 447 code items** (385 `js_import` require hints +
  62 `js_export` hints). The map went from "no symbols at all" to a usable
  best-effort code surface for a hugely common JS style.
- `require(...)` bindings index as import-like, so they stay searchable under
  `--kind import` and do **not** pollute `--kind code_item`.
- `module.exports = X`, `exports.foo = X`, object-literal exports, and the
  `exports = module.exports = createApplication` chain are all picked up:
  `search "router" --kind code_item` now returns `lib/express.js` and
  `createApplication` is found there.

## Remaining gaps

1. **File search is metadata-only.** File documents index `path`, `language`,
   `kind`, and `top_level` — not file contents. In repos whose language has no
   code items (Rust, Java, C/C++), a term that appears only inside file bodies
   won't match; search falls back to path/wiki/repo_map hits (e.g. `fd`'s
   `search "search"` returned only the repo map). This is the single most
   impactful gap surfaced by the matrix.
2. **No symbols for Rust, Java, C/C++.** These rely entirely on wiki-first
   orientation. Acceptable today, but it is why gap #1 hurts most there.
3. **No `.vue`/`.svelte` `<script>` symbols.** `vitesse`'s code items come from
   `.ts` files; `.vue` components contribute to frontend detection and the file
   inventory but yield no symbols.
4. **Java `static void main` entrypoints not detected.** Conventional-path
   detection is Go/JS/TS/Rust/.NET-oriented; a Java app's main class would be
   missed (libraries like `gson` legitimately have none).
5. **camelCase tokenization.** The FTS `unicode61` tokenizer matches whole
   lowercased tokens, so `JsonConvert` does not match `json`+`convert`. Affects
   all languages equally.

## Recommended next steps

1. **Proceed toward broader dogfood / release work, not more language depth.**
   The map is useful across all six ecosystems; the highest-leverage work now is
   exercising the full contract→prompt→gate flow on real tasks and tightening
   packaging/docs, not adding grammars.
2. **If one map improvement is picked next, consider indexing file contents for
   lexical search** (bounded by size, still deterministic, no new grammars).
   This directly addresses gap #1 and helps every unsupported language at once —
   a better return than any single-language symbol extractor.
3. **Treat search ranking as good enough.** No further tuning is warranted from
   this matrix.

## Decision focus (answers)

- **Should Java/Rust symbols still be deferred?** Yes. Wiki-first orientation
  already covers navigation for `fd` and `gson`; two more full grammars are high
  cost for low marginal value. Indexing file contents would help these repos
  more broadly and more cheaply.
- **Is Vue `<script>` extraction worth doing next?** No. Vue repos already get
  frontend detection, `.vue` file inventory, `.ts` symbols, and entrypoints; the
  incremental value of `.vue` symbols is small.
- **Is search ranking good enough?** Yes — definitions lead, imports are
  de-emphasized and never leak into `code_item`, and wiki pages surface for
  orientation queries.
- **Is the map useful enough to proceed toward broader dogfood/release work?**
  Yes.

## Deferred items

- Symbol extraction for Rust, Java, C/C++ (would require new grammars).
- `.vue`/`.svelte` `<script>` extraction.
- File-content lexical indexing (candidate next map improvement, not done here).
- Java `main`-class entrypoint detection.
- LSP / SCIP / call graph / references and embeddings / vector search — out of
  scope by design; the map is a deterministic navigation aid, not semantic
  truth.
