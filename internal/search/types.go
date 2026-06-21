package search

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/heurema/pactum/internal/projectmap"
)

const (
	KindAny     = "any"
	KindRepoMap = "repo_map"
	KindLLMS    = "llms"
	KindWiki    = "wiki"
	KindFile    = "file"
)

var supportedKinds = map[string]struct{}{
	KindAny:     {},
	KindRepoMap: {},
	KindLLMS:    {},
	KindWiki:    {},
	KindFile:    {},
}

type Document struct {
	ID        string
	Kind      string
	Path      string
	Title     string
	Body      string
	Language  string
	CodeKind  string
	CreatedAt string
}

type IndexInput struct {
	GeneratedAt time.Time
	RepoMapBody []byte
	LLMSBody    []byte
	WikiPages   []projectmap.WikiPage
	Files       []projectmap.FileRecord
}

type QueryOptions struct {
	Query string
	Limit int
	Kind  string
}

// ResponseSchema identifies the machine-readable search response shape.
const ResponseSchema = "pactum.search.v1alpha1"

type Response struct {
	Schema  string   `json:"schema"`
	Query   string   `json:"query"`
	Results []Result `json:"results"`
}

type Result struct {
	Rank     int    `json:"rank"`
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Title    string `json:"title"`
	Language string `json:"language"`
	CodeKind string `json:"code_kind"`
	// StartLine, EndLine, and Signature are reserved for a future symbol layer.
	// They are never populated by the current file/wiki corpus and will always
	// be zero/empty in this version of pactum.
	StartLine int     `json:"start_line,omitempty"`
	EndLine   int     `json:"end_line,omitempty"`
	Signature string  `json:"signature,omitempty"`
	Score     float64 `json:"score"`
	Snippet   string  `json:"snippet"`
}

// HasRange reports whether the result carries a valid symbol line range.
// Reserved for a future symbol layer; always returns false in the current
// file/wiki corpus.
func (r Result) HasRange() bool {
	return validRange(r.StartLine, r.EndLine)
}

// validRange reports whether both lines are positive and end >= start.
func validRange(start, end int) bool {
	return start > 0 && end >= start
}

// Address returns the symbol-precise address "path:start-end" when the result
// has a valid range, otherwise the bare path.
func (r Result) Address() string {
	if r.HasRange() {
		return fmt.Sprintf("%s:%d-%d", r.Path, r.StartLine, r.EndLine)
	}
	return r.Path
}

func NormalizeKind(kind string) (string, error) {
	if kind == "" {
		return KindAny, nil
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if _, ok := supportedKinds[kind]; !ok {
		return "", fmt.Errorf("unsupported search kind %q", kind)
	}
	return kind, nil
}

func Documents(input IndexInput) []Document {
	createdAt := input.GeneratedAt.UTC().Format(time.RFC3339)
	documents := []Document{
		{
			ID:        "repo-map.md",
			Kind:      KindRepoMap,
			Path:      "map/repo-map.md",
			Title:     "Repository map",
			Body:      string(input.RepoMapBody),
			CreatedAt: createdAt,
		},
		{
			ID:        "llms.txt",
			Kind:      KindLLMS,
			Path:      "map/llms.txt",
			Title:     "LLM map pointer",
			Body:      string(input.LLMSBody),
			CreatedAt: createdAt,
		},
	}

	wikiPages := append([]projectmap.WikiPage(nil), input.WikiPages...)
	sort.Slice(wikiPages, func(i, j int) bool {
		return wikiPages[i].RelPath < wikiPages[j].RelPath
	})
	for _, page := range wikiPages {
		documents = append(documents, Document{
			ID:        "wiki:" + page.RelPath,
			Kind:      KindWiki,
			Path:      "map/wiki/" + page.RelPath,
			Title:     page.Title,
			Body:      string(page.Content),
			CreatedAt: createdAt,
		})
	}

	files := append([]projectmap.FileRecord(nil), input.Files...)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	for _, file := range files {
		body := strings.Join(nonEmpty(
			"path: "+file.Path,
			"language: "+file.Language,
			"kind: "+file.Kind,
			"top_level: "+topLevel(file.Path),
		), "\n")
		documents = append(documents, Document{
			ID:        "file:" + file.Path,
			Kind:      KindFile,
			Path:      file.Path,
			Title:     fileTitle(file.Path),
			Body:      body,
			Language:  file.Language,
			CodeKind:  file.Kind,
			CreatedAt: createdAt,
		})
	}

	sort.Slice(documents, func(i, j int) bool {
		return documents[i].ID < documents[j].ID
	})
	return documents
}

func ftsQuery(query string) string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			tokens = append(tokens, quoteFTSToken(field))
		}
	}
	return strings.Join(tokens, " ")
}

func quoteFTSToken(token string) string {
	return `"` + strings.ReplaceAll(token, `"`, `""`) + `"`
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.HasSuffix(value, ": ") {
			continue
		}
		result = append(result, value)
	}
	return result
}

func fileTitle(path string) string {
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return path
	}
	return base
}

func topLevel(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func ErrMissingIndex(path string) error {
	return missingIndexError{path: path}
}

type missingIndexError struct {
	path string
}

func (e missingIndexError) Error() string {
	return "search index is missing: " + e.path
}

func IsMissingIndex(err error) bool {
	var target missingIndexError
	return errors.As(err, &target)
}

func ErrStaleIndex(path string) error {
	return staleIndexError{path: path}
}

type staleIndexError struct {
	path string
}

func (e staleIndexError) Error() string {
	return "search index schema is stale: " + e.path
}

// IsStaleIndex reports whether err signals an index built against an older,
// incompatible stored shape. Callers should treat it as a prompt to run
// `pactum map refresh` rather than reading the stale rows.
func IsStaleIndex(err error) bool {
	var target staleIndexError
	return errors.As(err, &target)
}
