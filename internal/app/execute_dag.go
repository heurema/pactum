package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	looppkg "github.com/heurema/pactum/internal/loop"
)

const defaultLoopMax = 3

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type taskBlockerProposed struct {
	PathsInScopeAdd []string `json:"paths_in_scope_add,omitempty"`
}

type taskBlocker struct {
	Reason       string               `json:"reason"`
	Files        []string             `json:"files"`
	Why          string               `json:"why"`
	Proposed     *taskBlockerProposed `json:"proposed,omitempty"`
	NextCommands []string             `json:"next_commands,omitempty"`
	ArtifactPath string               `json:"artifact_path,omitempty"`
}

type taskStateEntry struct {
	TaskID           string                `json:"task_id"`
	Status           string                `json:"status"`
	Attempts         int                   `json:"attempts"`
	By               string                `json:"by,omitempty"`
	FilesTouched     []string              `json:"files_touched,omitempty"`
	SnapshotRef      string                `json:"snapshot_ref,omitempty"`
	BaselineResult   *baselineCheckResult  `json:"baseline_result,omitempty"`
	ValidationResult *taskValidationResult `json:"validation_result,omitempty"`
	Blocker          *taskBlocker          `json:"blocker,omitempty"`
	ContextPackBytes int                   `json:"context_pack_bytes,omitempty"`
}

type tasksRunLevel struct {
	TerminalState  string `json:"terminal_state,omitempty"`
	TerminalTaskID string `json:"terminal_task_id,omitempty"`
	Error          string `json:"error,omitempty"`
}

type tasksStateDocument struct {
	ContractSHA256 string           `json:"contract_sha256"`
	Run            tasksRunLevel    `json:"run"`
	Tasks          []taskStateEntry `json:"tasks"`
}

type loopSummaryDocument struct {
	ContractSHA256       string         `json:"contract_sha256"`
	TerminalState        string         `json:"terminal_state"`
	TasksDone            int            `json:"tasks_done"`
	TasksBlocked         int            `json:"tasks_blocked"`
	TasksBlockedUpstream int            `json:"tasks_blocked_upstream"`
	TotalRetries         int            `json:"total_retries"`
	BaselineGreenRate    float64        `json:"baseline_green_rate"`
	ContextPackBytes     map[string]int `json:"context_pack_bytes,omitempty"`
	Error                string         `json:"error,omitempty"`
}

type snapshotEntry struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	Mode      uint32 `json:"mode"`
	SHA256    string `json:"sha256,omitempty"`
	SymTarget string `json:"sym_target,omitempty"`
}

type snapshotManifest struct {
	Entries []snapshotEntry `json:"entries"`
}

type sentinelBlocker struct {
	Reason   string               `json:"reason"`
	Why      string               `json:"why"`
	Files    []string             `json:"files,omitempty"`
	Proposed *taskBlockerProposed `json:"proposed,omitempty"`
}

type dagLastResult struct {
	Schema         string `json:"schema"`
	ContractSHA256 string `json:"contract_sha256"`
	TerminalState  string `json:"terminal_state"`
	Passed         bool   `json:"passed"`
	TasksDone      int    `json:"tasks_done,omitempty"`
	TasksBlocked   int    `json:"tasks_blocked,omitempty"`
}

type dagRunResponse struct {
	ContractSHA256 string           `json:"contract_sha256"`
	TerminalState  string           `json:"terminal_state"`
	Passed         bool             `json:"passed"`
	TasksDone      int              `json:"tasks_done,omitempty"`
	TasksBlocked   int              `json:"tasks_blocked,omitempty"`
	NextCommands   []string         `json:"next_commands,omitempty"`
	BlockedTasks   []taskStateEntry `json:"blocked_tasks,omitempty"`
}

// ---------------------------------------------------------------------------
// planDAGRun — main orchestrator
// ---------------------------------------------------------------------------

func planDAGRun(a App, stdout io.Writer, liveOutput io.Writer, prep executionPreparation, timeout time.Duration, jsonOutput bool) error {
	root := prep.Root
	runID := prep.State.RunID
	contractSHA256 := prep.ContractSHA256
	tasks := prep.Contract.Plan.Tasks

	loopMax := defaultLoopMax
	config, err := readConfig(prep.Paths.Config)
	if err == nil && config.Pipeline.Execute.Loop != nil && config.Pipeline.Execute.Loop.Max > 0 {
		loopMax = config.Pipeline.Execute.Loop.Max
	}

	if err := activeStore.MkdirAll(prep.RunPaths.ExecuteDir); err != nil {
		return err
	}

	// Validate DAG
	if err := validateDAGDependencies(tasks); err != nil {
		taskEntries := initTaskEntries(tasks)
		// Record a structured invalid_dag blocker on each task with an unknown dep.
		knownIDs := make(map[string]bool, len(tasks))
		for _, t := range tasks {
			knownIDs[t.ID] = true
		}
		for i, t := range tasks {
			for _, dep := range t.DependsOn {
				if !knownIDs[dep] {
					taskEntries[i].Status = "blocked"
					taskEntries[i].Blocker = &taskBlocker{
						Reason: "invalid_dag",
						Files:  []string{},
						Why:    fmt.Sprintf("depends on unknown task %q", dep),
					}
					break
				}
			}
		}
		state := tasksStateDocument{
			ContractSHA256: contractSHA256,
			Run: tasksRunLevel{
				TerminalState: "error",
				Error:         err.Error(),
			},
			Tasks: taskEntries,
		}
		_ = writeTasksState(prep.RunPaths.TasksStateJSON, state)
		return fmt.Errorf("%w", dagTerminalError{"error", err.Error()})
	}

	// Read or create tasks-state.json
	var taskState tasksStateDocument
	if isRegularFile(prep.RunPaths.TasksStateJSON) {
		loaded, err := readTasksState(prep.RunPaths.TasksStateJSON)
		if err != nil {
			return err
		}
		if loaded.ContractSHA256 != contractSHA256 {
			return fmt.Errorf("cannot resume: tasks-state.json was written for a different contract (sha256 mismatch)")
		}
		taskState = loaded
	} else {
		// Fresh run: check dirty-in-scope precondition
		if err := checkDirtyInScope(root, prep.Paths.HashesJSONL, tasks); err != nil {
			errState := tasksStateDocument{
				ContractSHA256: contractSHA256,
				Run: tasksRunLevel{
					TerminalState: "error",
					Error:         err.Error(),
				},
				Tasks: initTaskEntries(tasks),
			}
			_ = writeTasksState(prep.RunPaths.TasksStateJSON, errState)
			return fmt.Errorf("%w", dagTerminalError{"error", err.Error()})
		}
		taskState = tasksStateDocument{
			ContractSHA256: contractSHA256,
			Run:            tasksRunLevel{},
			Tasks:          initTaskEntries(tasks),
		}
	}

	// If already in terminal state, report and exit
	if taskState.Run.TerminalState != "" {
		ts := taskState.Run.TerminalState
		_ = writeDAGLastResult(prep.RunPaths.LastResultJSON, dagLastResult{
			Schema:         "pactum.dag_last_result.v1alpha1",
			ContractSHA256: contractSHA256,
			TerminalState:  ts,
			Passed:         ts == "completed",
		})
		if jsonOutput {
			return writeJSONResponse(stdout, dagRunResponse{
				ContractSHA256: contractSHA256,
				TerminalState:  ts,
				Passed:         ts == "completed",
			})
		}
		writePlanDAGOutput(stdout, ts, taskState.Tasks, runID)
		if ts != "completed" {
			return dagTerminalError{ts, taskState.Run.Error}
		}
		return nil
	}

	// Resume: reset any "running" tasks back to "ready"
	for i := range taskState.Tasks {
		if taskState.Tasks[i].Status == "running" {
			taskState.Tasks[i].Status = "ready"
			taskState.Tasks[i].Blocker = nil
		}
	}

	// Initialize readiness for pending tasks
	refreshReadiness(tasks, taskState.Tasks)

	// Snapshot temp dir
	snapshotTmpDir := filepath.Join(prep.Paths.Workspace, "tmp", "dag-snapshots", runID)
	if err := os.MkdirAll(snapshotTmpDir, 0o755); err != nil {
		return fmt.Errorf("create snapshot tmp dir: %w", err)
	}

	// DAG drain loop
	tasksDir := prep.RunPaths.TasksDir
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return fmt.Errorf("create tasks dir: %w", err)
	}

	humanExitTaskID := ""

	for {
		next := pickNextReadyTask(tasks, taskState.Tasks)
		if next == nil {
			break
		}
		idx := taskIndex(taskState.Tasks, next.ID)
		if idx < 0 {
			break
		}

		taskState.Tasks[idx].Status = "running"
		if err := writeTasksState(prep.RunPaths.TasksStateJSON, taskState); err != nil {
			return err
		}

		taskAttemptsDir := filepath.Join(tasksDir, next.ID, "attempts")
		if err := os.MkdirAll(taskAttemptsDir, 0o755); err != nil {
			return fmt.Errorf("create task attempts dir for %s: %w", next.ID, err)
		}

		taskSnapshotDir := filepath.Join(snapshotTmpDir, next.ID)
		if err := os.MkdirAll(taskSnapshotDir, 0o755); err != nil {
			return fmt.Errorf("create task snapshot dir for %s: %w", next.ID, err)
		}

		updated, runErr := runTaskNode(a, prep, *next, taskState.Tasks[idx], taskAttemptsDir, taskSnapshotDir, loopMax, timeout)
		if runErr != nil {
			taskState.Run.TerminalState = "error"
			taskState.Run.Error = runErr.Error()
			_ = writeTasksState(prep.RunPaths.TasksStateJSON, taskState)
			return fmt.Errorf("%w", dagTerminalError{"error", runErr.Error()})
		}

		taskState.Tasks[idx] = updated

		if updated.Status == "blocked" && updated.Blocker != nil && updated.Blocker.Reason == "requires_human" {
			// Stop immediately; no more tasks
			taskState.Run.TerminalState = "human"
			taskState.Run.TerminalTaskID = next.ID
			humanExitTaskID = next.ID
			if err := writeTasksState(prep.RunPaths.TasksStateJSON, taskState); err != nil {
				return err
			}
			break
		}

		// Update readiness for dependents
		if updated.Status == "done" {
			refreshReadiness(tasks, taskState.Tasks)
		} else if updated.Status == "blocked" {
			// Mark transitive dependents as blocked-upstream
			markBlockedUpstream(tasks, taskState.Tasks, next.ID)
		}

		if err := writeTasksState(prep.RunPaths.TasksStateJSON, taskState); err != nil {
			return err
		}
	}

	// Determine terminal state
	terminalState := ""
	if taskState.Run.TerminalState != "" {
		terminalState = taskState.Run.TerminalState
	} else {
		allDone := true
		anyBlocked := false
		for _, e := range taskState.Tasks {
			if e.Status != "done" {
				allDone = false
			}
			if e.Status == "blocked" || e.Status == "blocked-upstream" {
				anyBlocked = true
			}
		}
		if anyBlocked {
			terminalState = "blocked"
		} else if allDone {
			// Run constitution gate
			gateState, gateErr := runDAGConstitutionGate(a, prep, timeout)
			if gateErr != nil {
				terminalState = "error"
				taskState.Run.Error = gateErr.Error()
			} else {
				terminalState = gateState
			}
		} else {
			terminalState = "error"
			taskState.Run.Error = "tasks in unexpected state"
		}
	}

	taskState.Run.TerminalState = terminalState
	if err := writeTasksState(prep.RunPaths.TasksStateJSON, taskState); err != nil {
		return err
	}

	// Build summary
	summary := buildLoopSummary(contractSHA256, terminalState, taskState.Tasks)
	if err := writeJSON(prep.RunPaths.LoopSummaryJSON, summary); err != nil {
		return err
	}

	// Append ledger event
	_ = appendExecutionDrainedEvent(prep.Paths.EventsJSONL, contractSHA256, terminalState, summary)

	// Write last-result.json
	lastResult := dagLastResult{
		Schema:         "pactum.dag_last_result.v1alpha1",
		ContractSHA256: contractSHA256,
		TerminalState:  terminalState,
		Passed:         terminalState == "completed",
		TasksDone:      summary.TasksDone,
		TasksBlocked:   summary.TasksBlocked + summary.TasksBlockedUpstream,
	}
	_ = writeDAGLastResult(prep.RunPaths.LastResultJSON, lastResult)

	if jsonOutput {
		resp := dagRunResponse{
			ContractSHA256: contractSHA256,
			TerminalState:  terminalState,
			Passed:         terminalState == "completed",
			TasksDone:      summary.TasksDone,
			TasksBlocked:   summary.TasksBlocked + summary.TasksBlockedUpstream,
		}
		// Populate blocked task entries with full blocker details for blocked/human runs.
		if terminalState == "blocked" || terminalState == "human" {
			for _, te := range taskState.Tasks {
				if te.Status == "blocked" && te.Blocker != nil {
					resp.BlockedTasks = append(resp.BlockedTasks, te)
				}
			}
		}
		if terminalState == "human" && humanExitTaskID != "" {
			idx := taskIndex(taskState.Tasks, humanExitTaskID)
			if idx >= 0 && taskState.Tasks[idx].Blocker != nil {
				resp.NextCommands = taskState.Tasks[idx].Blocker.NextCommands
			}
		}
		if err := writeJSONResponse(stdout, resp); err != nil {
			return err
		}
	} else {
		writePlanDAGOutput(stdout, terminalState, taskState.Tasks, runID)
	}

	if terminalState != "completed" {
		return dagTerminalError{terminalState, taskState.Run.Error}
	}
	return nil
}

// dagTerminalError is a non-zero exit that signals the DAG's terminal state.
type dagTerminalError struct {
	State string
	Msg   string
}

func (e dagTerminalError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("plan-DAG terminal state: %s: %s", e.State, e.Msg)
	}
	return fmt.Sprintf("plan-DAG terminal state: %s", e.State)
}

// ---------------------------------------------------------------------------
// DAG validation
// ---------------------------------------------------------------------------

func validateDAGDependencies(tasks []planTask) error {
	ids := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		ids[t.ID] = true
	}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("invalid_dag: task %q depends on unknown task %q", t.ID, dep)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Task state management
// ---------------------------------------------------------------------------

func initTaskEntries(tasks []planTask) []taskStateEntry {
	entries := make([]taskStateEntry, len(tasks))
	for i, t := range tasks {
		status := "pending"
		if len(t.DependsOn) == 0 {
			status = "ready"
		}
		entries[i] = taskStateEntry{TaskID: t.ID, Status: status}
	}
	return entries
}

func refreshReadiness(tasks []planTask, entries []taskStateEntry) {
	doneSet := make(map[string]bool)
	for _, e := range entries {
		if e.Status == "done" {
			doneSet[e.TaskID] = true
		}
	}
	for i, t := range tasks {
		if entries[i].Status != "pending" {
			continue
		}
		allDone := true
		for _, dep := range t.DependsOn {
			if !doneSet[dep] {
				allDone = false
				break
			}
		}
		if allDone {
			entries[i].Status = "ready"
		}
	}
}

func markBlockedUpstream(tasks []planTask, entries []taskStateEntry, blockedTaskID string) {
	// Find all tasks that (transitively) depend on blockedTaskID
	directDeps := map[string][]string{}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			directDeps[dep] = append(directDeps[dep], t.ID)
		}
	}

	// BFS from blockedTaskID
	queue := []string{blockedTaskID}
	visited := map[string]bool{blockedTaskID: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dependent := range directDeps[cur] {
			if visited[dependent] {
				continue
			}
			visited[dependent] = true
			idx := taskIndex(entries, dependent)
			if idx >= 0 && (entries[idx].Status == "pending" || entries[idx].Status == "ready") {
				entries[idx].Status = "blocked-upstream"
			}
			queue = append(queue, dependent)
		}
	}
}

func pickNextReadyTask(tasks []planTask, entries []taskStateEntry) *planTask {
	for i, t := range tasks {
		if entries[i].Status == "ready" {
			t2 := t
			return &t2
		}
	}
	return nil
}

func taskIndex(entries []taskStateEntry, taskID string) int {
	for i, e := range entries {
		if e.TaskID == taskID {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// runTaskNode
// ---------------------------------------------------------------------------

func runTaskNode(a App, prep executionPreparation, task planTask, entry taskStateEntry, taskAttemptsDir string, snapshotDir string, loopMax int, timeout time.Duration) (taskStateEntry, error) {
	root := prep.Root

	// Validate scope
	if err := validateTaskScope(root, task); err != nil {
		entry.Status = "blocked"
		blocker := &taskBlocker{
			Reason: "invalid_scope",
			Files:  []string{},
			Why:    err.Error(),
		}
		blocker.NextCommands = nextCommandsForBlockedTask(prep.State.RunID, task, blocker)
		entry.Blocker = blocker
		return entry, nil
	}

	// Baseline check
	baseline, err := runBaselineCheck(root, task.Validation, timeout)
	if err != nil {
		return entry, fmt.Errorf("baseline check for task %s: %w", task.ID, err)
	}
	entry.BaselineResult = &baseline

	if baseline.Recommendation == "block" {
		entry.Status = "blocked"
		blocker := &taskBlocker{
			Reason: "baseline_green",
			Files:  []string{},
			Why:    "task validation commands already pass before any changes",
		}
		blocker.NextCommands = nextCommandsForBlockedTask(prep.State.RunID, task, blocker)
		entry.Blocker = blocker
		return entry, nil
	}

	// Build task start content snapshot
	manifest, err := buildTaskSnapshot(root, task, snapshotDir)
	if err != nil {
		return entry, fmt.Errorf("build task snapshot for %s: %w", task.ID, err)
	}

	// Build task start hash baseline (whole tree)
	hashBaseline, err := buildTaskHashBaseline(root)
	if err != nil {
		return entry, fmt.Errorf("build hash baseline for %s: %w", task.ID, err)
	}

	// Build context pack
	pack, err := buildContextPack(root, task, prep.Contract, prep.RunPaths)
	if err != nil {
		return entry, fmt.Errorf("build context pack for %s: %w", task.ID, err)
	}
	contextPackText, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(pack.Path)))
	if readErr != nil {
		contextPackText = []byte("")
	}
	entry.ContextPackBytes = len(contextPackText)

	var capturedBlocker *taskBlocker
	var capturedValidation *taskValidationResult
	priorFailure := ""

	// Run loop
	outcome, loopErr := looppkg.Run(context.Background(), looppkg.Limits{Max: loopMax, Settle: 1, Patience: 0}, func(ctx context.Context, rc looppkg.RoundContext) (looppkg.RoundResult, error) {
		// Build prompt
		prompt := buildTaskPrompt(prep.Contract, task, string(contextPackText), priorFailure)

		// Run executor attempt
		attemptID, combinedOutput, _, attemptErr := runDAGTaskAttempt(a, prep, task, taskAttemptsDir, prompt, timeout)
		if attemptErr != nil {
			return looppkg.RoundResult{}, attemptErr
		}
		entry.Attempts++
		entry.By = prep.AgentName

		// Scan for PACTUM_BLOCKER sentinel
		if sentinel := scanForBlockerSentinel(combinedOutput); sentinel != nil {
			blocker := &taskBlocker{
				Reason:   sentinel.Reason,
				Files:    sentinel.Files,
				Why:      sentinel.Why,
				Proposed: sentinel.Proposed,
			}
			if blocker.Files == nil {
				blocker.Files = []string{}
			}
			// Write diff artifact
			diff, _ := computeScopeDiff(root, task, manifest)
			attemptDir := filepath.Join(taskAttemptsDir, attemptID)
			artifactPath, _ := writeDiffArtifact(root, attemptDir, diff)
			blocker.ArtifactPath = artifactPath
			blocker.NextCommands = nextCommandsForBlockedTask(prep.State.RunID, task, blocker)

			capturedBlocker = blocker
			if sentinel.Reason == "requires_human" {
				return looppkg.RoundResult{Human: &looppkg.HumanExit{Reason: "requires_human"}}, nil
			}
			return looppkg.RoundResult{Human: &looppkg.HumanExit{Reason: sentinel.Reason}}, nil
		}

		// Detect out-of-scope changes
		outOfScope, err := detectOutOfScopeChanges(root, task, hashBaseline)
		if err != nil {
			return looppkg.RoundResult{}, err
		}
		if len(outOfScope) > 0 {
			blocker := &taskBlocker{
				Reason: "out_of_scope",
				Files:  outOfScope,
				Why:    fmt.Sprintf("executor modified %d file(s) outside paths_in_scope", len(outOfScope)),
			}
			diff, _ := computeScopeDiff(root, task, manifest)
			attemptDir := filepath.Join(taskAttemptsDir, attemptID)
			artifactPath, _ := writeDiffArtifact(root, attemptDir, diff)
			blocker.ArtifactPath = artifactPath
			blocker.NextCommands = nextCommandsForBlockedTask(prep.State.RunID, task, blocker)
			capturedBlocker = blocker

			// Restore in-scope snapshot
			if restoreErr := restoreFromSnapshot(root, task, manifest, snapshotDir); restoreErr != nil {
				return looppkg.RoundResult{}, fmt.Errorf("restore snapshot after out-of-scope escape: %w", restoreErr)
			}

			return looppkg.RoundResult{Human: &looppkg.HumanExit{Reason: "out_of_scope"}}, nil
		}

		// Run task validation
		validationResult, err := runTaskValidation(root, task.Validation, timeout)
		if err != nil {
			return looppkg.RoundResult{}, err
		}
		capturedValidation = &validationResult

		if validationResult.Passed {
			return looppkg.RoundResult{Clean: true}, nil
		}

		// Build prior failure text for next retry
		var sb strings.Builder
		for _, cmd := range validationResult.Commands {
			if cmd.ExitCode != 0 || cmd.TimedOut {
				fmt.Fprintf(&sb, "Command: %s\n", cmd.Command)
				fmt.Fprintf(&sb, "Exit code: %d\n", cmd.ExitCode)
				if cmd.TimedOut {
					fmt.Fprintf(&sb, "Timed out: true\n")
				}
				if strings.TrimSpace(cmd.Stdout) != "" {
					fmt.Fprintf(&sb, "Stdout:\n%s\n", cmd.Stdout)
				}
				if strings.TrimSpace(cmd.Stderr) != "" {
					fmt.Fprintf(&sb, "Stderr:\n%s\n", cmd.Stderr)
				}
			}
		}
		priorFailure = sb.String()

		return looppkg.RoundResult{Clean: false, Progress: true}, nil
	})

	if loopErr != nil {
		return entry, loopErr
	}

	switch outcome.Reason {
	case "settled":
		entry.Status = "done"
		entry.ValidationResult = capturedValidation
		entry.FilesTouched = computeFilesTouched(root, task, manifest)
	case "max":
		// validation_unmet - write diff and restore snapshot
		diff, _ := computeScopeDiff(root, task, manifest)
		entry.FilesTouched = computeFilesTouched(root, task, manifest)
		lastAttemptID := lastAttemptIDFromDir(taskAttemptsDir)
		if lastAttemptID != "" {
			artifactPath, _ := writeDiffArtifact(root, filepath.Join(taskAttemptsDir, lastAttemptID), diff)
			blocker := &taskBlocker{
				Reason:       "validation_unmet",
				Files:        []string{},
				Why:          "task validation commands did not pass after maximum attempts",
				ArtifactPath: artifactPath,
			}
			blocker.NextCommands = nextCommandsForBlockedTask(prep.State.RunID, task, blocker)
			capturedBlocker = blocker
		}
		entry.Status = "blocked"
		entry.Blocker = capturedBlocker
		entry.ValidationResult = capturedValidation
		// Restore in-scope snapshot
		if restoreErr := restoreFromSnapshot(root, task, manifest, snapshotDir); restoreErr != nil {
			return entry, fmt.Errorf("restore snapshot after validation block: %w", restoreErr)
		}
	case "human":
		if capturedBlocker != nil {
			entry.Status = "blocked"
			entry.Blocker = capturedBlocker
			entry.ValidationResult = capturedValidation
			entry.FilesTouched = computeFilesTouched(root, task, manifest)
			// Restore for out_of_scope, but NOT for requires_human
			if capturedBlocker.Reason != "requires_human" {
				if restoreErr := restoreFromSnapshot(root, task, manifest, snapshotDir); restoreErr != nil {
					return entry, fmt.Errorf("restore snapshot after %s block: %w", capturedBlocker.Reason, restoreErr)
				}
			}
		}
	}

	return entry, nil
}

func lastAttemptIDFromDir(attemptsDir string) string {
	entries, err := os.ReadDir(attemptsDir)
	if err != nil {
		return ""
	}
	last := ""
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "attempt_") {
			if e.Name() > last {
				last = e.Name()
			}
		}
	}
	return last
}

// ---------------------------------------------------------------------------
// Scope validation
// ---------------------------------------------------------------------------

func validateTaskScope(root string, task planTask) error {
	if len(task.PathsInScope) == 0 {
		return fmt.Errorf("task %q: paths_in_scope must be non-empty", task.ID)
	}
	cleanRoot := filepath.Clean(root)
	for _, p := range task.PathsInScope {
		if p == "" || p == "." {
			return fmt.Errorf("task %q: paths_in_scope entry %q is invalid (empty or '.')", task.ID, p)
		}
		if strings.HasPrefix(p, "/") {
			return fmt.Errorf("task %q: paths_in_scope entry %q must be relative (no leading /)", task.ID, p)
		}
		if strings.Contains(p, "..") {
			return fmt.Errorf("task %q: paths_in_scope entry %q must not contain '..'", task.ID, p)
		}
		// Check if it resolves to repo root
		abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(p)))
		if abs == cleanRoot {
			return fmt.Errorf("task %q: paths_in_scope entry %q resolves to the repo root", task.ID, p)
		}
	}
	return nil
}

// scopeContains reports whether filePath is under scope entry scopeEntry (prefix-based).
func scopeContains(scopeEntry, filePath string) bool {
	// Normalize both to forward slashes
	e := filepath.ToSlash(filepath.Clean(scopeEntry))
	f := filepath.ToSlash(filepath.Clean(filePath))
	return f == e || strings.HasPrefix(f, e+"/")
}

// inEffectiveScope reports whether filePath is in effective scope:
// in paths_in_scope AND NOT in paths_out_of_scope.
func inEffectiveScope(task planTask, filePath string) bool {
	filePath = filepath.ToSlash(filePath)
	inScope := false
	for _, p := range task.PathsInScope {
		if scopeContains(p, filePath) {
			inScope = true
			break
		}
	}
	if !inScope {
		return false
	}
	for _, p := range task.PathsOutOfScope {
		if scopeContains(p, filePath) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Snapshot functions
// ---------------------------------------------------------------------------

func buildTaskSnapshot(root string, task planTask, snapshotDir string) (snapshotManifest, error) {
	if err := os.MkdirAll(filepath.Join(snapshotDir, "blobs"), 0o755); err != nil {
		return snapshotManifest{}, err
	}

	var entries []snapshotEntry

	for _, scopeEntry := range task.PathsInScope {
		absScope := filepath.Join(root, filepath.FromSlash(scopeEntry))
		err := filepath.Walk(absScope, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)

			if info.IsDir() {
				return nil
			}

			// Skip if not in effective scope
			if !inEffectiveScope(task, relSlash) {
				return nil
			}

			linfo, err := os.Lstat(path)
			if err != nil {
				return nil
			}

			if linfo.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return nil
				}
				entries = append(entries, snapshotEntry{
					Path:      relSlash,
					Type:      "symlink",
					Mode:      uint32(linfo.Mode()),
					SymTarget: target,
				})
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256(data)
			shaHex := hex.EncodeToString(sum[:])
			blobPath := filepath.Join(snapshotDir, "blobs", shaHex)
			if _, err := os.Stat(blobPath); os.IsNotExist(err) {
				if err := os.WriteFile(blobPath, data, 0o644); err != nil {
					return err
				}
			}
			entries = append(entries, snapshotEntry{
				Path:   relSlash,
				Type:   "regular",
				Mode:   uint32(linfo.Mode()),
				SHA256: shaHex,
			})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return snapshotManifest{}, err
		}
	}

	manifest := snapshotManifest{Entries: entries}
	if manifest.Entries == nil {
		manifest.Entries = []snapshotEntry{}
	}

	tmpPath := filepath.Join(snapshotDir, "manifest.json.tmp")
	finalPath := filepath.Join(snapshotDir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return snapshotManifest{}, err
	}
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return snapshotManifest{}, err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return snapshotManifest{}, err
	}
	return manifest, nil
}

func restoreFromSnapshot(root string, task planTask, manifest snapshotManifest, snapshotDir string) error {
	// Build set of in-scope files from snapshot
	snapshotFiles := make(map[string]snapshotEntry, len(manifest.Entries))
	for _, e := range manifest.Entries {
		snapshotFiles[e.Path] = e
	}

	// Restore files from snapshot
	for _, entry := range manifest.Entries {
		absPath := filepath.Join(root, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return err
		}
		switch entry.Type {
		case "symlink":
			// Remove existing and recreate
			_ = os.Remove(absPath)
			if err := os.Symlink(entry.SymTarget, absPath); err != nil {
				return err
			}
		default:
			blobPath := filepath.Join(snapshotDir, "blobs", entry.SHA256)
			data, err := os.ReadFile(blobPath)
			if err != nil {
				continue
			}
			// Check if content already matches; still re-apply mode unconditionally.
			if current, err := os.ReadFile(absPath); err == nil {
				if currentSum := sha256.Sum256(current); hex.EncodeToString(currentSum[:]) == entry.SHA256 {
					// Content matches; ensure mode is also restored.
					if linfo, err := os.Lstat(absPath); err == nil {
						if linfo.Mode()&0o777 != os.FileMode(entry.Mode)&0o777 {
							_ = os.Chmod(absPath, os.FileMode(entry.Mode)&0o777)
						}
					}
					continue
				}
			}
			if err := os.WriteFile(absPath, data, os.FileMode(entry.Mode)&0o777); err != nil {
				return err
			}
		}
	}

	// Delete files that exist in-scope but were NOT in the snapshot
	for _, scopeEntry := range task.PathsInScope {
		absScope := filepath.Join(root, filepath.FromSlash(scopeEntry))
		_ = filepath.Walk(absScope, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			if !inEffectiveScope(task, relSlash) {
				return nil
			}
			if _, exists := snapshotFiles[relSlash]; !exists {
				_ = os.Remove(path)
			}
			return nil
		})
	}
	return nil
}

func buildTaskHashBaseline(root string) (map[string]string, error) {
	result := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		// Skip .heurema directory
		if relSlash == ".heurema" || strings.HasPrefix(relSlash, ".heurema/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		// Skip symlinks for simplicity
		linfo, err := os.Lstat(path)
		if err != nil {
			return nil
		}
		if linfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		result[relSlash] = hex.EncodeToString(sum[:])
		return nil
	})
	return result, err
}

func detectOutOfScopeChanges(root string, task planTask, baseline map[string]string) ([]string, error) {
	var outOfScope []string
	current := make(map[string]string)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == ".heurema" || strings.HasPrefix(relSlash, ".heurema/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		linfo, err := os.Lstat(path)
		if err != nil {
			return nil
		}
		if linfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		current[relSlash] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Check for new/modified files outside effective scope
	for path, hash := range current {
		baseHash, existed := baseline[path]
		if existed && hash == baseHash {
			continue
		}
		// File is new or changed
		if !inEffectiveScope(task, path) {
			outOfScope = append(outOfScope, path)
		}
	}

	// Check for deleted files outside effective scope
	for path := range baseline {
		if _, exists := current[path]; !exists {
			if !inEffectiveScope(task, path) {
				outOfScope = append(outOfScope, path)
			}
		}
	}

	return outOfScope, nil
}

// ---------------------------------------------------------------------------
// Diff artifact
// ---------------------------------------------------------------------------

func computeScopeDiff(root string, task planTask, manifest snapshotManifest) (string, error) {
	var sb strings.Builder

	// Index snapshot entries by path to detect new files below.
	snapshotByPath := make(map[string]snapshotEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		snapshotByPath[entry.Path] = entry
	}

	// Modified or deleted in-scope files (were in snapshot).
	for _, entry := range manifest.Entries {
		if entry.Type != "regular" {
			continue
		}
		absPath := filepath.Join(root, filepath.FromSlash(entry.Path))
		currentData, err := os.ReadFile(absPath)
		if err != nil {
			// File was deleted.
			fmt.Fprintf(&sb, "--- a/%s\n", entry.Path)
			fmt.Fprintf(&sb, "+++ /dev/null\n")
			continue
		}
		currentSum := sha256.Sum256(currentData)
		if hex.EncodeToString(currentSum[:]) == entry.SHA256 {
			continue
		}
		fmt.Fprintf(&sb, "--- a/%s\n", entry.Path)
		fmt.Fprintf(&sb, "+++ b/%s\n", entry.Path)
		fmt.Fprintf(&sb, "@@ file changed @@\n")
	}

	// Newly-created in-scope files (not present in snapshot).
	for _, scopeEntry := range task.PathsInScope {
		absScope := filepath.Join(root, filepath.FromSlash(scopeEntry))
		_ = filepath.Walk(absScope, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			if !inEffectiveScope(task, relSlash) {
				return nil
			}
			if _, exists := snapshotByPath[relSlash]; !exists {
				fmt.Fprintf(&sb, "--- /dev/null\n")
				fmt.Fprintf(&sb, "+++ b/%s\n", relSlash)
				fmt.Fprintf(&sb, "@@ new file @@\n")
			}
			return nil
		})
	}

	return sb.String(), nil
}

// computeFilesTouched returns the repo-relative paths of all in-scope files that
// were created, modified, or deleted compared to the task's start-state snapshot.
func computeFilesTouched(root string, task planTask, manifest snapshotManifest) []string {
	snapshotByPath := make(map[string]snapshotEntry, len(manifest.Entries))
	for _, e := range manifest.Entries {
		snapshotByPath[e.Path] = e
	}

	var files []string

	// Modified or deleted files (were in snapshot).
	for _, entry := range manifest.Entries {
		if entry.Type != "regular" {
			continue
		}
		absPath := filepath.Join(root, filepath.FromSlash(entry.Path))
		currentData, err := os.ReadFile(absPath)
		if err != nil {
			files = append(files, entry.Path) // deleted
			continue
		}
		currentSum := sha256.Sum256(currentData)
		if hex.EncodeToString(currentSum[:]) != entry.SHA256 {
			files = append(files, entry.Path) // modified
		}
	}

	// Newly-created in-scope files (not in snapshot).
	for _, scopeEntry := range task.PathsInScope {
		absScope := filepath.Join(root, filepath.FromSlash(scopeEntry))
		_ = filepath.Walk(absScope, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			if !inEffectiveScope(task, relSlash) {
				return nil
			}
			if _, exists := snapshotByPath[relSlash]; !exists {
				files = append(files, relSlash)
			}
			return nil
		})
	}

	return files
}

func writeDiffArtifact(root string, attemptDir string, diff string) (string, error) {
	if strings.TrimSpace(diff) == "" {
		return "", nil
	}
	diffPath := filepath.Join(attemptDir, "scope.diff")
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, diffPath)
	if err != nil {
		return diffPath, nil
	}
	return filepath.ToSlash(rel), nil
}

// ---------------------------------------------------------------------------
// Task prompt
// ---------------------------------------------------------------------------

func buildTaskPrompt(contract draftContract, task planTask, contextPackText string, priorFailure string) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, "# Executor Prompt")
	fmt.Fprintln(&sb)
	fmt.Fprintf(&sb, "This prompt is prepared from an approved Pactum contract for plan task %s.\n", task.ID)
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, "## Contract goal")
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, contract.Goal)
	fmt.Fprintln(&sb)
	if task.Title != "" {
		fmt.Fprintf(&sb, "## Task: %s\n\n%s\n\n", task.ID, task.Title)
	} else {
		fmt.Fprintf(&sb, "## Task: %s\n\n", task.ID)
	}
	if len(task.DependsOn) > 0 {
		fmt.Fprintln(&sb, "### Depends on")
		for _, dep := range task.DependsOn {
			fmt.Fprintf(&sb, "- %s\n", dep)
		}
		fmt.Fprintln(&sb)
	}
	if len(task.Acceptance) > 0 {
		fmt.Fprintln(&sb, "### Acceptance criteria")
		for _, a := range task.Acceptance {
			fmt.Fprintf(&sb, "- %s\n", a)
		}
		fmt.Fprintln(&sb)
	}
	if len(task.Validation) > 0 {
		fmt.Fprintln(&sb, "### Validation commands")
		for _, v := range task.Validation {
			fmt.Fprintf(&sb, "- %s\n", v)
		}
		fmt.Fprintln(&sb)
	}
	if len(task.PathsInScope) > 0 {
		fmt.Fprintln(&sb, "### Paths in scope")
		for _, p := range task.PathsInScope {
			fmt.Fprintf(&sb, "- %s\n", p)
		}
		fmt.Fprintln(&sb)
	}
	if len(task.PathsOutOfScope) > 0 {
		fmt.Fprintln(&sb, "### Paths out of scope")
		for _, p := range task.PathsOutOfScope {
			fmt.Fprintf(&sb, "- %s\n", p)
		}
		fmt.Fprintln(&sb)
	}
	fmt.Fprintln(&sb, "### Instructions")
	fmt.Fprintln(&sb, `Stay strictly within paths_in_scope. If you determine the task requires modifying`)
	fmt.Fprintln(&sb, `a file outside paths_in_scope, do NOT modify it. Instead emit a blocker sentinel`)
	fmt.Fprintln(&sb, `on a line by itself:`)
	fmt.Fprintln(&sb, `  PACTUM_BLOCKER: {"reason":"out_of_scope","files":["<path>"],"why":"<explanation>","proposed":{"paths_in_scope_add":["<path>"]}}`)
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, `If the task requires human judgment (conflicting requirements, ambiguous contract`)
	fmt.Fprintln(&sb, `intent), emit:`)
	fmt.Fprintln(&sb, `  PACTUM_BLOCKER: {"reason":"requires_human","why":"<explanation>"}`)
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, "After completing your changes, your code must pass all validation commands.")

	if priorFailure != "" {
		fmt.Fprintln(&sb)
		fmt.Fprintln(&sb, "## Prior validation failure (attempt N-1)")
		fmt.Fprintln(&sb)
		fmt.Fprintln(&sb, priorFailure)
	}

	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, "## Context pack")
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, contextPackText)

	return sb.String()
}

// ---------------------------------------------------------------------------
// PACTUM_BLOCKER sentinel scanner
// ---------------------------------------------------------------------------

func scanForBlockerSentinel(output string) *sentinelBlocker {
	const prefix = "PACTUM_BLOCKER: "
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		jsonPart := strings.TrimPrefix(line, prefix)
		var sentinel sentinelBlocker
		if err := json.Unmarshal([]byte(jsonPart), &sentinel); err == nil {
			return &sentinel
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// runDAGTaskAttempt
// ---------------------------------------------------------------------------

func runDAGTaskAttempt(a App, prep executionPreparation, task planTask, taskAttemptsDir string, prompt string, timeout time.Duration) (attemptID string, combinedOutput string, exitCode int, err error) {
	// ArtifactDir is run-relative (relative to the run directory in .heurema)
	// e.g. "execute/tasks/t1/attempts". The transport prepends root+workspace+runs+runID.
	artifactDirRunRel := filepath.ToSlash(filepath.Join("execute", "tasks", task.ID, "attempts"))

	// Get next attempt ID using the absolute on-disk dir (already created by caller)
	agentAttemptLifecycleMu.Lock()
	attemptID, err = nextAgentAttemptID(taskAttemptsDir, "attempt")
	if err != nil {
		agentAttemptLifecycleMu.Unlock()
		return "", "", 0, err
	}
	agentAttemptLifecycleMu.Unlock()

	// Derive the absolute attempt dir (must match what the transport creates)
	attemptDir := filepath.Join(prep.Root, ".heurema", "pactum", "runs", prep.State.RunID,
		filepath.FromSlash(artifactDirRunRel), attemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return attemptID, "", 0, err
	}

	// Write prompt to the attempt dir
	promptPath := filepath.Join(attemptDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return attemptID, "", 0, err
	}

	// Repo-relative prompt path (for the transport's PromptRepoPath)
	promptRelPath := filepath.ToSlash(filepath.Join(".heurema", "pactum", "runs", prep.State.RunID,
		"execute", "tasks", task.ID, "attempts", attemptID, "prompt.md"))

	// Build write-path-allowed for task scope
	writePathAllowed := taskWritePathAllowed(task)

	runResult, runErr := a.agentTransport().Run(agents.RunRequest{
		RepoRoot:         prep.Root,
		RunID:            prep.State.RunID,
		AttemptID:        attemptID,
		Agent:            prep.Agent,
		PromptRepoPath:   promptRelPath,
		ArtifactDir:      artifactDirRunRel,
		Timeout:          timeout,
		LiveOutput:       nil,
		WritePathAllowed: writePathAllowed,
		Model:            prep.ModelSpec,
	})
	if runErr != nil && runResult.StartedAt == "" {
		return attemptID, "", -1, runErr
	}

	// Read combined output. StdoutPath/StderrPath from the transport are run-relative
	// (e.g. "execute/tasks/t1/attempts/attempt_001/stdout.log"); to get the
	// absolute path, prepend root + workspace + runs + runID.
	runsDir := filepath.Join(prep.Root, ".heurema", "pactum", "runs", prep.State.RunID)
	var outputSB strings.Builder
	if runResult.StdoutPath != "" {
		stdoutAbs := filepath.Join(runsDir, filepath.FromSlash(runResult.StdoutPath))
		if data, err := os.ReadFile(stdoutAbs); err == nil {
			outputSB.Write(data)
		}
	}
	if runResult.StderrPath != "" {
		stderrAbs := filepath.Join(runsDir, filepath.FromSlash(runResult.StderrPath))
		if data, err := os.ReadFile(stderrAbs); err == nil {
			outputSB.Write(data)
		}
	}

	// Append usage record
	createdAt := runResult.FinishedAt
	if strings.TrimSpace(createdAt) == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	_ = appendUsageRecord(prep.Root, prep.State.RunID, attemptID, "execute-dag-"+task.ID, prep.AgentName, prep.ModelSpec.Model, prep.Agent, runResult.Usage, createdAt)

	return attemptID, outputSB.String(), runResult.ExitCode, nil
}

// taskWritePathAllowed builds the predicate for per-task scope (prefix-based, not glob).
func taskWritePathAllowed(task planTask) func(repoRelPath string) bool {
	return func(repoRelPath string) bool {
		return inEffectiveScope(task, repoRelPath)
	}
}

// ---------------------------------------------------------------------------
// Constitution gate
// ---------------------------------------------------------------------------

func runDAGConstitutionGate(a App, prep executionPreparation, timeout time.Duration) (string, error) {
	commands := nonEmptyValidationCommands(prep.Contract.Validation.Commands)
	if len(commands) == 0 {
		return "completed", nil
	}
	result, err := runTaskValidation(prep.Root, commands, timeout)
	if err != nil {
		return "gate_failed", err
	}
	if result.Passed {
		return "completed", nil
	}
	return "gate_failed", nil
}

// ---------------------------------------------------------------------------
// nextCommandsForBlockedTask
// ---------------------------------------------------------------------------

func nextCommandsForBlockedTask(runID string, task planTask, blocker *taskBlocker) []string {
	switch blocker.Reason {
	case "out_of_scope":
		cmds := []string{}
		if blocker.Proposed != nil {
			for _, p := range blocker.Proposed.PathsInScopeAdd {
				cmds = append(cmds, fmt.Sprintf("pactum contract revise --task-id %s --add-scope-path %s", task.ID, p))
			}
		}
		if len(cmds) == 0 {
			cmds = append(cmds, fmt.Sprintf("pactum contract revise %s", runID))
		}
		return cmds
	case "validation_unmet":
		return []string{fmt.Sprintf("pactum execute run %s", runID)}
	case "requires_human":
		return []string{fmt.Sprintf("pactum task show %s", runID)}
	case "baseline_green":
		return []string{fmt.Sprintf("pactum plan context %s", runID)}
	case "invalid_scope":
		return []string{fmt.Sprintf("pactum contract revise %s", runID)}
	default:
		return []string{}
	}
}

// ---------------------------------------------------------------------------
// Dirty-in-scope check
// ---------------------------------------------------------------------------

func checkDirtyInScope(root string, hashesJSONL string, tasks []planTask) error {
	expected, err := readHashRecords(hashesJSONL)
	if os.IsNotExist(err) {
		return nil // no hashes = no dirty check
	}
	if err != nil {
		return err
	}
	expectedByPath := make(map[string]string, len(expected))
	for _, record := range expected {
		expectedByPath[record.Path] = record.SHA256
	}
	// Walk the union of effective in-scope paths from all tasks
	for _, task := range tasks {
		for _, scopeEntry := range task.PathsInScope {
			absScope := filepath.Join(root, filepath.FromSlash(scopeEntry))
			walkErr := filepath.Walk(absScope, func(path string, info os.FileInfo, err error) error {
				if err != nil || info == nil || info.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return nil
				}
				relSlash := filepath.ToSlash(rel)
				if !inEffectiveScope(task, relSlash) {
					return nil
				}
				expectedHash, ok := expectedByPath[relSlash]
				if !ok {
					// New file not in project map — it's dirty
					return fmt.Errorf("file %q is new (not in project map) and in scope", relSlash)
				}
				currentHash, err := fileSHA256(filepath.Join(root, filepath.FromSlash(relSlash)))
				if err != nil {
					return nil
				}
				if currentHash != expectedHash {
					return fmt.Errorf("file %q is modified (dirty) in scope", relSlash)
				}
				return nil
			})
			if walkErr != nil {
				return walkErr
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Output
// ---------------------------------------------------------------------------

func writePlanDAGOutput(stdout io.Writer, terminalState string, entries []taskStateEntry, runID string) {
	fmt.Fprintf(stdout, "Terminal state: %s\n", terminalState)
	if terminalState == "completed" {
		return
	}
	for _, e := range entries {
		if e.Status != "blocked" && e.Status != "blocked-upstream" {
			continue
		}
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "--- BLOCKED TASK: %s ---\n", e.TaskID)
		if e.Blocker != nil {
			fmt.Fprintf(stdout, "Reason: %s\n", e.Blocker.Reason)
			if e.Blocker.Why != "" {
				fmt.Fprintf(stdout, "Why: %s\n", e.Blocker.Why)
			}
			if len(e.Blocker.NextCommands) > 0 {
				fmt.Fprintln(stdout, "Next steps:")
				for _, cmd := range e.Blocker.NextCommands {
					fmt.Fprintf(stdout, "  %s\n", cmd)
				}
			}
		} else if e.Status == "blocked-upstream" {
			fmt.Fprintln(stdout, "Reason: blocked-upstream (dependency blocked)")
		}
	}
}

// ---------------------------------------------------------------------------
// State I/O
// ---------------------------------------------------------------------------

func writeTasksState(path string, state tasksStateDocument) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeJSON(path, state)
}

func readTasksState(path string) (tasksStateDocument, error) {
	var state tasksStateDocument
	if err := readJSON(path, &state); err != nil {
		return tasksStateDocument{}, err
	}
	return state, nil
}

// ---------------------------------------------------------------------------
// Ledger event
// ---------------------------------------------------------------------------

type executionDrainedEvent struct {
	Type                 string `json:"type"`
	Timestamp            string `json:"timestamp"`
	ContractSHA256       string `json:"contract_sha256"`
	TerminalState        string `json:"terminal_state"`
	TasksDone            int    `json:"tasks_done"`
	TasksBlocked         int    `json:"tasks_blocked"`
	TasksBlockedUpstream int    `json:"tasks_blocked_upstream"`
}

func appendExecutionDrainedEvent(eventsJSONL string, contractSHA256 string, terminalState string, summary loopSummaryDocument) error {
	event := executionDrainedEvent{
		Type:                 "execution_drained",
		Timestamp:            time.Now().UTC().Format(time.RFC3339),
		ContractSHA256:       contractSHA256,
		TerminalState:        terminalState,
		TasksDone:            summary.TasksDone,
		TasksBlocked:         summary.TasksBlocked,
		TasksBlockedUpstream: summary.TasksBlockedUpstream,
	}
	return appendJSONLine(eventsJSONL, event)
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

func buildLoopSummary(contractSHA256 string, terminalState string, entries []taskStateEntry) loopSummaryDocument {
	done := 0
	blocked := 0
	blockedUpstream := 0
	totalRetries := 0
	baselineGreenCount := 0
	baselineTotal := 0
	contextPackBytes := make(map[string]int)

	for _, e := range entries {
		switch e.Status {
		case "done":
			done++
		case "blocked":
			blocked++
		case "blocked-upstream":
			blockedUpstream++
		}
		if e.Attempts > 1 {
			totalRetries += e.Attempts - 1
		}
		if e.BaselineResult != nil {
			baselineTotal++
			rec := e.BaselineResult.Recommendation
			if rec == "block" || rec == "signal" {
				baselineGreenCount++
			}
		}
		if e.ContextPackBytes > 0 {
			contextPackBytes[e.TaskID] = e.ContextPackBytes
		}
	}

	var baselineGreenRate float64
	if baselineTotal > 0 {
		baselineGreenRate = float64(baselineGreenCount) / float64(baselineTotal)
	}

	return loopSummaryDocument{
		ContractSHA256:       contractSHA256,
		TerminalState:        terminalState,
		TasksDone:            done,
		TasksBlocked:         blocked,
		TasksBlockedUpstream: blockedUpstream,
		TotalRetries:         totalRetries,
		BaselineGreenRate:    baselineGreenRate,
		ContextPackBytes:     contextPackBytes,
	}
}

// ---------------------------------------------------------------------------
// last-result.json
// ---------------------------------------------------------------------------

func writeDAGLastResult(path string, result dagLastResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeJSON(path, result)
}
