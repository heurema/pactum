package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

const (
	usageRecordSchemaVersion     = 1
	usageResponseSchema          = "pactum.usage.v1"
	usageWorkspaceResponseSchema = "pactum.usage.workspace.v1"
)

type UsageRecord struct {
	SchemaVersion int    `json:"schema_version"`
	RecordID      string `json:"record_id"`
	DedupKey      string `json:"dedup_key,omitempty"`
	RunID         string `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	Stage         string `json:"stage"`
	CreatedAt     string `json:"created_at"`
	Provider      string `json:"provider"`
	Agent         string `json:"agent"`
	AgentName     string `json:"agent_name,omitempty"`
	RequestModel  string `json:"request_model,omitempty"`
	ResponseModel string `json:"response_model,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`

	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int64 `json:"cache_creation_tokens,omitempty"`
	ReasoningTokens     int64 `json:"reasoning_tokens,omitempty"`

	Captured bool            `json:"captured"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

type usageCounts struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	ReasoningTokens     int64 `json:"reasoning_tokens"`
}

type usageBreakdown struct {
	Run             string  `json:"run,omitempty"`
	Stage           string  `json:"stage,omitempty"`
	Agent           string  `json:"agent,omitempty"`
	Model           string  `json:"model,omitempty"`
	Provider        string  `json:"provider,omitempty"`
	Calls           int     `json:"calls"`
	CapturedCalls   int     `json:"captured_calls"`
	UncapturedCalls int     `json:"uncaptured_calls"`
	CacheReadRatio  float64 `json:"cache_read_ratio"`
	usageCounts
}

type usageResponse struct {
	Schema          string           `json:"schema"`
	RunID           string           `json:"run_id"`
	Calls           int              `json:"calls"`
	CapturedCalls   int              `json:"captured_calls"`
	UncapturedCalls int              `json:"uncaptured_calls"`
	CacheReadRatio  float64          `json:"cache_read_ratio"`
	Total           usageCounts      `json:"total"`
	ByStage         []usageBreakdown `json:"by_stage"`
	ByAgent         []usageBreakdown `json:"by_agent"`
}

// usageWorkspaceResponse is the cross-run aggregate produced by `pactum usage
// --all`: a derived view recomputed on demand from the per-run usage.jsonl
// ledgers, never an authoritative store. Runs counts only the run ledgers that
// contributed at least one usage record.
type usageWorkspaceResponse struct {
	Schema          string           `json:"schema"`
	Runs            int              `json:"runs"`
	Calls           int              `json:"calls"`
	CapturedCalls   int              `json:"captured_calls"`
	UncapturedCalls int              `json:"uncaptured_calls"`
	CacheReadRatio  float64          `json:"cache_read_ratio"`
	Total           usageCounts      `json:"total"`
	ByRun           []usageBreakdown `json:"by_run"`
	ByStage         []usageBreakdown `json:"by_stage"`
	ByAgent         []usageBreakdown `json:"by_agent"`
	ByModel         []usageBreakdown `json:"by_model"`
}

func (a App) Usage(stdout io.Writer, runID string, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !runExists(paths, runID) {
		return fmt.Errorf("run not found: %s", runID)
	}

	response, err := usageForRun(paths, runID)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeUsage(stdout, response)
	return nil
}

// UsageAll reports the workspace-wide cross-run token aggregate. It reads the
// per-run usage ledgers as a derived view and is best-effort: a missing or
// corrupt ledger contributes nothing and is skipped, never fatal. An empty
// workspace reports a clean zero result.
func (a App) UsageAll(stdout io.Writer, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	response, err := workspaceUsage(paths)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeWorkspaceUsage(stdout, response)
	return nil
}

func appendUsageRecord(root string, runID string, attemptID string, stage string, agentName string, requestModel string, agent agents.AgentDescriptor, usage agents.TokenUsage, createdAt string) error {
	paths := artifacts.New(root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	if err := activeStore.MkdirAll(runPaths.LedgerDir); err != nil {
		return err
	}
	record := usageRecordFromRunResult(runID, attemptID, stage, agentName, requestModel, agent, usage, createdAt)
	return appendJSONLine(runPaths.UsageJSONL, record)
}

func usageRecordFromRunResult(runID string, attemptID string, stage string, agentName string, requestModel string, agent agents.AgentDescriptor, usage agents.TokenUsage, createdAt string) UsageRecord {
	raw := append(json.RawMessage(nil), usage.Raw...)
	return UsageRecord{
		SchemaVersion:       usageRecordSchemaVersion,
		RecordID:            usageRecordID(runID, attemptID, stage),
		RunID:               runID,
		AttemptID:           attemptID,
		Stage:               normalizeUsageStage(stage),
		CreatedAt:           usageCreatedAt(createdAt),
		Provider:            providerForAgent(agent),
		Agent:               agent.Name,
		AgentName:           strings.TrimSpace(agentName),
		RequestModel:        strings.TrimSpace(requestModel),
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		TotalTokens:         usage.TotalTokens,
		CacheReadTokens:     usage.CacheReadTokens,
		CacheCreationTokens: usage.CacheCreationTokens,
		ReasoningTokens:     usage.ReasoningTokens,
		Captured:            usage.Captured,
		Raw:                 raw,
	}
}

func usageCreatedAt(createdAt string) string {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	if parsed, err := time.Parse(time.RFC3339, createdAt); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return createdAt
}

func usageRecordID(runID string, attemptID string, stage string) string {
	sum := sha256.Sum256([]byte(runID + "\x00" + attemptID + "\x00" + normalizeUsageStage(stage)))
	return "usage_" + hex.EncodeToString(sum[:8])
}

func providerForAgent(agent agents.AgentDescriptor) string {
	switch agent.Name {
	case agents.BuiltinCodex:
		return "codex"
	case agents.BuiltinClaude:
		return "anthropic"
	default:
		if strings.TrimSpace(agent.Name) != "" {
			return agent.Name
		}
		return "unknown"
	}
}

func normalizeUsageStage(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return "unknown"
	}
	return stage
}

func usageForRun(paths artifacts.Paths, runID string) (usageResponse, error) {
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, err := readUsageRecords(runPaths.UsageJSONL)
	if err != nil {
		return usageResponse{}, err
	}
	return summarizeUsage(runID, records), nil
}

// workspaceUsage folds every run's usage ledger into one cross-run aggregate.
// Each run's ledger is read best-effort (readUsageRecords swallows missing and
// corrupt input), so a degraded run simply contributes nothing. Runs with no
// usage records are skipped and do not count toward the run total.
func workspaceUsage(paths artifacts.Paths) (usageWorkspaceResponse, error) {
	runIDs, err := listRunIDs(paths)
	if err != nil {
		return usageWorkspaceResponse{}, err
	}
	response := usageWorkspaceResponse{
		Schema:  usageWorkspaceResponseSchema,
		ByRun:   []usageBreakdown{},
		ByStage: []usageBreakdown{},
		ByAgent: []usageBreakdown{},
		ByModel: []usageBreakdown{},
	}
	byRun := map[string]*usageBreakdown{}
	byStage := map[string]*usageBreakdown{}
	byAgent := map[string]*usageBreakdown{}
	byModel := map[string]*usageBreakdown{}
	for _, runID := range runIDs {
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		records, _ := readUsageRecords(runPaths.UsageJSONL)
		if len(records) == 0 {
			continue
		}
		response.Runs++
		for _, record := range records {
			addRecordToUsageTotals(&response.Calls, &response.CapturedCalls, &response.UncapturedCalls, &response.Total, record)
			accumulateUsageByRun(byRun, runID, record)
			accumulateUsageByStage(byStage, record)
			accumulateUsageByAgent(byAgent, record)
			accumulateUsageByModel(byModel, record)
		}
	}
	response.CacheReadRatio = cacheReadRatio(response.Total)
	response.ByRun = sortedUsageBreakdowns(byRun)
	response.ByStage = sortedUsageBreakdowns(byStage)
	response.ByAgent = sortedUsageBreakdowns(byAgent)
	response.ByModel = sortedUsageBreakdowns(byModel)
	return response, nil
}

// readUsageRecords is best-effort: usage accounting must never fail status or
// `pactum usage`. An absent or unreadable ledger yields no records, a corrupt or
// oversized line is skipped, and a scan error degrades to the records read so
// far — none of these ever propagates an error to the caller.
func readUsageRecords(path string) ([]UsageRecord, error) {
	file, err := activeStore.Open(path)
	if err != nil {
		return []UsageRecord{}, nil
	}
	defer file.Close()

	records := []UsageRecord{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record UsageRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func summarizeUsage(runID string, records []UsageRecord) usageResponse {
	response := usageResponse{
		Schema:  usageResponseSchema,
		RunID:   runID,
		ByStage: []usageBreakdown{},
		ByAgent: []usageBreakdown{},
	}
	byStage := map[string]*usageBreakdown{}
	byAgent := map[string]*usageBreakdown{}
	for _, record := range records {
		addRecordToUsageTotals(&response.Calls, &response.CapturedCalls, &response.UncapturedCalls, &response.Total, record)
		accumulateUsageByStage(byStage, record)
		accumulateUsageByAgent(byAgent, record)
	}
	response.CacheReadRatio = cacheReadRatio(response.Total)
	response.ByStage = sortedUsageBreakdowns(byStage)
	response.ByAgent = sortedUsageBreakdowns(byAgent)
	return response
}

func accumulateUsageByStage(byStage map[string]*usageBreakdown, record UsageRecord) {
	key := normalizeUsageStage(record.Stage)
	stage := byStage[key]
	if stage == nil {
		stage = &usageBreakdown{Stage: key}
		byStage[key] = stage
	}
	addRecordToUsageBreakdown(stage, record)
}

func accumulateUsageByAgent(byAgent map[string]*usageBreakdown, record UsageRecord) {
	key := record.Agent
	if key == "" {
		key = "unknown"
	}
	agent := byAgent[key]
	if agent == nil {
		agent = &usageBreakdown{Agent: key, Provider: record.Provider}
		byAgent[key] = agent
	}
	addRecordToUsageBreakdown(agent, record)
	if agent.Provider == "" {
		agent.Provider = record.Provider
	}
}

func accumulateUsageByRun(byRun map[string]*usageBreakdown, runID string, record UsageRecord) {
	run := byRun[runID]
	if run == nil {
		run = &usageBreakdown{Run: runID}
		byRun[runID] = run
	}
	addRecordToUsageBreakdown(run, record)
}

func accumulateUsageByModel(byModel map[string]*usageBreakdown, record UsageRecord) {
	key := usageRecordModel(record)
	model := byModel[key]
	if model == nil {
		model = &usageBreakdown{Model: key}
		byModel[key] = model
	}
	addRecordToUsageBreakdown(model, record)
}

// usageRecordModel resolves the model dimension for a record: the response
// model when set (reserved for future use — the current recording path does
// not populate ResponseModel, so this keys on the requested model today), else
// the requested model, else "unknown".
func usageRecordModel(record UsageRecord) string {
	if model := strings.TrimSpace(record.ResponseModel); model != "" {
		return model
	}
	if model := strings.TrimSpace(record.RequestModel); model != "" {
		return model
	}
	return "unknown"
}

func addRecordToUsageTotals(calls *int, capturedCalls *int, uncapturedCalls *int, counts *usageCounts, record UsageRecord) {
	*calls += 1
	if record.Captured {
		*capturedCalls += 1
	} else {
		*uncapturedCalls += 1
	}
	addUsageCounts(counts, record)
}

func addRecordToUsageBreakdown(breakdown *usageBreakdown, record UsageRecord) {
	breakdown.Calls++
	if record.Captured {
		breakdown.CapturedCalls++
	} else {
		breakdown.UncapturedCalls++
	}
	addUsageCounts(&breakdown.usageCounts, record)
	breakdown.CacheReadRatio = cacheReadRatio(breakdown.usageCounts)
}

func addUsageCounts(counts *usageCounts, record UsageRecord) {
	counts.InputTokens += record.InputTokens
	counts.OutputTokens += record.OutputTokens
	counts.TotalTokens += record.TotalTokens
	counts.CacheReadTokens += record.CacheReadTokens
	counts.CacheCreationTokens += record.CacheCreationTokens
	counts.ReasoningTokens += record.ReasoningTokens
}

func cacheReadRatio(counts usageCounts) float64 {
	if counts.InputTokens <= 0 {
		return 0
	}
	return float64(counts.CacheReadTokens) / float64(counts.InputTokens)
}

func sortedUsageBreakdowns(values map[string]*usageBreakdown) []usageBreakdown {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	breakdowns := make([]usageBreakdown, 0, len(keys))
	for _, key := range keys {
		breakdowns = append(breakdowns, *values[key])
	}
	return breakdowns
}

func writeUsage(stdout io.Writer, response usageResponse) {
	fmt.Fprintln(stdout, "Pactum usage")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	fmt.Fprintf(stdout, "  calls: %d\n", response.Calls)
	fmt.Fprintf(stdout, "  captured calls: %d\n", response.CapturedCalls)
	fmt.Fprintf(stdout, "  uncaptured calls: %d\n", response.UncapturedCalls)
	writeUsageCounts(stdout, "  ", response.Total)
	fmt.Fprintf(stdout, "  cache read ratio: %.2f%%\n", response.CacheReadRatio*100)
	if len(response.ByStage) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "By stage:")
		for _, item := range response.ByStage {
			fmt.Fprintf(stdout, "  %s: calls=%d captured=%d total_tokens=%d input_tokens=%d output_tokens=%d\n", item.Stage, item.Calls, item.CapturedCalls, item.TotalTokens, item.InputTokens, item.OutputTokens)
		}
	}
	if len(response.ByAgent) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "By agent:")
		for _, item := range response.ByAgent {
			fmt.Fprintf(stdout, "  %s: provider=%s calls=%d captured=%d total_tokens=%d input_tokens=%d output_tokens=%d\n", item.Agent, item.Provider, item.Calls, item.CapturedCalls, item.TotalTokens, item.InputTokens, item.OutputTokens)
		}
	}
}

func writeUsageCounts(stdout io.Writer, indent string, counts usageCounts) {
	fmt.Fprintf(stdout, "%sinput tokens: %d\n", indent, counts.InputTokens)
	fmt.Fprintf(stdout, "%soutput tokens: %d\n", indent, counts.OutputTokens)
	fmt.Fprintf(stdout, "%stotal tokens: %d\n", indent, counts.TotalTokens)
	fmt.Fprintf(stdout, "%scache read tokens: %d\n", indent, counts.CacheReadTokens)
	fmt.Fprintf(stdout, "%scache creation tokens: %d\n", indent, counts.CacheCreationTokens)
	fmt.Fprintf(stdout, "%sreasoning tokens: %d\n", indent, counts.ReasoningTokens)
}

func writeWorkspaceUsage(stdout io.Writer, response usageWorkspaceResponse) {
	fmt.Fprintln(stdout, "Pactum usage (workspace)")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Workspace:")
	fmt.Fprintf(stdout, "  runs: %d\n", response.Runs)
	fmt.Fprintf(stdout, "  calls: %d\n", response.Calls)
	fmt.Fprintf(stdout, "  captured calls: %d\n", response.CapturedCalls)
	fmt.Fprintf(stdout, "  uncaptured calls: %d\n", response.UncapturedCalls)
	writeUsageCounts(stdout, "  ", response.Total)
	fmt.Fprintf(stdout, "  cache read ratio: %.2f%%\n", response.CacheReadRatio*100)
	writeWorkspaceUsageGroup(stdout, "By run:", response.ByRun, func(item usageBreakdown) string { return item.Run })
	writeWorkspaceUsageGroup(stdout, "By stage:", response.ByStage, func(item usageBreakdown) string { return item.Stage })
	writeWorkspaceUsageGroup(stdout, "By agent:", response.ByAgent, func(item usageBreakdown) string {
		return fmt.Sprintf("%s (provider=%s)", item.Agent, item.Provider)
	})
	writeWorkspaceUsageGroup(stdout, "By model:", response.ByModel, func(item usageBreakdown) string { return item.Model })
}

func writeWorkspaceUsageGroup(stdout io.Writer, heading string, items []usageBreakdown, label func(usageBreakdown) string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, heading)
	for _, item := range items {
		fmt.Fprintf(stdout, "  %s: calls=%d captured=%d total_tokens=%d input_tokens=%d output_tokens=%d cache_read_tokens=%d\n",
			label(item), item.Calls, item.CapturedCalls, item.TotalTokens, item.InputTokens, item.OutputTokens, item.CacheReadTokens)
	}
}
