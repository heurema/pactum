package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/artifacts"
)

func freshBoundaryMemoryItem() memoryItemRecord {
	return memoryItemRecord{
		ID:      "mem_001",
		Title:   "Prompt boundary memory",
		Summary: "Executor prompt boundary records selected memory.",
		Files:   []string{"internal/app/prompt.go"},
		Tags:    []string{"contract", "reviewed"},
		Freshness: &memoryItemFreshness{
			Status:  memoryFreshnessFresh,
			Reasons: []string{},
			Files:   []memoryFreshnessFile{},
		},
	}
}

func staleBoundaryMemoryItem() memoryItemRecord {
	return memoryItemRecord{
		ID:      "mem_002",
		Title:   "Old executor prompt boundary",
		Summary: "Old prompt build boundary behavior.",
		Files:   []string{"internal/app/execute.go"},
		Tags:    []string{"contract"},
		Freshness: &memoryItemFreshness{
			Status:  memoryFreshnessStale,
			Reasons: []string{"changed file internal/app/execute.go"},
			Files:   []memoryFreshnessFile{},
		},
	}
}

func unknownBoundaryMemoryItem() memoryItemRecord {
	return memoryItemRecord{
		ID:      "mem_003",
		Title:   "Prompt boundary unknown freshness",
		Summary: "Prompt build boundary unknown.",
		Files:   []string{"internal/app/missing.go"},
		Tags:    []string{"contract"},
		Freshness: &memoryItemFreshness{
			Status:  memoryFreshnessUnknown,
			Reasons: []string{"unknown file internal/app/missing.go"},
			Files:   []memoryFreshnessFile{},
		},
	}
}

func setupApprovedPromptWithMemoryItems(t *testing.T, root string, items ...memoryItemRecord) (App, artifacts.Paths, string, contractRunPathSet) {
	t.Helper()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	for _, item := range items {
		appendAcceptedMemoryItemForTest(t, paths, item)
	}
	runMemorySelectionCommand(t, app, "prompt", "build", runID)
	return app, paths, runID, runPaths
}

func TestPromptBuildManifestIncludesMemoryMetadata(t *testing.T) {
	root := t.TempDir()
	_, _, runID, runPaths := setupApprovedPromptWithMemoryItems(t, root,
		freshBoundaryMemoryItem(), staleBoundaryMemoryItem(), unknownBoundaryMemoryItem())

	manifest := readPromptManifestForTest(t, runPaths.PromptManifest)
	if manifest.Memory == nil {
		t.Fatalf("manifest memory metadata missing for %s", runID)
	}
	memory := manifest.Memory
	if memory.Context != "context/memory-context.md" || memory.Selection != "context/memory-selection.json" {
		t.Fatalf("unexpected memory artifact paths: %#v", memory)
	}
	if memory.ContextSHA256 == "" || memory.SelectionSHA256 == "" || memory.SourceSHA256 == "" {
		t.Fatalf("memory hashes should be non-empty: %#v", memory)
	}
	want := promptManifestMemorySelected{Total: 3, Fresh: 1, Stale: 1, Unknown: 1}
	if memory.Selected != want {
		t.Fatalf("memory selected counts = %#v, want %#v", memory.Selected, want)
	}
	if !manifest.Checks.MemoryContextReady || !manifest.Checks.MemorySourceCurrent {
		t.Fatalf("memory checks should be true: %#v", manifest.Checks)
	}
}

func TestPromptBuildWithoutMemoryStillWritesMemoryMetadata(t *testing.T) {
	root := t.TempDir()
	_, _, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	manifest := readPromptManifestForTest(t, runPaths.PromptManifest)
	if manifest.Memory == nil {
		t.Fatalf("manifest memory metadata missing for empty-memory run %s", runID)
	}
	if manifest.Memory.Selected.Total != 0 || manifest.Memory.Selected.Fresh != 0 || manifest.Memory.Selected.Stale != 0 || manifest.Memory.Selected.Unknown != 0 {
		t.Fatalf("empty-memory selected counts should be zero: %#v", manifest.Memory.Selected)
	}
	if manifest.Memory.SourceSHA256 == "" {
		t.Fatalf("empty-memory source hash should be deterministic and non-empty")
	}
	if manifest.Memory.LatestRefresh != nil {
		t.Fatalf("empty-memory latest refresh should be nil: %#v", manifest.Memory.LatestRefresh)
	}
	if !manifest.Checks.MemoryContextReady {
		t.Fatalf("memory_context_ready should be true: %#v", manifest.Checks)
	}

	// Source hash is deterministic across rebuilds with no source changes.
	second, err := memorySourceSHA256(artifacts.New(root))
	assertNoError(t, err)
	if second != manifest.Memory.SourceSHA256 {
		t.Fatalf("source hash not deterministic: %q vs %q", second, manifest.Memory.SourceSHA256)
	}
}

func TestPromptMDIncludesCompactMemorySection(t *testing.T) {
	root := t.TempDir()
	_, _, _, runPaths := setupApprovedPromptWithMemoryItems(t, root,
		freshBoundaryMemoryItem(), staleBoundaryMemoryItem())

	prompt := mustReadFile(t, runPaths.PromptMD)
	for _, want := range []string{
		"## Accepted memory",
		"Selected memory:",
		"- total: 2",
		"- fresh: 1",
		"- stale: 1",
		"mem_001 [fresh] score=",
		"mem_002 [stale] score=",
		"reason: changed file internal/app/execute.go",
		"- Stale memory may be outdated; verify before using.",
		"- Do not implement from memory alone.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt.md missing %q:\n%s", want, prompt)
		}
	}
}

func TestPromptMDMemorySectionEmpty(t *testing.T) {
	root := t.TempDir()
	_, _, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	prompt := mustReadFile(t, runPaths.PromptMD)
	if !strings.Contains(prompt, "## Accepted memory") || !strings.Contains(prompt, "Items:\n- none") {
		t.Fatalf("prompt.md empty memory section mismatch:\n%s", prompt)
	}
}

func TestPromptMDExecutorWordingIsCurrent(t *testing.T) {
	root := t.TempDir()
	_, _, runID := setupApprovedAndBuiltPrompt(t, root)
	runPaths := contractRunPaths(filepath.Join(artifacts.New(root).RunsDir, runID))

	prompt := mustReadFile(t, runPaths.PromptMD)
	for _, forbidden := range []string{
		"does not execute agents in this milestone",
		"when execution becomes available",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt.md contains stale milestone wording %q:\n%s", forbidden, prompt)
		}
	}
	for _, want := range []string{
		"`pactum execute run`",
		"validates contract and memory boundaries",
		"Pactum gate can run approved validation commands after execution",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt.md missing current wording %q:\n%s", want, prompt)
		}
	}
}

func TestExecutorContextIncludesMemoryCounts(t *testing.T) {
	root := t.TempDir()
	_, _, _, runPaths := setupApprovedPromptWithMemoryItems(t, root,
		freshBoundaryMemoryItem(), staleBoundaryMemoryItem())

	context := mustReadFile(t, runPaths.ExecutorContext)
	for _, want := range []string{
		"## Accepted memory",
		"- Selected items: 2",
		"- Fresh: 1",
		"- Stale: 1",
		"- Unknown: 0",
		"- Stale memory must be verified before use.",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("executor-context.md missing %q:\n%s", want, context)
		}
	}
}

func TestExecutePlanSucceedsWhenMemoryBoundaryMatches(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPromptWithMemoryItems(t, root, freshBoundaryMemoryItem())

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan exited %d, stderr: %s", code, stderr.String())
	}
	assertFile(t, runPaths.DryRunJSON)
}

func TestExecutePlanFailsWhenMemoryContextChanges(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPromptWithMemoryItems(t, root, freshBoundaryMemoryItem())

	mustWriteFile(t, runPaths.MemoryContextMD, mustReadFile(t, runPaths.MemoryContextMD)+"\ntampered\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail when memory context changed")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: memory context changed after prompt build") {
		t.Fatalf("memory context change stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.DryRunJSON)
}

func TestExecutePlanFailsWhenMemorySelectionChanges(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPromptWithMemoryItems(t, root, freshBoundaryMemoryItem())

	mustWriteFile(t, runPaths.MemorySelectionJSON, mustReadFile(t, runPaths.MemorySelectionJSON)+"\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail when memory selection changed")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: memory context changed after prompt build") {
		t.Fatalf("memory selection change stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanFailsWhenMemoryItemsChange(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPromptWithMemoryItems(t, root, freshBoundaryMemoryItem())

	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:      "mem_999",
		Title:   "Newly accepted prompt boundary",
		Summary: "Appended after prompt build.",
		Files:   []string{"internal/app/run.go"},
	})

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail when accepted memory items changed")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: accepted memory changed after prompt build") {
		t.Fatalf("memory items change stderr mismatch:\n%s", got)
	}
	assertNoFile(t, runPaths.DryRunJSON)
}

func TestExecutePlanFailsWhenMemoryRefreshesChange(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPromptWithMemoryItems(t, root, freshBoundaryMemoryItem())

	runMemorySelectionCommand(t, app, "memory", "refresh")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail when memory refreshes changed")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: accepted memory changed after prompt build") {
		t.Fatalf("memory refreshes change stderr mismatch:\n%s", got)
	}
}

func TestExecutePlanAllowsStaleSelectedMemory(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPromptWithMemoryItems(t, root, staleBoundaryMemoryItem())

	manifest := readPromptManifestForTest(t, runPaths.PromptManifest)
	if manifest.Memory == nil || manifest.Memory.Selected.Stale == 0 {
		t.Fatalf("expected a stale selected memory item at prompt build: %#v", manifest.Memory)
	}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute plan should succeed with stale-but-unchanged memory, stderr: %s", stderr.String())
	}
	assertFile(t, runPaths.DryRunJSON)

	prompt := mustReadFile(t, runPaths.PromptMD)
	if !strings.Contains(prompt, "[stale]") || !strings.Contains(prompt, "Stale memory may be outdated; verify before using.") {
		t.Fatalf("prompt.md should warn about stale memory:\n%s", prompt)
	}
}

func TestPromptShowDisplaysMemorySummary(t *testing.T) {
	root := t.TempDir()
	app, _, runID, _ := setupApprovedPromptWithMemoryItems(t, root,
		freshBoundaryMemoryItem(), staleBoundaryMemoryItem(), unknownBoundaryMemoryItem())

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"prompt", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Memory:",
		"selected: 3",
		"fresh: 1",
		"stale: 1",
		"unknown: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt show missing %q:\n%s", want, got)
		}
	}
}

func TestReviewerContextIncludesMemorySummaryWhenPromptBuilt(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendAcceptedMemoryItemForTest(t, paths, freshBoundaryMemoryItem())
	appendAcceptedMemoryItemForTest(t, paths, staleBoundaryMemoryItem())
	runMemorySelectionCommand(t, app, "prompt", "build", runID)

	writeReviewGateReportForTest(t, runPaths, runID, "passed")
	runReviewCommand(t, app, "review", "plan", runID)

	context := mustReadFile(t, runPaths.ReviewContextMD)
	for _, want := range []string{
		"## Accepted memory",
		"- Memory context: context/memory-context.md",
		"- Selected items: 2",
		"- Fresh: 1",
		"- Stale: 1",
		"- Stale memory may be outdated and must be verified.",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("reviewer-context.md missing %q:\n%s", want, context)
		}
	}
}

func TestMemoryBoundaryArtifactsArePortable(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupApprovedPromptContract(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	appendAcceptedMemoryItemForTest(t, paths, memoryItemRecord{
		ID:      "mem_001",
		Title:   "Prompt boundary memory " + root,
		Summary: "Boundary mentions " + filepath.ToSlash(root),
		Files:   []string{filepath.Join(root, "internal", "app", "prompt.go")},
		Tags:    []string{"contract"},
		Freshness: &memoryItemFreshness{
			Status:  memoryFreshnessStale,
			Reasons: []string{"changed file " + filepath.Join(root, "internal", "app", "execute.go")},
			Files:   []memoryFreshnessFile{},
		},
	})
	runMemorySelectionCommand(t, app, "prompt", "build", runID)
	writeReviewGateReportForTest(t, runPaths, runID, "passed")
	runReviewCommand(t, app, "review", "plan", runID)

	for name, content := range map[string]string{
		"contract/prompt-manifest.json": mustReadFile(t, runPaths.PromptManifest),
		"contract/prompt.md":            mustReadFile(t, runPaths.PromptMD),
		"context/executor-context.md":   mustReadFile(t, runPaths.ExecutorContext),
		"review/reviewer-context.md":    mustReadFile(t, runPaths.ReviewContextMD),
	} {
		assertDoesNotContainRoot(t, name, content, root)
	}
}

func TestExecutePlanFailsWhenMemoryMetadataMissing(t *testing.T) {
	root := t.TempDir()
	app, _, runID, runPaths := setupApprovedPromptWithMemoryItems(t, root, freshBoundaryMemoryItem())

	var generic map[string]any
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, runPaths.PromptManifest)), &generic))
	delete(generic, "memory")
	data, err := json.MarshalIndent(generic, "", "  ")
	assertNoError(t, err)
	mustWriteFile(t, runPaths.PromptManifest, string(data)+"\n")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "plan", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute plan should fail when memory metadata is missing")
	}
	if got := stderr.String(); !strings.Contains(got, "cannot prepare execution: executor prompt memory metadata is missing") {
		t.Fatalf("missing memory metadata stderr mismatch:\n%s", got)
	}
}
