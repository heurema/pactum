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

type statusResponse struct {
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
	Active int `json:"active"`
}

type memoryStatus struct {
	Items int `json:"items"`
	Stale int `json:"stale"`
}

type usageStatus struct {
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

func writeStatusNotInitialized(stdout io.Writer) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
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
	configHash, err := fileSHA256(paths.Config)
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

	return statusResponse{
		Initialized: true,
		RepoRoot:    root,
		Workspace:   paths.Workspace,
		ProjectMap:  mapStatus,
		Runs:        runsStatus{Active: activeRuns},
		Memory:      memoryStatus{Items: memoryItems, Stale: 0},
		Usage:       usageStatus{TotalTokens: 0, EstimatedCostUSD: 0},
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
	entries, err := os.ReadDir(paths.RunsDir)
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
		data, err := os.ReadFile(runPath)
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
			return 0, err
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
		if isRegularFile(artifact.path) {
			continue
		}
		status.StaleReasons = append(status.StaleReasons, "missing artifact: "+artifact.rel)
		if artifact.path == paths.SearchSQLite {
			status.SearchIndex = "missing"
		}
	}

	if isRegularFile(paths.MapManifest) {
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

	if isRegularFile(paths.HashesJSONL) {
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
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
