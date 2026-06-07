package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
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

func codexHelperAgentDescriptor() agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    agents.BuiltinCodex,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecutionHelperProcess", "--", "exec", "--json", "--dangerously-bypass-approvals-and-sandbox"},
		Input:   agents.InputPromptFile,
	}
}

func appendUsageRecordForTest(t *testing.T, path string, record UsageRecord) {
	t.Helper()
	assertNoError(t, appendJSONLine(path, record))
}
