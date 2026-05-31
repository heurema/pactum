package search

import (
	"database/sql"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

const defaultLimit = 10

func Query(dbPath string, options QueryOptions) (Response, error) {
	response := Response{Query: options.Query}
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
LIMIT ?`, match, kind, kind, limit)
	if err != nil {
		return response, err
	}
	defer rows.Close()

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
		result.Rank = len(response.Results) + 1
		result.Snippet = strings.TrimSpace(result.Snippet)
		response.Results = append(response.Results, result)
	}
	if err := rows.Err(); err != nil {
		return response, err
	}
	return response, nil
}
