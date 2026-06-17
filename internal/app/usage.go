package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

const (
	usageRecordSchemaVersion = 1
	usageResponseSchema      = "pactum.usage.v1alpha1"
	usageSummarySchema       = "pactum.usage_summary.v1alpha1"
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

// --- pactum.usage_summary.v1alpha1 ---

type usageSummaryScope struct {
	Kind  string  `json:"kind"`
	RunID *string `json:"run_id"`
}

type uncapturedSource struct {
	Provider string `json:"provider"`
	Stage    string `json:"stage"`
	Records  int    `json:"records"`
}

type usageSummaryCoverage struct {
	Records            int                `json:"records"`
	CapturedRecords    int                `json:"captured_records"`
	UncapturedRecords  int                `json:"uncaptured_records"`
	Complete           bool               `json:"complete"`
	Uncaptured         []uncapturedSource `json:"uncaptured"`
	UnparseableRecords int                `json:"unparseable_records"`
}

type usageSummaryTotals struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
	CapturedOnly bool  `json:"captured_only"`
	LowerBound   bool  `json:"lower_bound"`
}

type usageSummaryGroup struct {
	By              string `json:"by"`
	Key             string `json:"key"`
	label           string // human display label; absent from JSON
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	TotalTokens     int64  `json:"total_tokens"`
	Records         int    `json:"records"`
	CapturedRecords int    `json:"captured_records"`
}

type usageSummaryResponse struct {
	Schema   string               `json:"schema"`
	Scope    usageSummaryScope    `json:"scope"`
	Coverage usageSummaryCoverage `json:"coverage"`
	Totals   usageSummaryTotals   `json:"totals"`
	Groups   []usageSummaryGroup  `json:"groups"`
	Warnings []string             `json:"warnings"`
}

func (a App) Usage(stdout io.Writer, runID string, by string, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !runExists(paths, runID) {
		return fmt.Errorf("run not found: %s", runID)
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, warnings, unparseable, err := readUsageSummaryRecords(runPaths.UsageJSONL)
	if err != nil {
		return err
	}

	runIDVal := runID
	scope := usageSummaryScope{Kind: "run", RunID: &runIDVal}
	response := buildUsageSummary(scope, by, records, warnings, unparseable)

	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeUsageSummaryHuman(stdout, response, by)
	return nil
}

func (a App) UsageAll(stdout io.Writer, by string, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}

	runIDs, err := listRunIDs(paths)
	if err != nil {
		return err
	}

	var allRecords []UsageRecord
	var allWarnings []string
	allUnparseable := 0

	for _, id := range runIDs {
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, id))
		records, warnings, unparseable, err := readUsageSummaryRecords(runPaths.UsageJSONL)
		if err != nil {
			return err
		}
		allRecords = append(allRecords, records...)
		allWarnings = append(allWarnings, warnings...)
		allUnparseable += unparseable
	}

	scope := usageSummaryScope{Kind: "all", RunID: nil}
	response := buildUsageSummary(scope, by, allRecords, allWarnings, allUnparseable)

	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeUsageSummaryHuman(stdout, response, by)
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

// readUsageRecords is best-effort: usage accounting must never fail status or
// internal summaries. An absent or unreadable ledger yields no records, a corrupt
// or oversized line is skipped, and a scan error degrades to the records read so
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

// readUsageSummaryRecords reads usage records for `pactum usage`. The final
// line is always treated as potentially torn: if it fails JSON parsing it is
// skipped, a warning is added, and it is counted in unparseable. Non-final
// lines that fail parsing return an error. A missing file returns zero records
// and a warning instead of an error.
func readUsageSummaryRecords(path string) (records []UsageRecord, warnings []string, unparseable int, err error) {
	data, readErr := activeStore.ReadBytes(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, []string{fmt.Sprintf("no usage ledger: %s", filepath.Base(path))}, 0, nil
		}
		return nil, nil, 0, readErr
	}

	lines := splitJSONLLines(string(data))
	if len(lines) == 0 {
		return nil, nil, 0, nil
	}

	records = make([]UsageRecord, 0, len(lines))
	for i, line := range lines[:len(lines)-1] {
		var r UsageRecord
		if jsonErr := json.Unmarshal([]byte(line), &r); jsonErr != nil {
			return nil, nil, 0, fmt.Errorf("malformed usage record at line %d in %s: %w", i+1, filepath.Base(path), jsonErr)
		}
		records = append(records, r)
	}

	var last UsageRecord
	if jsonErr := json.Unmarshal([]byte(lines[len(lines)-1]), &last); jsonErr != nil {
		w := fmt.Sprintf("skipped unparseable final line in %s (possibly torn)", filepath.Base(path))
		return records, []string{w}, 1, nil
	}
	return append(records, last), nil, 0, nil
}

func splitJSONLLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
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

func buildUsageSummary(scope usageSummaryScope, by string, records []UsageRecord, warnings []string, unparseable int) usageSummaryResponse {
	var inputTokens, outputTokens, totalTokens int64
	capturedRecords := 0
	uncapturedMap := map[string]*uncapturedSource{}

	for _, r := range records {
		if r.Captured {
			capturedRecords++
			inputTokens += r.InputTokens
			outputTokens += r.OutputTokens
			totalTokens += r.TotalTokens
		} else {
			key := r.Provider + "\x00" + normalizeUsageStage(r.Stage)
			if uc, ok := uncapturedMap[key]; ok {
				uc.Records++
			} else {
				uncapturedMap[key] = &uncapturedSource{
					Provider: r.Provider,
					Stage:    normalizeUsageStage(r.Stage),
					Records:  1,
				}
			}
		}
	}

	uncaptured := sortedUncapturedSources(uncapturedMap)
	complete := len(uncaptured) == 0
	if warnings == nil {
		warnings = []string{}
	}

	return usageSummaryResponse{
		Schema: usageSummarySchema,
		Scope:  scope,
		Coverage: usageSummaryCoverage{
			Records:            len(records),
			CapturedRecords:    capturedRecords,
			UncapturedRecords:  len(records) - capturedRecords,
			Complete:           complete,
			Uncaptured:         uncaptured,
			UnparseableRecords: unparseable,
		},
		Totals: usageSummaryTotals{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  totalTokens,
			CapturedOnly: true,
			LowerBound:   !complete,
		},
		Groups:   buildSummaryGroups(by, records),
		Warnings: warnings,
	}
}

func sortedUncapturedSources(m map[string]*uncapturedSource) []uncapturedSource {
	if len(m) == 0 {
		return []uncapturedSource{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]uncapturedSource, 0, len(keys))
	for _, k := range keys {
		out = append(out, *m[k])
	}
	return out
}

func buildSummaryGroups(by string, records []UsageRecord) []usageSummaryGroup {
	type groupAccum struct {
		jsonKey         string
		displayLabel    string
		inputTokens     int64
		outputTokens    int64
		totalTokens     int64
		records         int
		capturedRecords int
	}
	groups := map[string]*groupAccum{}
	for _, r := range records {
		jsonKey, displayLabel := summaryGroupKey(by, r)
		g := groups[jsonKey]
		if g == nil {
			g = &groupAccum{jsonKey: jsonKey, displayLabel: displayLabel}
			groups[jsonKey] = g
		}
		g.records++
		if r.Captured {
			g.capturedRecords++
			g.inputTokens += r.InputTokens
			g.outputTokens += r.OutputTokens
			g.totalTokens += r.TotalTokens
		}
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]usageSummaryGroup, 0, len(keys))
	for _, k := range keys {
		g := groups[k]
		out = append(out, usageSummaryGroup{
			By:              by,
			Key:             g.jsonKey,
			label:           g.displayLabel,
			InputTokens:     g.inputTokens,
			OutputTokens:    g.outputTokens,
			TotalTokens:     g.totalTokens,
			Records:         g.records,
			CapturedRecords: g.capturedRecords,
		})
	}
	return out
}

func summaryGroupKey(by string, r UsageRecord) (jsonKey, displayLabel string) {
	switch by {
	case "model":
		k := usageRecordModel(r)
		return k, k
	case "agent":
		k := r.Agent
		if k == "" {
			k = "unknown"
		}
		label := strings.TrimSpace(r.AgentName)
		if label == "" {
			label = k
		}
		return k, label
	case "provider":
		k := r.Provider
		if k == "" {
			k = "unknown"
		}
		return k, k
	default: // "stage"
		k := normalizeUsageStage(r.Stage)
		return k, k
	}
}

func writeUsageSummaryHuman(stdout io.Writer, r usageSummaryResponse, by string) {
	if r.Scope.Kind == "all" {
		fmt.Fprintln(stdout, "Pactum usage (all runs)")
	} else {
		fmt.Fprintln(stdout, "Pactum usage")
		if r.Scope.RunID != nil {
			fmt.Fprintf(stdout, "\n  run: %s\n", *r.Scope.RunID)
		}
	}

	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "  %-24s  %10s  %10s  %10s  %s\n",
		strings.ToUpper(by), "INPUT", "OUTPUT", "TOTAL", "CAPTURED")
	for _, g := range r.Groups {
		label := g.label
		if label == "" {
			label = g.Key
		}
		captured := fmt.Sprintf("%d/%d", g.CapturedRecords, g.Records)
		fmt.Fprintf(stdout, "  %-24s  %10d  %10d  %10d  %s\n",
			label, g.InputTokens, g.OutputTokens, g.TotalTokens, captured)
	}

	fmt.Fprintln(stdout)
	totalLabel := "TOTAL"
	if !r.Coverage.Complete {
		totalLabel += " (LOWER BOUND)"
	}
	totalCaptured := fmt.Sprintf("%d/%d", r.Coverage.CapturedRecords, r.Coverage.Records)
	fmt.Fprintf(stdout, "  %-24s  %10d  %10d  %10d  %s\n",
		totalLabel, r.Totals.InputTokens, r.Totals.OutputTokens, r.Totals.TotalTokens, totalCaptured)

	fmt.Fprintln(stdout)
	if r.Coverage.Complete {
		fmt.Fprintf(stdout, "Coverage: %d/%d captured\n", r.Coverage.CapturedRecords, r.Coverage.Records)
	} else {
		fmt.Fprintf(stdout, "Coverage: %d/%d captured — LOWER BOUND\n", r.Coverage.CapturedRecords, r.Coverage.Records)
		for _, uc := range r.Coverage.Uncaptured {
			fmt.Fprintf(stdout, "  uncaptured: %s/%s (%d record(s))\n", uc.Provider, uc.Stage, uc.Records)
		}
	}

	if len(r.Warnings) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Warnings:")
		for _, w := range r.Warnings {
			fmt.Fprintf(stdout, "  - %s\n", w)
		}
	}
}
