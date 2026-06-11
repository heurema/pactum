package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
)

const (
	taskListSchema       = "pactum.task_list.v1"
	taskShowSchema       = "pactum.task.v1"
	taskUseSchema        = "pactum.task_use.v1"
	taskNewClarifySchema = "pactum.task_new_clarify.v1"
)

type taskCmd struct {
	New  taskNewCmd  `cmd:"" help:"Create a contract-first run for a task."`
	List taskListCmd `cmd:"" help:"List runs and their lifecycle status."`
	Show taskShowCmd `cmd:"" help:"Show a run's status and next step."`
	Use  taskUseCmd  `cmd:"" help:"Set the current run."`
}

type taskNewCmd struct {
	Task       string        `arg:"" name:"task" help:"Task to prepare a contract for."`
	Clarify    bool          `name:"clarify" help:"Run autonomous clarifier rounds on the new run."`
	Reviewer   string        `name:"reviewer" help:"Registry name (config agents) of the clarifier. Defaults to cross-model selection against the run executor."`
	MaxRounds  int           `name:"max-rounds" help:"Maximum clarifier rounds. Defaults to clarify.max_rounds."`
	Timeout    time.Duration `name:"timeout" default:"0" help:"Maximum idle duration without clarifier output. Defaults to timeouts.idle in the workspace config (25m when unset)."`
	JSONOutput bool          `name:"json" help:"Print machine-readable JSON output."`
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

func (c *taskNewCmd) Run(r *runner) error {
	return r.App.TaskNew(r.Stdout, r.Stderr, c.Task, taskNewOptions{
		Clarify:    c.Clarify,
		Reviewer:   c.Reviewer,
		MaxRounds:  c.MaxRounds,
		Timeout:    c.Timeout,
		JSONOutput: c.JSONOutput,
	})
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
	// Next holds the concrete runnable commands for the run's stage.
	// NextCommand predates it and is kept for compatibility.
	Next []string `json:"next"`
}

type taskUseResponse struct {
	Schema       string   `json:"schema"`
	CurrentRunID string   `json:"current_run_id"`
	Next         []string `json:"next"`
}

// taskNewResponse is the created run state plus the next affordance.
type taskNewResponse struct {
	contractRunState
	Next []string `json:"next"`
}

type taskNewOptions struct {
	Clarify    bool
	Reviewer   string
	MaxRounds  int
	Timeout    time.Duration
	JSONOutput bool
}

type taskNewClarifyResponse struct {
	Schema      string                     `json:"schema"`
	Run         contractRunState           `json:"run"`
	ClarifyLoop clarifyLoopSummaryDocument `json:"clarify_loop"`
	Next        []string                   `json:"next"`
}

// TaskNew creates a contract-first run for a task and records it as the current
// run. It replaces the old top-level `run "..." --contract-only` command. With
// --clarify it then runs the autonomous clarify loop against the new run, so
// the human is left with only the questions automation could not resolve.
func (a App) TaskNew(stdout io.Writer, liveOutput io.Writer, task string, options taskNewOptions) error {
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

	if !options.Clarify {
		if options.JSONOutput {
			return writeJSONResponse(stdout, taskNewResponse{contractRunState: state, Next: nextCommandsForRun(paths, state.RunID)})
		}
		writeTaskCreated(stdout, state)
		return nil
	}
	return a.taskNewClarify(stdout, liveOutput, paths, state, options)
}

// taskNewClarify runs the clarify loop against the just-created run and renders
// the combined output: the created run, the loop summary, and the open blocking
// questions awaiting the human. A loop failure never rolls back the run — the
// created-run section is printed before the loop starts (human mode), and the
// returned error names the run and how to re-run the loop on it.
func (a App) taskNewClarify(stdout io.Writer, liveOutput io.Writer, paths artifacts.Paths, state contractRunState, options taskNewOptions) error {
	if !options.JSONOutput {
		writeTaskCreatedRun(stdout, state)
	}
	summary, err := a.runTaskNewClarifyLoop(liveOutput, state.RunID, options)
	if err != nil {
		return clarifyLoopFailedError(state.RunID, err)
	}
	// Re-read the run state: the loop may have moved it (clarifying vs
	// contract_draft), and both output paths must report the fresh status.
	runPaths := contractRunPaths(runDirFor(paths, state.RunID))
	refreshed, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return err
	}
	if options.JSONOutput {
		return writeJSONResponse(stdout, taskNewClarifyResponse{
			Schema:      taskNewClarifySchema,
			Run:         refreshed,
			ClarifyLoop: summary,
			Next:        nextCommandsForRun(paths, state.RunID),
		})
	}
	status, err := buildClarificationStatus(runPaths, refreshed)
	if err != nil {
		return err
	}
	awaiting := openBlockingClarifyQuestions(status.Questions)

	fmt.Fprintln(stdout)
	writeClarifyLoopSummary(stdout, summary)
	fmt.Fprintln(stdout)
	writeTaskNewQuestionsAwaiting(stdout, awaiting)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	if len(awaiting) > 0 {
		fmt.Fprintln(stdout, "  Answer each question with: pactum clarify answer <question_id> \"<answer>\", then: pactum contract approve")
	} else {
		fmt.Fprintln(stdout, "  Review the contract draft, then: pactum contract approve")
	}
	return nil
}

// runTaskNewClarifyLoop reuses the clarify loop wholesale through its own
// command path (same rounds, terminals, artifacts, and ledger events as
// `pactum clarify run`) and parses its JSON summary so TaskNew can embed it.
func (a App) runTaskNewClarifyLoop(liveOutput io.Writer, runID string, options taskNewOptions) (clarifyLoopSummaryDocument, error) {
	var stdout bytes.Buffer
	if err := a.ClarifyLoop(&stdout, liveOutput, runID, clarifyLoopOptions{
		Reviewer:   options.Reviewer,
		MaxRounds:  options.MaxRounds,
		Timeout:    options.Timeout,
		JSONOutput: true,
	}); err != nil {
		return clarifyLoopSummaryDocument{}, err
	}
	var summary clarifyLoopSummaryDocument
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		return clarifyLoopSummaryDocument{}, err
	}
	return summary, nil
}

func openBlockingClarifyQuestions(questions []clarifyQuestionStatus) []clarifyQuestionStatus {
	open := []clarifyQuestionStatus{}
	for _, question := range questions {
		if question.Status == "open" && question.Blocking {
			open = append(open, question)
		}
	}
	return open
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
		return runNotFoundError(runID)
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
		Next:        nextCommandsForRun(paths, runID),
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
		return runNotFoundError(runID)
	}
	if err := writeCurrentRun(paths, runID); err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, taskUseResponse{Schema: taskUseSchema, CurrentRunID: runID, Next: nextCommandsForRun(paths, runID)})
	}
	fmt.Fprintf(stdout, "Current run set to %s\n", runID)
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
	writeTaskCreatedRun(stdout, state)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintln(stdout, "  Review the contract draft, then: pactum contract approve")
}

func writeTaskCreatedRun(stdout io.Writer, state contractRunState) {
	fmt.Fprintln(stdout, "Run created")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Status)
	fmt.Fprintf(stdout, "  task: %s\n", state.Task)
	fmt.Fprintln(stdout, "  current: yes")
}

// writeTaskNewQuestionsAwaiting renders the human's working set after the
// clarify loop: every open blocking question with the clarifier's
// recommendation, so the operator answers only what automation could not
// resolve.
func writeTaskNewQuestionsAwaiting(stdout io.Writer, questions []clarifyQuestionStatus) {
	fmt.Fprintln(stdout, "Questions awaiting you:")
	if len(questions) == 0 {
		fmt.Fprintln(stdout, "  (none — no open blocking questions remain)")
		return
	}
	for _, question := range questions {
		fmt.Fprintf(stdout, "  - %s %s\n", question.ID, question.Question)
		if question.Kind != "" {
			fmt.Fprintf(stdout, "    kind: %s\n", question.Kind)
		}
		if question.RecommendedAnswer != "" {
			confidence := question.Confidence
			if confidence == "" {
				confidence = "unknown"
			}
			fmt.Fprintf(stdout, "    recommended answer (confidence %s): %s\n", confidence, question.RecommendedAnswer)
		}
		if len(question.DependsOn) > 0 {
			fmt.Fprintf(stdout, "    depends on: %s\n", strings.Join(question.DependsOn, ", "))
		}
		if question.Blocked {
			fmt.Fprintln(stdout, "    blocked: waiting on unanswered prerequisites — answer those first")
		}
	}
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
