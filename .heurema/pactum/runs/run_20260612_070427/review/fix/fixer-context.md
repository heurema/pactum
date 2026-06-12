# Review Fixer Context

## Run
- Run id: run_20260612_070427
- Run status: contract_approved

## Approved contract
- Goal: Slice 1 of the agent file-navigation arc (design reference: docs/agent-file-navigation-design.md). Make search results symbol-addressable so an agent's first read of a large file is a ranged read instead of a grep-window-rewindow loop. (1) Plumb the per-symbol data the code-items index already stores — StartLine, EndLine, and Signature (see codeindex.Item) — through the search layer into code_item results: extend the FTS5 document metadata or result hydration so search.Result carries start_line, end_line, and signature for kind=code_item hits (other kinds leave them empty), expose them in pactum search human output as path:start-end and the signature, and in search --json. (2) Add a --symbol <name> filter to pactum search that restricts results to code_item hits whose symbol name matches (exact match preferred, case-insensitive; document the matching rule), so an agent can resolve a known identifier straight to its address. (3) Render the same addresses in the executor context: renderExecutorContext search-result lines gain the line range and signature for code_item hits (path:start-end — signature), and the prompt guidance text tells the agent to read symbol ranges directly instead of scanning whole files. (4) The map/search index rebuild stays deterministic; index schema or stored-shape changes must keep pactum map refresh reproducible — same tree in, same index out; bump internal schema markers if the stored shape changes rather than silently rereading stale rows. Tests pin: a code_item search result carries the correct range and signature; --symbol returns exactly the matching symbols; executor-context rendering shows ranged addresses; non-code_item results are unchanged.
- In scope:
  - Extend search indexing and result hydration so kind=code_item results can carry StartLine, EndLine, and Signature from codeindex.Item as start_line, end_line, and signature.
  - Update pactum search human output to render valid code_item ranges as path:start-end and include the signature when present.
  - Update pactum search --json so valid code_item symbol metadata is emitted as start_line, end_line, and signature while non-code_item result JSON remains unchanged.
  - Add pactum search --symbol <name> with exact case-insensitive matching against codeindex.Item.Name as exposed by search.Result.Title.
  - Allow --symbol without a positional query; when both query and --symbol are provided, apply the lexical query first and then restrict to exact symbol-name matches.
  - Reject --symbol combined with any --kind value other than omitted/default any or explicit code_item, with a clear usage error.
  - Return all duplicate exact symbol-name matches, ordered by existing deterministic ranking and tie-breakers and capped by --limit.
  - Update executor context search-result rendering so code_item hits with valid range metadata render as path:start-end and include the signature when present.
  - Update executor-context retrieval guidance to tell agents to read symbol ranges directly when ranged code_item results are available.
  - Update run-context search deduplication so distinct code_item document IDs or distinct valid start_line/end_line ranges are preserved.
  - Handle legacy or incompatible search index schemas as stale and instruct the user to run pactum map refresh.
  - Bump internal search index schema or stored-shape markers if the FTS5 stored shape changes.
  - Add focused tests for search result symbol metadata, --symbol behavior, executor-context rendering, stale schema handling, invalid metadata omission, and unchanged non-code_item output.
- Out of scope:
  - Do not change the contract goal.
  - Do not add embeddings, vector search, model-based context compression, or an LSP runtime dependency.
  - Do not implement pactum outline or contract-scoped skeleton rendering from later slices of docs/agent-file-navigation-design.md.
  - Do not change codeindex.Item extraction semantics except as needed to consume existing StartLine, EndLine, and Signature fields.
  - Do not include start_line, end_line, or signature fields in JSON results for repo_map, llms, wiki, file, or import hits.
  - Do not match --symbol against Parent, Package, Signature, CodeKind, import path, or synthesized qualified names.
  - Do not treat duplicate symbol names as an ambiguity error.
- Acceptance criteria:
  - A code_item search result created from a codeindex.Item with StartLine=3, EndLine=3, and Signature="type Runner struct" carries start_line=3, end_line=3, and signature="type Runner struct".
  - Human pactum search output for a code_item hit with valid range metadata shows the address as path:start-end and includes the signature.
  - pactum search --json includes start_line, end_line, and signature only for code_item hits with valid symbol metadata.
  - JSON output for repo_map, llms, wiki, file, and import hits does not gain start_line, end_line, or signature fields.
  - pactum search --symbol Runner works without a positional query and returns exact case-insensitive code_item title/name matches.
  - pactum search runner --symbol Runner applies the lexical query and then restricts results to exact case-insensitive symbol-name matches.
  - --symbol does not match Parent, Package, Signature, CodeKind, import path, or qualified names when the plain symbol name differs.
  - When multiple code_item results share the same exact symbol name, --symbol returns all matching results in deterministic result order subject to --limit.
  - pactum search --symbol Runner --kind file and other non-code_item kind filters fail with a clear usage error explaining that --symbol only applies to code_item results.
  - Executor context renders code_item search results with valid metadata as path:start-end and includes the signature when present.
  - Executor context preserves distinct code_item hits with different document IDs or different valid start_line/end_line ranges, even when path, title, and code_kind match.
  - Code_item hits with missing or invalid range metadata are still returned but do not render path:0-0 or dangling separators.
  - An incompatible or legacy search index schema is treated as stale and produces guidance to run pactum map refresh instead of silently reading stale rows.
  - Given the same tree and generated_at input, rebuilding the search index produces deterministic documents and result metadata.
- Validation commands:
  - go test ./internal/search
  - go test ./internal/app
  - make check

## Current review findings
- Summary: findings=13 open=13 resolved=0 blocking_open=3
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: --symbol combined with a lexical query filters exact symbol matches in Go AFTER the SQL candidate pool LIMIT (pool = max(limit*5, 50), ordered by raw bm25). Exact-name code_item hits that match the lexical query but rank below the pool cutoff are silently dropped and cannot be recovered by the re-rank. This violates the acceptance criterion that --symbol returns all matching results subject to --limit, and is inconsistent with the standalone --symbol path (querySymbol), which filters by title in SQL and finds every match. Fix: add the lower(title)=lower(?) predicate to the SQL WHERE clause when symbol is set so the pool only contains symbol matches.
    location: internal/search/query.go:151
  - f_009 severity=medium category=correctness blocking=true status=open: `--symbol` filtering after a capped FTS candidate pool can drop valid exact symbol matches before they are considered.
    location: internal/search/query.go:96
  - f_010 severity=medium category=correctness blocking=true status=open: Invalid-range code_item results can still emit `signature` in JSON output.
    location: internal/search/query.go:250
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_002 severity=low category=quality blocking=false status=open: docs/flow.md's pactum search section (the prose doc enumerating search flags and ranking semantics) was not updated for this change: no mention of --symbol, the now-optional positional query, or the new path:start-end ranged addresses. The contract goal asks for the matching rule to be documented; it is documented only in CLI help text and executor guidance, leaving flow.md under-describing the command. Advisory doc drift, not an incorrect statement.
    location: docs/flow.md:63
  - f_003 severity=low category=quality blocking=false status=open: The new searchpkg.IsStaleIndex(err) branch in buildRunSearchResults (the upgrade scenario: map manifest ready but search.sqlite predates the v2 schema, surfacing ErrStaleIndex from runContextSearch during task new) has no test. TestSearchStaleSchemaPrintsGuidance covers only the App.Search CLI path, and TestRunContractOnlySearchResultsWarnWhenIndexMissing covers only the mapStatus.Status==stale early return at run.go:428. Reusing the existing DROP TABLE meta setup with `task new` would pin this branch.
    location: internal/app/run.go:445
  - f_004 severity=low category=quality blocking=false status=open: The contract clause 'Return all duplicate exact symbol-name matches ... capped by --limit' is untested on both symbol paths: the SQL LIMIT in querySymbol (bare --symbol) and the post-filter truncation (query + --symbol). Duplicate-name tests (TestSymbolLookupReturnsExactCaseInsensitiveMatches, TestSymbolLookupReturnsDuplicateNamesWithDistinctRanges) use the default limit of 10 against 2 fixtures, so the cap is never exercised. A QueryOptions{Symbol: "Runner", Limit: 1} case would pin the cap and which duplicate wins under the deterministic order.
    location: internal/search/query.go:215
  - f_005 severity=low category=quality blocking=false status=open: ensureSchemaCurrent maps every error from the meta lookup to staleIndexError, not just the missing-table/missing-row cases it is designed for. Corruption, permission errors, and SQLITE_BUSY all surface to users as 'Search index is stale. Run: pactum map refresh' with exit code 0 (app.go:242-245, run.go:445-448), discarding the real error. The adjacent missing-index check in Query (query.go:43-48) distinguishes os.IsNotExist from other Stat errors and propagates the latter; the stale check should similarly treat only sql.ErrNoRows and the missing-meta-table error as stale and propagate the rest.
    location: internal/search/query.go:196
  - f_006 severity=medium category=quality blocking=false status=open: User-visible search changes (--symbol flag, optional positional query, ranged path:start-end human output, new start_line/end_line/signature JSON fields) have no CHANGELOG.md Unreleased entry, although the changelog records exactly this class of change and every recent feature milestone PR updated it.
    location: CHANGELOG.md:9
  - f_007 severity=medium category=quality blocking=false status=open: docs/flow.md's pactum search section (the prose doc that enumerates search flags) was not updated: it still shows the grammar as `pactum search "<query>"` with only --kind filtering, omitting --symbol, the now-optional positional query, and the ranged path:start-end + signature output for code_item hits.
    location: docs/flow.md:63
  - f_008 severity=low category=quality blocking=false status=open: The committed agent skill package still teaches only the pre-slice search workflow: SKILL.md and references/workflow.md enumerate --kind filters and query styles but never mention --symbol or ranged first reads, although skill-driven external agents are the primary audience for symbol-addressed navigation and are not reached by the updated executor-context guidance in internal/app/prompt.go.
    location: assets/agent-skills/pactum/SKILL.md:74
  - f_011 severity=medium category=quality blocking=false status=open: Invalid-range metadata coverage asserts that signature is retained and does not test that --json omits symbol metadata for invalid code_item ranges.
    location: internal/search/symbol_test.go:202
  - f_012 severity=medium category=quality blocking=false status=open: Stale-index detection treats any metadata read error as a stale schema, hiding unrelated SQLite failures behind refresh guidance.
    location: internal/search/query.go:196
  - f_013 severity=low category=quality blocking=false status=open: docs/flow.md still documents pactum search as query-only and only describes --kind; it does not document the new pactum search --symbol <name> lookup, optional positional query, or exact case-insensitive symbol matching.
    location: docs/flow.md:63

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
