package search

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Rebuild(dbPath string, input IndexInput) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		return err
	}
	return insertDocuments(db, Documents(input))
}

func createSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE documents (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			path TEXT NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL,
			language TEXT,
			code_kind TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE VIRTUAL TABLE documents_fts USING fts5(
			title,
			body,
			content='documents',
			content_rowid='rowid'
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("create search schema: %w", err)
		}
	}
	return nil
}

func insertDocuments(db *sql.DB, documents []Document) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	docStmt, err := tx.Prepare(`INSERT INTO documents (id, kind, path, title, body, language, code_kind, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer docStmt.Close()

	ftsStmt, err := tx.Prepare(`INSERT INTO documents_fts (rowid, title, body) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	for _, document := range documents {
		result, err := docStmt.Exec(
			document.ID,
			document.Kind,
			document.Path,
			document.Title,
			document.Body,
			document.Language,
			document.CodeKind,
			document.CreatedAt,
		)
		if err != nil {
			return err
		}
		rowID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := ftsStmt.Exec(rowID, document.Title, document.Body); err != nil {
			return err
		}
	}

	return tx.Commit()
}
