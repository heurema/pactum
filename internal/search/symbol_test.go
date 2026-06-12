package search

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/projectmap"

	_ "modernc.org/sqlite"
)

// buildSymbolTestIndex builds an index whose code items exercise the --symbol
// matching rules: duplicate names across packages, same-name methods on
// different parents, and a name (BuildRunner) that contains another symbol's
// name as a substring and in its signature.
func buildSymbolTestIndex(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 6, 12, 7, 4, 27, 0, time.UTC),
		CodeItems: []codeindex.Item{
			{Path: "internal/exec/runner.go", Kind: "go_type", Language: "go", Name: "Runner", Package: "exec", Exported: true, Signature: "type Runner struct", StartLine: 10, EndLine: 20},
			{Path: "internal/run/runner.go", Kind: "go_type", Language: "go", Name: "Runner", Package: "run", Exported: true, Signature: "type Runner struct", StartLine: 3, EndLine: 3},
			{Path: "internal/run/helper.go", Kind: "go_func", Language: "go", Name: "BuildRunner", Signature: "func BuildRunner() *Runner", StartLine: 2, EndLine: 4},
			{Path: "internal/cache/cache.go", Kind: "go_method", Language: "go", Name: "Start", Parent: "Cache", Signature: "func (c *Cache) Start()", StartLine: 5, EndLine: 8},
			{Path: "internal/cache/cache.go", Kind: "go_method", Language: "go", Name: "Start", Parent: "Worker", Signature: "func (w *Worker) Start()", StartLine: 15, EndLine: 18},
		},
	})
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}
	return dbPath
}

func TestCodeItemResultCarriesSymbolMetadata(t *testing.T) {
	dbPath := buildSearchTestIndex(t)

	response, err := Query(dbPath, QueryOptions{Query: "Runner", Kind: KindCodeItem})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one result", response.Results)
	}
	got := response.Results[0]
	if got.StartLine != 3 || got.EndLine != 3 || got.Signature != "type Runner struct" {
		t.Fatalf("result = %#v, want start_line=3 end_line=3 signature=%q", got, "type Runner struct")
	}
	if got.Address() != "internal/contract/runner.go:3-3" {
		t.Fatalf("Address() = %q, want internal/contract/runner.go:3-3", got.Address())
	}
}

func TestSymbolLookupReturnsExactCaseInsensitiveMatches(t *testing.T) {
	dbPath := buildSymbolTestIndex(t)

	// Standalone --symbol with no lexical query, lower-cased to prove
	// case-insensitivity. Both Runner definitions must come back, ordered
	// deterministically by path.
	response, err := Query(dbPath, QueryOptions{Symbol: "runner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("results = %#v, want both Runner definitions", response.Results)
	}
	for _, r := range response.Results {
		if r.Kind != KindCodeItem || !strings.EqualFold(r.Title, "Runner") {
			t.Fatalf("result %#v is not an exact Runner code_item", r)
		}
	}
	if response.Results[0].Path != "internal/exec/runner.go" || response.Results[1].Path != "internal/run/runner.go" {
		t.Fatalf("results out of deterministic path order: %#v", response.Results)
	}
}

func TestSymbolLookupMatchesNameNotParentOrSignature(t *testing.T) {
	dbPath := buildSymbolTestIndex(t)

	// "Cache" is only a Parent, never a symbol name.
	cache, err := Query(dbPath, QueryOptions{Symbol: "Cache"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cache.Results) != 0 {
		t.Fatalf("--symbol Cache matched a parent: %#v", cache.Results)
	}

	// BuildRunner is an exact name even though "Runner" is a substring of it and
	// appears in its signature; --symbol Runner must not match it.
	runner, err := Query(dbPath, QueryOptions{Symbol: "Runner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runner.Results {
		if r.Title == "BuildRunner" {
			t.Fatalf("--symbol Runner matched substring/signature name BuildRunner: %#v", r)
		}
	}
}

func TestSymbolFilterWithQueryRestrictsToExactName(t *testing.T) {
	dbPath := buildSymbolTestIndex(t)

	// The lexical query also surfaces BuildRunner (its signature tokenizes the
	// bare "Runner"); --symbol Runner restricts to the exact-name definitions.
	lexical, err := Query(dbPath, QueryOptions{Query: "Runner"})
	if err != nil {
		t.Fatal(err)
	}
	sawBuildRunner := false
	for _, r := range lexical.Results {
		if r.Title == "BuildRunner" {
			sawBuildRunner = true
		}
	}
	if !sawBuildRunner {
		t.Fatalf("lexical query should surface BuildRunner; results=%#v", lexical.Results)
	}

	restricted, err := Query(dbPath, QueryOptions{Query: "Runner", Symbol: "Runner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(restricted.Results) != 2 {
		t.Fatalf("restricted results = %#v, want both Runner definitions", restricted.Results)
	}
	for _, r := range restricted.Results {
		if r.Title != "Runner" {
			t.Fatalf("restricted result %#v is not exactly Runner", r)
		}
	}
}

func TestSymbolLookupReturnsDuplicateNamesWithDistinctRanges(t *testing.T) {
	dbPath := buildSymbolTestIndex(t)

	response, err := Query(dbPath, QueryOptions{Symbol: "Start"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 2 {
		t.Fatalf("results = %#v, want both Start methods", response.Results)
	}
	ranges := map[string]bool{}
	for _, r := range response.Results {
		ranges[r.Address()] = true
	}
	if !ranges["internal/cache/cache.go:5-8"] || !ranges["internal/cache/cache.go:15-18"] {
		t.Fatalf("distinct Start ranges not preserved: %#v", response.Results)
	}
}

func TestNonCodeItemResultsHaveNoSymbolMetadata(t *testing.T) {
	dbPath := buildSearchTestIndex(t)

	response, err := Query(dbPath, QueryOptions{Query: "runner", Kind: KindFile})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) == 0 {
		t.Fatal("expected a file hit")
	}
	for _, r := range response.Results {
		if r.StartLine != 0 || r.EndLine != 0 || r.Signature != "" || r.HasRange() {
			t.Fatalf("non-code_item result carries symbol metadata: %#v", r)
		}
		if r.Address() != r.Path {
			t.Fatalf("non-code_item Address() = %q, want bare path %q", r.Address(), r.Path)
		}
	}
}

func TestInvalidRangeMetadataOmittedButResultReturned(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 6, 12, 7, 4, 27, 0, time.UTC),
		CodeItems: []codeindex.Item{
			{Path: "internal/x/x.go", Kind: "go_func", Language: "go", Name: "Partial", Signature: "func Partial()", StartLine: 5, EndLine: 0},
		},
	})
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	response, err := Query(dbPath, QueryOptions{Symbol: "Partial"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want the Partial result", response.Results)
	}
	got := response.Results[0]
	if got.StartLine != 0 || got.EndLine != 0 || got.HasRange() {
		t.Fatalf("invalid range surfaced: %#v", got)
	}
	// Invalid range omits all symbol metadata, signature included, so --json
	// carries it only for code_item hits with a usable range.
	if got.Signature != "" {
		t.Fatalf("signature surfaced for invalid range: %#v", got)
	}
	if got.Address() != "internal/x/x.go" {
		t.Fatalf("Address() = %q, want bare path (never path:0-0)", got.Address())
	}
}

func TestStaleSchemaDetected(t *testing.T) {
	t.Run("legacy index without schema marker", func(t *testing.T) {
		dbPath := buildSearchTestIndex(t)
		execOnIndex(t, dbPath, `DROP TABLE meta`)

		_, err := Query(dbPath, QueryOptions{Query: "Runner"})
		if !IsStaleIndex(err) {
			t.Fatalf("Query error = %v, want stale index", err)
		}
	})

	t.Run("incompatible schema version", func(t *testing.T) {
		dbPath := buildSearchTestIndex(t)
		execOnIndex(t, dbPath, `UPDATE meta SET value = 'pactum.search.index.v1' WHERE key = 'schema_version'`)

		_, err := Query(dbPath, QueryOptions{Symbol: "Runner"})
		if !IsStaleIndex(err) {
			t.Fatalf("Query error = %v, want stale index", err)
		}
	})
}

func TestDocumentsCarryCodeItemMetadataDeterministically(t *testing.T) {
	input := IndexInput{
		GeneratedAt: time.Date(2026, 6, 12, 7, 4, 27, 0, time.UTC),
		Files:       []projectmap.FileRecord{{Path: "internal/run/runner.go", Kind: "source", Language: "Go"}},
		CodeItems: []codeindex.Item{
			{Path: "internal/run/runner.go", Kind: "go_type", Language: "go", Name: "Runner", Signature: "type Runner struct", StartLine: 3, EndLine: 3},
			{Path: "internal/run/runner.go", Kind: "go_import", Language: "go", Name: "example.com/dep", ImportPath: "example.com/dep", Signature: "import", StartLine: 1, EndLine: 1},
		},
	}

	first := Documents(input)
	second := Documents(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("Documents() is not deterministic for identical input")
	}

	var codeItem, importDoc, fileDoc *Document
	for i := range first {
		switch first[i].Kind {
		case KindCodeItem:
			codeItem = &first[i]
		case KindImport:
			importDoc = &first[i]
		case KindFile:
			fileDoc = &first[i]
		}
	}
	if codeItem == nil || importDoc == nil || fileDoc == nil {
		t.Fatalf("expected code_item, import, and file documents; got %#v", first)
	}
	if codeItem.StartLine != 3 || codeItem.EndLine != 3 || codeItem.Signature != "type Runner struct" {
		t.Fatalf("code_item document missing metadata: %#v", codeItem)
	}
	// Import and file documents never carry symbol metadata.
	if importDoc.StartLine != 0 || importDoc.EndLine != 0 || importDoc.Signature != "" {
		t.Fatalf("import document carries symbol metadata: %#v", importDoc)
	}
	if fileDoc.StartLine != 0 || fileDoc.EndLine != 0 || fileDoc.Signature != "" {
		t.Fatalf("file document carries symbol metadata: %#v", fileDoc)
	}
}

func execOnIndex(t *testing.T, dbPath, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

// TestSymbolLookupRespectsLimitAcrossDuplicates pins the cap: duplicate exact
// matches beyond --limit are cut in the deterministic path/range order.
func TestSymbolLookupRespectsLimitAcrossDuplicates(t *testing.T) {
	dbPath := buildSymbolTestIndex(t)
	response, err := Query(dbPath, QueryOptions{Symbol: "Runner", Limit: 1})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("limit 1 must cap duplicate symbol matches, got %d", len(response.Results))
	}
	if response.Results[0].Path != "internal/exec/runner.go" {
		t.Fatalf("cap must keep the deterministic first match, got %s", response.Results[0].Path)
	}
}

// TestEnsureSchemaCurrentSeparatesStaleFromRealErrors pins the error
// classification: a missing meta table or version row reads as a stale index,
// while an unrelated database failure surfaces as itself — refresh guidance
// must not hide real errors.
func TestEnsureSchemaCurrentSeparatesStaleFromRealErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	// No meta table at all: an index from an older binary — stale.
	if err := ensureSchemaCurrent(db, dbPath); !IsStaleIndex(err) {
		t.Fatalf("missing meta table must read as stale, got %v", err)
	}
	// Meta exists but holds no version row — stale.
	if _, err := db.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := ensureSchemaCurrent(db, dbPath); !IsStaleIndex(err) {
		t.Fatalf("missing version row must read as stale, got %v", err)
	}
	// A closed handle is a real failure, not staleness.
	db.Close()
	err = ensureSchemaCurrent(db, dbPath)
	if err == nil || IsStaleIndex(err) {
		t.Fatalf("a real database failure must not read as stale, got %v", err)
	}
}

// TestInvalidRangeOmitsAllSymbolMetadataInJSON pins the all-or-nothing rule
// end to end: a code item with an invalid recorded range exposes no symbol
// metadata at all, and the JSON encoding omits the fields rather than
// emitting zeros a consumer might read as line addresses.
func TestInvalidRangeOmitsAllSymbolMetadataInJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	err := Rebuild(dbPath, IndexInput{
		GeneratedAt: time.Date(2026, 6, 12, 7, 4, 27, 0, time.UTC),
		CodeItems: []codeindex.Item{
			{Path: "internal/x/x.go", Kind: "go_func", Language: "go", Name: "Broken", Signature: "func Broken()", StartLine: 9, EndLine: 4},
		},
	})
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}
	response, err := Query(dbPath, QueryOptions{Symbol: "Broken"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("want the symbol hit despite the broken range, got %#v", response.Results)
	}
	encoded, err := json.Marshal(response.Results[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"start_line", "end_line", "signature"} {
		if strings.Contains(string(encoded), `"`+field+`"`) {
			t.Fatalf("invalid range must omit %s in JSON, got %s", field, encoded)
		}
	}
}
