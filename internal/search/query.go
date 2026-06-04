package search

import (
	"database/sql"
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

	match := ftsQuery(options.Query)
	if match == "" {
		return response, nil
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return response, err
	}
	defer db.Close()

	// Fetch a larger candidate pool ordered by raw bm25 so the deterministic
	// re-rank below has material to reorder; without this, a strong import
	// match could crowd a weaker definition out of the top `limit` before the
	// penalty is ever applied.
	pool := limit * 5
	if pool < 50 {
		pool = 50
	}

	rows, err := db.Query(`
SELECT
	d.id,
	d.kind,
	d.path,
	d.title,
	COALESCE(d.language, ''),
	COALESCE(d.code_kind, ''),
	-- SQLite FTS5 bm25 uses lower scores for better matches; values are often negative.
	bm25(documents_fts) AS score,
	snippet(documents_fts, 1, '', '', '...', 16) AS snippet
FROM documents_fts
JOIN documents d ON documents_fts.rowid = d.rowid
WHERE documents_fts MATCH ? AND (? = 'any' OR d.kind = ?)
ORDER BY score ASC, d.kind ASC, d.path ASC, d.title ASC
LIMIT ?`, match, kind, kind, pool)
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
		if err := rows.Scan(
			&result.ID,
			&result.Kind,
			&result.Path,
			&result.Title,
			&result.Language,
			&result.CodeKind,
			&result.Score,
			&result.Snippet,
		); err != nil {
			return response, err
		}
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
