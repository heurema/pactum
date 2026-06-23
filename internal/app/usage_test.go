package app

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestExecuteRunUsageRecordsRegistryAgentName(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	// "pinned-codex" runs on the codex built-in: the usage record keeps the
	// underlying agent for cross-model comparison and adds the registry name.
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "pinned-codex", Model: "gpt-5"})
	app.AgentRegistry = testAgentRegistry(codexHelperAgentDescriptor())
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

// TestStatusAndUsageCommandSumRunUsage pins that status reports usage totals for
// the current run, and that pactum usage emits a valid pactum.usage_summary.v1alpha1
// response with the correct totals and groups.
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

	// status still reports usage totals from the best-effort path.
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

	// pactum usage emits pactum.usage_summary.v1alpha1.
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var summary usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.Schema != usageSummarySchema {
		t.Fatalf("schema = %q, want %q", summary.Schema, usageSummarySchema)
	}
	if summary.Scope.Kind != "run" || summary.Scope.RunID == nil || *summary.Scope.RunID != runID {
		t.Fatalf("scope mismatch: %#v", summary.Scope)
	}
	if summary.Totals.TotalTokens != 450 || !summary.Coverage.Complete || summary.Totals.LowerBound {
		t.Fatalf("totals/coverage mismatch: %#v %#v", summary.Totals, summary.Coverage)
	}
	if len(summary.Groups) != 2 {
		t.Fatalf("expected 2 stage groups, got %d: %#v", len(summary.Groups), summary.Groups)
	}

	// Human output contains the run id, group labels, and TOTAL row.
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"usage", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Pactum usage", runID, "execute", "fix", "TOTAL", "450"} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage output missing %q:\n%s", want, got)
		}
	}
}

// TestStatusAndUsageDegradeOnCorruptLedger pins that status (best-effort path)
// still succeeds on a corrupt ledger, while pactum usage returns an error for a
// malformed non-final line.
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
	// A non-final garbage line followed by an oversized last line.
	f, err := os.OpenFile(runPaths.UsageJSONL, os.O_APPEND|os.O_WRONLY, 0o644)
	assertNoError(t, err)
	_, _ = f.WriteString("{not valid json\n")
	_, _ = f.WriteString(strings.Repeat("x", 17*1024*1024) + "\n")
	assertNoError(t, f.Close())

	// status uses best-effort reading and must still succeed.
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"status", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status must not fail on corrupt usage ledger: exit %d, stderr: %s", code, stderr.String())
	}

	// pactum usage returns an error because the non-final line is malformed.
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code == 0 {
		t.Fatalf("pactum usage must fail on malformed non-final line, got exit 0:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "malformed") {
		t.Fatalf("error should mention malformed: %s", stderr.String())
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

// TestUsageSummaryRunLevel pins the basic per-run summary: schema, scope,
// coverage, totals, and groups for the default --by stage dimension.
func TestUsageSummaryRunLevel(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		Captured: true,
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u2", RunID: runID, Stage: "review",
		Provider: "anthropic", Agent: "claude",
		InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		Captured: true,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Schema != usageSummarySchema {
		t.Fatalf("schema = %q, want %q", got.Schema, usageSummarySchema)
	}
	if got.Scope.Kind != "run" || got.Scope.RunID == nil || *got.Scope.RunID != runID {
		t.Fatalf("scope mismatch: %#v", got.Scope)
	}
	if got.Coverage.Records != 2 || got.Coverage.CapturedRecords != 2 || got.Coverage.UncapturedRecords != 0 {
		t.Fatalf("coverage mismatch: %#v", got.Coverage)
	}
	if !got.Coverage.Complete || got.Totals.LowerBound {
		t.Fatalf("fully captured run must be complete and not lower-bound")
	}
	if got.Totals.TotalTokens != 450 || got.Totals.InputTokens != 300 || got.Totals.OutputTokens != 150 {
		t.Fatalf("totals mismatch: %#v", got.Totals)
	}
	if !got.Totals.CapturedOnly {
		t.Fatalf("totals.captured_only must be true")
	}
	if len(got.Groups) != 2 {
		t.Fatalf("expected 2 stage groups, got %d: %#v", len(got.Groups), got.Groups)
	}
	groups := indexSummaryGroups(got.Groups)
	if groups["execute"].InputTokens != 100 || groups["execute"].Records != 1 || groups["execute"].CapturedRecords != 1 {
		t.Fatalf("execute group mismatch: %#v", groups["execute"])
	}
	if groups["review"].TotalTokens != 300 {
		t.Fatalf("review group mismatch: %#v", groups["review"])
	}
	if got.Warnings == nil || len(got.Warnings) != 0 {
		t.Fatalf("fully captured run should have empty warnings slice: %#v", got.Warnings)
	}

	// Human output must include TOTAL row and coverage line.
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{"Pactum usage", "execute", "review", "TOTAL", "450", "Coverage:", "2/2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("human output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "LOWER BOUND") {
		t.Fatalf("fully captured run must not show LOWER BOUND:\n%s", text)
	}
}

// TestUsageSummaryAll pins --all aggregation: records from multiple runs are
// combined, scope.kind is "all", and scope.run_id is null.
func TestUsageSummaryAll(t *testing.T) {
	root := t.TempDir()
	app, paths, runA := setupContractRun(t, root)
	runB := "run_20260601_090000"

	appendWorkspaceUsageRecord(t, paths, runA, UsageRecord{
		RecordID: "ua1", RunID: runA, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, CacheReadTokens: 20, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})
	appendWorkspaceUsageRecord(t, paths, runB, UsageRecord{
		RecordID: "ub1", RunID: runB, Stage: "execute",
		Provider: "anthropic", Agent: "claude",
		InputTokens: 200, CacheReadTokens: 30, CacheCreationTokens: 5, OutputTokens: 100, TotalTokens: 300, Captured: true,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Schema != usageSummarySchema {
		t.Fatalf("schema mismatch: %q", got.Schema)
	}
	if got.Scope.Kind != "all" || got.Scope.RunID != nil {
		t.Fatalf("scope must be all with null run_id: %#v", got.Scope)
	}
	if got.Coverage.Records != 2 || got.Totals.TotalTokens != 450 {
		t.Fatalf("aggregation mismatch: %#v %#v", got.Coverage, got.Totals)
	}
	if !got.Coverage.Complete || got.Totals.LowerBound {
		t.Fatalf("all captured should be complete and not lower-bound")
	}
	// cache and effective_units aggregate across runs.
	if got.Totals.CacheReadTokens != 50 || got.Totals.CacheCreationTokens != 5 {
		t.Fatalf("--all cache tokens: read=%d create=%d, want 50/5", got.Totals.CacheReadTokens, got.Totals.CacheCreationTokens)
	}
	wantRatio := 50.0 / 300.0
	if got.Totals.CacheReadRatio < wantRatio-0.001 || got.Totals.CacheReadRatio > wantRatio+0.001 {
		t.Fatalf("--all cache_read_ratio = %f, want ~%f", got.Totals.CacheReadRatio, wantRatio)
	}
	// codex: fresh=80+read=20+output=50 → 80+2+250=332; anthropic: fresh=165+read=30+create=5+output=100 → 165+3+500+6.25=674.25
	const wantEff = 332.0 + 674.25
	if got.Totals.EffectiveUnits < wantEff-0.1 || got.Totals.EffectiveUnits > wantEff+0.1 {
		t.Fatalf("--all effective_units = %f, want ~%f", got.Totals.EffectiveUnits, wantEff)
	}

	// Human output.
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", "--all"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	if !strings.Contains(text, "Pactum usage (all runs)") {
		t.Fatalf("--all human output must say 'all runs':\n%s", text)
	}
	for _, want := range []string{"execute", "TOTAL", "450"} {
		if !strings.Contains(text, want) {
			t.Fatalf("--all human output missing %q:\n%s", want, text)
		}
	}
}

// TestUsageSummaryByDimensions pins that each --by value produces groups keyed
// by the selected dimension with deterministic, sorted output.
func TestUsageSummaryByDimensions(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex", AgentName: "helper",
		RequestModel: "gpt-5",
		InputTokens:  100, CacheReadTokens: 20, OutputTokens: 50, TotalTokens: 150,
		Captured: true,
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u2", RunID: runID, Stage: "review",
		Provider: "anthropic", Agent: "claude", AgentName: "opus",
		RequestModel: "claude-opus",
		InputTokens:  200, CacheReadTokens: 30, CacheCreationTokens: 5, OutputTokens: 100, TotalTokens: 300,
		Captured: true,
	})

	// wantCacheRead is [groups[0].CacheReadTokens, groups[1].CacheReadTokens] after sorting by key.
	// stage: execute(codex,20) < review(anthropic,30); others: anthropic/claude/claude-opus(30) < codex/gpt-5(20).
	cases := []struct {
		by            string
		wantKey       []string
		wantCacheRead [2]int64
	}{
		{"stage", []string{"execute", "review"}, [2]int64{20, 30}},
		{"model", []string{"claude-opus", "gpt-5"}, [2]int64{30, 20}},
		{"agent", []string{"claude", "codex"}, [2]int64{30, 20}},
		{"provider", []string{"anthropic", "codex"}, [2]int64{30, 20}},
	}

	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		if code := app.Run([]string{"usage", runID, "--by", tc.by, "--json"}, &stdout, &stderr); code != 0 {
			t.Fatalf("usage --by %s --json exited %d, stderr: %s", tc.by, code, stderr.String())
		}
		var got usageSummaryResponse
		assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

		if len(got.Groups) != len(tc.wantKey) {
			t.Fatalf("--by %s: got %d groups, want %d: %#v", tc.by, len(got.Groups), len(tc.wantKey), got.Groups)
		}
		for i, key := range tc.wantKey {
			if got.Groups[i].Key != key {
				t.Fatalf("--by %s group[%d].key = %q, want %q", tc.by, i, got.Groups[i].Key, key)
			}
			if got.Groups[i].By != tc.by {
				t.Fatalf("--by %s group[%d].by = %q, want %q", tc.by, i, got.Groups[i].By, tc.by)
			}
		}
		// Groups must be sorted deterministically.
		for i := 1; i < len(got.Groups); i++ {
			if got.Groups[i].Key < got.Groups[i-1].Key {
				t.Fatalf("--by %s groups not sorted: %q before %q", tc.by, got.Groups[i-1].Key, got.Groups[i].Key)
			}
		}
		// cache_read_tokens must aggregate per group regardless of --by dimension.
		if got.Groups[0].CacheReadTokens != tc.wantCacheRead[0] || got.Groups[1].CacheReadTokens != tc.wantCacheRead[1] {
			t.Fatalf("--by %s cache_read_tokens: [%d, %d], want [%d, %d]",
				tc.by, got.Groups[0].CacheReadTokens, got.Groups[1].CacheReadTokens,
				tc.wantCacheRead[0], tc.wantCacheRead[1])
		}
		// effective_units must be positive for captured groups.
		for _, g := range got.Groups {
			if g.CapturedRecords > 0 && g.EffectiveUnits <= 0 {
				t.Fatalf("--by %s group %q: effective_units should be positive for captured records, got %f", tc.by, g.Key, g.EffectiveUnits)
			}
		}
	}

	// --by agent: JSON key is agent field; human label is agent_name.
	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--by", "agent"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --by agent exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	// agent_name values should appear as human labels.
	if !strings.Contains(text, "opus") || !strings.Contains(text, "helper") {
		t.Fatalf("--by agent human output should display agent_name labels:\n%s", text)
	}
}

// TestUsageSummaryJSONOutput pins the full pactum.usage_summary.v1alpha1 schema:
// all required fields are present, groups have the right captured-only token rule,
// and warnings is a non-null array.
func TestUsageSummaryJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		Captured: true,
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u2", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 999, OutputTokens: 999, TotalTokens: 1998,
		Captured: false, // uncaptured — must not add to group totals
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}

	// Unmarshal into a generic map to check field presence.
	var raw map[string]any
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &raw))
	for _, field := range []string{"schema", "scope", "coverage", "totals", "groups", "warnings"} {
		if _, ok := raw[field]; !ok {
			t.Fatalf("JSON output missing top-level field %q", field)
		}
	}

	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Schema != usageSummarySchema {
		t.Fatalf("schema = %q", got.Schema)
	}
	// Coverage includes the unparseable_records field.
	if got.Coverage.UnparseableRecords != 0 {
		t.Fatalf("no torn lines, unparseable_records should be 0")
	}
	if got.Coverage.Records != 2 || got.Coverage.CapturedRecords != 1 || got.Coverage.UncapturedRecords != 1 {
		t.Fatalf("coverage records mismatch: %#v", got.Coverage)
	}
	// Totals: captured-only tokens.
	if got.Totals.TotalTokens != 150 || !got.Totals.CapturedOnly {
		t.Fatalf("totals must sum only captured records: %#v", got.Totals)
	}
	// Groups: captured-only tokens per group.
	if len(got.Groups) != 1 || got.Groups[0].Key != "execute" {
		t.Fatalf("unexpected groups: %#v", got.Groups)
	}
	eg := got.Groups[0]
	if eg.Records != 2 || eg.CapturedRecords != 1 {
		t.Fatalf("group records/captured mismatch: %#v", eg)
	}
	if eg.TotalTokens != 150 {
		t.Fatalf("group tokens must sum only captured: %#v", eg)
	}
	// Warnings is a non-null array.
	if got.Warnings == nil {
		t.Fatalf("warnings must be a non-null array")
	}
}

// TestUsageSummaryLowerBound pins that any uncaptured record makes totals a
// lower bound: coverage.complete=false, totals.lower_bound=true, and the human
// output tags TOTAL with LOWER BOUND and lists uncaptured provider/stage pairs.
func TestUsageSummaryLowerBound(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_cap", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u_uncap", RunID: runID, Stage: "review",
		Provider: "anthropic", Agent: "claude",
		Captured: false,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Coverage.Complete || !got.Totals.LowerBound {
		t.Fatalf("uncaptured record must set complete=false and lower_bound=true")
	}
	if got.Coverage.UncapturedRecords != 1 {
		t.Fatalf("uncaptured_records = %d, want 1", got.Coverage.UncapturedRecords)
	}
	if len(got.Coverage.Uncaptured) != 1 {
		t.Fatalf("uncaptured list len = %d, want 1: %#v", len(got.Coverage.Uncaptured), got.Coverage.Uncaptured)
	}
	uc := got.Coverage.Uncaptured[0]
	if uc.Provider != "anthropic" || uc.Stage != "review" || uc.Records != 1 {
		t.Fatalf("uncaptured source mismatch: %#v", uc)
	}
	// Totals reflect only captured records.
	if got.Totals.TotalTokens != 150 {
		t.Fatalf("lower-bound total must be captured-only: %#v", got.Totals)
	}

	// Human output.
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	if !strings.Contains(text, "LOWER BOUND") {
		t.Fatalf("human output must contain LOWER BOUND:\n%s", text)
	}
	if !strings.Contains(text, "anthropic") || !strings.Contains(text, "review") {
		t.Fatalf("human output must list uncaptured provider/stage:\n%s", text)
	}
}

// TestUsageSummaryMissingLedger pins that a missing usage.jsonl for a run
// returns zero totals and a warning instead of an error.
func TestUsageSummaryMissingLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	// task new creates an empty usage.jsonl; remove it to simulate a missing ledger.
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	_ = os.Remove(runPaths.UsageJSONL)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage on missing ledger must not fail: exit %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Coverage.Records != 0 || got.Totals.TotalTokens != 0 {
		t.Fatalf("missing ledger should yield zero records and zero tokens: %#v", got)
	}
	if len(got.Warnings) == 0 {
		t.Fatalf("missing ledger should emit a warning: %#v", got.Warnings)
	}
	if !strings.Contains(got.Warnings[0], "no usage ledger") {
		t.Fatalf("warning should mention 'no usage ledger': %v", got.Warnings)
	}
}

// TestUsageSummaryAllMissingLedger pins that --all treats a run with no usage
// ledger as zero records with a warning, rather than an error.
func TestUsageSummaryAllMissingLedger(t *testing.T) {
	root := t.TempDir()
	app, paths, runA := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runA))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runA, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})
	// A second run with no ledger.
	assertNoError(t, os.MkdirAll(filepath.Join(paths.RunsDir, "run_20260601_090000"), 0o755))

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all must not fail on missing ledger: exit %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Coverage.Records != 1 || got.Totals.TotalTokens != 150 {
		t.Fatalf("only the run with a valid ledger should count: %#v", got)
	}
	if len(got.Warnings) == 0 {
		t.Fatalf("missing ledger should emit a warning in --all mode: %#v", got.Warnings)
	}
}

// TestUsageSummaryMalformedRecord pins that a non-final malformed JSONL line
// is an error (not silently skipped).
func TestUsageSummaryMalformedRecord(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})
	// Non-final malformed line.
	f, err := os.OpenFile(runPaths.UsageJSONL, os.O_APPEND|os.O_WRONLY, 0o644)
	assertNoError(t, err)
	_, _ = f.WriteString("{not valid json\n")
	// A valid final line after the malformed one.
	assertNoError(t, appendJSONLine(runPaths.UsageJSONL, UsageRecord{
		RecordID: "u3", RunID: runID, Stage: "review",
		Provider: "anthropic", Agent: "claude",
		InputTokens: 50, OutputTokens: 25, TotalTokens: 75, Captured: true,
	}))
	assertNoError(t, f.Close())

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code == 0 {
		t.Fatalf("usage must fail on non-final malformed line, got exit 0:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "malformed") {
		t.Fatalf("error must mention 'malformed': %s", stderr.String())
	}
}

// TestUsageSummaryPartialFinalLine pins that the final line of a usage JSONL
// is always treated as potentially torn: if it fails JSON parsing it is skipped,
// a warning is emitted, and it is counted in coverage.unparseable_records.
func TestUsageSummaryPartialFinalLine(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})
	// Torn final line: truncated JSON.
	f, err := os.OpenFile(runPaths.UsageJSONL, os.O_APPEND|os.O_WRONLY, 0o644)
	assertNoError(t, err)
	_, _ = f.WriteString(`{"record_id":"torn","stage":"execute"`)
	assertNoError(t, f.Close())

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("torn final line must not fail: exit %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	// Only the first (valid) record counts.
	if got.Coverage.Records != 1 {
		t.Fatalf("records = %d, want 1 (torn line excluded)", got.Coverage.Records)
	}
	if got.Totals.TotalTokens != 150 {
		t.Fatalf("tokens must reflect only the valid record: %#v", got.Totals)
	}
	// The torn line is counted as unparseable.
	if got.Coverage.UnparseableRecords != 1 {
		t.Fatalf("unparseable_records = %d, want 1", got.Coverage.UnparseableRecords)
	}
	// A warning is emitted.
	if len(got.Warnings) == 0 || !strings.Contains(got.Warnings[0], "torn") {
		t.Fatalf("torn line must emit a warning: %#v", got.Warnings)
	}
	// The torn line is excluded from coverage.complete / lower_bound calculation.
	if !got.Coverage.Complete || got.Totals.LowerBound {
		t.Fatalf("torn line must not affect coverage.complete or totals.lower_bound")
	}
}

// TestUsageSummaryOversizedFinalLine pins that an oversized (>16MB) final line
// is treated the same as a torn line: skipped, warned, counted as unparseable.
func TestUsageSummaryOversizedFinalLine(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})
	// Oversized final line: not valid JSON, triggers torn-line treatment.
	f, err := os.OpenFile(runPaths.UsageJSONL, os.O_APPEND|os.O_WRONLY, 0o644)
	assertNoError(t, err)
	_, _ = f.WriteString(strings.Repeat("x", 17*1024*1024) + "\n")
	assertNoError(t, f.Close())

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("oversized final line must not fail: exit %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Coverage.Records != 1 || got.Coverage.UnparseableRecords != 1 {
		t.Fatalf("oversized final line must be counted as unparseable: %#v", got.Coverage)
	}
}

// TestUsageSummaryNoWorkspaceMutation pins that pactum usage never creates,
// modifies, or deletes any file or directory during execution.
func TestUsageSummaryNoWorkspaceMutation(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})

	before := workspaceSnapshot(t, paths)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	if code := app.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all exited %d, stderr: %s", code, stderr.String())
	}

	after := workspaceSnapshot(t, paths)
	if before != after {
		t.Fatalf("pactum usage must not mutate the workspace:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestUsageSummaryAllEmptyWorkspace pins that usage --all on an empty workspace
// reports zero records with no error.
func TestUsageSummaryAllEmptyWorkspace(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	a := testApp(root)

	var stdout, stderr bytes.Buffer
	if code := a.Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()

	if code := a.Run([]string{"usage", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --all on empty workspace exited %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))
	if got.Coverage.Records != 0 || got.Totals.TotalTokens != 0 {
		t.Fatalf("empty workspace should report zero: %#v", got)
	}
	if got.Groups == nil || len(got.Groups) != 0 {
		t.Fatalf("empty groups must be an empty non-null slice: %#v", got.Groups)
	}
	if !got.Coverage.Complete {
		t.Fatalf("empty workspace should be complete (no uncaptured records)")
	}
}

// TestUsageSummaryCurrentRunDefault pins that pactum usage with no run_id
// resolves the current run when one is set.
func TestUsageSummaryCurrentRunDefault(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150, Captured: true,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage (no run_id) exited %d, stderr: %s", code, stderr.String())
	}
	var summary usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &summary))
	if summary.Scope.Kind != "run" || summary.Scope.RunID == nil || *summary.Scope.RunID != runID {
		t.Fatalf("expected scope run=%s, got %#v", runID, summary.Scope)
	}
	if summary.Totals.TotalTokens != 150 {
		t.Fatalf("expected 150 total tokens, got %d", summary.Totals.TotalTokens)
	}
}

// TestUsageSummaryNoRunIDNoCurrentRunFails pins that pactum usage with no
// run_id and no resolvable current run exits non-zero with a clear error.
func TestUsageSummaryNoRunIDNoCurrentRunFails(t *testing.T) {
	root := t.TempDir()
	app, paths, _ := setupContractRun(t, root)
	// A second active run makes the resolver ambiguous (len(active) != 1).
	_ = runContractOnlyForTask(t, app, "second task")
	assertNoError(t, os.Remove(currentRunPointerPath(paths)))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"usage"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("usage (no run_id, ambiguous) exited 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "run id") {
		t.Fatalf("expected 'run id' in error, got: %s", stderr.String())
	}
}

// TestUsageSummaryCacheAndEffectiveUnits pins that cache token fields and
// effective_units are correctly aggregated in JSON totals and groups, and that
// the human table shows the new columns with correct values.
func TestUsageSummaryCacheAndEffectiveUnits(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// codex: fresh=75, cache_read=25, output=50
	// effective = 75*1.0 + 25*0.1 + 50*5.0 + 0*1.0 = 327.5
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, CacheReadTokens: 25, OutputTokens: 50, TotalTokens: 150,
		Captured: true,
	})
	// anthropic: fresh=140, cache_read=50, cache_create=10, output=100
	// effective = 140*1.0 + 50*0.1 + 100*5.0 + 10*1.25 = 657.5
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u2", RunID: runID, Stage: "fix",
		Provider: "anthropic", Agent: "claude",
		InputTokens: 200, CacheReadTokens: 50, CacheCreationTokens: 10,
		OutputTokens: 100, TotalTokens: 300,
		Captured: true,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	// Totals: sum of both captured records.
	if got.Totals.CacheReadTokens != 75 {
		t.Fatalf("totals.cache_read_tokens = %d, want 75", got.Totals.CacheReadTokens)
	}
	if got.Totals.CacheCreationTokens != 10 {
		t.Fatalf("totals.cache_creation_tokens = %d, want 10", got.Totals.CacheCreationTokens)
	}
	// cache_read_ratio = 75/300 = 0.25
	if got.Totals.CacheReadRatio < 0.249 || got.Totals.CacheReadRatio > 0.251 {
		t.Fatalf("totals.cache_read_ratio = %f, want ~0.25", got.Totals.CacheReadRatio)
	}
	// effective_units = 327.5 + 657.5 = 985.0
	if got.Totals.EffectiveUnits < 984.9 || got.Totals.EffectiveUnits > 985.1 {
		t.Fatalf("totals.effective_units = %f, want ~985.0", got.Totals.EffectiveUnits)
	}

	// Groups: execute group (codex).
	groups := indexSummaryGroups(got.Groups)
	exec := groups["execute"]
	if exec.CacheReadTokens != 25 || exec.CacheCreationTokens != 0 {
		t.Fatalf("execute group cache tokens: read=%d, create=%d, want 25/0", exec.CacheReadTokens, exec.CacheCreationTokens)
	}
	if exec.CacheReadRatio < 0.249 || exec.CacheReadRatio > 0.251 {
		t.Fatalf("execute group cache_read_ratio = %f, want ~0.25", exec.CacheReadRatio)
	}
	if exec.EffectiveUnits < 327.4 || exec.EffectiveUnits > 327.6 {
		t.Fatalf("execute group effective_units = %f, want ~327.5", exec.EffectiveUnits)
	}

	// Human table must include the new columns and a known value.
	stdout.Reset()
	stderr.Reset()
	if code := app.Run([]string{"usage", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage exited %d, stderr: %s", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{"CR_READ", "CR_WRITE", "CACHE%", "EFFECTIVE", "25.0%", "985"} {
		if !strings.Contains(text, want) {
			t.Fatalf("human output missing %q:\n%s", want, text)
		}
	}
}

// TestUsageSummaryUncapturedContributesZeroCache pins that uncaptured records
// do not add to cache_read_tokens, cache_creation_tokens, or effective_units.
func TestUsageSummaryUncapturedContributesZeroCache(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 100, CacheReadTokens: 25, OutputTokens: 50, TotalTokens: 150,
		Captured: true,
	})
	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u2", RunID: runID, Stage: "review",
		Provider: "anthropic", Agent: "claude",
		InputTokens: 9999, CacheReadTokens: 8000, CacheCreationTokens: 500,
		OutputTokens: 999, TotalTokens: 9999,
		Captured: false, // must not contribute to any totals
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Totals.CacheReadTokens != 25 {
		t.Fatalf("uncaptured must not add cache_read: got %d, want 25", got.Totals.CacheReadTokens)
	}
	if got.Totals.CacheCreationTokens != 0 {
		t.Fatalf("uncaptured must not add cache_creation: got %d, want 0", got.Totals.CacheCreationTokens)
	}
	if got.Totals.EffectiveUnits <= 0 {
		t.Fatalf("effective_units should be positive from the captured record: %f", got.Totals.EffectiveUnits)
	}

	groups := indexSummaryGroups(got.Groups)
	if groups["execute"].CacheReadTokens != 25 {
		t.Fatalf("execute group cache_read_tokens = %d, want 25", groups["execute"].CacheReadTokens)
	}
	if groups["review"].CacheReadTokens != 0 || groups["review"].EffectiveUnits != 0 {
		t.Fatalf("review group (uncaptured) must have zero cache/effective: %#v", groups["review"])
	}
}

// TestUsageSummaryZeroInputCacheRatio pins that cache_read_ratio is 0 when
// input_tokens is 0, not a division-by-zero error or NaN.
func TestUsageSummaryZeroInputCacheRatio(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	appendUsageRecordForTest(t, runPaths.UsageJSONL, UsageRecord{
		RecordID: "u1", RunID: runID, Stage: "execute",
		Provider: "codex", Agent: "codex",
		InputTokens: 0, CacheReadTokens: 0, OutputTokens: 0, TotalTokens: 0,
		Captured: true,
	})

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"usage", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("usage --json exited %d, stderr: %s", code, stderr.String())
	}
	var got usageSummaryResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &got))

	if got.Totals.CacheReadRatio != 0 {
		t.Fatalf("cache_read_ratio with zero input must be 0, got %f", got.Totals.CacheReadRatio)
	}
	if len(got.Groups) != 1 || got.Groups[0].CacheReadRatio != 0 {
		t.Fatalf("group cache_read_ratio with zero input must be 0: %#v", got.Groups)
	}
}

// --- helpers ---

func appendWorkspaceUsageRecord(t *testing.T, paths artifacts.Paths, runID string, record UsageRecord) {
	t.Helper()
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendUsageRecordForTest(t, runPaths.UsageJSONL, record)
}

func indexSummaryGroups(groups []usageSummaryGroup) map[string]usageSummaryGroup {
	m := map[string]usageSummaryGroup{}
	for _, g := range groups {
		m[g.Key] = g
	}
	return m
}

func codexHelperAgentDescriptor() agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    agents.BuiltinCodex,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestExecutionHelperProcess", "--"},
		Input:   agents.InputPromptFile,
	}
}

func appendUsageRecordForTest(t *testing.T, path string, record UsageRecord) {
	t.Helper()
	assertNoError(t, appendJSONLine(path, record))
}

// workspaceSnapshot returns a sorted, newline-joined list of all paths under
// the workspace directory, each annotated with its size, for mutation
// detection. Including the size means a rewrite to an existing file (same
// path, different content) is detected even when no paths change.
func workspaceSnapshot(t *testing.T, paths artifacts.Paths) string {
	t.Helper()
	var entries []string
	err := filepath.Walk(paths.Workspace, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(paths.Workspace, p)
		if relErr != nil {
			return relErr
		}
		entries = append(entries, fmt.Sprintf("%s\t%d", rel, info.Size()))
		return nil
	})
	assertNoError(t, err)
	return strings.Join(entries, "\n")
}
