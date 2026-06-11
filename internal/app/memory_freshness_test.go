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

func TestMemoryAcceptRecordsFreshnessForFiles(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)
	mustWriteFile(t, filepath.Join(root, "internal/app/memory.go"), "package app\n")

	runMemoryCommand(t, app, "memory", "propose", runID)
	runMemoryCommand(t, app, "memory", "accept", runID)

	items := readMemoryItemsForTest(t, paths.MemoryItems)
	if len(items) != 1 || items[0].Freshness == nil {
		t.Fatalf("accepted item missing freshness: %#v", items)
	}
	freshness := items[0].Freshness
	if freshness.Status != memoryFreshnessFresh || len(freshness.Files) != 1 {
		t.Fatalf("freshness mismatch: %#v", freshness)
	}
	file := freshness.Files[0]
	if file.Path != "internal/app/memory.go" || file.Status != memoryFileUnchanged || file.AcceptedSHA256 == "" || file.CurrentSHA256 == "" {
		t.Fatalf("file freshness mismatch: %#v", file)
	}
	wantHash, err := fileSHA256(filepath.Join(root, "internal/app/memory.go"))
	assertNoError(t, err)
	if file.AcceptedSHA256 != wantHash || file.CurrentSHA256 != wantHash {
		t.Fatalf("hash mismatch: %#v want %s", file, wantHash)
	}
	if !strings.Contains(mustReadFile(t, runPaths.MemoryAcceptanceJSON), "mem_001") {
		t.Fatalf("acceptance missing item id")
	}
}

func TestMemoryAcceptMarksMissingFilesStale(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedReviewedMemoryRun(t, root)

	runMemoryCommand(t, app, "memory", "propose", runID)
	runMemoryCommand(t, app, "memory", "accept", runID)

	items := readMemoryItemsForTest(t, paths.MemoryItems)
	freshness := items[0].Freshness
	if freshness == nil || freshness.Status != memoryFreshnessStale {
		t.Fatalf("missing file should make accepted item stale: %#v", items[0])
	}
	if len(freshness.Files) != 1 || freshness.Files[0].Status != memoryFileMissing {
		t.Fatalf("missing file status mismatch: %#v", freshness.Files)
	}
	if !hasString(freshness.Reasons, "missing file internal/app/memory.go") {
		t.Fatalf("missing file reason absent: %#v", freshness.Reasons)
	}
}

func TestMemoryRefreshBeforeInitPrintsGuidance(t *testing.T) {
	root := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"memory", "refresh"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("memory refresh before init exited %d, want 1, stderr: %s", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "not initialized") {
		t.Fatalf("memory refresh before init stderr mismatch:\n%s", got)
	}
}

func TestMemoryRefreshWithNoItems(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory refresh exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "total: 0") || !strings.Contains(got, "refreshes.jsonl") {
		t.Fatalf("memory refresh no-items output mismatch:\n%s", got)
	}
	refreshes := readMemoryRefreshesForTest(t, paths.MemoryRefreshes)
	if len(refreshes) != 1 || refreshes[0].ID != "memory_refresh_001" || len(refreshes[0].Items) != 0 {
		t.Fatalf("unexpected no-items refresh: %#v", refreshes)
	}
	if got := mustReadFile(t, paths.ProjectMemory); !strings.Contains(got, "No accepted memory items.") {
		t.Fatalf("project memory not regenerated for no items:\n%s", got)
	}
}

func TestMemoryRefreshDetectsChangedFile(t *testing.T) {
	root := t.TempDir()
	app, paths, _, _ := setupAcceptedMemoryWithExistingFile(t, root)
	mustWriteFile(t, filepath.Join(root, "internal/app/memory.go"), "package app\n\nconst changed = true\n")

	runMemoryCommand(t, app, "memory", "refresh")

	refresh := latestMemoryRefreshForTest(t, paths.MemoryRefreshes)
	item := refresh.Items[0]
	if item.Status != memoryFreshnessStale || len(item.Files) != 1 || item.Files[0].Status != memoryFileChanged {
		t.Fatalf("changed file refresh mismatch: %#v", refresh)
	}
	if item.Files[0].AcceptedSHA256 == "" || item.Files[0].CurrentSHA256 == "" || item.Files[0].AcceptedSHA256 == item.Files[0].CurrentSHA256 {
		t.Fatalf("changed file hashes mismatch: %#v", item.Files[0])
	}
}

func TestMemoryRefreshDetectsMissingFile(t *testing.T) {
	root := t.TempDir()
	app, paths, _, _ := setupAcceptedMemoryWithExistingFile(t, root)
	assertNoError(t, os.Remove(filepath.Join(root, "internal/app/memory.go")))

	runMemoryCommand(t, app, "memory", "refresh")

	refresh := latestMemoryRefreshForTest(t, paths.MemoryRefreshes)
	item := refresh.Items[0]
	if item.Status != memoryFreshnessStale || len(item.Files) != 1 || item.Files[0].Status != memoryFileMissing {
		t.Fatalf("missing file refresh mismatch: %#v", refresh)
	}
	if !hasString(item.Reasons, "missing file internal/app/memory.go") {
		t.Fatalf("missing refresh reason absent: %#v", item.Reasons)
	}
}

func TestMemoryStaleReadOnly(t *testing.T) {
	root := t.TempDir()
	app, paths, _, _ := setupAcceptedMemoryWithExistingFile(t, root)
	mustWriteFile(t, filepath.Join(root, "internal/app/memory.go"), "package app\n\nconst changed = true\n")
	runMemoryCommand(t, app, "memory", "refresh")

	beforeItems := mustReadFile(t, paths.MemoryItems)
	beforeRefreshes := mustReadFile(t, paths.MemoryRefreshes)
	beforeProjectMemory := mustReadFile(t, paths.ProjectMemory)
	beforeLedger := mustReadFile(t, paths.EventsJSONL)

	runMemoryCommand(t, app, "memory", "stale")
	runMemoryCommand(t, app, "memory", "stale", "--json")

	if got := mustReadFile(t, paths.MemoryItems); got != beforeItems {
		t.Fatalf("memory stale mutated items.jsonl")
	}
	if got := mustReadFile(t, paths.MemoryRefreshes); got != beforeRefreshes {
		t.Fatalf("memory stale mutated refreshes.jsonl")
	}
	if got := mustReadFile(t, paths.ProjectMemory); got != beforeProjectMemory {
		t.Fatalf("memory stale mutated project-memory.md")
	}
	if got := mustReadFile(t, paths.EventsJSONL); got != beforeLedger {
		t.Fatalf("memory stale appended ledger events")
	}
}

func TestMemoryStaleReportsLatestRefresh(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{ID: "mem_001", Title: "Review proposal boundary"})
	appendMemoryRefreshForTest(t, paths, memoryRefreshRecord{
		Schema:    memoryRefreshSchema,
		ID:        "memory_refresh_001",
		CreatedAt: "2026-06-02T10:00:00Z",
		Items: []memoryRefreshItem{{
			MemoryItemID: "mem_001",
			Status:       memoryFreshnessStale,
			Reasons:      []string{"changed file internal/app/review.go"},
		}},
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "stale"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory stale exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "mem_001 Review proposal boundary") || !strings.Contains(got, "reason: changed file internal/app/review.go") {
		t.Fatalf("memory stale latest refresh output mismatch:\n%s", got)
	}
}

func TestMemorySearchIncludesFreshnessAndStalePenalty(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:         "mem_001",
		AcceptedAt: "2026-06-02T09:00:00Z",
		Title:      "Review proposal",
		Summary:    "Boundary handling",
	})
	appendMemoryRefreshForTest(t, paths, memoryRefreshRecord{
		Schema: "pactum.memory_refresh.v1",
		ID:     "memory_refresh_001",
		Items: []memoryRefreshItem{{
			MemoryItemID: "mem_001",
			Status:       memoryFreshnessStale,
			Reasons:      []string{"changed file internal/app/review.go"},
		}},
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "search", "review proposal", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search --json exited %d, stderr: %s", code, stderr.String())
	}
	var response memorySearchResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if len(response.Selected) != 1 {
		t.Fatalf("expected selected stale item: %#v", response)
	}
	item := response.Selected[0]
	if item.Score != 6 || item.Freshness.Status != memoryFreshnessStale || !hasString(item.Freshness.Reasons, "changed file internal/app/review.go") {
		t.Fatalf("stale search selection mismatch: %#v", item)
	}
}

func TestMemorySearchExcludesStaleItemAfterPenalty(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:    "mem_001",
		Title: "Unrelated title",
		Tags:  []string{"review"},
	})
	appendMemoryRefreshForTest(t, paths, memoryRefreshRecord{
		Schema: "pactum.memory_refresh.v1",
		ID:     "memory_refresh_001",
		Items: []memoryRefreshItem{{
			MemoryItemID: "mem_001",
			Status:       memoryFreshnessStale,
			Reasons:      []string{"changed file internal/app/review.go"},
		}},
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "search", "review", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search --json exited %d, stderr: %s", code, stderr.String())
	}
	var response memorySearchResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if len(response.Selected) != 0 {
		t.Fatalf("stale item with score after penalty <= 0 should be excluded: %#v", response.Selected)
	}
}

func TestRunMemoryContextIncludesFreshnessInfo(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{ID: "mem_001", Title: "Freshness boundary"})
	appendMemoryRefreshForTest(t, paths, memoryRefreshRecord{
		Schema: "pactum.memory_refresh.v1",
		ID:     "memory_refresh_001",
		Items: []memoryRefreshItem{{
			MemoryItemID: "mem_001",
			Status:       memoryFreshnessStale,
			Reasons:      []string{"changed file internal/app/memory.go"},
		}},
	})

	runID := runContractOnlyForTask(t, app, "freshness boundary")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	contextMD := mustReadFile(t, runPaths.MemoryContextMD)
	if !strings.Contains(contextMD, "Freshness: stale") || !strings.Contains(contextMD, "Reasons: changed file internal/app/memory.go") {
		t.Fatalf("memory context missing freshness info:\n%s", contextMD)
	}
}

func TestProjectMemoryIncludesFreshnessAfterRefresh(t *testing.T) {
	root := t.TempDir()
	app, paths, _, _ := setupAcceptedMemoryWithExistingFile(t, root)
	mustWriteFile(t, filepath.Join(root, "internal/app/memory.go"), "package app\n\nconst changed = true\n")

	runMemoryCommand(t, app, "memory", "refresh")

	projectMemory := mustReadFile(t, paths.ProjectMemory)
	if !strings.Contains(projectMemory, "Freshness: stale") || !strings.Contains(projectMemory, "Files: internal/app/memory.go") {
		t.Fatalf("project memory missing freshness after refresh:\n%s", projectMemory)
	}
}

func TestMemoryAcceptPreservesLatestRefreshInProjectMemory(t *testing.T) {
	root := t.TempDir()
	app, paths, _, _ := setupAcceptedMemoryWithExistingFile(t, root)
	mustWriteFile(t, filepath.Join(root, "internal/app/memory.go"), "package app\n\nconst changed = true\n")
	runMemoryCommand(t, app, "memory", "refresh")

	runID := runContractOnlyForTask(t, app, "second memory item")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	pin, err := reviewStateFreshnessPin(runPaths)
	assertNoError(t, err)
	candidate := memoryCandidateDocument{
		Schema:       memoryCandidateSchema,
		RunID:        runID,
		CreatedAt:    "2026-06-02T10:00:00Z",
		Source:       "deterministic",
		Status:       "proposed",
		FreshnessPin: pin,
		Contract:     memoryCandidateContract{Goal: "second memory item"},
		Outcome: memoryCandidateOutcome{
			GateStatus:   "passed",
			ReviewStatus: "approved",
		},
		Changes: memoryCandidateChanges{},
		Review:  memoryCandidateReview{},
		Artifacts: memoryCandidateArtifacts{
			Contract: "contract/contract.json",
		},
	}
	assertNoError(t, writeJSON(runPaths.MemoryCandidateJSON, candidate))
	runMemoryCommand(t, app, "memory", "accept", runID)

	projectMemory := mustReadFile(t, paths.ProjectMemory)
	mem001 := projectMemorySection(t, projectMemory, "mem_001")
	if !strings.Contains(mem001, "- Freshness: stale") {
		t.Fatalf("mem_001 should preserve latest stale refresh after accepting mem_002:\n%s", projectMemory)
	}
	mem002 := projectMemorySection(t, projectMemory, "mem_002")
	if !strings.Contains(mem002, "- Freshness: unknown") {
		t.Fatalf("new memory item should use embedded accept freshness fallback:\n%s", projectMemory)
	}
}

func TestMemoryFreshnessBackwardCompatibilityUnknown(t *testing.T) {
	root := t.TempDir()
	app, paths := setupInitializedMemoryWorkspace(t, root)
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:      "mem_001",
		Title:   "Legacy memory",
		Summary: "Old item without freshness",
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"memory", "search", "legacy memory", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("memory search --json exited %d, stderr: %s", code, stderr.String())
	}
	var response memorySearchResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &response))
	if len(response.Selected) != 1 || response.Selected[0].Freshness.Status != memoryFreshnessUnknown {
		t.Fatalf("legacy item should search with unknown freshness: %#v", response)
	}
}

func TestMemoryFreshnessArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, paths, _, _ := setupAcceptedMemoryWithExistingFile(t, root)
	mustWriteFile(t, filepath.Join(root, "internal/app/memory.go"), "package app\n\nconst changed = true\n")
	runMemoryCommand(t, app, "memory", "refresh")
	runID := runContractOnlyForTask(t, app, "memory freshness")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	for name, content := range map[string]string{
		"memory/refreshes.jsonl":        mustReadFile(t, paths.MemoryRefreshes),
		"memory/project-memory.md":      mustReadFile(t, paths.ProjectMemory),
		"context/memory-context.md":     mustReadFile(t, runPaths.MemoryContextMD),
		"context/memory-selection.json": mustReadFile(t, runPaths.MemorySelectionJSON),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func setupAcceptedMemoryWithExistingFile(t *testing.T, root string) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID, runPaths := setupApprovedReviewedMemoryRun(t, root)
	mustWriteFile(t, filepath.Join(root, "internal/app/memory.go"), "package app\n")
	runMemoryCommand(t, app, "memory", "propose", runID)
	runMemoryCommand(t, app, "memory", "accept", runID)
	return app, paths, runID, runPaths
}

func readMemoryRefreshesForTest(t *testing.T, path string) []memoryRefreshRecord {
	t.Helper()
	refreshes, err := readMemoryRefreshes(path)
	assertNoError(t, err)
	return refreshes
}

func latestMemoryRefreshForTest(t *testing.T, path string) memoryRefreshRecord {
	t.Helper()
	refreshes := readMemoryRefreshesForTest(t, path)
	if len(refreshes) == 0 {
		t.Fatalf("expected at least one refresh")
	}
	return refreshes[len(refreshes)-1]
}

func appendMemoryRefreshForTest(t *testing.T, paths artifacts.Paths, refresh memoryRefreshRecord) {
	t.Helper()
	if refresh.Schema == "" {
		refresh.Schema = memoryRefreshSchema
	}
	if refresh.ID == "" {
		refresh.ID = "memory_refresh_001"
	}
	if refresh.CreatedAt == "" {
		refresh.CreatedAt = "2026-06-02T10:00:00Z"
	}
	assertNoError(t, appendJSONLine(paths.MemoryRefreshes, refresh))
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func projectMemorySection(t *testing.T, content string, itemID string) string {
	t.Helper()
	start := strings.Index(content, "### "+itemID+" ")
	if start < 0 {
		t.Fatalf("project memory missing %s:\n%s", itemID, content)
	}
	rest := content[start:]
	next := strings.Index(rest[len("### "):], "\n### ")
	if next < 0 {
		return rest
	}
	return rest[:len("### ")+next]
}
