package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// ---------------------------------------------------------------------------
// DAG helper process — invoked as a subprocess for executor tests
// ---------------------------------------------------------------------------

// TestDAGHelperProcess is the subprocess entry-point for DAG executor tests.
// It is NOT a real test; it exits immediately when PACTUM_HELPER_PROCESS != "1".
//
// Supported env vars (all relative to CWD = repo root):
//
//	PACTUM_HELPER_EMIT_BLOCKER  — emit a PACTUM_BLOCKER sentinel line
//	PACTUM_HELPER_DELETE_FILES  — JSON array of relative paths to delete (runs before WRITE)
//	PACTUM_HELPER_WRITE_FILES   — JSON array of {"path":"...","data":"..."} to write
//	PACTUM_HELPER_CHMOD_FILES   — JSON array of {"path":"...","mode":<octal int>} to chmod
//	PACTUM_HELPER_EXIT          — exit code (default 0)
func TestDAGHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_HELPER_PROCESS") != "1" {
		return
	}
	if blocker := os.Getenv("PACTUM_HELPER_EMIT_BLOCKER"); blocker != "" {
		fmt.Printf("PACTUM_BLOCKER: %s\n", blocker)
	}
	// DELETE runs before WRITE so a path can be removed then recreated as a different type.
	if raw := os.Getenv("PACTUM_HELPER_DELETE_FILES"); raw != "" {
		var paths []string
		if err := json.Unmarshal([]byte(raw), &paths); err == nil {
			for _, p := range paths {
				_ = os.Remove(p)
			}
		}
	}
	if raw := os.Getenv("PACTUM_HELPER_WRITE_FILES"); raw != "" {
		var writes []struct {
			Path string `json:"path"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal([]byte(raw), &writes); err == nil {
			for _, w := range writes {
				if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err == nil {
					_ = os.WriteFile(w.Path, []byte(w.Data), 0o644)
				}
			}
		}
	}
	if raw := os.Getenv("PACTUM_HELPER_CHMOD_FILES"); raw != "" {
		var chmods []struct {
			Path string `json:"path"`
			Mode int    `json:"mode"`
		}
		if err := json.Unmarshal([]byte(raw), &chmods); err == nil {
			for _, c := range chmods {
				_ = os.Chmod(c.Path, os.FileMode(c.Mode))
			}
		}
	}
	if raw := os.Getenv("PACTUM_HELPER_EXIT"); raw != "" {
		code, _ := strconv.Atoi(raw)
		os.Exit(code)
	}
	os.Exit(0)
}

// ---------------------------------------------------------------------------
// Test setup helpers
// ---------------------------------------------------------------------------

// dagHelperDescriptorForEngine builds an agent descriptor for the given engine
// that invokes TestDAGHelperProcess.
func dagHelperDescriptorForEngine(engine string) agents.AgentDescriptor {
	return agents.AgentDescriptor{
		Name:    engine,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestDAGHelperProcess", "--"},
		Input:   agents.InputPromptFile,
	}
}

// setupDAGContract creates a full run with an approved+built-prompt DAG contract.
// Plan tasks are added BEFORE approval so that contract revise does not need
// --allow-approval-reset. Returns (app, paths, runID).
//
// The function creates a "src" subdirectory in root so scope checks find real files.
func setupDAGContract(t *testing.T, root string, tasks []planTask) (App, artifacts.Paths, string) {
	t.Helper()

	// Create a real file so scope checks have something to look at.
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	// Build contract run with both engines routing to the DAG helper.
	app, paths, runID := setupContractRun(t, root)
	// Override both claude and codex engines so whichever default is picked,
	// the DAG helper subprocess runs.
	app.AgentRegistry = testAgentRegistry(
		dagHelperDescriptorForEngine(agents.BuiltinClaude),
		dagHelperDescriptorForEngine(agents.BuiltinCodex),
	)

	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	var stdout, stderr bytes.Buffer

	// Step 1: revise with goal + scope + validation + plan (before approval)
	revDoc := writeReviseDocForTest(t, runPaths, map[string]any{
		"goal": "dag integration test goal",
		"scope": map[string]any{
			"in":  []string{"src"},
			"out": []string{},
		},
		"acceptance_criteria": []string{"all tasks done"},
		"validation":          map[string]any{"commands": []string{"true"}},
		"plan": map[string]any{
			"tasks": tasks,
		},
	})
	code := app.Run([]string{"contract", "revise", runID, "--from", revDoc}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise (plan) exited %d, stdout: %s, stderr: %s", code, stdout.String(), stderr.String())
	}

	// Step 2: approve
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}

	// Step 3: map refresh + prompt build
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"map", "refresh"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("map refresh exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"prompt", "build", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("prompt build exited %d, stderr: %s", code, stderr.String())
	}

	return app, paths, runID
}

// makeTask creates a planTask with a "src" scope that always has validation "true".
func makeTask(id string, deps []string) planTask {
	return planTask{
		ID:           id,
		Title:        id + " title",
		DependsOn:    deps,
		Acceptance:   []string{"acceptance for " + id},
		Validation:   []string{"true"},
		PathsInScope: []string{"src"},
	}
}

// readTasksStateForTest reads tasks-state.json, failing if it does not exist.
func readTasksStateForTest(t *testing.T, path string) tasksStateDocument {
	t.Helper()
	var state tasksStateDocument
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tasks-state.json: %v", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse tasks-state.json: %v", err)
	}
	return state
}

// taskEntryStatus returns the status of a task in the state document.
func taskEntryStatus(state tasksStateDocument, taskID string) string {
	for _, e := range state.Tasks {
		if e.TaskID == taskID {
			return e.Status
		}
	}
	return ""
}

// countAttempts counts executor invocations for a given task by counting
// attempt_* dirs inside execute/tasks/<taskID>/attempts/.
func countAttempts(t *testing.T, tasksDir string, taskID string) int {
	t.Helper()
	attemptsDir := filepath.Join(tasksDir, taskID, "attempts")
	entries, err := os.ReadDir(attemptsDir)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("readdir %s: %v", attemptsDir, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "attempt_") {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestPlanDAGLinearTwoTasksCompletes verifies that a two-task linear chain
// (t1 → t2) completes when the helper exits 0 and validation is "true".
func TestPlanDAGLinearTwoTasksCompletes(t *testing.T) {
	root := t.TempDir()
	tasks := []planTask{makeTask("t1", nil), makeTask("t2", []string{"t1"})}
	app, paths, runID := setupDAGContract(t, root, tasks)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	if state.Run.TerminalState != "completed" {
		t.Fatalf("terminal_state: want completed, got %s", state.Run.TerminalState)
	}
	if taskEntryStatus(state, "t1") != "done" {
		t.Fatalf("t1 status: want done, got %s", taskEntryStatus(state, "t1"))
	}
	if taskEntryStatus(state, "t2") != "done" {
		t.Fatalf("t2 status: want done, got %s", taskEntryStatus(state, "t2"))
	}

	// last-result.json
	var lastResult dagLastResult
	data, err := os.ReadFile(runPaths.LastResultJSON)
	if err != nil {
		t.Fatalf("read last-result.json: %v", err)
	}
	if err := json.Unmarshal(data, &lastResult); err != nil {
		t.Fatalf("parse last-result.json: %v", err)
	}
	if !lastResult.Passed {
		t.Fatalf("last-result.json passed: want true, got false")
	}

	got := stdout.String()
	if !strings.Contains(got, "Terminal state: completed") {
		t.Fatalf("stdout missing 'Terminal state: completed':\n%s", got)
	}
}

// TestPlanDAGFanIn verifies that a fan-in DAG (t1+t2 independent, t3 depends
// on both) completes when all validation commands pass.
func TestPlanDAGFanIn(t *testing.T) {
	root := t.TempDir()
	tasks := []planTask{
		makeTask("t1", nil),
		makeTask("t2", nil),
		makeTask("t3", []string{"t1", "t2"}),
	}
	app, paths, runID := setupDAGContract(t, root, tasks)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	if state.Run.TerminalState != "completed" {
		t.Fatalf("terminal_state: want completed, got %s", state.Run.TerminalState)
	}
	for _, id := range []string{"t1", "t2", "t3"} {
		if taskEntryStatus(state, id) != "done" {
			t.Fatalf("%s status: want done, got %s", id, taskEntryStatus(state, id))
		}
	}
}

// TestPlanDAGBlockedNodeBlocksUpstream verifies that when t1 is blocked (its
// validation always fails), t2 (which depends on t1) becomes blocked-upstream,
// while t3 (independent) still runs and succeeds.
func TestPlanDAGBlockedNodeBlocksUpstream(t *testing.T) {
	root := t.TempDir()
	// t1 has a validation command that always fails
	t1 := planTask{
		ID:           "t1",
		Acceptance:   []string{"ok"},
		Validation:   []string{"false"},
		PathsInScope: []string{"src"},
	}
	t2 := makeTask("t2", []string{"t1"})
	t3 := makeTask("t3", nil)
	tasks := []planTask{t1, t2, t3}
	app, paths, runID := setupDAGContract(t, root, tasks)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail when tasks are blocked")
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	if state.Run.TerminalState != "blocked" {
		t.Fatalf("terminal_state: want blocked, got %s", state.Run.TerminalState)
	}
	if taskEntryStatus(state, "t1") != "blocked" {
		t.Fatalf("t1 status: want blocked, got %s", taskEntryStatus(state, "t1"))
	}
	if taskEntryStatus(state, "t2") != "blocked-upstream" {
		t.Fatalf("t2 status: want blocked-upstream, got %s", taskEntryStatus(state, "t2"))
	}
	if taskEntryStatus(state, "t3") != "done" {
		t.Fatalf("t3 status: want done, got %s", taskEntryStatus(state, "t3"))
	}
}

// TestPlanDAGPlanlessContractRunsSingleShot verifies that a contract with no
// plan tasks takes the existing single-shot execute path (no tasks-state.json
// or loop-summary.json written, but at least one execute/attempts/ dir exists).
func TestPlanDAGPlanlessContractRunsSingleShot(t *testing.T) {
	root := t.TempDir()
	// setupApprovedBuiltPromptWithHelperAgent uses TestExecutionHelperProcess as helper
	app, paths, runID := setupApprovedBuiltPromptWithHelperAgent(t, root, "helper")
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// TestExecutionHelperProcess env vars
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXPECTED_CWD", root)
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID, "--agent", "helper"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}

	// tasks-state.json and loop-summary.json must NOT exist (single-shot path)
	if _, err := os.Stat(runPaths.TasksStateJSON); err == nil {
		t.Fatalf("tasks-state.json should not exist for planless contract")
	}
	if _, err := os.Stat(runPaths.LoopSummaryJSON); err == nil {
		t.Fatalf("loop-summary.json should not exist for planless contract")
	}

	// At least one attempt dir must exist (single-shot created it)
	if _, err := os.Stat(runPaths.AttemptsDir); err != nil {
		t.Fatalf("execute/attempts dir should exist for single-shot run: %v", err)
	}
}

// TestPlanDAGRequiresHumanStopsRun verifies that when the executor emits a
// PACTUM_BLOCKER requires_human sentinel, the DAG stops and reports terminal_state=human.
func TestPlanDAGRequiresHumanStopsRun(t *testing.T) {
	root := t.TempDir()
	tasks := []planTask{makeTask("t1", nil)}
	app, paths, runID := setupDAGContract(t, root, tasks)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EMIT_BLOCKER", `{"reason":"requires_human","why":"conflicting requirements"}`)
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail for requires_human")
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	if state.Run.TerminalState != "human" {
		t.Fatalf("terminal_state: want human, got %s", state.Run.TerminalState)
	}
}

// TestPlanDAGInvalidDagErrors verifies that a contract with an unknown dependency
// (t1 depends on "nonexistent") results in terminal_state=error immediately.
//
// NOTE: The pactum contract revise command validates the plan and rejects unknown
// deps, so we bypass revise by writing the contract directly.
func TestPlanDAGInvalidDagErrors(t *testing.T) {
	// validateDAGDependencies is the in-process function; test it directly since
	// the CLI's contract revise already blocks such plans.
	tasks := []planTask{
		{
			ID:           "t1",
			DependsOn:    []string{"nonexistent"},
			Acceptance:   []string{"ok"},
			Validation:   []string{"true"},
			PathsInScope: []string{"src"},
		},
	}
	err := validateDAGDependencies(tasks)
	if err == nil {
		t.Fatal("validateDAGDependencies should return error for unknown dep")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("error should mention the missing dep, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid_dag") {
		t.Fatalf("error should contain 'invalid_dag', got: %v", err)
	}
}

// TestPlanDAGDefaultLoopMax verifies that when no loop.max is configured, the
// executor is invoked exactly defaultLoopMax (3) times for a task that never passes.
func TestPlanDAGDefaultLoopMax(t *testing.T) {
	root := t.TempDir()
	// Task validation always fails — loop will exhaust attempts
	t1 := planTask{
		ID:           "t1",
		Acceptance:   []string{"ok"},
		Validation:   []string{"false"},
		PathsInScope: []string{"src"},
	}
	app, paths, runID := setupDAGContract(t, root, []planTask{t1})
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail when validation never passes")
	}

	attempts := countAttempts(t, runPaths.TasksDir, "t1")
	if attempts != defaultLoopMax {
		t.Fatalf("expected %d executor attempts, got %d", defaultLoopMax, attempts)
	}
}

// TestPlanDAGHumanOutput verifies writePlanDAGOutput formats output correctly
// for both completed and blocked terminal states.
func TestPlanDAGHumanOutput(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		var out bytes.Buffer
		writePlanDAGOutput(&out, "completed", []taskStateEntry{
			{TaskID: "t1", Status: "done"},
		}, "run_001")
		got := out.String()
		if !strings.Contains(got, "Terminal state: completed") {
			t.Fatalf("missing 'Terminal state: completed': %s", got)
		}
	})

	t.Run("blocked", func(t *testing.T) {
		var out bytes.Buffer
		writePlanDAGOutput(&out, "blocked", []taskStateEntry{
			{
				TaskID: "t1",
				Status: "blocked",
				Blocker: &taskBlocker{
					Reason:       "validation_unmet",
					Why:          "tests never passed",
					NextCommands: []string{"pactum execute run run_001"},
				},
			},
		}, "run_001")
		got := out.String()
		if !strings.Contains(got, "Terminal state: blocked") {
			t.Fatalf("missing 'Terminal state: blocked': %s", got)
		}
		if !strings.Contains(got, "--- BLOCKED TASK: t1 ---") {
			t.Fatalf("missing blocked task header: %s", got)
		}
		if !strings.Contains(got, "Next steps:") {
			t.Fatalf("missing 'Next steps:': %s", got)
		}
	})
}

// TestPlanDAGContractSHAMismatch verifies that when tasks-state.json was written
// for a different contract SHA, execute run returns a non-zero exit with a mismatch message.
func TestPlanDAGContractSHAMismatch(t *testing.T) {
	root := t.TempDir()
	tasks := []planTask{makeTask("t1", nil)}
	app, paths, runID := setupDAGContract(t, root, tasks)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Pre-populate tasks-state.json with a wrong SHA
	wrongState := tasksStateDocument{
		ContractSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		Run:            tasksRunLevel{},
		Tasks:          initTaskEntries(tasks),
	}
	if err := writeTasksState(runPaths.TasksStateJSON, wrongState); err != nil {
		t.Fatalf("write wrong tasks-state: %v", err)
	}

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail on SHA mismatch")
	}
	if got := stderr.String(); !strings.Contains(got, "sha256 mismatch") {
		t.Fatalf("stderr should mention sha256 mismatch, got: %s", got)
	}
}

// TestPlanDAGExecutionDrainedEvent verifies that after a completed run, the
// events.jsonl file contains an execution_drained event.
func TestPlanDAGExecutionDrainedEvent(t *testing.T) {
	root := t.TempDir()
	tasks := []planTask{makeTask("t1", nil)}
	app, paths, runID := setupDAGContract(t, root, tasks)

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}

	// Read events.jsonl and find execution_drained
	data, err := os.ReadFile(paths.EventsJSONL)
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "execution_drained") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events.jsonl does not contain execution_drained event:\n%s", string(data))
	}
}

// TestPlanDAGResumeSkipsDoneTask verifies that when tasks-state.json already
// has t1=done and t2=ready, only t2 runs (t1 is not executed again).
func TestPlanDAGResumeSkipsDoneTask(t *testing.T) {
	root := t.TempDir()
	tasks := []planTask{makeTask("t1", nil), makeTask("t2", []string{"t1"})}
	app, paths, runID := setupDAGContract(t, root, tasks)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Read the real contract SHA
	var contract draftContract
	data, err := os.ReadFile(runPaths.ContractJSON)
	if err != nil {
		t.Fatalf("read contract.json: %v", err)
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("parse contract.json: %v", err)
	}
	sha, err := contractVersionHash(contract)
	if err != nil {
		t.Fatalf("hash contract: %v", err)
	}

	// Pre-populate tasks-state.json with t1=done, t2=ready
	preState := tasksStateDocument{
		ContractSHA256: sha,
		Run:            tasksRunLevel{},
		Tasks: []taskStateEntry{
			{TaskID: "t1", Status: "done"},
			{TaskID: "t2", Status: "ready"},
		},
	}
	if err := writeTasksState(runPaths.TasksStateJSON, preState); err != nil {
		t.Fatalf("write pre-state: %v", err)
	}

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("execute run exited %d, stderr: %s\nstdout: %s", code, stderr.String(), stdout.String())
	}

	// t1 should have 0 executor attempts (it was pre-seeded as done)
	attemptsT1 := countAttempts(t, runPaths.TasksDir, "t1")
	if attemptsT1 != 0 {
		t.Fatalf("t1 should not have run (already done), but got %d attempts", attemptsT1)
	}
	// t2 should have exactly 1 executor attempt
	attemptsT2 := countAttempts(t, runPaths.TasksDir, "t2")
	if attemptsT2 != 1 {
		t.Fatalf("t2 should have run exactly once, got %d attempts", attemptsT2)
	}
}

// ---------------------------------------------------------------------------
// Unit-level tests (no subprocess)
// ---------------------------------------------------------------------------

func TestPlanDAGValidateDAGDependencies(t *testing.T) {
	t.Run("valid_linear", func(t *testing.T) {
		tasks := []planTask{
			{ID: "a", PathsInScope: []string{"src"}},
			{ID: "b", DependsOn: []string{"a"}, PathsInScope: []string{"src"}},
		}
		if err := validateDAGDependencies(tasks); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("missing_dep", func(t *testing.T) {
		tasks := []planTask{
			{ID: "a", DependsOn: []string{"missing"}, PathsInScope: []string{"src"}},
		}
		err := validateDAGDependencies(tasks)
		if err == nil {
			t.Fatal("expected error for missing dep")
		}
	})
}

func TestPlanDAGInitTaskEntries(t *testing.T) {
	tasks := []planTask{
		{ID: "t1"},
		{ID: "t2", DependsOn: []string{"t1"}},
	}
	entries := initTaskEntries(tasks)
	if entries[0].Status != "ready" {
		t.Fatalf("t1 should be ready (no deps), got %s", entries[0].Status)
	}
	if entries[1].Status != "pending" {
		t.Fatalf("t2 should be pending (has deps), got %s", entries[1].Status)
	}
}

func TestPlanDAGRefreshReadiness(t *testing.T) {
	tasks := []planTask{
		{ID: "t1"},
		{ID: "t2", DependsOn: []string{"t1"}},
		{ID: "t3", DependsOn: []string{"t1", "t2"}},
	}
	entries := []taskStateEntry{
		{TaskID: "t1", Status: "done"},
		{TaskID: "t2", Status: "pending"},
		{TaskID: "t3", Status: "pending"},
	}
	refreshReadiness(tasks, entries)
	if entries[1].Status != "ready" {
		t.Fatalf("t2 should become ready after t1 done, got %s", entries[1].Status)
	}
	if entries[2].Status != "pending" {
		t.Fatalf("t3 should remain pending (t2 not done), got %s", entries[2].Status)
	}
}

func TestPlanDAGMarkBlockedUpstream(t *testing.T) {
	tasks := []planTask{
		{ID: "t1"},
		{ID: "t2", DependsOn: []string{"t1"}},
		{ID: "t3", DependsOn: []string{"t2"}},
		{ID: "t4"},
	}
	entries := []taskStateEntry{
		{TaskID: "t1", Status: "blocked"},
		{TaskID: "t2", Status: "pending"},
		{TaskID: "t3", Status: "ready"},
		{TaskID: "t4", Status: "ready"},
	}
	markBlockedUpstream(tasks, entries, "t1")
	if entries[1].Status != "blocked-upstream" {
		t.Fatalf("t2 should be blocked-upstream, got %s", entries[1].Status)
	}
	if entries[2].Status != "blocked-upstream" {
		t.Fatalf("t3 should be blocked-upstream (transitive), got %s", entries[2].Status)
	}
	if entries[3].Status != "ready" {
		t.Fatalf("t4 should remain ready (unrelated), got %s", entries[3].Status)
	}
}

func TestPlanDAGInEffectiveScope(t *testing.T) {
	task := planTask{
		PathsInScope:    []string{"src", "lib"},
		PathsOutOfScope: []string{"src/vendor"},
	}
	cases := []struct {
		path    string
		inScope bool
	}{
		{"src/main.go", true},
		{"src/vendor/dep.go", false},
		{"lib/util.go", true},
		{"cmd/main.go", false},
		{"src/vendor", false},
	}
	for _, c := range cases {
		got := inEffectiveScope(task, c.path)
		if got != c.inScope {
			t.Errorf("inEffectiveScope(%q) = %v, want %v", c.path, got, c.inScope)
		}
	}
}

func TestPlanDAGScanForBlockerSentinel(t *testing.T) {
	output := "some output\nPACTUM_BLOCKER: {\"reason\":\"requires_human\",\"why\":\"conflict\"}\nmore output"
	sentinel := scanForBlockerSentinel(output)
	if sentinel == nil {
		t.Fatal("expected sentinel to be parsed")
	}
	if sentinel.Reason != "requires_human" {
		t.Fatalf("sentinel.Reason: want requires_human, got %s", sentinel.Reason)
	}
	if sentinel.Why != "conflict" {
		t.Fatalf("sentinel.Why: want conflict, got %s", sentinel.Why)
	}
}

func TestPlanDAGScanForBlockerSentinelNoMatch(t *testing.T) {
	output := "no sentinel here"
	if sentinel := scanForBlockerSentinel(output); sentinel != nil {
		t.Fatalf("expected nil sentinel, got %+v", sentinel)
	}
}

func TestPlanDAGBuildLoopSummary(t *testing.T) {
	entries := []taskStateEntry{
		{TaskID: "t1", Status: "done", Attempts: 2, BaselineResult: &baselineCheckResult{Recommendation: "proceed"}},
		{TaskID: "t2", Status: "done", Attempts: 1, BaselineResult: &baselineCheckResult{Recommendation: "proceed"}},
		{TaskID: "t3", Status: "blocked"},
		{TaskID: "t4", Status: "blocked-upstream"},
	}
	summary := buildLoopSummary("sha256abc", "blocked", entries)
	if summary.TasksDone != 2 {
		t.Fatalf("TasksDone: want 2, got %d", summary.TasksDone)
	}
	if summary.TasksBlocked != 1 {
		t.Fatalf("TasksBlocked: want 1, got %d", summary.TasksBlocked)
	}
	if summary.TasksBlockedUpstream != 1 {
		t.Fatalf("TasksBlockedUpstream: want 1, got %d", summary.TasksBlockedUpstream)
	}
	if summary.TotalRetries != 1 { // t1 had 2 attempts = 1 retry
		t.Fatalf("TotalRetries: want 1, got %d", summary.TotalRetries)
	}
}

func TestPlanDAGNextCommandsForBlockedTask(t *testing.T) {
	task := planTask{ID: "t1"}
	runID := "run_001"

	cases := []struct {
		reason string
		want   string
	}{
		{"requires_human", "pactum task show"},
		{"validation_unmet", "pactum execute run"},
		{"baseline_green", "pactum plan context"},
		{"invalid_scope", "pactum contract revise"},
	}
	for _, c := range cases {
		blocker := &taskBlocker{Reason: c.reason}
		cmds := nextCommandsForBlockedTask(runID, task, blocker)
		if len(cmds) == 0 {
			t.Errorf("reason %s: expected non-empty commands", c.reason)
			continue
		}
		if !strings.Contains(cmds[0], c.want) {
			t.Errorf("reason %s: command %q does not contain %q", c.reason, cmds[0], c.want)
		}
	}

	// out_of_scope with proposed paths
	outOfScope := &taskBlocker{
		Reason: "out_of_scope",
		Proposed: &taskBlockerProposed{
			PathsInScopeAdd: []string{"cmd"},
		},
	}
	cmds := nextCommandsForBlockedTask(runID, task, outOfScope)
	if len(cmds) == 0 || !strings.Contains(cmds[0], "t1") {
		t.Errorf("out_of_scope with proposed: expected task ID in command, got %v", cmds)
	}
}

func TestPlanDAGPickNextReadyTask(t *testing.T) {
	tasks := []planTask{
		{ID: "t1"},
		{ID: "t2"},
		{ID: "t3"},
	}
	entries := []taskStateEntry{
		{TaskID: "t1", Status: "done"},
		{TaskID: "t2", Status: "ready"},
		{TaskID: "t3", Status: "pending"},
	}
	next := pickNextReadyTask(tasks, entries)
	if next == nil || next.ID != "t2" {
		t.Fatalf("expected t2 to be picked, got %v", next)
	}
}

func TestPlanDAGValidateTaskScope(t *testing.T) {
	root := t.TempDir()

	t.Run("no_paths_in_scope", func(t *testing.T) {
		task := planTask{ID: "t1"}
		err := validateTaskScope(root, task)
		if err == nil || !strings.Contains(err.Error(), "non-empty") {
			t.Fatalf("expected non-empty error, got %v", err)
		}
	})

	t.Run("root_path", func(t *testing.T) {
		task := planTask{ID: "t1", PathsInScope: []string{"."}}
		err := validateTaskScope(root, task)
		if err == nil {
			t.Fatal("expected error for '.' scope entry")
		}
	})

	t.Run("absolute_path", func(t *testing.T) {
		task := planTask{ID: "t1", PathsInScope: []string{"/etc/passwd"}}
		err := validateTaskScope(root, task)
		if err == nil || !strings.Contains(err.Error(), "no leading /") {
			t.Fatalf("expected absolute path error, got %v", err)
		}
	})

	t.Run("dotdot_path", func(t *testing.T) {
		task := planTask{ID: "t1", PathsInScope: []string{"../escape"}}
		err := validateTaskScope(root, task)
		if err == nil || !strings.Contains(err.Error(), "..") {
			t.Fatalf("expected dotdot error, got %v", err)
		}
	})

	t.Run("valid_path", func(t *testing.T) {
		task := planTask{ID: "t1", PathsInScope: []string{"src"}}
		err := validateTaskScope(root, task)
		if err != nil {
			t.Fatalf("unexpected error for valid scope: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// f_004: invalid_dag records structured per-task blocker in tasks-state.json
// ---------------------------------------------------------------------------

// TestPlanDAGInvalidDagRecordsStructuredBlocker calls planDAGRun directly with
// a contract that has a task depending on a nonexistent task_id and verifies
// that tasks-state.json contains a structured invalid_dag blocker entry.
func TestPlanDAGInvalidDagRecordsStructuredBlocker(t *testing.T) {
	root := t.TempDir()
	paths := artifacts.New(root)
	runID := "run_invalid_dag_blocker"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	runPaths := contractRunPaths(runDir)

	badTask := planTask{
		ID:           "t1",
		DependsOn:    []string{"nonexistent"},
		Acceptance:   []string{"ok"},
		Validation:   []string{"true"},
		PathsInScope: []string{"src"},
	}
	prep := executionPreparation{
		Root:           root,
		Paths:          paths,
		RunPaths:       runPaths,
		State:          contractRunState{RunID: runID},
		Contract:       draftContract{Plan: &contractPlan{Tasks: []planTask{badTask}}},
		ContractSHA256: "test-sha256",
	}

	var stdout, stderr bytes.Buffer
	err := planDAGRun(App{}, &stdout, &stderr, prep, 0, false)
	if err == nil {
		t.Fatal("expected error for invalid_dag")
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	if state.Run.TerminalState != "error" {
		t.Fatalf("terminal_state: want error, got %s", state.Run.TerminalState)
	}

	var foundBlocker *taskBlocker
	for _, e := range state.Tasks {
		if e.Blocker != nil && e.Blocker.Reason == "invalid_dag" {
			foundBlocker = e.Blocker
			break
		}
	}
	if foundBlocker == nil {
		t.Fatal("no task entry with invalid_dag blocker found in tasks-state.json")
	}
	if !strings.Contains(foundBlocker.Why, "nonexistent") {
		t.Fatalf("blocker.why should mention 'nonexistent', got: %s", foundBlocker.Why)
	}
}

// ---------------------------------------------------------------------------
// f_008: workspace-boundary safety paths — out-of-scope detection + restore
// ---------------------------------------------------------------------------

// TestPlanDAGOutOfScopeBlocker verifies that when the executor writes a file
// outside paths_in_scope, a structured out_of_scope blocker is recorded with
// a non-empty next_commands array and a non-empty scope.diff artifact; and
// that the in-scope snapshot is restored while the out-of-scope file remains.
func TestPlanDAGOutOfScopeBlocker(t *testing.T) {
	root := t.TempDir()
	t1 := planTask{
		ID:           "t1",
		Acceptance:   []string{"ok"},
		Validation:   []string{"true"},
		PathsInScope: []string{"src"},
	}
	app, paths, runID := setupDAGContract(t, root, []planTask{t1})
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Helper: modify an in-scope file (for a non-empty diff) AND create an
	// out-of-scope file to trigger the out_of_scope detector.
	writeFiles, _ := json.Marshal([]map[string]string{
		{"path": "src/main.go", "data": "package modified\n"},
		{"path": "other/oos.txt", "data": "out of scope\n"},
	})
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")
	t.Setenv("PACTUM_HELPER_WRITE_FILES", string(writeFiles))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail when out-of-scope escape detected: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	if taskEntryStatus(state, "t1") != "blocked" {
		t.Fatalf("t1 status: want blocked, got %s", taskEntryStatus(state, "t1"))
	}

	var t1Entry taskStateEntry
	for _, e := range state.Tasks {
		if e.TaskID == "t1" {
			t1Entry = e
			break
		}
	}
	if t1Entry.Blocker == nil {
		t.Fatal("t1 blocker is nil")
	}
	if t1Entry.Blocker.Reason != "out_of_scope" {
		t.Fatalf("blocker.reason: want out_of_scope, got %s", t1Entry.Blocker.Reason)
	}
	if len(t1Entry.Blocker.NextCommands) == 0 {
		t.Fatal("blocker.next_commands must be non-empty for a blocked task")
	}

	// scope.diff artifact must be present, end with /scope.diff, and non-empty.
	if t1Entry.Blocker.ArtifactPath == "" {
		t.Fatal("blocker.artifact_path must be set: out_of_scope blocker following an executor attempt")
	}
	if !strings.HasSuffix(t1Entry.Blocker.ArtifactPath, "/scope.diff") {
		t.Fatalf("artifact_path must end with /scope.diff, got: %s", t1Entry.Blocker.ArtifactPath)
	}
	diffAbs := filepath.Join(root, filepath.FromSlash(t1Entry.Blocker.ArtifactPath))
	diffData, err := os.ReadFile(diffAbs)
	if err != nil {
		t.Fatalf("scope.diff not found at %s: %v", diffAbs, err)
	}
	if strings.TrimSpace(string(diffData)) == "" {
		t.Fatal("scope.diff must be non-empty")
	}
	if !strings.Contains(string(diffData), "src/main.go") {
		t.Fatalf("scope.diff should reference src/main.go:\n%s", diffData)
	}

	// In-scope file must be restored to its pre-task content.
	mainGo, err := os.ReadFile(filepath.Join(root, "src", "main.go"))
	if err != nil {
		t.Fatalf("read src/main.go after restore: %v", err)
	}
	if string(mainGo) != "package main\n" {
		t.Fatalf("src/main.go should be restored to original, got: %q", mainGo)
	}
	// Out-of-scope file must still exist (restore does not touch it).
	if _, err := os.Stat(filepath.Join(root, "other", "oos.txt")); err != nil {
		t.Fatalf("other/oos.txt should still exist after restore (out of scope): %v", err)
	}
}

// TestPlanDAGSnapshotRestoreOnBlock verifies that when a task blocks after
// exhausting executor attempts, the in-scope workspace is restored:
// modified files are reverted, deleted files are recreated, and new files
// are removed. It also verifies the scope.diff artifact includes newly-
// created files (f_001: computeScopeDiff now covers new in-scope files).
func TestPlanDAGSnapshotRestoreOnBlock(t *testing.T) {
	root := t.TempDir()

	// Create src/toDelete.go BEFORE setupDAGContract so that map refresh
	// includes it in the project map; this avoids a "new untracked file"
	// dirty-in-scope error on the fresh run.
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "toDelete.go"), []byte("package del\n"), 0o644); err != nil {
		t.Fatalf("write toDelete.go: %v", err)
	}

	t1 := planTask{
		ID:           "t1",
		Acceptance:   []string{"ok"},
		Validation:   []string{"false"},
		PathsInScope: []string{"src"},
	}
	app, paths, runID := setupDAGContract(t, root, []planTask{t1})
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Helper on every attempt: modify main.go, create newFile.go, delete toDelete.go.
	writeFiles, _ := json.Marshal([]map[string]string{
		{"path": "src/main.go", "data": "package modified\n"},
		{"path": "src/newFile.go", "data": "package new\n"},
	})
	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")
	t.Setenv("PACTUM_HELPER_WRITE_FILES", string(writeFiles))
	t.Setenv("PACTUM_HELPER_DELETE_FILES", `["src/toDelete.go"]`)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail when validation always fails: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	if taskEntryStatus(state, "t1") != "blocked" {
		t.Fatalf("t1 status: want blocked, got %s", taskEntryStatus(state, "t1"))
	}

	// Verify modified file is restored.
	mainGo, err := os.ReadFile(filepath.Join(root, "src", "main.go"))
	if err != nil {
		t.Fatalf("read src/main.go after restore: %v", err)
	}
	if string(mainGo) != "package main\n" {
		t.Fatalf("src/main.go should be restored to original, got: %q", mainGo)
	}
	// Verify deleted file is recreated.
	if _, err := os.Stat(filepath.Join(root, "src", "toDelete.go")); err != nil {
		t.Fatalf("src/toDelete.go should be recreated by restore: %v", err)
	}
	// Verify newly-created file is removed.
	if _, err := os.Stat(filepath.Join(root, "src", "newFile.go")); err == nil {
		t.Fatal("src/newFile.go should be deleted by restore (was not in snapshot)")
	}

	// scope.diff artifact must be present and include the newly-created file.
	var t1Entry taskStateEntry
	for _, e := range state.Tasks {
		if e.TaskID == "t1" {
			t1Entry = e
			break
		}
	}
	if t1Entry.Blocker == nil {
		t.Fatal("t1 blocker is nil")
	}
	if t1Entry.Blocker.ArtifactPath == "" {
		t.Fatal("blocker.artifact_path must be set for validation_unmet blocker")
	}
	if !strings.HasSuffix(t1Entry.Blocker.ArtifactPath, "/scope.diff") {
		t.Fatalf("artifact_path must end with /scope.diff, got: %s", t1Entry.Blocker.ArtifactPath)
	}
	diffAbs := filepath.Join(root, filepath.FromSlash(t1Entry.Blocker.ArtifactPath))
	diffData, err := os.ReadFile(diffAbs)
	if err != nil {
		t.Fatalf("scope.diff not found at %s: %v", diffAbs, err)
	}
	diffStr := string(diffData)
	if strings.TrimSpace(diffStr) == "" {
		t.Fatal("scope.diff must be non-empty")
	}
	// Must include newly-created file (exercises f_001 fix: computeScopeDiff covers new files).
	if !strings.Contains(diffStr, "src/newFile.go") {
		t.Fatalf("scope.diff should include newly-created src/newFile.go:\n%s", diffStr)
	}
	// Must include modified file.
	if !strings.Contains(diffStr, "src/main.go") {
		t.Fatalf("scope.diff should include modified src/main.go:\n%s", diffStr)
	}
}

// ---------------------------------------------------------------------------
// f_003: baseline_green_rate correctness
// ---------------------------------------------------------------------------

// TestPlanDAGBaselineGreenRateGreenRecommendations verifies that buildLoopSummary
// counts "block" and "signal" recommendations (already-green) toward the
// baseline_green_rate, and that "proceed" (baseline-red) is not counted.
func TestPlanDAGBaselineGreenRateGreenRecommendations(t *testing.T) {
	entries := []taskStateEntry{
		{TaskID: "t1", Status: "done", BaselineResult: &baselineCheckResult{Recommendation: "block"}},
		{TaskID: "t2", Status: "done", BaselineResult: &baselineCheckResult{Recommendation: "signal"}},
		{TaskID: "t3", Status: "done", BaselineResult: &baselineCheckResult{Recommendation: "proceed"}},
	}
	summary := buildLoopSummary("sha", "completed", entries)
	// "block" and "signal" = already-green; "proceed" = baseline-red (not green).
	want := 2.0 / 3.0
	if summary.BaselineGreenRate != want {
		t.Fatalf("BaselineGreenRate: want %f (2/3 green), got %f", want, summary.BaselineGreenRate)
	}

	// All "proceed" = all baseline-red → rate must be 0.
	allRed := []taskStateEntry{
		{TaskID: "t1", Status: "done", BaselineResult: &baselineCheckResult{Recommendation: "proceed"}},
		{TaskID: "t2", Status: "done", BaselineResult: &baselineCheckResult{Recommendation: "proceed"}},
	}
	summaryRed := buildLoopSummary("sha", "completed", allRed)
	if summaryRed.BaselineGreenRate != 0.0 {
		t.Fatalf("BaselineGreenRate: want 0.0 (all 'proceed' = baseline-red), got %f", summaryRed.BaselineGreenRate)
	}
}

// ---------------------------------------------------------------------------
// f_019: missing workspace-boundary tests
// ---------------------------------------------------------------------------

// TestPlanDAGDirtyInScopeRefusesStart verifies that when a file within
// paths_in_scope is dirty (new/modified vs the project map), execute run
// refuses to start and makes no executor attempts.
//
// Adding a file to src/ after map refresh makes the project map stale, which
// causes prepareExecution to reject the run before planDAGRun is called.
// The contract requirement is that the run is refused; tasks-state.json may
// or may not exist depending on which guard fires first.
func TestPlanDAGDirtyInScopeRefusesStart(t *testing.T) {
	root := t.TempDir()
	tasks := []planTask{makeTask("t1", nil)}
	app, paths, runID := setupDAGContract(t, root, tasks)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Add a new in-scope file after map refresh — this makes the project map stale,
	// which also means the in-scope working tree is dirty.
	if err := os.WriteFile(filepath.Join(root, "src", "dirty.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write dirty.go: %v", err)
	}

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail when working tree is dirty in scope: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}

	// No executor attempt should have been made.
	if attempts := countAttempts(t, runPaths.TasksDir, "t1"); attempts != 0 {
		t.Fatalf("no executor attempts should have run for dirty-scope refusal, got %d", attempts)
	}
	_ = paths
}

// TestPlanDAGCheckDirtyInScope is a unit test for the checkDirtyInScope function.
// It verifies that a new untracked in-scope file is detected as dirty, while a
// file outside the effective scope is ignored.
func TestPlanDAGCheckDirtyInScope(t *testing.T) {
	root := t.TempDir()

	// Create a hash record file with only "src/main.go" as a known file.
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	mainHash, err := fileSHA256(filepath.Join(srcDir, "main.go"))
	if err != nil {
		t.Fatalf("hash main.go: %v", err)
	}

	hashPath := filepath.Join(root, "hashes.jsonl")
	hashLine := fmt.Sprintf(`{"path":"src/main.go","sha256":"%s"}`, mainHash) + "\n"
	if err := os.WriteFile(hashPath, []byte(hashLine), 0o644); err != nil {
		t.Fatalf("write hashes: %v", err)
	}

	tasks := []planTask{{ID: "t1", PathsInScope: []string{"src"}}}

	// Clean tree — should pass.
	if err := checkDirtyInScope(root, hashPath, tasks); err != nil {
		t.Fatalf("clean tree: unexpected error: %v", err)
	}

	// Add a new untracked in-scope file — should fail.
	if err := os.WriteFile(filepath.Join(srcDir, "dirty.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write dirty.go: %v", err)
	}
	if err := checkDirtyInScope(root, hashPath, tasks); err == nil {
		t.Fatal("expected error for dirty in-scope file, got nil")
	}
	if err := os.Remove(filepath.Join(srcDir, "dirty.go")); err != nil {
		t.Fatalf("remove dirty.go: %v", err)
	}

	// A dirty file outside paths_in_scope must not trigger the check.
	otherDir := filepath.Join(root, "other")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "oos.go"), []byte("package other\n"), 0o644); err != nil {
		t.Fatalf("write oos.go: %v", err)
	}
	if err := checkDirtyInScope(root, hashPath, tasks); err != nil {
		t.Fatalf("out-of-scope dirty file should not block startup, got error: %v", err)
	}
}

// TestPlanDAGBaselineGreenHardBlock verifies that when a task's validation
// command is test-shaped and already passes, the task is blocked with reason
// "baseline_green" and no executor attempt is made.
func TestPlanDAGBaselineGreenHardBlock(t *testing.T) {
	root := t.TempDir()
	// "echo 'go test'" contains the "go test" marker (isTestShaped returns true)
	// and always exits 0, so the baseline recommendation is "block".
	t1 := planTask{
		ID:           "t1",
		Title:        "t1 title",
		Acceptance:   []string{"acceptance"},
		Validation:   []string{"echo 'go test'"},
		PathsInScope: []string{"src"},
	}
	app, paths, runID := setupDAGContract(t, root, []planTask{t1})
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	t.Setenv("PACTUM_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_HELPER_EXIT", "0")

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"execute", "run", runID}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("execute run should fail: baseline already green: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}

	state := readTasksStateForTest(t, runPaths.TasksStateJSON)
	var t1Entry taskStateEntry
	for _, e := range state.Tasks {
		if e.TaskID == "t1" {
			t1Entry = e
			break
		}
	}
	if t1Entry.Status != "blocked" {
		t.Fatalf("t1 status: want blocked, got %s", t1Entry.Status)
	}
	if t1Entry.Blocker == nil || t1Entry.Blocker.Reason != "baseline_green" {
		t.Fatalf("t1 blocker: want baseline_green, got %v", t1Entry.Blocker)
	}
	// No executor attempt should have been made.
	if attempts := countAttempts(t, runPaths.TasksDir, "t1"); attempts != 0 {
		t.Fatalf("no executor attempts should have been made for baseline_green block, got %d", attempts)
	}
	_ = paths
}

// TestPlanDAGSnapshotRestoreModeAndSymlink is a unit test of buildTaskSnapshot
// and restoreFromSnapshot covering two cases that the integration tests do not
// reach: (a) a file whose content is unchanged but whose executable bit was
// flipped (mode-only change), and (b) a symlink that was replaced by a regular
// file. Both must be fully restored.
func TestPlanDAGSnapshotRestoreModeAndSymlink(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	snapshotDir := filepath.Join(root, ".snap")

	task := planTask{ID: "t1", PathsInScope: []string{"src"}}

	// (a) Mode restore: create script.sh with mode 0644.
	scriptPath := filepath.Join(srcDir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatalf("write script.sh: %v", err)
	}
	// (b) Symlink restore: create link.go → main.go.
	symlinkPath := filepath.Join(srcDir, "link.go")
	if err := os.Symlink("main.go", symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	// Take the snapshot.
	manifest, err := buildTaskSnapshot(root, task, snapshotDir)
	if err != nil {
		t.Fatalf("buildTaskSnapshot: %v", err)
	}

	// Simulate executor: flip exec bit on script.sh (content unchanged).
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		t.Fatalf("chmod script.sh: %v", err)
	}
	// Simulate executor: delete symlink and write a regular file in its place.
	if err := os.Remove(symlinkPath); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	if err := os.WriteFile(symlinkPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write over symlink path: %v", err)
	}

	// Restore.
	if err := restoreFromSnapshot(root, task, manifest, snapshotDir); err != nil {
		t.Fatalf("restoreFromSnapshot: %v", err)
	}

	// (a) script.sh exec bit must be cleared (restored to 0644).
	scriptInfo, err := os.Lstat(scriptPath)
	if err != nil {
		t.Fatalf("stat script.sh: %v", err)
	}
	if scriptInfo.Mode()&0o111 != 0 {
		t.Fatalf("script.sh exec bit should be cleared after restore, got mode %v", scriptInfo.Mode())
	}

	// (b) link.go must be a symlink pointing to main.go again.
	linkInfo, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("stat link.go: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link.go should be a symlink after restore, got type %v", linkInfo.Mode())
	}
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("readlink link.go: %v", err)
	}
	if target != "main.go" {
		t.Fatalf("link.go symlink target: want main.go, got %s", target)
	}
}
