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

type MapRefreshResult struct {
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
	if err := ensureDirs([]string{paths.MapDir, paths.AreasDir, paths.MapRunsDir, paths.LedgerDir}); err != nil {
		return MapRefreshResult{}, err
	}

	runID, err := nextMapRunID(startedAt, paths.MapRunsDir)
	if err != nil {
		return MapRefreshResult{}, err
	}
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "map_refresh_started", Timestamp: startedAt, RunID: runID, RepoRoot: root}); err != nil {
		return MapRefreshResult{}, err
	}

	config, err := readConfig(paths.Config)
	if err != nil {
		return MapRefreshResult{}, err
	}
	configHash, err := fileSHA256(paths.Config)
	if err != nil {
		return MapRefreshResult{}, err
	}

	scan, err := projectmap.Scan(root, projectmap.ScanOptions{
		MaxFileBytes:  int64(config.ProjectMap.MaxFileBytes),
		CodeIndexMode: config.ProjectMap.CodeIndex,
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

	repoMap := projectmap.RenderRepoMap(".", startedAt, scan)
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
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "search_index_started", Timestamp: searchStartedAt, RunID: runID, RepoRoot: root}); err != nil {
		return MapRefreshResult{}, err
	}
	if err := searchpkg.Rebuild(paths.SearchSQLite, searchpkg.IndexInput{
		GeneratedAt: searchStartedAt,
		RepoMapBody: repoMap,
		LLMSBody:    llms,
		Files:       scan.Files,
		CodeItems:   scan.CodeItems,
	}); err != nil {
		return MapRefreshResult{}, err
	}
	searchFinishedAt := a.nowUTC()
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "search_index_finished", Timestamp: searchFinishedAt, RunID: runID, RepoRoot: root}); err != nil {
		return MapRefreshResult{}, err
	}

	mapManifest := projectmap.Manifest{
		Schema:       "pactum.map.manifest.v1",
		RunID:        runID,
		GeneratedAt:  startedAt,
		RepoRoot:     ".",
		ConfigHash:   configHash,
		FilesIndexed: len(scan.Files),
		FilesIgnored: scan.FilesIgnored,
		FilesSkipped: scan.FilesSkipped,
		CodeIndex: projectmap.CodeIndexManifest{
			Mode:               scan.CodeIndexMode,
			SupportedLanguages: codeindex.SupportedLanguages(),
			LanguagesSeen:      scan.CodeIndexLanguagesSeen,
			LanguagesIndexed:   scan.CodeIndexLanguagesIndexed,
			Items:              len(scan.CodeItems),
		},
		Warnings: scan.Warnings,
		Artifacts: map[string]string{
			"repo_map":     "map/repo-map.md",
			"llms":         "map/llms.txt",
			"files":        "map/files.jsonl",
			"code_items":   "map/code-items.jsonl",
			"hashes":       "map/hashes.jsonl",
			"search":       "map/search.sqlite",
			"areas_index":  "map/areas/_index.md",
			"map_manifest": "map/manifest.json",
		},
	}
	if err := writeJSON(paths.MapManifest, mapManifest); err != nil {
		return MapRefreshResult{}, err
	}

	finishedAt := a.nowUTC()
	result := MapRefreshResult{
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
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "map_refresh_finished", Timestamp: finishedAt, RunID: runID, RepoRoot: root}); err != nil {
		return MapRefreshResult{}, err
	}

	return result, nil
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
