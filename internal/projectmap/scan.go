package projectmap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/heurema/pactum/internal/codeindex"
)

type ScanOptions struct {
	MaxFileBytes  int64
	CodeIndexMode string
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

type ScanResult struct {
	Files                     []FileRecord
	Hashes                    []HashRecord
	CodeItems                 []codeindex.Item
	FilesIgnored              int
	FilesSkipped              int
	Languages                 map[string]int
	CodeIndexMode             string
	CodeIndexLanguagesSeen    []string
	CodeIndexLanguagesIndexed []string
	TopDirs                   []string
	Important                 []string
	Warnings                  []string
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
	scan := newScanBuilder(option)

	if paths, ok := gitCandidateFiles(root); ok {
		sort.Strings(paths)
		for _, rel := range paths {
			if err := scan.scanGitFile(root, rel); err != nil {
				return ScanResult{}, err
			}
		}
		return scan.finish(), nil
	}

	if err := scan.walk(root); err != nil {
		return ScanResult{}, err
	}
	return scan.finish(), nil
}

type scanBuilder struct {
	option                    ScanOptions
	result                    ScanResult
	topDirs                   map[string]struct{}
	important                 map[string]struct{}
	codeIndexLanguagesSeen    map[string]struct{}
	codeIndexLanguagesIndexed map[string]struct{}
}

func newScanBuilder(option ScanOptions) *scanBuilder {
	return &scanBuilder{
		option: option,
		result: ScanResult{
			Languages:     make(map[string]int),
			CodeIndexMode: codeindex.NormalizeMode(option.CodeIndexMode),
		},
		topDirs:                   make(map[string]struct{}),
		important:                 make(map[string]struct{}),
		codeIndexLanguagesSeen:    make(map[string]struct{}),
		codeIndexLanguagesIndexed: make(map[string]struct{}),
	}
}

func (scan *scanBuilder) walk(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		name := entry.Name()
		if entry.IsDir() {
			if _, ok := ignoredDirs[name]; ok {
				scan.result.FilesIgnored++
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if scan.ignoreBinaryExt(name) {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		return scan.scanFileInfo(path, rel, info)
	})
}

func (scan *scanBuilder) scanGitFile(root string, rel string) error {
	rel = filepath.ToSlash(rel)
	if isPactumWorkspacePath(rel) {
		scan.result.FilesIgnored++
		return nil
	}
	if scan.ignoreBinaryExt(filepath.Base(rel)) {
		return nil
	}

	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return scan.scanFileInfo(path, rel, info)
}

func (scan *scanBuilder) scanFileInfo(path string, rel string, info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		scan.result.FilesIgnored++
		return nil
	}
	if scan.option.MaxFileBytes > 0 && info.Size() > scan.option.MaxFileBytes {
		scan.result.FilesIgnored++
		scan.result.FilesSkipped++
		scan.result.Warnings = append(scan.result.Warnings, "skipped large file: "+rel+" exceeds map.max_file_bytes")
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
	scan.result.Files = append(scan.result.Files, record)
	scan.result.Hashes = append(scan.result.Hashes, HashRecord{Path: rel, SHA256: hash})
	if language != "Unknown" {
		scan.result.Languages[language]++
	}

	codeLanguage := codeindex.LanguageForPath(rel)
	if codeindex.IsSupported(codeLanguage) {
		scan.codeIndexLanguagesSeen[codeLanguage] = struct{}{}
	}
	if codeindex.Enabled(scan.result.CodeIndexMode) && codeindex.IsSupported(codeLanguage) {
		source, err := os.ReadFile(path)
		if err != nil {
			scan.result.Warnings = append(scan.result.Warnings, "read code file failed: "+rel+": "+err.Error())
		} else {
			extracted := codeindex.Extract(rel, codeLanguage, source)
			scan.result.CodeItems = append(scan.result.CodeItems, extracted.Items...)
			scan.result.Warnings = append(scan.result.Warnings, extracted.Warnings...)
			if len(extracted.Warnings) == 0 {
				scan.codeIndexLanguagesIndexed[codeLanguage] = struct{}{}
			}
		}
	}

	parts := strings.Split(rel, "/")
	if len(parts) > 1 {
		scan.topDirs[parts[0]] = struct{}{}
	}
	for _, candidate := range importantFiles {
		if rel == candidate {
			scan.important[candidate] = struct{}{}
		}
	}

	return nil
}

func (scan *scanBuilder) ignoreBinaryExt(name string) bool {
	if _, ok := ignoredBinaryExts[strings.ToLower(filepath.Ext(name))]; ok {
		scan.result.FilesIgnored++
		return true
	}
	return false
}

func (scan *scanBuilder) finish() ScanResult {
	sort.Slice(scan.result.Files, func(i, j int) bool {
		return scan.result.Files[i].Path < scan.result.Files[j].Path
	})
	sort.Slice(scan.result.Hashes, func(i, j int) bool {
		return scan.result.Hashes[i].Path < scan.result.Hashes[j].Path
	})
	codeindex.SortItems(scan.result.CodeItems)
	sort.Strings(scan.result.Warnings)

	scan.result.TopDirs = sortedKeys(scan.topDirs)
	scan.result.CodeIndexLanguagesSeen = sortedKeys(scan.codeIndexLanguagesSeen)
	scan.result.CodeIndexLanguagesIndexed = sortedKeys(scan.codeIndexLanguagesIndexed)
	for _, candidate := range importantFiles {
		if _, ok := scan.important[candidate]; ok {
			scan.result.Important = append(scan.result.Important, candidate)
		}
	}

	return scan.result
}

func gitCandidateFiles(root string) ([]string, bool) {
	command := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	output, err := command.Output()
	if err != nil {
		return nil, false
	}

	seen := make(map[string]struct{})
	paths := []string{}
	for _, raw := range strings.Split(string(output), "\x00") {
		rel, ok := normalizeGitPath(raw)
		if !ok {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		paths = append(paths, rel)
	}
	return paths, true
}

func normalizeGitPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	rel := filepath.ToSlash(path)
	rel = strings.TrimPrefix(rel, "./")
	if rel == "." || filepath.IsAbs(filepath.FromSlash(rel)) || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", false
	}
	return rel, true
}

func isPactumWorkspacePath(rel string) bool {
	return rel == ".heurema" || strings.HasPrefix(rel, ".heurema/")
}

func scanOption(options []ScanOptions) ScanOptions {
	if len(options) == 0 {
		return ScanOptions{CodeIndexMode: codeindex.ModeAuto}
	}
	option := options[0]
	option.CodeIndexMode = codeindex.NormalizeMode(option.CodeIndexMode)
	return option
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
	case ".vue":
		return "Vue"
	case ".svelte":
		return "Svelte"
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
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".vue", ".svelte", ".rs", ".java", ".c", ".h", ".cpp", ".cc", ".cxx", ".cs", ".sh", ".bash", ".zsh", ".html", ".htm", ".css", ".scss", ".sql":
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
