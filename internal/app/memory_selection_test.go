package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

func TestMemorySearchBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"memory", "search", "review proposal boundary"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search before init exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Pactum is not initialized. Run: pactum init") {
		t.Fatalf("memory search before init output mismatch:\n%s", got)
	}
}

func TestMemorySearchWithNoAcceptedMemory(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	if err := os.Remove(paths.MemoryItems); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	assertNoFile(t, paths.MemoryItems)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "search", "review proposal boundary"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "No accepted memory items matched.") {
		t.Fatalf("memory search no-memory output mismatch:\n%s", got)
	}
}

func TestMemorySearchFindsLexicalMatch(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Review proposal boundary",
		Summary:    "Reviewer proposals stay append-only.",
		Files:      []string{"internal/app/review_proposals.go"},
		Tags:       []string{"reviewed"},
		Candidate:  "runs/run_old/memory/memory-candidate.json",
	})
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_002",
		AcceptedAt: "2026-06-02T10:00:00Z",
		Title:      "Executor prompt manifest",
		Summary:    "Prompt build writes executor artifacts.",
		Files:      []string{"internal/app/prompt.go"},
		Tags:       []string{"contract"},
		Candidate:  "runs/run_other/memory/memory-candidate.json",
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "search", "review proposal boundary"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "1. mem_001 score=") || strings.Contains(got, "1. mem_002 score=") {
		t.Fatalf("memory search lexical output mismatch:\n%s", got)
	}
}

func TestMemorySearchJSONOutput(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Review proposal boundary",
		Summary:    "Proposal parsing keeps manual review append-only.",
		Files:      []string{"internal/app/review.go"},
		Tags:       []string{"reviewed"},
		Candidate:  "runs/run_old/memory/memory-candidate.json",
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "search", "review proposal", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search --json exited %d, stderr: %s", code, stderr.String())
	}
	var response memorySearchResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if response.Query != "review proposal" || response.Limit != defaultMemorySelectionLimit {
		t.Fatalf("memory search json metadata mismatch: %#v", response)
	}
	if len(response.Selected) != 1 || response.Selected[0].ID != "mem_001" || response.Selected[0].Title != "Review proposal boundary" || response.Selected[0].Score <= 0 {
		t.Fatalf("memory search json selected mismatch: %#v", response.Selected)
	}
}

func TestMemorySelectorDeterministicOrdering(t *testing.T) {
	root := t.TempDir()
	_, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{ID: "mem_002", AcceptedAt: "2026-06-02T09:00:00Z", Title: "Cache boundary"})
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{ID: "mem_001", AcceptedAt: "2026-06-02T09:00:00Z", Title: "Cache boundary"})
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{ID: "mem_003", AcceptedAt: "2026-06-02T10:00:00Z", Title: "Cache boundary"})

	selected, noUsefulTokens, err := selectAcceptedMemoryItems(paths.MemoryItems, root, "cache", 5)
	assertNoError(t, err)
	if noUsefulTokens {
		t.Fatalf("selector unexpectedly reported no useful tokens")
	}
	got := selectedMemoryIDs(selected)
	want := []string{"mem_003", "mem_001", "mem_002"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("selected order = %v, want %v", got, want)
	}
}

func TestRunCreatesMemoryContextWithNoMemory(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	runID := runContractOnlyForTask(t, app, "add accepted memory retrieval")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	assertFile(t, runPaths.MemoryContextMD)
	assertFile(t, runPaths.MemorySelectionJSON)
	selection := readMemorySelectionForTest(t, runPaths.MemorySelectionJSON)
	if selection.Schema != memorySelectionSchema || selection.RunID != runID || selection.QuerySource != "task" || len(selection.Selected) != 0 {
		t.Fatalf("unexpected run memory selection: %#v", selection)
	}
	if got := mustReadFile(t, runPaths.MemoryContextMD); !strings.Contains(got, "No accepted memory items matched this run.") {
		t.Fatalf("memory context missing empty selection guidance:\n%s", got)
	}
	if got := mustReadFile(t, runPaths.RepoContext); !strings.Contains(got, "Accepted memory context: context/memory-context.md") {
		t.Fatalf("repo context missing memory pointer:\n%s", got)
	}
}

func TestRunSelectsAcceptedMemoryFromTask(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Accepted memory retrieval",
		Summary:    "Run context should include selected memory.",
		Files:      []string{"internal/app/memory_selection.go"},
		Tags:       []string{"contract", "reviewed"},
		Candidate:  "runs/run_old/memory/memory-candidate.json",
	})

	runID := runContractOnlyForTask(t, app, "add accepted memory retrieval into run context")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	selection := readMemorySelectionForTest(t, runPaths.MemorySelectionJSON)
	if selection.QuerySource != "task" || len(selection.Selected) != 1 || selection.Selected[0].ID != "mem_001" {
		t.Fatalf("run memory selection mismatch: %#v", selection)
	}
	contextMD := mustReadFile(t, runPaths.MemoryContextMD)
	if !strings.Contains(contextMD, "mem_001") || !strings.Contains(contextMD, "Accepted memory retrieval") {
		t.Fatalf("memory context missing selected item:\n%s", contextMD)
	}
}

func TestPromptBuildRefreshesMemoryUsingContract(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPromptBuildMemoryRefresh(t, root)

	initial := readMemorySelectionForTest(t, runPaths.MemorySelectionJSON)
	if initial.QuerySource != "task" || len(initial.Selected) != 0 {
		t.Fatalf("initial task memory selection should be empty: %#v", initial)
	}
	runMemorySelectionCommand(t, app, "prompt", "build", runID)
	selection := readMemorySelectionForTest(t, runPaths.MemorySelectionJSON)
	if selection.QuerySource != "contract" || len(selection.Selected) != 1 || selection.Selected[0].ID != "mem_001" {
		t.Fatalf("prompt build memory selection mismatch: %#v", selection)
	}
	if !strings.Contains(mustReadFile(t, runPaths.MemoryContextMD), "Contract memory retrieval") {
		t.Fatalf("memory context was not refreshed from contract:\n%s", mustReadFile(t, runPaths.MemoryContextMD))
	}
}

func TestExecutorContextReferencesMemoryContext(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPromptBuildMemoryRefresh(t, root)
	runMemorySelectionCommand(t, app, "prompt", "build", runID)

	context := mustReadFile(t, runPaths.ExecutorContext)
	for _, want := range []string{
		"## Accepted memory",
		"Memory context: context/memory-context.md",
		"Selection: context/memory-selection.json",
		"Treat memory as context, not semantic truth.",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("executor context missing %q:\n%s", want, context)
		}
	}
}

func TestPromptReferencesMemoryContext(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupPromptBuildMemoryRefresh(t, root)
	runMemorySelectionCommand(t, app, "prompt", "build", runID)

	prompt := mustReadFile(t, runPaths.PromptMD)
	if !strings.Contains(prompt, "- Accepted memory context: context/memory-context.md") {
		t.Fatalf("prompt missing accepted memory pointer:\n%s", prompt)
	}
}

func TestReviewerContextReferencesMemoryContext(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")

	runReviewCommand(t, app, "review", "dry-run", runID)
	context := mustReadFile(t, runPaths.ReviewContextMD)
	if !strings.Contains(context, "Memory context: context/memory-context.md") {
		t.Fatalf("reviewer context missing memory pointer:\n%s", context)
	}
}

func TestMemorySelectionDoesNotIncludeZeroScoreItems(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Cache boundary",
		Summary:    "SQLite cache behavior.",
		Files:      []string{"internal/app/cache.go"},
		Tags:       []string{"cache"},
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "search", "review proposal", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search --json exited %d, stderr: %s", code, stderr.String())
	}
	var response memorySearchResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if len(response.Selected) != 0 {
		t.Fatalf("zero-score item should not be selected: %#v", response.Selected)
	}
}

func TestMemorySelectionUsefulTokenFiltering(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	runID := runContractOnlyForTask(t, app, "the an to add")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	selection := readMemorySelectionForTest(t, runPaths.MemorySelectionJSON)
	if len(selection.Selected) != 0 || !hasMemorySelectionNote(selection, "No useful memory query tokens were available.") {
		t.Fatalf("selection should record no useful token note: %#v", selection)
	}
}

func TestMemoryContextArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Portable memory " + root,
		Summary:    "Summary mentions " + filepath.ToSlash(root),
		Files:      []string{filepath.Join(root, "internal", "app", "memory_selection.go")},
		Tags:       []string{"portable"},
		Candidate:  filepath.Join(root, ".heurema", "pactum", "runs", "run_old", "memory", "memory-candidate.json"),
	})
	runID := runContractOnlyForTask(t, app, "portable memory")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runMemorySelectionCommand(t, app, "contract", "approve", runID)
	runMemorySelectionCommand(t, app, "prompt", "build", runID)

	for name, content := range map[string]string{
		"context/memory-context.md":     mustReadFile(t, runPaths.MemoryContextMD),
		"context/memory-selection.json": mustReadFile(t, runPaths.MemorySelectionJSON),
		"context/repo-context.md":       mustReadFile(t, runPaths.RepoContext),
		"context/executor-context.md":   mustReadFile(t, runPaths.ExecutorContext),
		"contract/prompt.md":            mustReadFile(t, runPaths.PromptMD),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestMemorySearchReadOnly(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Cache boundary",
		Summary:    "Read-only search fixture.",
		Files:      []string{"internal/app/memory.go"},
		Tags:       []string{"cache"},
	})
	beforeItems := mustReadFile(t, paths.MemoryItems)
	beforeEvents := mustReadFile(t, paths.EventsJSONL)

	runMemorySelectionCommand(t, app, "memory", "search", "cache boundary")
	runMemorySelectionCommand(t, app, "memory", "search", "cache boundary", "--json")

	if got := mustReadFile(t, paths.MemoryItems); got != beforeItems {
		t.Fatalf("memory search mutated items.jsonl")
	}
	if got := mustReadFile(t, paths.EventsJSONL); got != beforeEvents {
		t.Fatalf("memory search appended ledger events")
	}
}

func setupInitializedMemoryWorkspace(t *testing.T, root string) (App, artifacts.Paths) {
	t.Helper()
	mustWriteFile(t, filepath.Join(root, "README.md"), "# Example\n")
	app := testApp(root)
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init exited %d, stderr: %s", code, stderr.String())
	}
	return app, artifacts.New(root)
}

func runContractOnlyForTask(t *testing.T, app App, task string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"run", task, "--contract-only", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run --contract-only exited %d, stderr: %s", code, stderr.String())
	}
	var state contractRunState
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &state))
	return state.RunID
}

func appendAcceptedMemoryItemForTest(t *testing.T, paths artifacts.Paths, item memoryItemRecord) {
	t.Helper()
	if item.Schema == "" {
		item.Schema = memoryItemSchema
	}
	if item.RunID == "" {
		item.RunID = "run_old"
	}
	if item.AcceptedAt == "" {
		item.AcceptedAt = "2026-06-02T09:00:00Z"
	}
	if item.AcceptedBy == "" {
		item.AcceptedBy = "test"
	}
	if item.Source == "" {
		item.Source = "memory_candidate"
	}
	if item.Candidate == "" {
		item.Candidate = "runs/" + item.RunID + "/memory/memory-candidate.json"
	}
	assertNoError(t, appendJSONLine(paths.MemoryItems, item))
}

func readMemorySelectionForTest(t *testing.T, path string) memorySelectionDocument {
	t.Helper()
	var selection memorySelectionDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, path)), &selection))
	return selection
}

func selectedMemoryIDs(items []memorySelectedItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func hasMemorySelectionNote(selection memorySelectionDocument, note string) bool {
	for _, got := range selection.Notes {
		if got == note {
			return true
		}
	}
	return false
}

func setupPromptBuildMemoryRefresh(t *testing.T, root string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Contract memory retrieval",
		Summary:    "Prompt build refreshes selected accepted memory using contract fields.",
		Files:      []string{"internal/app/prompt.go"},
		Tags:       []string{"contract", "reviewed"},
		Candidate:  "runs/run_old/memory/memory-candidate.json",
	})
	runID := runContractOnlyForTask(t, app, "unrelated task")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	runMemorySelectionCommand(t, app, "contract", "revise", runID,
		"--goal", "add contract memory retrieval",
		"--add-in-scope", "Refresh memory context from approved contract",
		"--add-acceptance", "Contract memory retrieval selects accepted memory",
		"--add-validation", "go test ./...",
	)
	runMemorySelectionCommand(t, app, "contract", "approve", runID)
	return app, paths, runID, runPaths
}

func runMemorySelectionCommand(t *testing.T, app App, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := app.Run(args, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("%v exited %d, stdout: %s stderr: %s", args, code, stdout.String(), stderr.String())
	}
}
