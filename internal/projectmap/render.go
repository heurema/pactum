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

// RenderRepoMap renders the human-readable project map. The map is wiki-first:
// the generated wiki pages are listed before the best-effort code surface, and
// the page makes clear that code items are navigation hints rather than
// complete semantic truth.
func RenderRepoMap(root string, generatedAt time.Time, scan ScanResult, wiki []WikiPage) []byte {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "# Pactum Project Map\n\n")
	fmt.Fprintf(&buffer, "Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	fmt.Fprintf(&buffer, "Repository root: `%s`\n\n", root)

	fmt.Fprintf(&buffer, "## Summary\n\n")
	fmt.Fprintf(&buffer, "- Indexed files: %d\n", len(scan.Files))
	fmt.Fprintf(&buffer, "- Ignored files/directories: %d\n", scan.FilesIgnored)
	fmt.Fprintf(&buffer, "- Detected languages: %d\n", len(scan.Languages))
	fmt.Fprintf(&buffer, "- Code items (best-effort hints): %d\n\n", len(scan.CodeItems))

	fmt.Fprintf(&buffer, "## How to navigate this map\n\n")
	fmt.Fprintf(&buffer, "- Start with the wiki: read `wiki/overview.md` first.\n")
	fmt.Fprintf(&buffer, "- The wiki is generated from deterministic facts (file inventory and manifests).\n")
	fmt.Fprintf(&buffer, "- Code items are best-effort navigation hints, not complete semantic truth.\n")
	fmt.Fprintf(&buffer, "- Unsupported languages/framework files may have no code items.\n")
	fmt.Fprintf(&buffer, "- Imports are not treated as primary code surface.\n")
	fmt.Fprintf(&buffer, "- Source files remain the source of truth.\n\n")

	fmt.Fprintf(&buffer, "## Wiki pages\n\n")
	for _, page := range topLevelWikiPages(wiki) {
		fmt.Fprintf(&buffer, "- `wiki/%s` — %s\n", page.RelPath, page.Title)
	}
	areaPages := areaWikiPages(wiki)
	if len(areaPages) > 0 {
		fmt.Fprintf(&buffer, "- Area pages:\n")
		for _, page := range areaPages {
			fmt.Fprintf(&buffer, "  - `wiki/%s`\n", page.RelPath)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "## Project map artifacts\n\n")
	fmt.Fprintf(&buffer, "- `files.jsonl` — deterministic per-file metadata.\n")
	fmt.Fprintf(&buffer, "- `hashes.jsonl` — per-file content hashes.\n")
	fmt.Fprintf(&buffer, "- `code-items.jsonl` — best-effort symbol hints (incomplete by design).\n")
	fmt.Fprintf(&buffer, "- `search.sqlite` — local full-text search index.\n")
	fmt.Fprintf(&buffer, "- `manifest.json` — map manifest listing all artifacts.\n\n")

	fmt.Fprintf(&buffer, "## Files / areas\n\n")
	fmt.Fprintf(&buffer, "### Detected languages\n\n")
	if len(scan.Languages) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, item := range languageSummary(scan.Languages) {
			fmt.Fprintf(&buffer, "- %s: %d file(s)\n", item.Name, item.Count)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "### Top-level directories\n\n")
	if len(scan.TopDirs) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, dir := range scan.TopDirs {
			fmt.Fprintf(&buffer, "- `%s/` (see `wiki/areas/%s`)\n", dir, areaFileName(dir))
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "### Important files\n\n")
	if len(scan.Important) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, file := range scan.Important {
			fmt.Fprintf(&buffer, "- `%s`\n", file)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "### File tree\n\n")
	lines := treeLines(scan.Files, 3)
	if len(lines) == 0 {
		fmt.Fprintf(&buffer, "- No indexed files\n")
	} else {
		for _, line := range lines {
			fmt.Fprintf(&buffer, "- `%s`\n", line)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "## Code surface (best-effort code hints)\n\n")
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
	fmt.Fprintf(&buffer, "- Best-effort code hints are extracted for the starter language pack: Go, Python, JavaScript, TypeScript/TSX/JSX, and C#.\n")
	fmt.Fprintf(&buffer, "- Code items are best-effort navigation hints; imports are not treated as primary code surface.\n")
	fmt.Fprintf(&buffer, "- Unsupported languages/framework files may have no code items but still appear in the wiki and file inventory.\n")
	fmt.Fprintf(&buffer, "- Pactum does not perform LSP, references, call graph, or semantic analysis in this phase.\n")
	fmt.Fprintf(&buffer, "- The map is a navigation aid, not complete semantic truth.\n")
	fmt.Fprintf(&buffer, "- Source files remain the source of truth.\n\n")

	fmt.Fprintf(&buffer, "## Agent guidance\n\n")
	fmt.Fprintf(&buffer, "- Read the wiki first (`wiki/overview.md`), then drill into the relevant area page.\n")
	fmt.Fprintf(&buffer, "- Before adding new code, search/read relevant files and code items.\n")
	fmt.Fprintf(&buffer, "- Prefer existing exported functions/types when applicable.\n")
	fmt.Fprintf(&buffer, "- If ownership is unclear, ask for clarification instead of guessing.\n")

	return buffer.Bytes()
}

// RenderLLMS renders llms.txt as a compact router into the deterministic map
// wiki. It points agents at the wiki pages first and frames code items as
// best-effort hints rather than complete semantic truth.
func RenderLLMS() []byte {
	return []byte(strings.TrimSpace(`
# Pactum project map — agent router

This is a generated, deterministic Pactum project map. Read the map wiki first.
The wiki is generated from deterministic facts (file inventory and manifests).

Read order:
- Read map/wiki/overview.md first.
- Read map/wiki/structure.md for areas and their likely roles.
- Read map/wiki/commands.md for build and test commands.
- Read map/wiki/entrypoints.md for candidate entrypoints.
- Read map/wiki/config.md for configuration.
- Read map/wiki/tests.md for tests.
- See map/repo-map.md for the human-readable map.

Best-effort hints:
- Use map/code-items.jsonl only as best-effort symbol hints. They are incomplete by design.
- Use map/files.jsonl for the deterministic file inventory and hashes.
- Unsupported languages and framework files may have no code items.

Ground rules:
- Source files remain the source of truth.
- Not every possible symbol is indexed.
- Before creating new code, inspect relevant existing files.
- If ownership is unclear, ask for clarification.
`) + "\n")
}

// RenderAreaIndex renders the map/areas index as a pointer to the wiki-first
// area pages, which now live under map/wiki/areas/.
func RenderAreaIndex() []byte {
	return []byte(strings.TrimSpace(`
# Areas

Area pages are generated under map/wiki/areas/. Start at map/wiki/overview.md
and map/wiki/structure.md for the top-level areas and their likely roles.
`) + "\n")
}

func topLevelWikiPages(wiki []WikiPage) []WikiPage {
	var pages []WikiPage
	for _, page := range wiki {
		if !strings.Contains(page.RelPath, "/") {
			pages = append(pages, page)
		}
	}
	return pages
}

func areaWikiPages(wiki []WikiPage) []WikiPage {
	var pages []WikiPage
	for _, page := range wiki {
		if strings.HasPrefix(page.RelPath, "areas/") {
			pages = append(pages, page)
		}
	}
	return pages
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
		if item.IsEntryPoint() {
			if add(fmt.Sprintf("`%s`: `%s` `%s`", item.Path, item.Kind, item.Name)) {
				return lines
			}
		}
	}

	for _, item := range items {
		if item.IsImportLike() {
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
