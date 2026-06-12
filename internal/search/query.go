package search

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const defaultLimit = 10

// Deterministic ranking adjustments applied on top of the raw FTS5 bm25 score.
// bm25 is negative and "lower is better", so a boost subtracts from the score
// and a penalty adds to it. The values are intentionally small and additive:
// they reorder near-equal matches (for example a definition vs. an import of
// the same token) without overriding clearly stronger lexical matches. This is
// a light polish, not a relevance model — keep it simple.
const (
	// importPenalty pushes import-like entries below definitions and wiki
	// pages in unfiltered ("any") searches. It is not applied when the caller
	// explicitly asks for --kind import.
	importPenalty = 2.0
	// wikiPageBoost lifts the most navigation-relevant wiki pages
	// (entrypoints, commands, config) for unfiltered searches.
	wikiPageBoost = 1.0
	// exactMatchBoost rewards an exact (case-insensitive) match on a result's
	// title or path basename.
	exactMatchBoost = 0.5
)

// boostedWikiPages are the wiki pages that earn wikiPageBoost.
var boostedWikiPages = map[string]bool{
	"map/wiki/entrypoints.md": true,
	"map/wiki/commands.md":    true,
	"map/wiki/config.md":      true,
}

func Query(dbPath string, options QueryOptions) (Response, error) {
	response := Response{Schema: ResponseSchema, Query: options.Query}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return response, ErrMissingIndex(dbPath)
		}
		return response, err
	}

	kind, err := NormalizeKind(options.Kind)
	if err != nil {
		return response, err
	}
	limit := options.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	symbol := strings.TrimSpace(options.Symbol)
	match := ftsQuery(options.Query)
	if match == "" && symbol == "" {
		return response, nil
	}
	// --symbol resolves a known identifier straight to its address, so it only
	// ever yields code_item hits. The CLI rejects incompatible --kind values
	// before reaching here; forcing the kind keeps the candidate pool relevant.
	if symbol != "" {
		kind = KindCodeItem
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return response, err
	}
	defer db.Close()

	if err := ensureSchemaCurrent(db, dbPath); err != nil {
		return response, err
	}

	// Symbol-only lookup needs no lexical match: resolve the identifier directly
	// against code_item titles in deterministic order.
	if match == "" {
		results, err := querySymbol(db, symbol, limit)
		if err != nil {
			return response, err
		}
		response.Results = results
		return response, nil
	}

	// Fetch a larger candidate pool ordered by raw bm25 so the deterministic
	// re-rank below has material to reorder; without this, a strong import
	// match could crowd a weaker definition out of the top `limit` before the
	// penalty is ever applied.
	pool := limit * 5
	if pool < 50 {
		pool = 50
	}

	// --symbol restricts the pool to exact (case-insensitive) symbol-name matches
	// in SQL, mirroring querySymbol. Filtering in Go after the bm25 LIMIT could
	// drop an exact match that ranks below the pool cutoff; the predicate keeps
	// every match in the pool so the cap applies to symbol hits only.
	where := "documents_fts MATCH ? AND (? = 'any' OR d.kind = ?)"
	queryArgs := []any{match, kind, kind}
	if symbol != "" {
		where += " AND lower(d.title) = lower(?)"
		queryArgs = append(queryArgs, symbol)
	}
	queryArgs = append(queryArgs, pool)

	rows, err := db.Query(`
SELECT
	d.id,
	d.kind,
	d.path,
	d.title,
	COALESCE(d.language, ''),
	COALESCE(d.code_kind, ''),
	d.start_line,
	d.end_line,
	d.signature,
	-- SQLite FTS5 bm25 uses lower scores for better matches; values are often negative.
	bm25(documents_fts) AS score,
	snippet(documents_fts, 1, '', '', '...', 16) AS snippet
FROM documents_fts
JOIN documents d ON documents_fts.rowid = d.rowid
WHERE `+where+`
ORDER BY score ASC, d.kind ASC, d.path ASC, d.title ASC
LIMIT ?`, queryArgs...)
	if err != nil {
		return response, err
	}
	defer rows.Close()

	type ranked struct {
		result   Result
		adjusted float64
	}
	var candidates []ranked
	for rows.Next() {
		var result Result
		var startLine, endLine int
		var signature string
		if err := rows.Scan(
			&result.ID,
			&result.Kind,
			&result.Path,
			&result.Title,
			&result.Language,
			&result.CodeKind,
			&startLine,
			&endLine,
			&signature,
			&result.Score,
			&result.Snippet,
		); err != nil {
			return response, err
		}
		hydrateSymbolMetadata(&result, startLine, endLine, signature)
		result.Snippet = strings.TrimSpace(result.Snippet)
		candidates = append(candidates, ranked{
			result:   result,
			adjusted: result.Score + rankAdjustment(options.Query, kind, result),
		})
	}
	if err := rows.Err(); err != nil {
		return response, err
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.adjusted != b.adjusted {
			return a.adjusted < b.adjusted
		}
		if a.result.Kind != b.result.Kind {
			return a.result.Kind < b.result.Kind
		}
		if a.result.Path != b.result.Path {
			return a.result.Path < b.result.Path
		}
		return a.result.Title < b.result.Title
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	for i, candidate := range candidates {
		result := candidate.result
		result.Rank = i + 1
		response.Results = append(response.Results, result)
	}
	return response, nil
}

// ensureSchemaCurrent treats an index whose stored shape predates the current
// schema marker as stale, so the caller can prompt for `pactum map refresh`
// instead of reading rows with the wrong columns. A legacy index has no meta
// table, which surfaces as a query error here.
func ensureSchemaCurrent(db *sql.DB, dbPath string) error {
	var version string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	switch {
	case err == nil:
		if version != indexSchemaVersion {
			return ErrStaleIndex(dbPath)
		}
		return nil
	case errors.Is(err, sql.ErrNoRows), isMissingTableError(err):
		// No meta table or no version row: an index written by an older
		// binary. Anything else (I/O, corruption, locking) is a real failure
		// that refresh guidance would only hide.
		return ErrStaleIndex(dbPath)
	default:
		return fmt.Errorf("search index meta: %w", err)
	}
}

// isMissingTableError matches SQLite's "no such table" failure shape, the one
// non-ErrNoRows error an older index legitimately produces.
func isMissingTableError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// querySymbol resolves a bare --symbol lookup (no lexical query) to its
// code_item hits, matching Result.Title exactly and case-insensitively. Results
// are ordered deterministically by path then range so duplicate symbol names
// across packages return stably.
func querySymbol(db *sql.DB, symbol string, limit int) ([]Result, error) {
	rows, err := db.Query(`
SELECT id, kind, path, title, COALESCE(language, ''), COALESCE(code_kind, ''), start_line, end_line, signature
FROM documents
WHERE kind = ? AND lower(title) = lower(?)
ORDER BY path ASC, start_line ASC, id ASC
LIMIT ?`, KindCodeItem, symbol, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var result Result
		var startLine, endLine int
		var signature string
		if err := rows.Scan(
			&result.ID,
			&result.Kind,
			&result.Path,
			&result.Title,
			&result.Language,
			&result.CodeKind,
			&startLine,
			&endLine,
			&signature,
		); err != nil {
			return nil, err
		}
		hydrateSymbolMetadata(&result, startLine, endLine, signature)
		result.Rank = len(results) + 1
		results = append(results, result)
	}
	return results, rows.Err()
}

// hydrateSymbolMetadata copies stored symbol metadata onto a result, but only
// when the range is valid. Invalid rows surface neither a "path:0-0" address nor
// a dangling signature, so --json carries start_line, end_line, and signature
// solely for code_item hits with usable metadata.
func hydrateSymbolMetadata(result *Result, startLine, endLine int, signature string) {
	if validRange(startLine, endLine) {
		result.StartLine = startLine
		result.EndLine = endLine
		result.Signature = signature
	}
}

// rankAdjustment returns the deterministic delta added to a result's bm25 score
// before sorting. Negative values improve rank (boost); positive values worsen
// it (penalty). kindFilter is the normalized --kind filter; the import penalty
// only applies to unfiltered ("any") searches so an explicit --kind import
// request is never penalized.
func rankAdjustment(query string, kindFilter string, result Result) float64 {
	var adjustment float64
	if kindFilter == KindAny && result.Kind == KindImport {
		adjustment += importPenalty
	}
	if kindFilter == KindAny && result.Kind == KindWiki && boostedWikiPages[result.Path] {
		adjustment -= wikiPageBoost
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized != "" {
		if strings.ToLower(result.Title) == normalized || strings.ToLower(path.Base(result.Path)) == normalized {
			adjustment -= exactMatchBoost
		}
	}
	return adjustment
}
