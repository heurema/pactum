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
)

type Manifest struct {
	Schema       string            `json:"schema"`
	RunID        string            `json:"run_id"`
	GeneratedAt  time.Time         `json:"generated_at"`
	RepoRoot     string            `json:"repo_root"`
	FilesIndexed int               `json:"files_indexed"`
	FilesIgnored int               `json:"files_ignored"`
	FilesSkipped int               `json:"files_skipped"`
	Entries      int               `json:"entries"`
	Warnings     []string          `json:"warnings,omitempty"`
	Artifacts    map[string]string `json:"artifacts"`
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
	fmt.Fprintf(&buffer, "- Entries: %d\n\n", len(scan.Entries))

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

	fmt.Fprintf(&buffer, "## Important entrypoints\n\n")
	entrypoints := importantEntryLines(scan.Entries, 50)
	if len(entrypoints) == 0 {
		fmt.Fprintf(&buffer, "- None detected\n")
	} else {
		for _, line := range entrypoints {
			fmt.Fprintf(&buffer, "- %s\n", line)
		}
	}
	fmt.Fprintf(&buffer, "\n")

	fmt.Fprintf(&buffer, "## Agent guidance\n\n")
	fmt.Fprintf(&buffer, "- Before adding new code, search/read relevant files and entrypoints.\n")
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
- Use `+"`entries.jsonl`"+` for high-value Go entrypoints.
- Do not assume the map is complete semantic truth.
- Before creating new code, inspect relevant existing files.
- If uncertain, ask for clarification.
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

func importantEntryLines(entries []EntryRecord, limit int) []string {
	lines := make([]string, 0, len(entries))
	add := func(line string) bool {
		lines = append(lines, line)
		return limit > 0 && len(lines) >= limit
	}

	for _, entry := range entries {
		switch entry.Kind {
		case "go_main":
			if add(fmt.Sprintf("`%s`: `func main()`", entry.Path)) {
				return lines
			}
		case "go_package":
			if entry.IsMain {
				if add(fmt.Sprintf("`%s`: main package `%s`", entry.Path, entry.Package)) {
					return lines
				}
			}
		}
	}

	for _, entry := range entries {
		switch entry.Kind {
		case "go_func":
			if entry.Exported {
				if add(fmt.Sprintf("`%s`: exported func `%s.%s`", entry.Path, entry.Package, entry.Name)) {
					return lines
				}
			}
		case "go_type":
			if entry.Exported {
				if add(fmt.Sprintf("`%s`: exported type `%s.%s`", entry.Path, entry.Package, entry.Name)) {
					return lines
				}
			}
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
