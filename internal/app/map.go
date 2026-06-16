package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/projectmap"
	searchpkg "github.com/heurema/pactum/internal/search"
)

const mapManifestSchema = "pactum.map.manifest.v1"

const mapRefreshSchema = "pactum.map_refresh.v1alpha1"

// mapConfigHashScope marks a manifest whose config_hash pins only the
// canonicalized map: config section (see mapConfigHash). A manifest without this
// marker holds the legacy whole-file pin and is treated as stale once.
const mapConfigHashScope = "map"

// mapConfigHash pins the map-relevant config: a deterministic hash of the
// normalized map: section only (max_file_bytes and code_index). The code_index
// is normalized through codeindex.NormalizeMode — the same folding Scan applies
// — so aliases and the empty default that all resolve to "auto" hash alike and
// do not falsely invalidate the map. Editing unrelated sections such as agents
// or review.panel — or only comments and key order — never changes this hash;
// changing a map parameter does.
func mapConfigHash(m mapConfig) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("max_file_bytes=%d\x00code_index=%s", m.MaxFileBytes, codeindex.NormalizeMode(m.CodeIndex))))
	return hex.EncodeToString(sum[:])
}

type MapRefreshResult struct {
	Schema       string    `json:"schema"`
	RunID        string    `json:"run_id"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	RepoRoot     string    `json:"repo_root"`
	FilesIndexed int       `json:"files_indexed"`
	FilesIgnored int       `json:"files_ignored"`
	FilesSkipped int       `json:"files_skipped"`
	CodeItems    int       `json:"code_items"`
	Warnings     int       `json:"warnings"`
	SearchIndex  string    `json:"search_index"`
}

// mapRefreshResponse is the refresh result plus the next affordance; map
// refresh prints no human Next: hints, so next mirrors that as empty.
type mapRefreshResponse struct {
	MapRefreshResult
	Next []string `json:"next"`
}

func (a App) RefreshMap(root string) (MapRefreshResult, error) {
	return a.refreshMap(root, a.nowUTC())
}

func (a App) refreshMap(root string, startedAt time.Time) (MapRefreshResult, error) {
	if err := projectmap.ValidateRoot(root); err != nil {
		return MapRefreshResult{}, err
	}
	paths := artifacts.New(root)
	if !isDir(paths.Workspace) {
		return MapRefreshResult{}, fmt.Errorf("Pactum is not initialized. Run: pactum init")
	}
	if _, err := readWorkspaceManifest(paths.Manifest); err != nil {
		return MapRefreshResult{}, err
	}
	if err := ensureDirs([]string{paths.MapDir, paths.AreasDir, paths.WikiDir, paths.WikiAreasDir, paths.MapRunsDir, paths.LedgerDir}); err != nil {
		return MapRefreshResult{}, err
	}

	runID, err := nextMapRunID(startedAt, paths.MapRunsDir)
	if err != nil {
		return MapRefreshResult{}, err
	}
	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "map_refresh_started", Timestamp: startedAt, RunID: runID}); err != nil {
		return MapRefreshResult{}, err
	}

	config, err := readConfig(paths.Config)
	if err != nil {
		return MapRefreshResult{}, err
	}
	configHash := mapConfigHash(config.Map)

	scan, err := projectmap.Scan(root, projectmap.ScanOptions{
		MaxFileBytes:  int64(config.Map.MaxFileBytes),
		CodeIndexMode: config.Map.CodeIndex,
	})
	if err != nil {
		return MapRefreshResult{}, err
	}
	if err := projectmap.WriteJSONL(paths.FilesJSONL, scan.Files); err != nil {
		return MapRefreshResult{}, err
	}
	if err := projectmap.WriteJSONL(paths.HashesJSONL, scan.Hashes); err != nil {
		return MapRefreshResult{}, err
	}
	if err := projectmap.WriteJSONL(paths.CodeItemsJSONL, scan.CodeItems); err != nil {
		return MapRefreshResult{}, err
	}

	wiki := projectmap.RenderWiki(root, startedAt, scan)
	if err := writeWikiPages(paths, wiki); err != nil {
		return MapRefreshResult{}, err
	}

	repoMap := projectmap.RenderRepoMap(".", startedAt, scan, wiki)
	llms := projectmap.RenderLLMS()
	if err := os.WriteFile(paths.RepoMap, repoMap, 0o644); err != nil {
		return MapRefreshResult{}, err
	}
	if err := os.WriteFile(paths.LLMS, llms, 0o644); err != nil {
		return MapRefreshResult{}, err
	}
	if err := os.WriteFile(paths.AreaIndex, projectmap.RenderAreaIndex(), 0o644); err != nil {
		return MapRefreshResult{}, err
	}

	searchStartedAt := a.nowUTC()
	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "search_index_started", Timestamp: searchStartedAt, RunID: runID}); err != nil {
		return MapRefreshResult{}, err
	}
	if err := searchpkg.Rebuild(paths.SearchSQLite, searchpkg.IndexInput{
		GeneratedAt: searchStartedAt,
		RepoMapBody: repoMap,
		LLMSBody:    llms,
		WikiPages:   wiki,
		Files:       scan.Files,
		CodeItems:   scan.CodeItems,
	}); err != nil {
		return MapRefreshResult{}, err
	}
	searchFinishedAt := a.nowUTC()
	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "search_index_finished", Timestamp: searchFinishedAt, RunID: runID}); err != nil {
		return MapRefreshResult{}, err
	}

	mapManifest := projectmap.Manifest{
		Schema:          mapManifestSchema,
		RunID:           runID,
		GeneratedAt:     startedAt,
		RepoRoot:        ".",
		ConfigHash:      configHash,
		ConfigHashScope: mapConfigHashScope,
		FilesIndexed:    len(scan.Files),
		FilesIgnored:    scan.FilesIgnored,
		FilesSkipped:    scan.FilesSkipped,
		CodeIndex: projectmap.CodeIndexManifest{
			Mode:               scan.CodeIndexMode,
			SupportedLanguages: codeindex.SupportedLanguages(),
			LanguagesSeen:      scan.CodeIndexLanguagesSeen,
			LanguagesIndexed:   scan.CodeIndexLanguagesIndexed,
			Items:              len(scan.CodeItems),
		},
		Warnings: scan.Warnings,
		Artifacts: map[string]string{
			"repo_map":         "map/repo-map.md",
			"llms":             "map/llms.txt",
			"files":            "map/files.jsonl",
			"code_items":       "map/code-items.jsonl",
			"hashes":           "map/hashes.jsonl",
			"search":           "map/search.sqlite",
			"areas_index":      "map/areas/_index.md",
			"map_manifest":     "map/manifest.json",
			"wiki_overview":    "map/wiki/overview.md",
			"wiki_structure":   "map/wiki/structure.md",
			"wiki_commands":    "map/wiki/commands.md",
			"wiki_entrypoints": "map/wiki/entrypoints.md",
			"wiki_config":      "map/wiki/config.md",
			"wiki_tests":       "map/wiki/tests.md",
			"wiki_areas":       "map/wiki/areas/",
		},
	}
	if err := writeJSON(paths.MapManifest, mapManifest); err != nil {
		return MapRefreshResult{}, err
	}

	finishedAt := a.nowUTC()
	result := MapRefreshResult{
		Schema:       mapRefreshSchema,
		RunID:        runID,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		RepoRoot:     ".",
		FilesIndexed: len(scan.Files),
		FilesIgnored: scan.FilesIgnored,
		FilesSkipped: scan.FilesSkipped,
		CodeItems:    len(scan.CodeItems),
		Warnings:     len(scan.Warnings),
		SearchIndex:  "ready",
	}
	if err := writeJSON(filepath.Join(paths.MapRunsDir, runID+".json"), result); err != nil {
		return MapRefreshResult{}, err
	}
	if err := updateWorkspaceMapRun(paths.Manifest, runID, finishedAt); err != nil {
		return MapRefreshResult{}, err
	}
	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "map_refresh_finished", Timestamp: finishedAt, RunID: runID}); err != nil {
		return MapRefreshResult{}, err
	}

	return result, nil
}

// writeWikiPages regenerates the map wiki directory from scratch and writes
// every page. The wiki directory is removed first so stale area pages (from
// renamed or removed top-level directories) never linger; the wiki is a
// rebuildable artifact, so this is safe.
func writeWikiPages(paths artifacts.Paths, pages []projectmap.WikiPage) error {
	if err := os.RemoveAll(paths.WikiDir); err != nil {
		return err
	}
	if err := ensureDirs([]string{paths.WikiDir, paths.WikiAreasDir}); err != nil {
		return err
	}
	for _, page := range pages {
		dest := filepath.Join(paths.WikiDir, filepath.FromSlash(page.RelPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, page.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func nextMapRunID(startedAt time.Time, runsDir string) (string, error) {
	base := "map_" + startedAt.Format("20060102_150405")
	candidate := base
	for suffix := 2; ; suffix++ {
		path := filepath.Join(runsDir, candidate+".json")
		if _, err := os.Stat(path); err == nil {
			candidate = fmt.Sprintf("%s_%02d", base, suffix)
			continue
		} else if os.IsNotExist(err) {
			return candidate, nil
		} else {
			return "", err
		}
	}
}

func updateWorkspaceMapRun(path string, runID string, updatedAt time.Time) error {
	manifest, err := readWorkspaceManifest(path)
	if err != nil {
		return err
	}
	manifest.Map.CurrentRunID = runID
	manifest.UpdatedAt = updatedAt
	return writeJSON(path, manifest)
}

func writeMapRefreshResult(stdout io.Writer, result MapRefreshResult) {
	fmt.Fprintln(stdout, "Project map refreshed")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintf(stdout, "  files indexed: %d\n", result.FilesIndexed)
	fmt.Fprintf(stdout, "  files ignored: %d\n", result.FilesIgnored)
	fmt.Fprintf(stdout, "  files skipped: %d\n", result.FilesSkipped)
	fmt.Fprintf(stdout, "  code items: %d\n", result.CodeItems)
	fmt.Fprintf(stdout, "  warnings: %d\n", result.Warnings)
	fmt.Fprintf(stdout, "  search index: %s\n", result.SearchIndex)
}

func fileSHA256(path string) (string, error) {
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

func filesystemRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
