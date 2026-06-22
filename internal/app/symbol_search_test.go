package app

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"

	_ "modernc.org/sqlite"
)

func TestSearchWithoutQueryFails(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code := testApp(root).Run([]string{"search"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit for search with no query; stdout:\n%s", stdout.String())
	}
}

func TestSearchStaleSchemaPrintsGuidance(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	// Simulate a legacy index built before the schema marker existed.
	db, err := sql.Open("sqlite", artifacts.New(root).SearchSQLite)
	assertNoError(t, err)
	_, err = db.Exec(`DROP TABLE meta`)
	assertNoError(t, err)
	assertNoError(t, db.Close())

	stdout.Reset()
	stderr.Reset()
	if code := testApp(root).Run([]string{"search", "Runner"}, &stdout, &stderr); code != 0 {
		t.Fatalf("search exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Search index is stale. Run: pactum map refresh") {
		t.Fatalf("stale schema did not produce refresh guidance:\n%s", got)
	}
}
