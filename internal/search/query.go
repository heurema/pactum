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
// they reorder near-equal matches without overriding clearly stronger lexical
// matches. This is a light polish, not a relevance model — keep it simple.
const (
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

	if err := ensureSchemaCurrent(db, dbPath); err != nil {
		return response, err
	}

	// Fetch a larger candidate pool ordered by raw bm25 so the deterministic
	// re-rank below has material to reorder.
	pool := limit * 5
	if pool < 50 {
		pool = 50
	}

	where := "documents_fts MATCH ? AND (? = 'any' OR d.kind = ?)"
	queryArgs := []any{match, kind, kind, pool}

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

// rankAdjustment returns the deterministic delta added to a result's bm25 score
// before sorting. Negative values improve rank (boost); positive values worsen
// it (penalty). kindFilter is the normalized --kind filter.
func rankAdjustment(query string, kindFilter string, result Result) float64 {
	var adjustment float64
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
