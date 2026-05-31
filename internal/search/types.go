package search

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/projectmap"
)

const (
	KindAny      = "any"
	KindRepoMap  = "repo_map"
	KindLLMS     = "llms"
	KindFile     = "file"
	KindCodeItem = "code_item"
)

var supportedKinds = map[string]struct{}{
	KindAny:      {},
	KindRepoMap:  {},
	KindLLMS:     {},
	KindFile:     {},
	KindCodeItem: {},
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
	Files       []projectmap.FileRecord
	CodeItems   []codeindex.Item
}

type QueryOptions struct {
	Query string
	Limit int
	Kind  string
}

type Response struct {
	Query   string   `json:"query"`
	Results []Result `json:"results"`
}

type Result struct {
	Rank     int     `json:"rank"`
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Path     string  `json:"path"`
	Title    string  `json:"title"`
	Language string  `json:"language"`
	CodeKind string  `json:"code_kind"`
	Score    float64 `json:"score"`
	Snippet  string  `json:"snippet"`
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

	codeItems := append([]codeindex.Item(nil), input.CodeItems...)
	codeindex.SortItems(codeItems)
	for _, item := range codeItems {
		body := strings.Join(nonEmpty(
			"name: "+item.Name,
			"kind: "+item.Kind,
			"language: "+item.Language,
			"package: "+item.Package,
			"parent: "+item.Parent,
			"import_path: "+item.ImportPath,
			"signature: "+item.Signature,
			"path: "+item.Path,
		), "\n")
		documents = append(documents, Document{
			ID:        fmt.Sprintf("code_item:%s:%s:%s:%d", item.Path, item.Kind, item.Name, item.StartLine),
			Kind:      KindCodeItem,
			Path:      item.Path,
			Title:     item.Name,
			Body:      body,
			Language:  item.Language,
			CodeKind:  item.Kind,
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
			tokens = append(tokens, field)
		}
	}
	return strings.Join(tokens, " ")
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
