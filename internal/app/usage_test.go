package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

func TestRunAgentAttemptLifecycleAppendsUsageRecord(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, err := readUsageRecords(runPaths.UsageJSONL)
	assertNoError(t, err)
	if len(records) != 1 {
		t.Fatalf("usage record count = %d, want 1: %#v", len(records), records)
	}
	record := records[0]
	// agent records the engine inferred from the entry's model; agent_name the
	// registry name.
	if record.RunID != runID || record.AttemptID != "attempt_001" || record.Stage != "execute" || record.Agent != "claude" || record.AgentName != "helper" {
		t.Fatalf("unexpected usage record identity: %#v", record)
	}
	if record.Captured {
		t.Fatalf("helper output carries no parsable usage, so it stays uncaptured: %#v", record)
	}
}

func TestExecuteRunRecordsCapturedCodexUsage(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	app.AgentRegistry = testAgentRegistry(codexHelperAgentDescriptor())
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_HELPER_CODEX_USAGE", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, err := readUsageRecords(runPaths.UsageJSONL)
	assertNoError(t, err)
	if len(records) != 1 {
		t.Fatalf("usage record count = %d, want 1: %#v", len(records), records)
	}
	record := records[0]
	if !record.Captured || record.Provider != "codex" || record.Agent != "codex" || record.AgentName != "codex" {
		t.Fatalf("codex usage should be captured with provider identity: %#v", record)
	}
	if record.InputTokens != 120 || record.OutputTokens != 50 || record.TotalTokens != 170 {
		t.Fatalf("codex usage counts mismatch: %#v", record)
	}
	if record.CacheReadTokens != 30 || record.ReasoningTokens != 10 || len(record.Raw) == 0 {
		t.Fatalf("codex usage classes/raw mismatch: %#v", record)
	}
}

func TestExecuteRunUsageRecordsRegistryAgentName(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	// "pinned-codex" runs on the codex built-in: the usage record keeps the
	// underlying agent for cross-model comparison and adds the registry name.
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "pinned-codex", Model: "gpt-5"})
	app.AgentRegistry = testAgentRegistry(codexHelperAgentDescriptor())
	runReviewCommand(t, app, "map", "refresh")
	runReviewCommand(t, app, "prompt", "build", runID)
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_HELPER_CODEX_USAGE", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "pinned-codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s", code, stderr.String())
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, err := readUsageRecords(runPaths.UsageJSONL)
	assertNoError(t, err)
	if len(records) != 1 {
		t.Fatalf("usage record count = %d, want 1: %#v", len(records), records)
	}
	record := records[0]
	if record.AgentName != "pinned-codex" || record.Agent != "codex" || record.Provider != "codex" {
		t.Fatalf("usage record should carry the registry name alongside the underlying agent: %#v", record)
	}
}

func TestReviewRunRecordsCapturedCodexUsage(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	app.AgentRegistry = testAgentRegistry(codexReviewerHelperAgentDescriptor())
	t.Setenv("PACTUM_REVIEWER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_REVIEWER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_REVIEWER_CODEX_USAGE", "1")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"review", "run", runID, "--reviewer", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, err := readUsageRecords(runPaths.UsageJSONL)
	assertNoError(t, err)
	// One usage record per lens attempt, each under the registry name.
	if len(records) != len(reviewLenses) {
		t.Fatalf("usage record count = %d, want %d: %#v", len(records), len(reviewLenses), records)
	}
	for _, record := range records {
		if !record.Captured || record.Stage != "review" || record.Provider != "codex" || record.Agent != "codex" || record.AgentName != "codex" {
			t.Fatalf("reviewer usage should be captured with review identity: %#v", record)
		}
		if record.InputTokens != 210 || record.OutputTokens != 65 || record.TotalTokens != 275 {
			t.Fatalf("reviewer usage counts mismatch: %#v", record)
		}
		if record.CacheReadTokens != 40 || record.ReasoningTokens != 15 || len(record.Raw) == 0 {
			t.Fatalf("reviewer usage classes/raw mismatch: %#v", record)
		}
	}
}

func TestExecuteRunUsageParseMissWarnsButSucceeds(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	app.AgentRegistry = testAgentRegistry(codexHelperAgentDescriptor())
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run with parse miss exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage capture warning") {
		t.Fatalf("parse miss should warn on live stderr:\n%s", stderr.String())
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, err := readUsageRecords(runPaths.UsageJSONL)
	assertNoError(t, err)
	if len(records) != 1 || records[0].Captured {
		t.Fatalf("parse miss should record one uncaptured usage record: %#v", records)
	}
}

func TestStatusAndUsageCommandSumRunUsage(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion:   usageRecordSchemaVersion,
		RecordID:        "usage_001",
		RunID:           runID,
		AttemptID:       "attempt_001",
		Stage:           "execute",
		Provider:        "codex",
		Agent:           "codex",
		InputTokens:     100,
		OutputTokens:    50,
		TotalTokens:     150,
		CacheReadTokens: 25,
		Captured:        true,
		CreatedAt:       "2026-06-07T18:00:00Z",
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion:       usageRecordSchemaVersion,
		RecordID:            "usage_002",
		RunID:               runID,
		AttemptID:           "attempt_002",
		Stage:               "fix",
		Provider:            "anthropic",
		Agent:               "claude",
		InputTokens:         200,
		OutputTokens:        100,
		TotalTokens:         300,
		CacheReadTokens:     50,
		CacheCreationTokens: 10,
		Captured:            true,
		CreatedAt:           "2026-06-07T18:05:00Z",
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"status", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("status --json exited %d, stderr: %s", code, stderr.String())
	}
	var status statusResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &status))
	if status.Usage.RunID != runID || status.Usage.TotalTokens != 450 || status.Usage.CacheReadTokens != 75 {
		t.Fatalf("status usage mismatch: %#v", status.Usage)
	}
	if status.Usage.CacheReadRatio != 0.25 {
		t.Fatalf("status cache read ratio = %v, want 0.25", status.Usage.CacheReadRatio)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var usage usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &usage))
	if usage.RunID != runID || usage.Total.TotalTokens != 450 || len(usage.ByStage) != 2 || len(usage.ByAgent) != 2 {
		t.Fatalf("usage response mismatch: %#v", usage)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"usage", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Pactum usage", "total tokens: 450", "effective units: 985.00", "cache hit rate: 25.00%", "execute:", "codex:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage output missing %q:\n%s", want, got)
		}
	}
}

func TestStatusAndUsageDegradeOnCorruptLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		SchemaVersion: usageRecordSchemaVersion,
		RecordID:      "usage_001",
		RunID:         runID,
		Stage:         "execute",
		Provider:      "codex",
		Agent:         "codex",
		InputTokens:   100,
		OutputTokens:  50,
		TotalTokens:   150,
		Captured:      true,
	})
	// A garbage line and an oversized (>16MB) line must be skipped, never fail:
	// usage accounting is best-effort and may not break status / pactum usage.
	f, err := os.OpenFile(runPaths.UsageJSONL, os.O_APPEND|os.O_WRONLY, 0o644)
	assertNoError(t, err)
	_, _ = f.WriteString("{not valid json\n")
	_, _ = f.WriteString(strings.Repeat("x", 17*1024*1024) + "\n")
	assertNoError(t, f.Close())

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"status", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status must not fail on corrupt usage ledger: exit %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage must not fail on corrupt usage ledger: exit %d, stderr: %s", code, stderr.String())
	}
	var usage usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &usage))
	if usage.Total.TotalTokens != 150 {
		t.Fatalf("usage should sum only the valid record (150), got %#v", usage.Total)
	}
}

func TestUsageAllAggregatesAcrossRuns(t *testing.T) {
	root := t.TempDir()
	app, paths, runA := setupContractRun(t, root)
	runB := "run_20260601_090000"

	appendWorkspaceUsageRecord(t, paths, runA, UsageRecord{
		RecordID:        "usage_a1",
		RunID:           runA,
		Stage:           "execute",
		Provider:        "codex",
		Agent:           "codex",
		RequestModel:    "gpt-5-codex",
		InputTokens:     100,
		OutputTokens:    50,
		TotalTokens:     150,
		CacheReadTokens: 25,
		Captured:        true,
	})
	appendWorkspaceUsageRecord(t, paths, runA, UsageRecord{
		RecordID:            "usage_a2",
		RunID:               runA,
		Stage:               "review",
		Provider:            "anthropic",
		Agent:               "claude",
		RequestModel:        "claude-opus",
		InputTokens:         200,
		OutputTokens:        100,
		TotalTokens:         300,
		CacheReadTokens:     50,
		CacheCreationTokens: 10,
		Captured:            true,
	})
	appendWorkspaceUsageRecord(t, paths, runB, UsageRecord{
		RecordID:        "usage_b1",
		RunID:           runB,
		Stage:           "execute",
		Provider:        "codex",
		Agent:           "codex",
		RequestModel:    "gpt-5-codex",
		InputTokens:     300,
		OutputTokens:    150,
		TotalTokens:     450,
		CacheReadTokens: 100,
		Captured:        true,
	})
	appendWorkspaceUsageRecord(t, paths, runB, UsageRecord{
		RecordID:     "usage_b2",
		RunID:        runB,
		Stage:        "fix",
		Provider:     "codex",
		Agent:        "codex",
		RequestModel: "gpt-5-codex",
		InputTokens:  100,
		OutputTokens: 20,
		TotalTokens:  120,
		Captured:     false,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageWorkspaceResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Schema != usageWorkspaceResponseSchema {
		t.Fatalf("schema = %q, want %q", got.Schema, usageWorkspaceResponseSchema)
	}
	if got.Runs != 2 || got.Calls != 4 || got.CapturedCalls != 3 || got.UncapturedCalls != 1 {
		t.Fatalf("workspace counts mismatch: %#v", got)
	}
	// The uncaptured fix record (usage_b2) is unknown usage: it is counted as a
	// call but contributes no tokens to any total or breakdown.
	if got.Total.TotalTokens != 900 || got.Total.InputTokens != 600 || got.Total.OutputTokens != 300 {
		t.Fatalf("workspace token totals mismatch: %#v", got.Total)
	}
	if got.Total.CacheReadTokens != 175 || got.Total.CacheCreationTokens != 10 {
		t.Fatalf("workspace cache totals mismatch: %#v", got.Total)
	}
	if got.CacheReadRatio != float64(175)/float64(600) {
		t.Fatalf("cache read ratio = %v, want 175/600", got.CacheReadRatio)
	}

	byRun := indexUsageBreakdowns(got.ByRun, func(b usageBreakdown) string { return b.Run })
	if len(byRun) != 2 || byRun[runA].TotalTokens != 450 || byRun[runB].TotalTokens != 450 {
		t.Fatalf("by-run breakdown mismatch: %#v", got.ByRun)
	}
	byStage := indexUsageBreakdowns(got.ByStage, func(b usageBreakdown) string { return b.Stage })
	if len(byStage) != 3 || byStage["execute"].TotalTokens != 600 || byStage["execute"].Calls != 2 {
		t.Fatalf("by-stage breakdown mismatch: %#v", got.ByStage)
	}
	if byStage["execute"].CacheReadTokens != 125 || byStage["review"].TotalTokens != 300 || byStage["fix"].TotalTokens != 0 {
		t.Fatalf("by-stage detail mismatch: %#v", got.ByStage)
	}
	// The fix stage holds only the uncaptured record: a call, but no tokens.
	if byStage["fix"].Calls != 1 || byStage["fix"].CapturedCalls != 0 || byStage["fix"].UncapturedCalls != 1 {
		t.Fatalf("uncaptured fix stage should count a call with no tokens: %#v", byStage["fix"])
	}
	byAgent := indexUsageBreakdowns(got.ByAgent, func(b usageBreakdown) string { return b.Agent })
	if len(byAgent) != 2 || byAgent["codex"].TotalTokens != 600 || byAgent["codex"].Calls != 3 {
		t.Fatalf("by-agent breakdown mismatch: %#v", got.ByAgent)
	}
	if byAgent["codex"].CapturedCalls != 2 || byAgent["codex"].UncapturedCalls != 1 || byAgent["codex"].Provider != "codex" {
		t.Fatalf("by-agent provenance mismatch: %#v", byAgent["codex"])
	}
	if byAgent["claude"].Provider != "anthropic" || byAgent["claude"].TotalTokens != 300 {
		t.Fatalf("by-agent claude mismatch: %#v", byAgent["claude"])
	}
	byModel := indexUsageBreakdowns(got.ByModel, func(b usageBreakdown) string { return b.Model })
	if len(byModel) != 2 || byModel["gpt-5-codex"].TotalTokens != 600 || byModel["claude-opus"].TotalTokens != 300 {
		t.Fatalf("by-model breakdown mismatch: %#v", got.ByModel)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", "--all"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{"Pactum usage (workspace)", "runs: 2", "total tokens: 900", "cache hit rate:", "By run:", "By stage:", "By agent:", "By model:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("workspace usage output missing %q:\n%s", want, text)
		}
	}
}

func TestUsageAllSkipsCorruptAndMissingLedgers(t *testing.T) {
	root := t.TempDir()
	app, paths, runA := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runA))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID:     "usage_ok",
		RunID:        runA,
		Stage:        "execute",
		Provider:     "codex",
		Agent:        "codex",
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		Captured:     true,
	})
	// Corrupt and oversized lines in runA must be skipped, never fatal.
	f, err := os.OpenFile(runPaths.UsageJSONL, os.O_APPEND|os.O_WRONLY, 0o644)
	assertNoError(t, err)
	_, _ = f.WriteString("{not valid json\n")
	_, _ = f.WriteString(strings.Repeat("x", 17*1024*1024) + "\n")
	assertNoError(t, f.Close())

	// A run directory that exists with no usage ledger contributes nothing.
	assertNoError(t, os.MkdirAll(filepath.Join(paths.RunsDir, "run_20260601_090000"), 0o755))

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all must not fail on corrupt/missing ledgers: exit %d, stderr: %s", code, stderr.String())
	}
	var got usageWorkspaceResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Runs != 1 || got.Calls != 1 || got.Total.TotalTokens != 150 {
		t.Fatalf("usage --all should count only the one valid record: %#v", got)
	}
}

func TestUsageAllEmptyWorkspaceReportsZero(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()

	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}
	var got usageWorkspaceResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Runs != 0 || got.Calls != 0 || got.Total.TotalTokens != 0 || got.CacheReadRatio != 0 {
		t.Fatalf("empty workspace should report a clean zero: %#v", got)
	}
	// Breakdown slices serialize as [] (not null) so the JSON shape is stable.
	if got.ByRun == nil || got.ByStage == nil || got.ByAgent == nil || got.ByModel == nil {
		t.Fatalf("empty workspace breakdowns should be empty slices, not null: %#v", got)
	}
	if len(got.ByRun) != 0 || len(got.ByStage) != 0 || len(got.ByAgent) != 0 || len(got.ByModel) != 0 {
		t.Fatalf("empty workspace breakdowns should be empty: %#v", got)
	}
}

func TestUsageAllCacheRatioGuardsZeroInput(t *testing.T) {
	root := t.TempDir()
	app, paths, runA := setupContractRun(t, root)
	appendWorkspaceUsageRecord(t, paths, runA, UsageRecord{
		RecordID:     "usage_no_input",
		RunID:        runA,
		Stage:        "execute",
		Provider:     "codex",
		Agent:        "codex",
		InputTokens:  0,
		OutputTokens: 10,
		TotalTokens:  10,
		Captured:     true,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}
	var got usageWorkspaceResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Total.InputTokens != 0 || got.CacheReadRatio != 0 {
		t.Fatalf("cache read ratio must be 0 when input is 0 (divide-by-zero guard): %#v", got)
	}
}

func TestUsageAllRejectsRunIDArg(t *testing.T) {
	root := t.TempDir()
	app, _, runA := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runA, "--all"}, &stdout, &stderr); code == 0 {
		t.Fatalf("usage with both run_id and --all should error, got exit 0:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "run_id") && !strings.Contains(stderr.String(), "--all") {
		t.Fatalf("error should explain the run_id/--all conflict, got stderr:\n%s", stderr.String())
	}
}

// TestUsageEffectiveUnitsPerProvider pins the per-provider effective-unit math:
// Anthropic weights cache writes 1.25x, Codex prices them as fresh; both weight
// cache reads 0.1x and output 5x.
func TestUsageEffectiveUnitsPerProvider(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	// Codex: fresh=75, read=25, output=50 -> 75 + 25*0.1 + 50*5 = 327.5
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_codex", RunID: runID, AttemptID: "attempt_001", Stage: "execute",
		Provider: "codex", Agent: "codex", InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		CacheReadTokens: 25, Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})
	// Anthropic: fresh=140, write=10, read=50, output=100 ->
	// 140 + 10*1.25 + 50*0.1 + 100*5 = 657.5
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_claude", RunID: runID, AttemptID: "attempt_002", Stage: "review",
		Provider: "anthropic", Agent: "claude", InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		CacheReadTokens: 50, CacheCreationTokens: 10, Captured: true, CreatedAt: "2026-06-07T18:05:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if units := fmtUnits(got.Total.EffectiveUnits); units != "985.00" {
		t.Fatalf("total effective units = %s, want 985.00", units)
	}
	byAgent := indexUsageBreakdowns(got.ByAgent, func(b usageBreakdown) string { return b.Agent })
	if units := fmtUnits(byAgent["codex"].EffectiveUnits); units != "327.50" {
		t.Fatalf("codex effective units = %s, want 327.50", units)
	}
	if units := fmtUnits(byAgent["claude"].EffectiveUnits); units != "657.50" {
		t.Fatalf("claude effective units = %s, want 657.50", units)
	}
}

// TestUsagePerRunByAttempt pins the per-run by_attempt breakdown: one row per
// attempt_id carrying stage, agent, provider, tokens, effective_units, and the
// cache hit ratio. The workspace view does not add by_attempt.
func TestUsagePerRunByAttempt(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, AttemptID: "attempt_001", Stage: "execute",
		Provider: "codex", Agent: "codex", InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		CacheReadTokens: 25, Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u2", RunID: runID, AttemptID: "attempt_002", Stage: "review",
		Provider: "anthropic", Agent: "claude", InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		CacheReadTokens: 50, CacheCreationTokens: 10, Captured: true, CreatedAt: "2026-06-07T18:05:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	byAttempt := indexUsageBreakdowns(got.ByAttempt, func(b usageBreakdown) string { return b.Attempt })
	if len(byAttempt) != 2 {
		t.Fatalf("by_attempt rows = %d, want 2: %#v", len(byAttempt), got.ByAttempt)
	}
	first := byAttempt["attempt_001"]
	if first.Stage != "execute" || first.Agent != "codex" || first.Provider != "codex" || first.TotalTokens != 150 {
		t.Fatalf("attempt_001 row mismatch: %#v", first)
	}
	if fmtUnits(first.EffectiveUnits) != "327.50" || first.CacheReadRatio != 0.25 {
		t.Fatalf("attempt_001 derived metrics mismatch: %#v", first)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all --json exited %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "by_attempt") {
		t.Fatalf("workspace usage must not add by_attempt:\n%s", stdout.String())
	}
}

// TestUsagePerRunByAttemptStageScopedKeys pins that attempt_001 from the execute
// stage and attempt_001 from a fix round — distinct attempts whose IDs collide
// because numbering restarts per stage — stay separate by_attempt rows rather
// than merging their tokens under one mislabeled row.
func TestUsagePerRunByAttemptStageScopedKeys(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_exec", RunID: runID, AttemptID: "attempt_001", Stage: "execute",
		Provider: "codex", Agent: "codex", InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_fix", RunID: runID, AttemptID: "attempt_001", Stage: "fix",
		Provider: "anthropic", Agent: "claude", InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		Captured: true, CreatedAt: "2026-06-07T18:05:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if len(got.ByAttempt) != 2 {
		t.Fatalf("colliding attempt_001 ids must stay separate rows, got %d: %#v", len(got.ByAttempt), got.ByAttempt)
	}
	byStage := indexUsageBreakdowns(got.ByAttempt, func(b usageBreakdown) string { return b.Stage })
	if byStage["execute"].Attempt != "attempt_001" || byStage["execute"].TotalTokens != 150 || byStage["execute"].Agent != "codex" {
		t.Fatalf("execute attempt row mismatch: %#v", byStage["execute"])
	}
	if byStage["fix"].Attempt != "attempt_001" || byStage["fix"].TotalTokens != 300 || byStage["fix"].Agent != "claude" {
		t.Fatalf("fix attempt row mismatch: %#v", byStage["fix"])
	}
}

// TestUsageMixedCapturedRowAnnotatesUnreported pins that a breakdown row mixing
// captured and uncaptured calls keeps its captured token counts but also flags
// the uncaptured remainder, rather than silently dropping it.
func TestUsageMixedCapturedRowAnnotatesUnreported(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_cap", RunID: runID, AttemptID: "attempt_001", Stage: "execute",
		Provider: "codex", Agent: "codex", InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_uncap", RunID: runID, AttemptID: "attempt_002", Stage: "review",
		Provider: "codex", Agent: "codex", Captured: false, CreatedAt: "2026-06-07T18:05:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	// The by-agent codex row mixes one captured and one uncaptured call.
	if !strings.Contains(text, "total_tokens=150") || !strings.Contains(text, "(1 calls: usage not reported)") {
		t.Fatalf("mixed by-agent row should keep captured tokens and flag the uncaptured call:\n%s", text)
	}
}

// TestUsageAllSortsByRunDescendingWithTop pins that by_run is sorted by captured
// total tokens descending and that --top caps only that list while the totals
// still aggregate every run.
func TestUsageAllSortsByRunDescendingWithTop(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	runs := map[string]int64{
		"run_20260101_000001": 100,
		"run_20260101_000002": 500,
		"run_20260101_000003": 300,
	}
	for runID, total := range runs {
		appendWorkspaceUsageRecord(t, paths, runID, UsageRecord{
			RecordID: "u_" + runID, RunID: runID, AttemptID: "attempt_001", Stage: "execute",
			Provider: "codex", Agent: "codex", InputTokens: total, OutputTokens: 0, TotalTokens: total,
			Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
		})
	}

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageWorkspaceResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if len(got.ByRun) != 3 {
		t.Fatalf("by_run rows = %d, want 3: %#v", len(got.ByRun), got.ByRun)
	}
	if got.ByRun[0].Run != "run_20260101_000002" || got.ByRun[1].Run != "run_20260101_000003" || got.ByRun[2].Run != "run_20260101_000001" {
		t.Fatalf("by_run not sorted by total tokens descending: %#v", got.ByRun)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", "--all", "--top", "2", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all --top 2 exited %d, stderr: %s", code, stderr.String())
	}
	var capped usageWorkspaceResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &capped))
	if len(capped.ByRun) != 2 || capped.ByRun[0].Run != "run_20260101_000002" || capped.ByRun[1].Run != "run_20260101_000003" {
		t.Fatalf("--top 2 should keep the two heaviest runs: %#v", capped.ByRun)
	}
	// Totals still aggregate every run even though the list is capped.
	if capped.Runs != 3 || capped.Total.TotalTokens != 900 {
		t.Fatalf("--top must not change workspace totals: %#v", capped)
	}
}

// TestUsageTopValidation pins that --top requires --all and rejects non-positive
// values, with each error naming --top.
func TestUsageTopValidation(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)
	cases := [][]string{
		{"usage", runID, "--top", "2"},
		{"usage", "--all", "--top", "0"},
		{"usage", "--all", "--top", "-1"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		if code := app.Run(args, &stdout, &stderr); code == 0 {
			t.Fatalf("%v should fail, got exit 0:\n%s", args, stdout.String())
		}
		if !strings.Contains(stderr.String(), "--top") {
			t.Fatalf("%v error should name --top, got: %s", args, stderr.String())
		}
	}
}

// TestUsageUncapturedRowsAnnotatedNotZero pins that an uncaptured call renders as
// "usage not reported" rather than a misleading zero row, while a genuine
// captured zero-token row stays a real zero.
func TestUsageUncapturedRowsAnnotatedNotZero(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	// Captured but genuinely zero tokens.
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_zero", RunID: runID, AttemptID: "attempt_001", Stage: "execute",
		Provider: "codex", Agent: "codex", Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})
	// Uncaptured: token fields present but unknown usage, must not total.
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_uncaptured", RunID: runID, AttemptID: "attempt_002", Stage: "review",
		Provider: "codex", Agent: "codex", InputTokens: 999, OutputTokens: 999, TotalTokens: 1998,
		Captured: false, CreatedAt: "2026-06-07T18:05:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Total.TotalTokens != 0 || got.CapturedCalls != 1 || got.UncapturedCalls != 1 {
		t.Fatalf("uncaptured tokens must not total: %#v", got)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	if !strings.Contains(text, "review: calls=1, usage not reported by the agent") {
		t.Fatalf("uncaptured stage row should be annotated honestly:\n%s", text)
	}
	if !strings.Contains(text, "execute: calls=1 captured=1 total_tokens=0") {
		t.Fatalf("captured zero-token row should stay a real zero:\n%s", text)
	}
}

// TestUsageUnsupportedProviderEffectiveUnitsUnavailable pins that a captured
// record from a provider with no multipliers keeps its raw tokens, reports zero
// effective units, and is counted and annotated as effective-units-unavailable.
func TestUsageUnsupportedProviderEffectiveUnitsUnavailable(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_future", RunID: runID, AttemptID: "attempt_001", Stage: "execute",
		Provider: "future-llm", Agent: "future", InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Total.TotalTokens != 150 || got.Total.EffectiveUnits != 0 || got.EffectiveUnitsUnavailableCalls != 1 {
		t.Fatalf("unsupported provider should keep tokens but no effective units: %#v", got)
	}
	byAgent := indexUsageBreakdowns(got.ByAgent, func(b usageBreakdown) string { return b.Agent })
	if byAgent["future"].EffectiveUnitsUnavailableCalls != 1 || byAgent["future"].EffectiveUnits != 0 || byAgent["future"].TotalTokens != 150 {
		t.Fatalf("by-agent unsupported row mismatch: %#v", byAgent["future"])
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "effective units unavailable") {
		t.Fatalf("unsupported-provider row should be annotated:\n%s", stdout.String())
	}
}

func fmtUnits(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func appendWorkspaceUsageRecord(t *testing.T, paths artifacts.Paths, runID string, record UsageRecord) {
	t.Helper()
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, record)
}

func indexUsageBreakdowns(items []usageBreakdown, key func(usageBreakdown) string) map[string]usageBreakdown {
	indexed := map[string]usageBreakdown{}
	for _, item := range items {
		indexed[key(item)] = item
	}
	return indexed
}

func codexHelperAgentDescriptor() agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    agents.BuiltinCodex,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecutionHelperProcess", "--", "exec", "--json", "--dangerously-bypass-approvals-and-sandbox"},
		Input:   agents.InputPromptFile,
	}
}

func codexReviewerHelperAgentDescriptor() agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    agents.BuiltinCodex,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestReviewerHelperProcess", "--", "exec", "--json", "--sandbox", "read-only"},
		Input:   agents.InputPromptFile,
	}
}

func appendUsageRecordForTest(t *testing.T, path string, record UsageRecord) {
	t.Helper()
	assertNoError(t, appendJSONLine(path, record))
}

// TestUsageEffectiveUnitsOpenAIProvider pins the "openai" arm of the provider
// switch: writes are free (counted as fresh input), cached reads cost 0.1x.
func TestUsageEffectiveUnitsOpenAIProvider(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	// fresh=60, read=40, output=20 -> 60 + 40*0.1 + 20*5 = 164
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_openai", RunID: runID, AttemptID: "attempt_001", Stage: "execute",
		Provider: "openai", Agent: "codex", InputTokens: 100, OutputTokens: 20, TotalTokens: 120,
		CacheReadTokens: 40, Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if units := fmtUnits(got.Total.EffectiveUnits); units != "164.00" {
		t.Fatalf("openai effective units = %s, want 164.00", units)
	}
}

// TestUsageAllWorkspaceEffectiveUnitsAggregation pins workspace-level
// effective_units: the sum over runs appears in the workspace total (JSON and
// the human summary block).
func TestUsageAllWorkspaceEffectiveUnitsAggregation(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	// Two runs: codex 100 fresh (=100.0) and anthropic 100 fresh + 100 output
	// (= 100 + 500 = 600.0) -> workspace total 700.00.
	appendWorkspaceUsageRecord(t, paths, "run_20260101_000001", UsageRecord{
		RecordID: "u_1", RunID: "run_20260101_000001", AttemptID: "attempt_001", Stage: "execute",
		Provider: "codex", Agent: "codex", InputTokens: 100, TotalTokens: 100,
		Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
	})
	appendWorkspaceUsageRecord(t, paths, "run_20260101_000002", UsageRecord{
		RecordID: "u_2", RunID: "run_20260101_000002", AttemptID: "attempt_001", Stage: "execute",
		Provider: "anthropic", Agent: "claude", InputTokens: 100, OutputTokens: 100, TotalTokens: 200,
		Captured: true, CreatedAt: "2026-06-07T18:05:00Z",
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageWorkspaceResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if units := fmtUnits(got.Total.EffectiveUnits); units != "700.00" {
		t.Fatalf("workspace effective units = %s, want 700.00", units)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", "--all"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "effective units: 700.00") {
		t.Fatalf("human summary must show workspace effective units:\n%s", stdout.String())
	}
}

// TestUsageAllHumanOrderingAndTop pins the human output: by-run lines appear
// sorted by total tokens descending, and --top caps the printed list.
func TestUsageAllHumanOrderingAndTop(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	runs := map[string]int64{
		"run_20260101_000001": 100,
		"run_20260101_000002": 500,
		"run_20260101_000003": 300,
	}
	for runID, total := range runs {
		appendWorkspaceUsageRecord(t, paths, runID, UsageRecord{
			RecordID: "u_" + runID, RunID: runID, AttemptID: "attempt_001", Stage: "execute",
			Provider: "codex", Agent: "codex", InputTokens: total, TotalTokens: total,
			Captured: true, CreatedAt: "2026-06-07T18:00:00Z",
		})
	}

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	i1 := strings.Index(out, "run_20260101_000002")
	i2 := strings.Index(out, "run_20260101_000003")
	i3 := strings.Index(out, "run_20260101_000001")
	if i1 == -1 || i2 == -1 || i3 == -1 || !(i1 < i2 && i2 < i3) {
		t.Fatalf("human by-run lines not sorted descending (%d, %d, %d):\n%s", i1, i2, i3, out)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", "--all", "--top", "1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all --top 1 exited %d, stderr: %s", code, stderr.String())
	}
	out = stdout.String()
	if !strings.Contains(out, "run_20260101_000002") {
		t.Fatalf("--top 1 must keep the largest run:\n%s", out)
	}
	if strings.Contains(out, "run_20260101_000001") || strings.Contains(out, "run_20260101_000003") {
		t.Fatalf("--top 1 must cap the human by-run list:\n%s", out)
	}
}
