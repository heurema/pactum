package app

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
)

const (
	taskListSchema    = "pactum.task_list.v1"
	taskShowSchema    = "pactum.task.v1"
	taskUseSchema     = "pactum.task_use.v1"
	taskCurrentSchema = "pactum.task_current.v1"
)

type taskCmd struct {
	New     taskNewCmd     `cmd:"" help:"Create a contract-first run for a task."`
	List    taskListCmd    `cmd:"" help:"List runs and their lifecycle status."`
	Show    taskShowCmd    `cmd:"" help:"Show a run's status and next step."`
	Use     taskUseCmd     `cmd:"" help:"Set the current run."`
	Current taskCurrentCmd `cmd:"" help:"Show the current run."`
}

type taskNewCmd struct {
	Task       string `arg:"" name:"task" help:"Task to prepare a contract for."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type taskListCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

type taskShowCmd struct {
	RunID      string `arg:"" optional:"" name:"run_id" help:"Run id to show. Defaults to the current run."`
	Latest     bool   `name:"latest" help:"Show the most recent run."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type taskUseCmd struct {
	RunID      string `arg:"" name:"run_id" help:"Run id to make current."`
	JSONOutput bool   `name:"json" help:"Print machine-readable JSON output."`
}

type taskCurrentCmd struct {
	JSONOutput bool `name:"json" help:"Print machine-readable JSON output."`
}

func (c *taskNewCmd) Run(r *runner) error {
	return r.App.TaskNew(r.Stdout, c.Task, c.JSONOutput)
}

func (c *taskListCmd) Run(r *runner) error {
	return r.App.TaskList(r.Stdout, c.JSONOutput)
}

func (c *taskShowCmd) Run(r *runner) error {
	runID, ok, err := r.App.resolveRunArgReadOnly(r.Stdout, c.RunID, c.Latest, c.JSONOutput)
	if err != nil || !ok {
		return err
	}
	return r.App.TaskShow(r.Stdout, runID, c.JSONOutput)
}

func (c *taskUseCmd) Run(r *runner) error {
	return r.App.TaskUse(r.Stdout, c.RunID, c.JSONOutput)
}

func (c *taskCurrentCmd) Run(r *runner) error {
	return r.App.TaskCurrent(r.Stdout, c.JSONOutput)
}

type taskListItem struct {
	RunID       string `json:"run_id"`
	Task        string `json:"task"`
	Status      string `json:"status"`
	Current     bool   `json:"current"`
	NextCommand string `json:"next_command,omitempty"`
}

type taskListResponse struct {
	Schema       string         `json:"schema"`
	CurrentRunID string         `json:"current_run_id,omitempty"`
	Runs         []taskListItem `json:"runs"`
}

type taskShowResponse struct {
	Schema      string `json:"schema"`
	RunID       string `json:"run_id"`
	Task        string `json:"task"`
	Status      string `json:"status"`
	Current     bool   `json:"current"`
	NextCommand string `json:"next_command,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type taskUseResponse struct {
	Schema       string `json:"schema"`
	CurrentRunID string `json:"current_run_id"`
}

type taskCurrentResponse struct {
	Schema       string `json:"schema"`
	CurrentRunID string `json:"current_run_id,omitempty"`
	Exists       bool   `json:"exists"`
}

// TaskNew creates a contract-first run for a task and records it as the current
// run. It replaces the old top-level `run "..." --contract-only` command.
func (a App) TaskNew(stdout io.Writer, task string, jsonOutput bool) error {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		return errNotInitialized
	}
	paths := artifacts.New(root)

	state, err := a.createContractOnlyRun(root, task)
	if err != nil {
		return err
	}
	if err := writeCurrentRun(paths, state.RunID); err != nil {
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeTaskCreated(stdout, state)
	return nil
}

// TaskList lists every run with its derived lifecycle status, marking the
// current run.
func (a App) TaskList(stdout io.Writer, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}

	ids, err := listRunIDs(paths)
	if err != nil {
		return err
	}
	current, _ := readCurrentRun(paths)

	items := make([]taskListItem, 0, len(ids))
	for _, id := range ids {
		task, _ := readRunTask(paths, id)
		status := deriveRunStatus(paths, id)
		items = append(items, taskListItem{
			RunID:       id,
			Task:        task,
			Status:      status,
			Current:     id == current,
			NextCommand: nextCommandForStatus(status),
		})
	}
	response := taskListResponse{Schema: taskListSchema, CurrentRunID: current, Runs: items}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeTaskList(stdout, response)
	return nil
}

// TaskShow shows a single run's status and next step.
func (a App) TaskShow(stdout io.Writer, runID string, jsonOutput bool) error {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	_ = root
	if !runExists(paths, runID) {
		return fmt.Errorf("run not found: %s", runID)
	}
	state, err := readContractRunState(contractRunPaths(runDirFor(paths, runID)).RunJSON)
	if err != nil {
		return err
	}
	current, _ := readCurrentRun(paths)
	status := deriveRunStatus(paths, runID)
	response := taskShowResponse{
		Schema:      taskShowSchema,
		RunID:       runID,
		Task:        state.Task,
		Status:      status,
		Current:     runID == current,
		NextCommand: nextCommandForStatus(status),
		CreatedAt:   state.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   state.UpdatedAt.Format(time.RFC3339),
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeTaskShow(stdout, response)
	return nil
}

// TaskUse records runID as the current run.
func (a App) TaskUse(stdout io.Writer, runID string, jsonOutput bool) error {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		return errNotInitialized
	}
	paths := artifacts.New(root)
	if !runExists(paths, runID) {
		return fmt.Errorf("run not found: %s", runID)
	}
	if err := writeCurrentRun(paths, runID); err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, taskUseResponse{Schema: taskUseSchema, CurrentRunID: runID})
	}
	fmt.Fprintf(stdout, "Current run set to %s\n", runID)
	return nil
}

// TaskCurrent prints the current run pointer, if any.
func (a App) TaskCurrent(stdout io.Writer, jsonOutput bool) error {
	_, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return err
	}
	current, exists := readCurrentRun(paths)
	if exists && !runExists(paths, current) {
		// Stale pointer (run removed): report as no current run.
		current, exists = "", false
	}
	if jsonOutput {
		return writeJSONResponse(stdout, taskCurrentResponse{Schema: taskCurrentSchema, CurrentRunID: current, Exists: exists})
	}
	if !exists {
		fmt.Fprintln(stdout, "No current run. Set one with: pactum task use <run_id>")
		return nil
	}
	fmt.Fprintln(stdout, current)
	return nil
}

func runDirFor(paths artifacts.Paths, runID string) string {
	return filepath.Join(paths.RunsDir, runID)
}

func readRunTask(paths artifacts.Paths, runID string) (string, error) {
	state, err := readContractRunState(contractRunPaths(runDirFor(paths, runID)).RunJSON)
	if err != nil {
		return "", err
	}
	return state.Task, nil
}

func writeTaskCreated(stdout io.Writer, state contractRunState) {
	fmt.Fprintln(stdout, "Run created")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Status)
	fmt.Fprintf(stdout, "  task: %s\n", state.Task)
	fmt.Fprintln(stdout, "  current: yes")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintln(stdout, "  Review the contract draft, then: pactum contract approve")
}

func writeTaskList(stdout io.Writer, response taskListResponse) {
	fmt.Fprintln(stdout, "Runs")
	fmt.Fprintln(stdout)
	if len(response.Runs) == 0 {
		fmt.Fprintln(stdout, "  (none) — create one with: pactum task new \"<task>\"")
		return
	}
	for _, run := range response.Runs {
		marker := " "
		if run.Current {
			marker = "*"
		}
		fmt.Fprintf(stdout, "%s %s  %s  %s\n", marker, run.RunID, run.Status, run.Task)
	}
}

func writeTaskShow(stdout io.Writer, response taskShowResponse) {
	fmt.Fprintln(stdout, "Run")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  task: %s\n", response.Task)
	fmt.Fprintf(stdout, "  status: %s\n", response.Status)
	if response.Current {
		fmt.Fprintln(stdout, "  current: yes")
	}
	if response.NextCommand != "" {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Next:")
		fmt.Fprintf(stdout, "  %s\n", response.NextCommand)
	}
}
