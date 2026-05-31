package projectmap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ScanOptions struct {
	MaxFileBytes int64
	IncludeGoAST bool
}

type FileRecord struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Language  string `json:"language"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type HashRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type EntryRecord struct {
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Package      string `json:"package,omitempty"`
	Exported     bool   `json:"exported,omitempty"`
	ImportPath   string `json:"import_path,omitempty"`
	IsMain       bool   `json:"is_main,omitempty"`
	ImportsCount int    `json:"imports_count,omitempty"`
}

type ScanResult struct {
	Files        []FileRecord
	Hashes       []HashRecord
	Entries      []EntryRecord
	FilesIgnored int
	FilesSkipped int
	Languages    map[string]int
	TopDirs      []string
	Important    []string
	Warnings     []string
}

var ignoredDirs = map[string]struct{}{
	".git":         {},
	".heurema":     {},
	"node_modules": {},
	"vendor":       {},
	".venv":        {},
	"dist":         {},
	"build":        {},
	"target":       {},
}

var ignoredBinaryExts = map[string]struct{}{
	".png":   {},
	".jpg":   {},
	".jpeg":  {},
	".gif":   {},
	".pdf":   {},
	".zip":   {},
	".tar":   {},
	".gz":    {},
	".exe":   {},
	".dll":   {},
	".so":    {},
	".dylib": {},
}

var importantFiles = []string{
	"README.md",
	"go.mod",
	"package.json",
	"pyproject.toml",
	"Cargo.toml",
	"Makefile",
	"Dockerfile",
}

func Scan(root string, options ...ScanOptions) (ScanResult, error) {
	option := scanOption(options)
	result := ScanResult{
		Languages: make(map[string]int),
	}
	topDirs := make(map[string]struct{})
	important := make(map[string]struct{})

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		name := entry.Name()
		if entry.IsDir() {
			if _, ok := ignoredDirs[name]; ok {
				result.FilesIgnored++
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if _, ok := ignoredBinaryExts[strings.ToLower(filepath.Ext(name))]; ok {
			result.FilesIgnored++
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			result.FilesIgnored++
			return nil
		}
		if option.MaxFileBytes > 0 && info.Size() > option.MaxFileBytes {
			result.FilesIgnored++
			result.FilesSkipped++
			result.Warnings = append(result.Warnings, "skipped large file: "+rel+" exceeds project_map.max_file_bytes")
			return nil
		}

		hash, err := sha256File(path)
		if err != nil {
			return err
		}

		language := inferLanguage(rel)
		record := FileRecord{
			Path:      rel,
			Kind:      inferKind(rel),
			Language:  language,
			SizeBytes: info.Size(),
			SHA256:    hash,
		}
		result.Files = append(result.Files, record)
		result.Hashes = append(result.Hashes, HashRecord{Path: rel, SHA256: hash})
		if language != "Unknown" {
			result.Languages[language]++
		}
		if option.IncludeGoAST && strings.ToLower(filepath.Ext(name)) == ".go" {
			entries, warnings := extractGoEntries(path, rel)
			result.Entries = append(result.Entries, entries...)
			result.Warnings = append(result.Warnings, warnings...)
		}

		parts := strings.Split(rel, "/")
		if len(parts) > 1 {
			topDirs[parts[0]] = struct{}{}
		}
		for _, candidate := range importantFiles {
			if rel == candidate {
				important[candidate] = struct{}{}
			}
		}

		return nil
	})
	if err != nil {
		return ScanResult{}, err
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})
	sort.Slice(result.Hashes, func(i, j int) bool {
		return result.Hashes[i].Path < result.Hashes[j].Path
	})
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].Path != result.Entries[j].Path {
			return result.Entries[i].Path < result.Entries[j].Path
		}
		if result.Entries[i].Kind != result.Entries[j].Kind {
			return result.Entries[i].Kind < result.Entries[j].Kind
		}
		if result.Entries[i].Name != result.Entries[j].Name {
			return result.Entries[i].Name < result.Entries[j].Name
		}
		return result.Entries[i].ImportPath < result.Entries[j].ImportPath
	})
	sort.Strings(result.Warnings)

	result.TopDirs = sortedKeys(topDirs)
	for _, candidate := range importantFiles {
		if _, ok := important[candidate]; ok {
			result.Important = append(result.Important, candidate)
		}
	}

	return result, nil
}

func scanOption(options []ScanOptions) ScanOptions {
	if len(options) == 0 {
		return ScanOptions{}
	}
	return options[0]
}

func extractGoEntries(path string, rel string) ([]EntryRecord, []string) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{"read Go file failed: " + rel + ": " + err.Error()}
	}

	file, err := parser.ParseFile(token.NewFileSet(), rel, source, 0)
	if err != nil {
		return nil, []string{"go parse failed: " + rel + ": " + err.Error()}
	}

	packageName := file.Name.Name
	isMainPackage := packageName == "main"
	entries := []EntryRecord{{
		Path:         rel,
		Kind:         "go_package",
		Name:         packageName,
		Package:      packageName,
		IsMain:       isMainPackage,
		ImportsCount: len(file.Imports),
	}}

	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			importPath = importSpec.Path.Value
		}
		entries = append(entries, EntryRecord{
			Path:       rel,
			Kind:       "go_import",
			Name:       importPath,
			Package:    packageName,
			ImportPath: importPath,
		})
	}

	for _, decl := range file.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			if typed.Recv != nil {
				continue
			}
			if isMainPackage && typed.Name.Name == "main" {
				entries = append(entries, EntryRecord{
					Path:    rel,
					Kind:    "go_main",
					Name:    "main",
					Package: packageName,
					IsMain:  true,
				})
				continue
			}
			if !typed.Name.IsExported() {
				continue
			}
			entries = append(entries, EntryRecord{
				Path:     rel,
				Kind:     "go_func",
				Name:     typed.Name.Name,
				Package:  packageName,
				Exported: true,
			})
		case *ast.GenDecl:
			if typed.Tok != token.TYPE {
				continue
			}
			for _, spec := range typed.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Name.IsExported() {
					continue
				}
				entries = append(entries, EntryRecord{
					Path:     rel,
					Kind:     "go_type",
					Name:     typeSpec.Name.Name,
					Package:  packageName,
					Exported: true,
				})
			}
		}
	}

	return entries, nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func inferKind(path string) string {
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(name))

	switch {
	case isSourceExt(ext) || hasShellShebangLikeName(name):
		return "source"
	case ext == ".md" || ext == ".txt" || ext == ".rst" || ext == ".adoc":
		return "doc"
	case isConfigName(name) || isConfigExt(ext):
		return "config"
	default:
		return "other"
	}
}

func inferLanguage(path string) string {
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(name))

	switch ext {
	case ".go":
		return "Go"
	case ".md":
		return "Markdown"
	case ".txt":
		return "Text"
	case ".json":
		return "JSON"
	case ".yaml", ".yml":
		return "YAML"
	case ".toml":
		return "TOML"
	case ".js", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts":
		return "TypeScript"
	case ".tsx":
		return "TSX"
	case ".jsx":
		return "JSX"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".c":
		return "C"
	case ".h":
		return "C/C++ Header"
	case ".cpp", ".cc", ".cxx":
		return "C++"
	case ".cs":
		return "C#"
	case ".sh":
		return "Shell"
	case ".bash":
		return "Bash"
	case ".zsh":
		return "Zsh"
	case ".html", ".htm":
		return "HTML"
	case ".css":
		return "CSS"
	case ".scss":
		return "SCSS"
	case ".sql":
		return "SQL"
	}

	switch name {
	case "Dockerfile":
		return "Dockerfile"
	case "Makefile":
		return "Make"
	case "go.mod", "go.sum":
		return "Go module"
	default:
		return "Unknown"
	}
}

func isSourceExt(ext string) bool {
	switch ext {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".rs", ".java", ".c", ".h", ".cpp", ".cc", ".cxx", ".cs", ".sh", ".bash", ".zsh", ".html", ".htm", ".css", ".scss", ".sql":
		return true
	default:
		return false
	}
}

func hasShellShebangLikeName(name string) bool {
	return name == "bash" || name == "sh" || name == "zsh"
}

func isConfigName(name string) bool {
	switch name {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "pyproject.toml", "Cargo.toml", "Cargo.lock", "Makefile", "Dockerfile", ".gitignore", ".dockerignore":
		return true
	default:
		return false
	}
}

func isConfigExt(ext string) bool {
	switch ext {
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".env", ".lock":
		return true
	default:
		return false
	}
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func ValidateRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("repository root is not a directory")
	}
	return nil
}
