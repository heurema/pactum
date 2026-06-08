package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper", "--yes"}, &stdout, &stderr)
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
	if record.RunID != runID || record.AttemptID != "attempt_001" || record.Stage != "execute" || record.Agent != "helper" {
		t.Fatalf("unexpected usage record identity: %#v", record)
	}
	if record.Captured {
		t.Fatalf("custom helper usage should be uncaptured: %#v", record)
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
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex", "--yes"}, &stdout, &stderr)
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
	if !record.Captured || record.Provider != "codex" || record.Agent != "codex" {
		t.Fatalf("codex usage should be captured with provider identity: %#v", record)
	}
	if record.InputTokens != 120 || record.OutputTokens != 50 || record.TotalTokens != 170 {
		t.Fatalf("codex usage counts mismatch: %#v", record)
	}
	if record.CacheReadTokens != 30 || record.ReasoningTokens != 10 || len(record.Raw) == 0 {
		t.Fatalf("codex usage classes/raw mismatch: %#v", record)
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
	code := app.Run([]string{"review", "run", runID, "--reviewer", "codex", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("review run exited %d, stderr: %s", code, stderr.String())
	}

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	records, err := readUsageRecords(runPaths.UsageJSONL)
	assertNoError(t, err)
	if len(records) != 1 {
		t.Fatalf("usage record count = %d, want 1: %#v", len(records), records)
	}
	record := records[0]
	if !record.Captured || record.Stage != "review" || record.Provider != "codex" || record.Agent != "codex" {
		t.Fatalf("reviewer usage should be captured with review identity: %#v", record)
	}
	if record.InputTokens != 210 || record.OutputTokens != 65 || record.TotalTokens != 275 {
		t.Fatalf("reviewer usage counts mismatch: %#v", record)
	}
	if record.CacheReadTokens != 40 || record.ReasoningTokens != 15 || len(record.Raw) == 0 {
		t.Fatalf("reviewer usage classes/raw mismatch: %#v", record)
	}
}

func TestExecuteRunUsageParseMissWarnsButSucceeds(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedAndBuiltPrompt(t, root)
	app.AgentRegistry = testAgentRegistry(codexHelperAgentDescriptor())
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "codex", "--yes"}, &stdout, &stderr)
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
	for _, want := range []string{"Pactum usage", "total tokens: 450", "cache read ratio: 25.00%", "execute:", "codex:"} {
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
	if got.Total.TotalTokens != 1020 || got.Total.InputTokens != 700 || got.Total.OutputTokens != 320 {
		t.Fatalf("workspace token totals mismatch: %#v", got.Total)
	}
	if got.Total.CacheReadTokens != 175 || got.Total.CacheCreationTokens != 10 {
		t.Fatalf("workspace cache totals mismatch: %#v", got.Total)
	}
	if got.CacheReadRatio != 0.25 {
		t.Fatalf("cache read ratio = %v, want 0.25", got.CacheReadRatio)
	}

	byRun := indexUsageBreakdowns(got.ByRun, func(b usageBreakdown) string { return b.Run })
	if len(byRun) != 2 || byRun[runA].TotalTokens != 450 || byRun[runB].TotalTokens != 570 {
		t.Fatalf("by-run breakdown mismatch: %#v", got.ByRun)
	}
	byStage := indexUsageBreakdowns(got.ByStage, func(b usageBreakdown) string { return b.Stage })
	if len(byStage) != 3 || byStage["execute"].TotalTokens != 600 || byStage["execute"].Calls != 2 {
		t.Fatalf("by-stage breakdown mismatch: %#v", got.ByStage)
	}
	if byStage["execute"].CacheReadTokens != 125 || byStage["review"].TotalTokens != 300 || byStage["fix"].TotalTokens != 120 {
		t.Fatalf("by-stage detail mismatch: %#v", got.ByStage)
	}
	byAgent := indexUsageBreakdowns(got.ByAgent, func(b usageBreakdown) string { return b.Agent })
	if len(byAgent) != 2 || byAgent["codex"].TotalTokens != 720 || byAgent["codex"].Calls != 3 {
		t.Fatalf("by-agent breakdown mismatch: %#v", got.ByAgent)
	}
	if byAgent["codex"].CapturedCalls != 2 || byAgent["codex"].UncapturedCalls != 1 || byAgent["codex"].Provider != "codex" {
		t.Fatalf("by-agent provenance mismatch: %#v", byAgent["codex"])
	}
	if byAgent["claude"].Provider != "anthropic" || byAgent["claude"].TotalTokens != 300 {
		t.Fatalf("by-agent claude mismatch: %#v", byAgent["claude"])
	}
	byModel := indexUsageBreakdowns(got.ByModel, func(b usageBreakdown) string { return b.Model })
	if len(byModel) != 2 || byModel["gpt-5-codex"].TotalTokens != 720 || byModel["claude-opus"].TotalTokens != 300 {
		t.Fatalf("by-model breakdown mismatch: %#v", got.ByModel)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", "--all"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{"Pactum usage (workspace)", "runs: 2", "total tokens: 1020", "cache read ratio: 25.00%", "By run:", "By stage:", "By agent:", "By model:"} {
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
