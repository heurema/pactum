# Memory Candidate

## Run
- Run id: run_20260612_070427
- Source: deterministic

## Contract
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

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - internal/search/query.go
  - internal/search/symbol_test.go
- New files: none
- Missing files: none

## Clarifications
- q_001: For `pactum search --symbol <name>`, should "symbol name" mean only `codeindex.Item.Name` / `search.Result.Title`, or should it also match qualified forms such as `Parent.Name`, package-qualified names, `Signature`, or `CodeKind`?
  Answer: `--symbol <name>` matches only `codeindex.Item.Name` as exposed in `search.Result.Title`, using exact case-insensitive comparison. It does not match `Parent`, `Package`, `Signature`, `CodeKind`, or synthesized qualified names.
- q_002: Should `--symbol` be usable as a standalone lookup (`pactum search --symbol Runner`) even though the current CLI requires a positional query, or only as a filter on an existing query (`pactum search Runner --symbol Runner`)?
  Answer: Make the positional query optional when `--symbol` is provided. With only `--symbol`, return matching `code_item` symbols directly. When both query and `--symbol` are present, keep the lexical query and then restrict results to the exact symbol match.
- q_003: What should happen for an incompatible kind filter such as `pactum search Runner --symbol Runner --kind file` or `--kind import`?
  Answer: Allow `--symbol` only with omitted/default `--kind any` or explicit `--kind code_item`. Reject other `--kind` values with a clear usage error explaining that `--symbol` only applies to `code_item` results.
- q_004: If multiple files or parents define the same symbol name, for example two `Runner` code items in different packages, should `--symbol Runner` return all exact matches or treat the duplicate as ambiguous?
  Answer: Return all exact case-insensitive `code_item` matches, ordered by the existing deterministic ranking and tie-breakers, capped by `--limit`. Do not report duplicates as an error; users can raise `--limit` if needed.
- q_005: For `pactum search --json`, should `start_line`, `end_line`, and `signature` be omitted from non-`code_item` results to keep their JSON shape unchanged, or included with zero/empty values because `search.Result` now has those fields?
  Answer: In JSON output, include `start_line`, `end_line`, and `signature` only for `kind=code_item` hits with valid symbol metadata. Omit those fields for `repo_map`, `llms`, `wiki`, `file`, and `import` results so non-code_item JSON results remain unchanged.
- q_006: If a `code_item` hit has incomplete range metadata, for example `StartLine=5` and `EndLine=0` in a fixture or an incompatible `search.sqlite` built before the new stored columns, what should search and executor-context rendering do?
  Answer: Treat an incompatible/legacy search index schema as stale and tell the user to run `pactum map refresh`. For an individual `code_item` row with missing or invalid range data, still return the result but omit the ranged address and absent signature; never render `path:0-0` or a dangling separator.
- q_007: If two distinct `code_item` hits have the same `path`, `title`, and `code_kind` but different parents or ranges, for example `Cache.Start` and `Worker.Start` methods in the same file, should executor-context search results keep both ranged addresses or dedupe them as one result?
  Answer: Treat distinct `code_item` document IDs or distinct valid `start_line`/`end_line` ranges as distinct results in run-context search and executor-context rendering. Dedupe only the same underlying result resurfaced by multiple targeted queries.

## Review Decisions
- f_001 [medium] resolved internal/search/query.go:151: --symbol combined with a lexical query filters exact symbol matches in Go AFTER the SQL candidate pool LIMIT (pool = max(limit*5, 50), ordered by raw bm25). Exact-name code_item hits that match the lexical query but rank below the pool cutoff are silently dropped and cannot be recovered by the re-rank. This violates the acceptance criterion that --symbol returns all matching results subject to --limit, and is inconsistent with the standalone --symbol path (querySymbol), which filters by title in SQL and finds every match. Fix: add the lower(title)=lower(?) predicate to the SQL WHERE clause when symbol is set so the pool only contains symbol matches.
  Resolution: Added `lower(d.title) = lower(?)` to the FTS WHERE clause in query.go when symbol is set, so the bm25 candidate pool contains only exact symbol matches before the LIMIT — no exact match can fall below the pool cutoff. Removed the now-redundant Go post-filter (strings.EqualFold) at the former query.go:151. Mirrors querySymbol's SQL predicate so query+symbol and bare-symbol paths agree.
- f_002 [low] resolved docs/flow.md:63: docs/flow.md's pactum search section (the prose doc enumerating search flags and ranking semantics) was not updated for this change: no mention of --symbol, the now-optional positional query, or the new path:start-end ranged addresses. The contract goal asks for the matching rule to be documented; it is documented only in CLI help text and executor guidance, leaving flow.md under-describing the command. Advisory doc drift, not an incorrect statement.
- f_003 [low] resolved internal/app/run.go:445: The new searchpkg.IsStaleIndex(err) branch in buildRunSearchResults (the upgrade scenario: map manifest ready but search.sqlite predates the v2 schema, surfacing ErrStaleIndex from runContextSearch during task new) has no test. TestSearchStaleSchemaPrintsGuidance covers only the App.Search CLI path, and TestRunContractOnlySearchResultsWarnWhenIndexMissing covers only the mapStatus.Status==stale early return at run.go:428. Reusing the existing DROP TABLE meta setup with `task new` would pin this branch.
- f_004 [low] resolved internal/search/query.go:215: The contract clause 'Return all duplicate exact symbol-name matches ... capped by --limit' is untested on both symbol paths: the SQL LIMIT in querySymbol (bare --symbol) and the post-filter truncation (query + --symbol). Duplicate-name tests (TestSymbolLookupReturnsExactCaseInsensitiveMatches, TestSymbolLookupReturnsDuplicateNamesWithDistinctRanges) use the default limit of 10 against 2 fixtures, so the cap is never exercised. A QueryOptions{Symbol: "Runner", Limit: 1} case would pin the cap and which duplicate wins under the deterministic order.
- f_005 [low] resolved internal/search/query.go:196: ensureSchemaCurrent maps every error from the meta lookup to staleIndexError, not just the missing-table/missing-row cases it is designed for. Corruption, permission errors, and SQLITE_BUSY all surface to users as 'Search index is stale. Run: pactum map refresh' with exit code 0 (app.go:242-245, run.go:445-448), discarding the real error. The adjacent missing-index check in Query (query.go:43-48) distinguishes os.IsNotExist from other Stat errors and propagates the latter; the stale check should similarly treat only sql.ErrNoRows and the missing-meta-table error as stale and propagate the rest.
- f_006 [medium] resolved CHANGELOG.md:9: User-visible search changes (--symbol flag, optional positional query, ranged path:start-end human output, new start_line/end_line/signature JSON fields) have no CHANGELOG.md Unreleased entry, although the changelog records exactly this class of change and every recent feature milestone PR updated it.
- f_007 [medium] resolved docs/flow.md:63: docs/flow.md's pactum search section (the prose doc that enumerates search flags) was not updated: it still shows the grammar as `pactum search "<query>"` with only --kind filtering, omitting --symbol, the now-optional positional query, and the ranged path:start-end + signature output for code_item hits.
- f_008 [low] resolved assets/agent-skills/pactum/SKILL.md:74: The committed agent skill package still teaches only the pre-slice search workflow: SKILL.md and references/workflow.md enumerate --kind filters and query styles but never mention --symbol or ranged first reads, although skill-driven external agents are the primary audience for symbol-addressed navigation and are not reached by the updated executor-context guidance in internal/app/prompt.go.
- f_009 [medium] resolved internal/search/query.go:96: `--symbol` filtering after a capped FTS candidate pool can drop valid exact symbol matches before they are considered.
  Resolution: Same root cause as f_001 (this finding points at the pool cap, query.go:96). The symbol predicate now lives in SQL so the capped pool (max(limit*5,50)) only ever holds exact symbol-name matches; valid matches can no longer be dropped before consideration. One fix addresses both locations.
- f_010 [medium] resolved internal/search/query.go:250: Invalid-range code_item results can still emit `signature` in JSON output.
  Resolution: hydrateSymbolMetadata in query.go set result.Signature unconditionally, so invalid-range code_item hits emitted `signature` in --json. Moved the signature assignment inside the validRange guard so start_line, end_line, and signature are populated together only for valid ranges, satisfying the 'only for code_item hits with valid symbol metadata' criterion. Updated TestInvalidRangeMetadataOmittedButResultReturned, which had asserted the old signature-retained behavior, to assert omission.
- f_011 [medium] resolved internal/search/symbol_test.go:202: Invalid-range metadata coverage asserts that signature is retained and does not test that --json omits symbol metadata for invalid code_item ranges.
- f_012 [medium] resolved internal/search/query.go:196: Stale-index detection treats any metadata read error as a stale schema, hiding unrelated SQLite failures behind refresh guidance.
- f_013 [low] resolved docs/flow.md:63: docs/flow.md still documents pactum search as query-only and only describes --kind; it does not document the new pactum search --symbol <name> lookup, optional positional query, or exact case-insensitive symbol matching.
- Proposal summary: pending=0 accepted=13 rejected=0

## Reusable Project Knowledge
- scope: in scope: Extend search indexing and result hydration so kind=code_item results can carry StartLine, EndLine, and Signature from codeindex.Item as start_line, end_line, and signature.
- scope: in scope: Update pactum search human output to render valid code_item ranges as path:start-end and include the signature when present.
- scope: in scope: Update pactum search --json so valid code_item symbol metadata is emitted as start_line, end_line, and signature while non-code_item result JSON remains unchanged.
- scope: in scope: Add pactum search --symbol <name> with exact case-insensitive matching against codeindex.Item.Name as exposed by search.Result.Title.
- scope: in scope: Allow --symbol without a positional query; when both query and --symbol are provided, apply the lexical query first and then restrict to exact symbol-name matches.
- scope: in scope: Reject --symbol combined with any --kind value other than omitted/default any or explicit code_item, with a clear usage error.
- scope: in scope: Return all duplicate exact symbol-name matches, ordered by existing deterministic ranking and tie-breakers and capped by --limit.
- scope: in scope: Update executor context search-result rendering so code_item hits with valid range metadata render as path:start-end and include the signature when present.
- scope: in scope: Update executor-context retrieval guidance to tell agents to read symbol ranges directly when ranged code_item results are available.
- scope: in scope: Update run-context search deduplication so distinct code_item document IDs or distinct valid start_line/end_line ranges are preserved.
- scope: in scope: Handle legacy or incompatible search index schemas as stale and instruct the user to run pactum map refresh.
- scope: in scope: Bump internal search index schema or stored-shape markers if the FTS5 stored shape changes.
- scope: in scope: Add focused tests for search result symbol metadata, --symbol behavior, executor-context rendering, stale schema handling, invalid metadata omission, and unchanged non-code_item output.
- scope: out of scope: Do not change the contract goal.
- scope: out of scope: Do not add embeddings, vector search, model-based context compression, or an LSP runtime dependency.
- scope: out of scope: Do not implement pactum outline or contract-scoped skeleton rendering from later slices of docs/agent-file-navigation-design.md.
- scope: out of scope: Do not change codeindex.Item extraction semantics except as needed to consume existing StartLine, EndLine, and Signature fields.
- scope: out of scope: Do not include start_line, end_line, or signature fields in JSON results for repo_map, llms, wiki, file, or import hits.
- scope: out of scope: Do not match --symbol against Parent, Package, Signature, CodeKind, import path, or synthesized qualified names.
- scope: out of scope: Do not treat duplicate symbol names as an ambiguity error.
- clarification: q_001: For `pactum search --symbol <name>`, should "symbol name" mean only `codeindex.Item.Name` / `search.Result.Title`, or should it also match qualified forms such as `Parent.Name`, package-qualified names, `Signature`, or `CodeKind`? Answer: `--symbol <name>` matches only `codeindex.Item.Name` as exposed in `search.Result.Title`, using exact case-insensitive comparison. It does not match `Parent`, `Package`, `Signature`, `CodeKind`, or synthesized qualified names.
- clarification: q_002: Should `--symbol` be usable as a standalone lookup (`pactum search --symbol Runner`) even though the current CLI requires a positional query, or only as a filter on an existing query (`pactum search Runner --symbol Runner`)? Answer: Make the positional query optional when `--symbol` is provided. With only `--symbol`, return matching `code_item` symbols directly. When both query and `--symbol` are present, keep the lexical query and then restrict results to the exact symbol match.
- clarification: q_003: What should happen for an incompatible kind filter such as `pactum search Runner --symbol Runner --kind file` or `--kind import`? Answer: Allow `--symbol` only with omitted/default `--kind any` or explicit `--kind code_item`. Reject other `--kind` values with a clear usage error explaining that `--symbol` only applies to `code_item` results.
- clarification: q_004: If multiple files or parents define the same symbol name, for example two `Runner` code items in different packages, should `--symbol Runner` return all exact matches or treat the duplicate as ambiguous? Answer: Return all exact case-insensitive `code_item` matches, ordered by the existing deterministic ranking and tie-breakers, capped by `--limit`. Do not report duplicates as an error; users can raise `--limit` if needed.
- clarification: q_005: For `pactum search --json`, should `start_line`, `end_line`, and `signature` be omitted from non-`code_item` results to keep their JSON shape unchanged, or included with zero/empty values because `search.Result` now has those fields? Answer: In JSON output, include `start_line`, `end_line`, and `signature` only for `kind=code_item` hits with valid symbol metadata. Omit those fields for `repo_map`, `llms`, `wiki`, `file`, and `import` results so non-code_item JSON results remain unchanged.
- clarification: q_006: If a `code_item` hit has incomplete range metadata, for example `StartLine=5` and `EndLine=0` in a fixture or an incompatible `search.sqlite` built before the new stored columns, what should search and executor-context rendering do? Answer: Treat an incompatible/legacy search index schema as stale and tell the user to run `pactum map refresh`. For an individual `code_item` row with missing or invalid range data, still return the result but omit the ranged address and absent signature; never render `path:0-0` or a dangling separator.
- clarification: q_007: If two distinct `code_item` hits have the same `path`, `title`, and `code_kind` but different parents or ranges, for example `Cache.Start` and `Worker.Start` methods in the same file, should executor-context search results keep both ranged addresses or dedupe them as one result? Answer: Treat distinct `code_item` document IDs or distinct valid `start_line`/`end_line` ranges as distinct results in run-context search and executor-context rendering. Dedupe only the same underlying result resurfaced by multiple targeted queries.
- review_resolution: f_001 resolved: --symbol combined with a lexical query filters exact symbol matches in Go AFTER the SQL candidate pool LIMIT (pool = max(limit*5, 50), ordered by raw bm25). Exact-name code_item hits that match the lexical query but rank below the pool cutoff are silently dropped and cannot be recovered by the re-rank. This violates the acceptance criterion that --symbol returns all matching results subject to --limit, and is inconsistent with the standalone --symbol path (querySymbol), which filters by title in SQL and finds every match. Fix: add the lower(title)=lower(?) predicate to the SQL WHERE clause when symbol is set so the pool only contains symbol matches.; resolution: Added `lower(d.title) = lower(?)` to the FTS WHERE clause in query.go when symbol is set, so the bm25 candidate pool contains only exact symbol matches before the LIMIT — no exact match can fall below the pool cutoff. Removed the now-redundant Go post-filter (strings.EqualFold) at the former query.go:151. Mirrors querySymbol's SQL predicate so query+symbol and bare-symbol paths agree.
- review_resolution: f_002 resolved: docs/flow.md's pactum search section (the prose doc enumerating search flags and ranking semantics) was not updated for this change: no mention of --symbol, the now-optional positional query, or the new path:start-end ranged addresses. The contract goal asks for the matching rule to be documented; it is documented only in CLI help text and executor guidance, leaving flow.md under-describing the command. Advisory doc drift, not an incorrect statement.
- review_resolution: f_003 resolved: The new searchpkg.IsStaleIndex(err) branch in buildRunSearchResults (the upgrade scenario: map manifest ready but search.sqlite predates the v2 schema, surfacing ErrStaleIndex from runContextSearch during task new) has no test. TestSearchStaleSchemaPrintsGuidance covers only the App.Search CLI path, and TestRunContractOnlySearchResultsWarnWhenIndexMissing covers only the mapStatus.Status==stale early return at run.go:428. Reusing the existing DROP TABLE meta setup with `task new` would pin this branch.
- review_resolution: f_004 resolved: The contract clause 'Return all duplicate exact symbol-name matches ... capped by --limit' is untested on both symbol paths: the SQL LIMIT in querySymbol (bare --symbol) and the post-filter truncation (query + --symbol). Duplicate-name tests (TestSymbolLookupReturnsExactCaseInsensitiveMatches, TestSymbolLookupReturnsDuplicateNamesWithDistinctRanges) use the default limit of 10 against 2 fixtures, so the cap is never exercised. A QueryOptions{Symbol: "Runner", Limit: 1} case would pin the cap and which duplicate wins under the deterministic order.
- review_resolution: f_005 resolved: ensureSchemaCurrent maps every error from the meta lookup to staleIndexError, not just the missing-table/missing-row cases it is designed for. Corruption, permission errors, and SQLITE_BUSY all surface to users as 'Search index is stale. Run: pactum map refresh' with exit code 0 (app.go:242-245, run.go:445-448), discarding the real error. The adjacent missing-index check in Query (query.go:43-48) distinguishes os.IsNotExist from other Stat errors and propagates the latter; the stale check should similarly treat only sql.ErrNoRows and the missing-meta-table error as stale and propagate the rest.
- review_resolution: f_006 resolved: User-visible search changes (--symbol flag, optional positional query, ranged path:start-end human output, new start_line/end_line/signature JSON fields) have no CHANGELOG.md Unreleased entry, although the changelog records exactly this class of change and every recent feature milestone PR updated it.
- review_resolution: f_007 resolved: docs/flow.md's pactum search section (the prose doc that enumerates search flags) was not updated: it still shows the grammar as `pactum search "<query>"` with only --kind filtering, omitting --symbol, the now-optional positional query, and the ranged path:start-end + signature output for code_item hits.
- review_resolution: f_008 resolved: The committed agent skill package still teaches only the pre-slice search workflow: SKILL.md and references/workflow.md enumerate --kind filters and query styles but never mention --symbol or ranged first reads, although skill-driven external agents are the primary audience for symbol-addressed navigation and are not reached by the updated executor-context guidance in internal/app/prompt.go.
- review_resolution: f_009 resolved: `--symbol` filtering after a capped FTS candidate pool can drop valid exact symbol matches before they are considered.; resolution: Same root cause as f_001 (this finding points at the pool cap, query.go:96). The symbol predicate now lives in SQL so the capped pool (max(limit*5,50)) only ever holds exact symbol-name matches; valid matches can no longer be dropped before consideration. One fix addresses both locations.
- review_resolution: f_010 resolved: Invalid-range code_item results can still emit `signature` in JSON output.; resolution: hydrateSymbolMetadata in query.go set result.Signature unconditionally, so invalid-range code_item hits emitted `signature` in --json. Moved the signature assignment inside the validRange guard so start_line, end_line, and signature are populated together only for valid ranges, satisfying the 'only for code_item hits with valid symbol metadata' criterion. Updated TestInvalidRangeMetadataOmittedButResultReturned, which had asserted the old signature-retained behavior, to assert omission.
- review_resolution: f_011 resolved: Invalid-range metadata coverage asserts that signature is retained and does not test that --json omits symbol metadata for invalid code_item ranges.
- review_resolution: f_012 resolved: Stale-index detection treats any metadata read error as a stale schema, hiding unrelated SQLite failures behind refresh guidance.
- review_resolution: f_013 resolved: docs/flow.md still documents pactum search as query-only and only describes --kind; it does not document the new pactum search --symbol <name> lookup, optional positional query, or exact case-insensitive symbol matching.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- review_resolution: proposal p_007 accepted as f_007
- review_resolution: proposal p_008 accepted as f_008
- review_resolution: proposal p_009 accepted as f_009
- review_resolution: proposal p_010 accepted as f_010
- review_resolution: proposal p_011 accepted as f_011
- review_resolution: proposal p_012 accepted as f_012
- review_resolution: proposal p_013 accepted as f_013
- validation: go test ./internal/search passed
- validation: go test ./internal/app passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
