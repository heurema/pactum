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

func TestSearchSymbolFlagReturnsUnsupportedError(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "internal", "contracts", "runner.go"), `package contracts

type Runner struct{}
`)

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code := testApp(root).Run([]string{"search", "--symbol", "Runner"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("search --symbol must exit non-zero; stdout:\n%s", stdout.String())
	}
	got := stderr.String()
	if !strings.Contains(got, "no longer supported") {
		t.Fatalf("--symbol error must say 'no longer supported':\n%s", got)
	}
}

func TestSearchWithoutQueryOrSymbolFails(t *testing.T) {
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
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("expected a usage error:\n%s", stderr.String())
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

func TestMapCodeIndexDeprecated(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	mustWriteFile(t, configPath, `version: v1alpha1
agents:
  - name: claude
    model: claude-opus-4-8
map:
  code_index: auto
`)
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("readConfig returned error for legacy code_index: %v", err)
	}
	found := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "deprecated") && strings.Contains(w, "code_index") {
			found = true
		}
	}
	if !found {
		t.Fatalf("readConfig should warn about deprecated map.code_index; warnings: %v", cfg.Warnings)
	}
}

func TestMapCodeIndexDeprecatedEmptyDefault(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	mustWriteFile(t, configPath, `version: v1alpha1
agents:
  - name: claude
    model: claude-opus-4-8
`)
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("readConfig returned error: %v", err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("readConfig should not warn without code_index; warnings: %v", cfg.Warnings)
	}
}
