package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/heurema/pactum/internal/artifacts"
)

const statusSchema = "pactum.status.v1alpha1"

type statusResponse struct {
	Schema      string       `json:"schema"`
	Initialized bool         `json:"initialized"`
	RepoRoot    string       `json:"repo_root,omitempty"`
	Workspace   string       `json:"workspace,omitempty"`
	Runs        runsStatus   `json:"runs,omitempty"`
	Memory      memoryStatus `json:"memory,omitempty"`
	Usage       usageStatus  `json:"usage,omitempty"`
	Message     string       `json:"message,omitempty"`
	// Next holds the concrete runnable commands for the current run's stage.
	// Runs.NextCommand predates it and is kept for compatibility.
	Next []string `json:"next"`
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
		Fix         string `json:"fix"`
	}{
		Initialized: false,
		Message:     "Pactum is not initialized. Run: pactum init",
		Fix:         "pactum init",
	})
}

func (a App) workspaceStatus(root string) (statusResponse, error) {
	paths := artifacts.New(root)
	activeRuns, err := countActiveRuns(paths)
	if err != nil {
		return statusResponse{}, err
	}
	memoryItems, err := countMemoryItems(paths)
	if err != nil {
		return statusResponse{}, err
	}

	runs := runsStatus{Active: activeRuns}
	next := []string{}
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
		switch {
		case currentValid:
			runs.NextCommand = nextCommandForStatus(paths, currentID, deriveRunStatus(paths, currentID))
			next = nextCommandsForRun(paths, currentID)
		case len(active) == 1:
			runs.NextCommand = nextCommandForStatus(paths, active[0], deriveRunStatus(paths, active[0]))
			next = nextCommandsForRun(paths, active[0])
		default:
			runs.NextCommand = "pactum task use " + latestID
			next = []string{"pactum task use " + latestID}
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
		Runs:        runs,
		Memory:      memoryStatus{Items: memoryItems, Stale: 0},
		Usage:       usage,
		Next:        next,
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

func isRegularFile(path string) bool {
	return activeStore.Exists(path)
}
