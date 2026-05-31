package projectmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/codeindex"
)

type Manifest struct {
	Schema       string            `json:"schema"`
	RunID        string            `json:"run_id"`
	GeneratedAt  time.Time         `json:"generated_at"`
	RepoRoot     string            `json:"repo_root"`
	ConfigHash   string            `json:"config_hash,omitempty"`
	FilesIndexed int               `json:"files_indexed"`
	FilesIgnored int               `json:"files_ignored"`
	FilesSkipped int               `json:"files_skipped"`
	CodeIndex    CodeIndexManifest `json:"code_index"`
	Warnings     []string          `json:"warnings,omitempty"`
	Artifacts    map[string]string `json:"artifacts"`
}

type CodeIndexManifest struct {
	Mode               string   `json:"mode"`
	SupportedLanguages []string `json:"supported_languages"`
	LanguagesSeen      []string `json:"languages_seen"`
	LanguagesIndexed   []string `json:"languages_indexed"`
	Items              int      `json:"items"`
}

func WriteJSONL[T any](path string, records []T) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func RenderRepoMap(root string, generatedAt time.Time, scan ScanResult) []byte {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "# Pactum Project Map\n\n")
	fmt.Fprintf(&buffer, "Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	fmt.Fprintf(&buffer, "Repository root: `%s`\n\n", root)

	fmt.Fprintf(&buffer, "## Summary\n\n")
	fmt.Fprintf(&buffer, "- Indexed files: %d\n", len(scan.Files))
	fmt.Fprintf(&buffer, "- Ignored files/directories: %d\n", scan.FilesIgnored)
	fmt.Fprintf(&buffer, "- Detected languages: %d\n", len(scan.Languages))
	fmt.Fprintf(&buffer, "- Code items: %d\n\n", len(scan.CodeItems))

	fmt.Fprintf(&buffer, "## Detected languages\n\n")
	if len(scan.Languages) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, item := range languageSummary(scan.Languages) {
			fmt.Fprintf(&buffer, "- %s: %d file(s)\n", item.Name, item.Count)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "## Top-level directories\n\n")
	if len(scan.TopDirs) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, dir := range scan.TopDirs {
			fmt.Fprintf(&buffer, "- `%s/`\n", dir)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "## Code surface\n\n")
	codeSurface := codeSurfaceLines(scan.CodeItems, 80)
	if len(codeSurface) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, line := range codeSurface {
			fmt.Fprintf(&buffer, "- %s\n", line)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "## Language support\n\n")
	fmt.Fprintf(&buffer, "- File metadata is collected for common source, config, and documentation files.\n")
	fmt.Fprintf(&buffer, "- Thin code index is enabled for the starter language pack: Go, Python, JavaScript, TypeScript/TSX/JSX, and C#.\n")
	fmt.Fprintf(&buffer, "- Pactum does not perform LSP, references, call graph, or semantic analysis in this phase.\n")
	fmt.Fprintf(&buffer, "- The map is a navigation aid, not complete semantic truth.\n\n")

	fmt.Fprintf(&buffer, "## Agent guidance\n\n")
	fmt.Fprintf(&buffer, "- Before adding new code, search/read relevant files and code items.\n")
	fmt.Fprintf(&buffer, "- Prefer existing exported functions/types when applicable.\n")
	fmt.Fprintf(&buffer, "- If ownership is unclear, ask for clarification instead of guessing.\n\n")

	fmt.Fprintf(&buffer, "## Important files\n\n")
	if len(scan.Important) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, file := range scan.Important {
			fmt.Fprintf(&buffer, "- `%s`\n", file)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "## File tree\n\n")
	lines := treeLines(scan.Files, 3)
	if len(lines) == 0 {
		fmt.Fprintf(&buffer, "- No indexed files\n")
	} else {
		for _, line := range lines {
			fmt.Fprintf(&buffer, "- `%s`\n", line)
		}
	}

	return buffer.Bytes()
}

func RenderLLMS() []byte {
	return []byte(strings.TrimSpace(`
# Pactum map pointer

This is a generated Pactum project map.

- Start with `+"`repo-map.md`"+`.
- Use `+"`files.jsonl`"+` for deterministic per-file metadata and hashes.
- Use `+"`code-items.jsonl`"+` for high-value code surface.
- Thin code index covers Go, Python, JavaScript, TypeScript/TSX/JSX, and C# in this build.
- The map is not complete semantic truth.
- Not every possible symbol is indexed.
- Before creating new code, inspect relevant existing files.
- If ownership is unclear, ask for clarification.
`) + "\n")
}

func RenderAreaIndex() []byte {
	return []byte(strings.TrimSpace(`
# Areas

Area detection is intentionally not implemented yet. Future Pactum versions will populate this directory with maintained ownership and feature-area views.
`) + "\n")
}

type languageItem struct {
	Name  string
	Count int
}

func languageSummary(languages map[string]int) []languageItem {
	items := make([]languageItem, 0, len(languages))
	for name, count := range languages {
		items = append(items, languageItem{Name: name, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Name < items[j].Name
		}
		return items[i].Count > items[j].Count
	})
	return items
}

func codeSurfaceLines(items []codeindex.Item, limit int) []string {
	lines := make([]string, 0, len(items))
	add := func(line string) bool {
		lines = append(lines, line)
		return limit > 0 && len(lines) >= limit
	}

	for _, item := range items {
		switch item.Kind {
		case "go_main", "py_main":
			if add(fmt.Sprintf("`%s`: `%s` `%s`", item.Path, item.Kind, item.Name)) {
				return lines
			}
		}
	}

	for _, item := range items {
		switch item.Kind {
		case "go_import", "py_import", "js_import", "ts_import", "cs_using", "cs_namespace", "go_package", "py_module":
			continue
		}
		label := item.Name
		if item.Package != "" && item.Parent == "" {
			label = item.Package + "." + item.Name
		}
		if item.Parent != "" {
			label = item.Parent + "." + item.Name
		}
		if label == "" {
			continue
		}
		if add(fmt.Sprintf("`%s`: `%s` `%s`", item.Path, item.Kind, label)) {
			return lines
		}
	}

	return lines
}

func treeLines(files []FileRecord, maxDepth int) []string {
	seen := make(map[string]struct{})
	for _, file := range files {
		parts := strings.Split(file.Path, "/")
		if len(parts) <= maxDepth {
			seen[file.Path] = struct{}{}
			continue
		}
		trimmed := filepath.ToSlash(filepath.Join(parts[:maxDepth]...)) + "/..."
		seen[trimmed] = struct{}{}
	}
	lines := make([]string, 0, len(seen))
	for line := range seen {
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
}
