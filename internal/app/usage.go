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

// Effective-unit multipliers normalize each token class to a "fresh input"
// unit (= 1.0) using the providers' standard published price ratios, so a
// cache-heavy run reads far cheaper than its raw token count suggests. The
// ratios below are relative to fresh input:
//   - Anthropic: cache write 1.25x, cache read 0.1x, output 5x — the prompt
//     caching premium and the ~5x output:input ratio
//     (platform.claude.com/docs prompt caching + pricing).
//   - OpenAI/Codex: cached read 0.1x, output 5x; OpenAI charges no cache-write
//     premium, so writes price as fresh input (1.0x)
//     (developers.openai.com prompt caching).
//
// effective_units is a derived display metric only — it is never persisted in
// UsageRecord. It is computed per record from the record's own provider, so a
// breakdown aggregating several providers sums each record's units.
const (
	effectiveFreshInputMultiplier = 1.0
	effectiveOutputMultiplier     = 5.0
	effectiveCacheReadMultiplier  = 0.1
	anthropicCacheWriteMultiplier = 1.25
	// OpenAI/Codex cache writes carry no premium: they price as fresh input.
	openAICacheWriteMultiplier = 1.0
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
	// EffectiveUnits is the provider-weighted cost proxy described above. It is
	// derived on demand and never written to the ledger.
	EffectiveUnits float64 `json:"effective_units"`
}

type usageBreakdown struct {
	Run      string `json:"run,omitempty"`
	Stage    string `json:"stage,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	Attempt  string `json:"attempt_id,omitempty"`
	Calls    int    `json:"calls"`
	// EffectiveUnitsUnavailableCalls counts captured calls whose provider has no
	// effective-unit multipliers: their raw tokens still total, but they add no
	// effective units.
	CapturedCalls                  int     `json:"captured_calls"`
	UncapturedCalls                int     `json:"uncaptured_calls"`
	EffectiveUnitsUnavailableCalls int     `json:"effective_units_unavailable_calls"`
	CacheReadRatio                 float64 `json:"cache_read_ratio"`
	usageCounts
}

type usageResponse struct {
	Schema                         string           `json:"schema"`
	RunID                          string           `json:"run_id"`
	Calls                          int              `json:"calls"`
	CapturedCalls                  int              `json:"captured_calls"`
	UncapturedCalls                int              `json:"uncaptured_calls"`
	EffectiveUnitsUnavailableCalls int              `json:"effective_units_unavailable_calls"`
	CacheReadRatio                 float64          `json:"cache_read_ratio"`
	Total                          usageCounts      `json:"total"`
	ByStage                        []usageBreakdown `json:"by_stage"`
	ByAgent                        []usageBreakdown `json:"by_agent"`
	ByAttempt                      []usageBreakdown `json:"by_attempt"`
}

// usageWorkspaceResponse is the cross-run aggregate produced by `pactum usage
// --all`: a derived view recomputed on demand from the per-run usage.jsonl
// ledgers, never an authoritative store. Runs counts only the run ledgers that
// contributed at least one usage record.
type usageWorkspaceResponse struct {
	Schema                         string           `json:"schema"`
	Runs                           int              `json:"runs"`
	Calls                          int              `json:"calls"`
	CapturedCalls                  int              `json:"captured_calls"`
	UncapturedCalls                int              `json:"uncaptured_calls"`
	EffectiveUnitsUnavailableCalls int              `json:"effective_units_unavailable_calls"`
	CacheReadRatio                 float64          `json:"cache_read_ratio"`
	Total                          usageCounts      `json:"total"`
	ByRun                          []usageBreakdown `json:"by_run"`
	ByStage                        []usageBreakdown `json:"by_stage"`
	ByAgent                        []usageBreakdown `json:"by_agent"`
	ByModel                        []usageBreakdown `json:"by_model"`
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
// workspace reports a clean zero result. A positive top caps only the sorted
// by-run list; the workspace totals and other breakdowns still aggregate every
// run.
func (a App) UsageAll(stdout io.Writer, jsonOutput bool, top int) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	response, err := workspaceUsage(paths)
	if err != nil {
		return err
	}
	if top > 0 && len(response.ByRun) > top {
		response.ByRun = response.ByRun[:top]
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
			addRecordToUsageTotals(&response.Calls, &response.CapturedCalls, &response.UncapturedCalls, &response.EffectiveUnitsUnavailableCalls, &response.Total, record)
			accumulateUsageByRun(byRun, runID, record)
			accumulateUsageByStage(byStage, record)
			accumulateUsageByAgent(byAgent, record)
			accumulateUsageByModel(byModel, record)
		}
	}
	response.CacheReadRatio = cacheReadRatio(response.Total)
	response.ByRun = sortUsageBreakdownsByTotalTokensDesc(byRun)
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
		Schema:    usageResponseSchema,
		RunID:     runID,
		ByStage:   []usageBreakdown{},
		ByAgent:   []usageBreakdown{},
		ByAttempt: []usageBreakdown{},
	}
	byStage := map[string]*usageBreakdown{}
	byAgent := map[string]*usageBreakdown{}
	byAttempt := map[string]*usageBreakdown{}
	for _, record := range records {
		addRecordToUsageTotals(&response.Calls, &response.CapturedCalls, &response.UncapturedCalls, &response.EffectiveUnitsUnavailableCalls, &response.Total, record)
		accumulateUsageByStage(byStage, record)
		accumulateUsageByAgent(byAgent, record)
		accumulateUsageByAttempt(byAttempt, record)
	}
	response.CacheReadRatio = cacheReadRatio(response.Total)
	response.ByStage = sortedUsageBreakdowns(byStage)
	response.ByAgent = sortedUsageBreakdowns(byAgent)
	response.ByAttempt = sortedUsageBreakdowns(byAttempt)
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

func accumulateUsageByAttempt(byAttempt map[string]*usageBreakdown, record UsageRecord) {
	attemptID := strings.TrimSpace(record.AttemptID)
	if attemptID == "" {
		attemptID = "unknown"
	}
	stage := normalizeUsageStage(record.Stage)
	// Attempt IDs restart at 1 per stage (execute vs review-fix keep separate
	// attempts directories), so attempt_001 collides across stages. Key by
	// stage+attempt — the same disambiguation usageRecordID uses — so each
	// distinct attempt is its own row.
	key := stage + "\x00" + attemptID
	attempt := byAttempt[key]
	if attempt == nil {
		attempt = &usageBreakdown{
			Attempt:  attemptID,
			Stage:    stage,
			Agent:    record.Agent,
			Provider: record.Provider,
		}
		byAttempt[key] = attempt
	}
	addRecordToUsageBreakdown(attempt, record)
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

func addRecordToUsageTotals(calls *int, capturedCalls *int, uncapturedCalls *int, unavailableCalls *int, counts *usageCounts, record UsageRecord) {
	*calls += 1
	if record.Captured {
		*capturedCalls += 1
	} else {
		*uncapturedCalls += 1
	}
	if !addUsageCounts(counts, record) {
		*unavailableCalls += 1
	}
}

func addRecordToUsageBreakdown(breakdown *usageBreakdown, record UsageRecord) {
	breakdown.Calls++
	if record.Captured {
		breakdown.CapturedCalls++
	} else {
		breakdown.UncapturedCalls++
	}
	if !addUsageCounts(&breakdown.usageCounts, record) {
		breakdown.EffectiveUnitsUnavailableCalls++
	}
	breakdown.CacheReadRatio = cacheReadRatio(breakdown.usageCounts)
}

// addUsageCounts folds one record's tokens and effective units into counts. An
// uncaptured record is unknown usage: it contributes nothing (it is tracked
// separately as an uncaptured call), so this returns true without touching the
// counts. A captured record adds its raw tokens; it adds effective units only
// when its provider has multipliers — when it does not, the raw tokens still
// count but the function returns false so the caller records an
// effective-units-unavailable call.
func addUsageCounts(counts *usageCounts, record UsageRecord) bool {
	if !record.Captured {
		return true
	}
	counts.InputTokens += record.InputTokens
	counts.OutputTokens += record.OutputTokens
	counts.TotalTokens += record.TotalTokens
	counts.CacheReadTokens += record.CacheReadTokens
	counts.CacheCreationTokens += record.CacheCreationTokens
	counts.ReasoningTokens += record.ReasoningTokens
	units, ok := recordEffectiveUnits(record)
	if !ok {
		return false
	}
	counts.EffectiveUnits += units
	return true
}

// recordEffectiveUnits weights a captured record's token classes into the
// provider-neutral effective-unit proxy. It reports false for providers with no
// multipliers so the caller can flag the row as effective-units-unavailable.
func recordEffectiveUnits(record UsageRecord) (float64, bool) {
	write := float64(record.CacheCreationTokens)
	read := float64(record.CacheReadTokens)
	// Both providers normalize InputTokens to include cache reads (and, for
	// Anthropic, cache writes), so fresh input is what remains.
	fresh := float64(record.InputTokens) - read - write
	if fresh < 0 {
		fresh = 0
	}
	output := float64(record.OutputTokens)
	base := fresh*effectiveFreshInputMultiplier + read*effectiveCacheReadMultiplier + output*effectiveOutputMultiplier
	switch record.Provider {
	case "anthropic":
		return base + write*anthropicCacheWriteMultiplier, true
	case "codex", "openai":
		return base + write*openAICacheWriteMultiplier, true
	default:
		return 0, false
	}
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

// sortUsageBreakdownsByTotalTokensDesc orders run rows by captured total tokens
// descending so the heaviest runs lead; ties fall back to the deterministic key
// order (run id ascending) for stable output.
func sortUsageBreakdownsByTotalTokensDesc(values map[string]*usageBreakdown) []usageBreakdown {
	breakdowns := sortedUsageBreakdowns(values)
	sort.SliceStable(breakdowns, func(i, j int) bool {
		return breakdowns[i].TotalTokens > breakdowns[j].TotalTokens
	})
	return breakdowns
}

func writeUsage(stdout io.Writer, response usageResponse) {
	fmt.Fprintln(stdout, "Pactum usage")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	writeUsageSummary(stdout, response.Calls, response.CapturedCalls, response.UncapturedCalls, response.EffectiveUnitsUnavailableCalls, response.Total, response.CacheReadRatio)
	writeUsageBreakdownGroup(stdout, "By stage:", response.ByStage, func(item usageBreakdown) string { return item.Stage })
	writeUsageBreakdownGroup(stdout, "By agent:", response.ByAgent, func(item usageBreakdown) string { return item.Agent })
	writeUsageBreakdownGroup(stdout, "By attempt:", response.ByAttempt, func(item usageBreakdown) string {
		return fmt.Sprintf("%s (stage=%s, agent=%s, provider=%s)", item.Attempt, item.Stage, item.Agent, item.Provider)
	})
}

func writeWorkspaceUsage(stdout io.Writer, response usageWorkspaceResponse) {
	fmt.Fprintln(stdout, "Pactum usage (workspace)")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Workspace:")
	fmt.Fprintf(stdout, "  runs: %d\n", response.Runs)
	writeUsageSummary(stdout, response.Calls, response.CapturedCalls, response.UncapturedCalls, response.EffectiveUnitsUnavailableCalls, response.Total, response.CacheReadRatio)
	writeUsageBreakdownGroup(stdout, "By run:", response.ByRun, func(item usageBreakdown) string { return item.Run })
	writeUsageBreakdownGroup(stdout, "By stage:", response.ByStage, func(item usageBreakdown) string { return item.Stage })
	writeUsageBreakdownGroup(stdout, "By agent:", response.ByAgent, func(item usageBreakdown) string {
		return fmt.Sprintf("%s (provider=%s)", item.Agent, item.Provider)
	})
	writeUsageBreakdownGroup(stdout, "By model:", response.ByModel, func(item usageBreakdown) string { return item.Model })
}

// writeUsageSummary renders the shared call/token summary that leads both the
// per-run and workspace human output: call counts, token classes, the derived
// effective units, and the cache hit rate.
func writeUsageSummary(stdout io.Writer, calls, capturedCalls, uncapturedCalls, unavailableCalls int, counts usageCounts, cacheHitRate float64) {
	fmt.Fprintf(stdout, "  calls: %d\n", calls)
	fmt.Fprintf(stdout, "  captured calls: %d\n", capturedCalls)
	fmt.Fprintf(stdout, "  uncaptured calls: %d\n", uncapturedCalls)
	if unavailableCalls > 0 {
		fmt.Fprintf(stdout, "  effective units unavailable calls: %d\n", unavailableCalls)
	}
	writeUsageCounts(stdout, "  ", counts)
	fmt.Fprintf(stdout, "  effective units: %.2f\n", counts.EffectiveUnits)
	fmt.Fprintf(stdout, "  cache hit rate: %.2f%%\n", cacheHitRate*100)
}

func writeUsageCounts(stdout io.Writer, indent string, counts usageCounts) {
	fmt.Fprintf(stdout, "%sinput tokens: %d\n", indent, counts.InputTokens)
	fmt.Fprintf(stdout, "%soutput tokens: %d\n", indent, counts.OutputTokens)
	fmt.Fprintf(stdout, "%stotal tokens: %d\n", indent, counts.TotalTokens)
	fmt.Fprintf(stdout, "%scache read tokens: %d\n", indent, counts.CacheReadTokens)
	fmt.Fprintf(stdout, "%scache creation tokens: %d\n", indent, counts.CacheCreationTokens)
	fmt.Fprintf(stdout, "%sreasoning tokens: %d\n", indent, counts.ReasoningTokens)
}

func writeUsageBreakdownGroup(stdout io.Writer, heading string, items []usageBreakdown, label func(usageBreakdown) string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, heading)
	for _, item := range items {
		fmt.Fprintf(stdout, "  %s: %s\n", label(item), usageBreakdownDetail(item))
	}
}

// usageBreakdownDetail renders one breakdown row's token detail. A row with no
// captured calls is unknown usage — the agent reported nothing — so it is
// annotated honestly rather than printed as a misleading zero. Captured rows
// (including genuine zero-token rows) print their counts, effective units, and,
// where input exists, the cache hit rate; a mixed row whose tokens cover only
// some of its calls flags the uncaptured remainder, and a trailing note flags
// any calls whose provider has no effective-unit multipliers.
func usageBreakdownDetail(item usageBreakdown) string {
	if item.Calls > 0 && item.CapturedCalls == 0 {
		return fmt.Sprintf("calls=%d, usage not reported by the agent", item.Calls)
	}
	detail := fmt.Sprintf("calls=%d captured=%d total_tokens=%d input_tokens=%d output_tokens=%d cache_read_tokens=%d effective_units=%.2f",
		item.Calls, item.CapturedCalls, item.TotalTokens, item.InputTokens, item.OutputTokens, item.CacheReadTokens, item.EffectiveUnits)
	if item.InputTokens > 0 {
		detail += fmt.Sprintf(" cache_hit_rate=%.2f%%", item.CacheReadRatio*100)
	}
	if item.UncapturedCalls > 0 {
		detail += fmt.Sprintf(" (%d calls: usage not reported)", item.UncapturedCalls)
	}
	if item.EffectiveUnitsUnavailableCalls > 0 {
		detail += " effective units unavailable"
	}
	return detail
}
