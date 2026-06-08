package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/codeindex"
	"github.com/heurema/pactum/internal/projectmap"
)

const statusSchema = "pactum.status.v1"

type statusResponse struct {
	Schema      string           `json:"schema"`
	Initialized bool             `json:"initialized"`
	RepoRoot    string           `json:"repo_root,omitempty"`
	Workspace   string           `json:"workspace,omitempty"`
	ProjectMap  projectMapStatus `json:"project_map,omitempty"`
	Runs        runsStatus       `json:"runs,omitempty"`
	Memory      memoryStatus     `json:"memory,omitempty"`
	Usage       usageStatus      `json:"usage,omitempty"`
	Message     string           `json:"message,omitempty"`
}

type projectMapStatus struct {
	Status       string   `json:"status"`
	RunID        string   `json:"run_id"`
	FilesIndexed int      `json:"files_indexed"`
	CodeItems    int      `json:"code_items"`
	SearchIndex  string   `json:"search_index"`
	StaleReasons []string `json:"stale_reasons"`
}

type runsStatus struct {
	Active       int    `json:"active"`
	LatestRunID  string `json:"latest_run_id,omitempty"`
	LatestStatus string `json:"latest_status,omitempty"`
	CurrentRunID string `json:"current_run_id,omitempty"`
	NextCommand  string `json:"next_command,omitempty"`
}

type memoryStatus struct {
	Items int `json:"items"`
	Stale int `json:"stale"`
}

type usageStatus struct {
	RunID               string  `json:"run_id,omitempty"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	ReasoningTokens     int64   `json:"reasoning_tokens"`
	CapturedCalls       int     `json:"captured_calls"`
	UncapturedCalls     int     `json:"uncaptured_calls"`
	CacheReadRatio      float64 `json:"cache_read_ratio"`
	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`
}

func writeStatusNotInitialized(stdout io.Writer) error {
	return writeJSONResponse(stdout, struct {
		Initialized bool   `json:"initialized"`
		Message     string `json:"message"`
	}{
		Initialized: false,
		Message:     "Pactum is not initialized. Run: pactum init",
	})
}

func (a App) workspaceStatus(root string) (statusResponse, error) {
	paths := artifacts.New(root)
	config, err := readConfig(paths.Config)
	if err != nil {
		return statusResponse{}, err
	}
	configHash, err := storeFileSHA256(paths.Config)
	if err != nil {
		return statusResponse{}, err
	}
	manifest, err := readWorkspaceManifest(paths.Manifest)
	if err != nil {
		return statusResponse{}, err
	}
	mapStatus, err := inspectProjectMap(root, paths, config, configHash, manifest.Map.CurrentRunID)
	if err != nil {
		return statusResponse{}, err
	}
	activeRuns, err := countActiveRuns(paths)
	if err != nil {
		return statusResponse{}, err
	}
	memoryItems, err := countMemoryItems(paths)
	if err != nil {
		return statusResponse{}, err
	}

	runs := runsStatus{Active: activeRuns}
	usageRunID := ""
	if latestID, hasLatest, err := latestRunID(paths); err != nil {
		return statusResponse{}, err
	} else if hasLatest {
		runs.LatestRunID = latestID
		runs.LatestStatus = deriveRunStatus(paths, latestID)
		usageRunID = latestID
		currentID, hasCurrent := readCurrentRun(paths)
		currentValid := hasCurrent && runExists(paths, currentID)
		if currentValid {
			runs.CurrentRunID = currentID
			usageRunID = currentID
		}
		active, err := activeRunIDs(paths)
		if err != nil {
			return statusResponse{}, err
		}
		// The next command must actually be runnable: a bare staged command only
		// works when an omitted run id resolves to a single run (current, or the
		// sole active run). Otherwise point the user at selecting a run.
		switch {
		case currentValid:
			runs.NextCommand = nextCommandForStatus(deriveRunStatus(paths, currentID))
		case len(active) == 1:
			runs.NextCommand = nextCommandForStatus(deriveRunStatus(paths, active[0]))
		default:
			runs.NextCommand = "pactum task use " + latestID
		}
	}
	usage, err := runUsageStatus(paths, usageRunID)
	if err != nil {
		return statusResponse{}, err
	}

	return statusResponse{
		Schema:      statusSchema,
		Initialized: true,
		RepoRoot:    root,
		Workspace:   paths.Workspace,
		ProjectMap:  mapStatus,
		Runs:        runs,
		Memory:      memoryStatus{Items: memoryItems, Stale: 0},
		Usage:       usage,
	}, nil
}

func runUsageStatus(paths artifacts.Paths, runID string) (usageStatus, error) {
	if strings.TrimSpace(runID) == "" {
		return usageStatus{}, nil
	}
	summary, err := usageForRun(paths, runID)
	if err != nil {
		return usageStatus{}, err
	}
	return usageStatus{
		RunID:               runID,
		InputTokens:         summary.Total.InputTokens,
		OutputTokens:        summary.Total.OutputTokens,
		TotalTokens:         summary.Total.TotalTokens,
		CacheReadTokens:     summary.Total.CacheReadTokens,
		CacheCreationTokens: summary.Total.CacheCreationTokens,
		ReasoningTokens:     summary.Total.ReasoningTokens,
		CapturedCalls:       summary.CapturedCalls,
		UncapturedCalls:     summary.UncapturedCalls,
		CacheReadRatio:      summary.CacheReadRatio,
		EstimatedCostUSD:    0,
	}, nil
}

func countMemoryItems(paths artifacts.Paths) (int, error) {
	items, err := readJSONLines[memoryItemRecord](paths.MemoryItems)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func countActiveRuns(paths artifacts.Paths) (int, error) {
	entries, err := activeStore.ReadDir(paths.RunsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	active := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runPath := filepath.Join(paths.RunsDir, entry.Name(), "run.json")
		data, err := activeStore.ReadBytes(runPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		var state struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(data, &state); err != nil {
			// A run still being created concurrently may have a partially
			// written run.json; skip it rather than fail the whole status.
			continue
		}
		if !isTerminalRunStatus(state.Status) {
			active++
		}
	}
	return active, nil
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case "completed", "cancelled", "failed":
		return true
	default:
		return false
	}
}

func inspectProjectMap(root string, paths artifacts.Paths, config configFile, configHash string, currentRunID string) (projectMapStatus, error) {
	status := projectMapStatus{
		Status:       "fresh",
		RunID:        currentRunID,
		SearchIndex:  "ready",
		StaleReasons: []string{},
	}

	for _, artifact := range requiredMapArtifacts(paths) {
		if filesystemRegularFile(artifact.path) {
			continue
		}
		status.StaleReasons = append(status.StaleReasons, "missing artifact: "+artifact.rel)
		if artifact.path == paths.SearchSQLite {
			status.SearchIndex = "missing"
		}
	}

	if filesystemRegularFile(paths.MapManifest) {
		mapManifest, err := readMapManifest(paths.MapManifest)
		if err != nil {
			status.StaleReasons = append(status.StaleReasons, "invalid artifact: map/manifest.json")
		} else {
			status.RunID = mapManifest.RunID
			status.FilesIndexed = mapManifest.FilesIndexed
			status.CodeItems = mapManifest.CodeIndex.Items
			if mapManifest.ConfigHash != "" && mapManifest.ConfigHash != configHash {
				status.StaleReasons = append(status.StaleReasons, "config changed: .heurema/pactum/config.yaml")
			}
		}
	}

	if filesystemRegularFile(paths.HashesJSONL) {
		reasons, err := hashStaleReasons(root, paths.HashesJSONL, config)
		if err != nil {
			return projectMapStatus{}, err
		}
		status.StaleReasons = append(status.StaleReasons, reasons...)
	}

	if len(status.StaleReasons) > 0 {
		status.Status = "stale"
	}
	return status, nil
}

type mapArtifact struct {
	rel  string
	path string
}

func requiredMapArtifacts(paths artifacts.Paths) []mapArtifact {
	return []mapArtifact{
		{rel: "map/manifest.json", path: paths.MapManifest},
		{rel: "map/files.jsonl", path: paths.FilesJSONL},
		{rel: "map/hashes.jsonl", path: paths.HashesJSONL},
		{rel: "map/code-items.jsonl", path: paths.CodeItemsJSONL},
		{rel: "map/repo-map.md", path: paths.RepoMap},
		{rel: "map/llms.txt", path: paths.LLMS},
		{rel: "map/search.sqlite", path: paths.SearchSQLite},
	}
}

func hashStaleReasons(root string, hashesPath string, config configFile) ([]string, error) {
	oldHashes, err := readHashRecords(hashesPath)
	if err != nil {
		return nil, fmt.Errorf("read hashes: %w", err)
	}
	scan, err := projectmap.Scan(root, projectmap.ScanOptions{
		MaxFileBytes:  int64(config.ProjectMap.MaxFileBytes),
		CodeIndexMode: codeindex.ModeOff,
	})
	if err != nil {
		return nil, err
	}

	currentHashes := make(map[string]string, len(scan.Hashes))
	for _, record := range scan.Hashes {
		currentHashes[record.Path] = record.SHA256
	}

	reasons := []string{}
	oldSeen := make(map[string]struct{}, len(oldHashes))
	for _, old := range oldHashes {
		oldSeen[old.Path] = struct{}{}
		if current, ok := currentHashes[old.Path]; ok {
			if current != old.SHA256 {
				reasons = append(reasons, "changed file: "+old.Path)
			}
			continue
		}

		path := filepath.Join(root, filepath.FromSlash(old.Path))
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			reasons = append(reasons, "missing file: "+old.Path)
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			reasons = append(reasons, "missing file: "+old.Path)
			continue
		}
		hash, err := fileSHA256(path)
		if err != nil {
			return nil, err
		}
		if hash != old.SHA256 {
			reasons = append(reasons, "changed file: "+old.Path)
		}
	}

	for _, current := range scan.Hashes {
		if _, ok := oldSeen[current.Path]; !ok {
			reasons = append(reasons, "new file: "+current.Path)
		}
	}

	return reasons, nil
}

func readHashRecords(path string) ([]projectmap.HashRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	records := []projectmap.HashRecord{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record projectmap.HashRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func isRegularFile(path string) bool {
	return activeStore.Exists(path)
}
