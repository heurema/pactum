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

- Start with `+"`repo-map.md`"+` for a concise repository overview.
- Use `+"`files.jsonl`"+` for deterministic per-file metadata and hashes.
- Search or read the project map before adding new code.
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
