package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
)

func TestContractReviseUsesSwappedStore(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	assertNoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	assertNoError(t, os.MkdirAll(paths.Workspace, 0o755))

	fake := newMemoryStore()
	swapActiveStore(t, fake)

	runID := "run_20260608_010203"
	runDir := filepath.Join(paths.RunsDir, runID)
	runPaths := contractRunPaths(runDir)
	assertNoError(t, fake.MkdirAll(runDir))

	state := contractRunState{
		Schema:    runSchema,
		RunID:     runID,
		Status:    "contract_draft",
		Task:      "swap store",
		CreatedAt: time.Date(2026, 6, 8, 1, 2, 3, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 8, 1, 2, 3, 0, time.UTC),
		RepoRoot:  ".",
		Workspace: artifacts.WorkspaceRel,
	}
	contract := draftContract{
		Schema:             contractSchema,
		RunID:              runID,
		Status:             "draft",
		Goal:               "old goal",
		Scope:              draftContractScope{In: []string{}, Out: []string{}},
		AcceptanceCriteria: []string{},
		Validation:         draftValidation{Commands: []string{}},
		Assumptions:        []string{},
		OpenQuestions:      []string{},
	}
	assertNoError(t, writeJSON(runPaths.RunJSON, state))
	assertNoError(t, writeJSON(runPaths.ContractJSON, contract))
	assertNoError(t, writeJSON(runPaths.ApprovalJSON, pendingApprovalState()))
	assertNoError(t, activeStore.WriteBytes(runPaths.ContractMD, []byte("# Contract\n\nold goal\n"), 0o644))
	assertNoError(t, activeStore.WriteBytes(runPaths.PromptMD, []byte("# Executor Prompt\n"), 0o644))

	fromFile := writeReviseDocForTest(t, runPaths, map[string]any{"goal": "updated goal"})
	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"contract", "revise", runID, "--from", fromFile, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stderr: %s, stdout: %s", code, stderr.String(), stdout.String())
	}

	updated, err := readDraftContract(runPaths.ContractJSON)
	assertNoError(t, err)
	if updated.Goal != "updated goal" {
		t.Fatalf("contract goal = %q, want updated goal", updated.Goal)
	}
	if !fake.Exists(paths.EventsJSONL) {
		t.Fatalf("events ledger was not written through fake store")
	}
	events, err := fake.ReadBytes(paths.EventsJSONL)
	assertNoError(t, err)
	if !strings.Contains(string(events), `"type":"contract_revised"`) {
		t.Fatalf("events ledger missing contract_revised event:\n%s", events)
	}
	for _, path := range []string{runPaths.ContractJSON, runPaths.ContractMD, runPaths.PromptMD, paths.EventsJSONL} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("durable artifact unexpectedly exists on filesystem: %s", path)
		}
	}
}
