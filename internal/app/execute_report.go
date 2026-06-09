package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	executeDryRunArtifact     = "execute/dry-run.json"
	executeLastResultArtifact = "execute/last-result.json"
	executeLogExcerptBytes    = 8 * 1024
	executeLogExcerptLines    = 80
)

type executeStatusResponse struct {
	RunID       string                `json:"run_id"`
	RunStatus   string                `json:"run_status"`
	PromptReady bool                  `json:"prompt_ready"`
	DryRun      executeArtifactStatus `json:"dry_run"`
	Attempts    executeAttemptsStatus `json:"attempts"`
	LastResult  executeLastResult     `json:"last_result"`
}

type executeArtifactStatus struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path"`
}

type executeAttemptsStatus struct {
	Count         int    `json:"count"`
	LastAttemptID string `json:"last_attempt_id"`
}

type executeLastResult struct {
	Exists    bool   `json:"exists"`
	Path      string `json:"path"`
	AttemptID string `json:"attempt_id,omitempty"`
	ExitCode  int    `json:"exit_code"`
	TimedOut  bool   `json:"timed_out"`
}

type executeShowResponse struct {
	RunID         string         `json:"run_id"`
	AttemptID     string         `json:"attempt_id"`
	Request       map[string]any `json:"request"`
	Result        map[string]any `json:"result"`
	StdoutExcerpt *string        `json:"stdout_excerpt,omitempty"`
	StderrExcerpt *string        `json:"stderr_excerpt,omitempty"`
}

type executeReportContext struct {
	RunPaths contractRunPathSet
	State    contractRunState
}

type executionAttemptSummary struct {
	ID     string
	Paths  attemptPathSet
	Result executionResultDocument
}

type logExcerpt struct {
	Text      string
	Truncated bool
}

func (a App) ExecuteStatus(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadExecuteReportContext(stdout, runID)
	if err != nil || !ok {
		return err
	}

	response, err := buildExecuteStatusResponse(context)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeExecuteStatus(stdout, response)
	return nil
}

func (a App) ExecuteShow(stdout io.Writer, runID string, attemptID string, logs bool, jsonOutput bool) error {
	context, ok, err := a.loadExecuteReportContext(stdout, runID)
	if err != nil || !ok {
		return err
	}

	attempt, found, err := loadExecutionAttempt(context, attemptID)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintf(stdout, "No execution attempts found. Run: pactum execute run %s\n", runID)
		return nil
	}

	if jsonOutput {
		response, err := buildExecuteShowResponse(attempt, logs)
		if err != nil {
			return err
		}
		return writeJSONResponse(stdout, response)
	}

	var stdoutExcerpt, stderrExcerpt *logExcerpt
	if logs {
		stdoutLog, err := readExecutionLogExcerpt(attempt.Paths.StdoutLog)
		if err != nil {
			return err
		}
		stderrLog, err := readExecutionLogExcerpt(attempt.Paths.StderrLog)
		if err != nil {
			return err
		}
		stdoutExcerpt = &stdoutLog
		stderrExcerpt = &stderrLog
	}
	writeExecuteShow(stdout, attempt, stdoutExcerpt, stderrExcerpt)
	return nil
}

func (a App) loadExecuteReportContext(stdout io.Writer, runID string) (executeReportContext, bool, error) {
	base, ok, err := a.loadRunStateContext(stdout, runID, false)
	if err != nil || !ok {
		return executeReportContext{}, false, err
	}
	return executeReportContext{
		RunPaths: base.RunPaths,
		State:    base.State,
	}, true, nil
}

func buildExecuteStatusResponse(context executeReportContext) (executeStatusResponse, error) {
	attempts, err := listExecutionAttemptIDs(context.RunPaths.AttemptsDir)
	if err != nil {
		return executeStatusResponse{}, err
	}

	response := executeStatusResponse{
		RunID:       context.State.RunID,
		RunStatus:   context.State.Status,
		PromptReady: executionPromptReady(context.RunPaths.PromptManifest),
		DryRun: executeArtifactStatus{
			Exists: isRegularFile(context.RunPaths.DryRunJSON),
			Path:   executeDryRunArtifact,
		},
		Attempts: executeAttemptsStatus{
			Count: len(attempts),
		},
		LastResult: executeLastResult{
			Exists: isRegularFile(context.RunPaths.LastResultJSON),
			Path:   executeLastResultArtifact,
		},
	}
	if len(attempts) > 0 {
		response.Attempts.LastAttemptID = attempts[len(attempts)-1]
	}
	if response.LastResult.Exists {
		var result executionResultDocument
		if err := readJSON(context.RunPaths.LastResultJSON, &result); err != nil {
			return executeStatusResponse{}, err
		}
		response.LastResult.AttemptID = result.AttemptID
		response.LastResult.ExitCode = result.ExitCode
		response.LastResult.TimedOut = result.TimedOut
	} else if response.Attempts.LastAttemptID != "" {
		resultPath := executionAttemptPaths(context.RunPaths, response.Attempts.LastAttemptID).ResultJSON
		if isRegularFile(resultPath) {
			var result executionResultDocument
			if err := readJSON(resultPath, &result); err != nil {
				return executeStatusResponse{}, err
			}
			response.LastResult.Exists = true
			response.LastResult.Path = filepath.ToSlash(filepath.Join("execute", "attempts", response.Attempts.LastAttemptID, "result.json"))
			response.LastResult.AttemptID = result.AttemptID
			response.LastResult.ExitCode = result.ExitCode
			response.LastResult.TimedOut = result.TimedOut
		}
	}
	return response, nil
}

func executionPromptReady(path string) bool {
	if !isRegularFile(path) {
		return false
	}
	manifest, err := readPromptManifest(path)
	return err == nil && manifest.Status == "ready"
}

func listExecutionAttemptIDs(attemptsDir string) ([]string, error) {
	entries, err := activeStore.ReadDir(attemptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type attemptEntry struct {
		id     string
		number int
	}
	attempts := make([]attemptEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var number int
		if _, err := fmt.Sscanf(entry.Name(), "attempt_%03d", &number); err == nil {
			attempts = append(attempts, attemptEntry{id: entry.Name(), number: number})
		}
	}
	sort.Slice(attempts, func(i, j int) bool {
		return attempts[i].number < attempts[j].number
	})
	ids := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		ids = append(ids, attempt.id)
	}
	return ids, nil
}

func loadExecutionAttempt(context executeReportContext, attemptID string) (executionAttemptSummary, bool, error) {
	attemptID = strings.TrimSpace(attemptID)
	var result executionResultDocument
	resultLoaded := false
	if attemptID == "" {
		if isRegularFile(context.RunPaths.LastResultJSON) {
			if err := readJSON(context.RunPaths.LastResultJSON, &result); err != nil {
				return executionAttemptSummary{}, false, err
			}
			resultLoaded = true
			if result.AttemptID != "" {
				attemptID = result.AttemptID
			}
		}
		if attemptID == "" {
			attempts, err := listExecutionAttemptIDs(context.RunPaths.AttemptsDir)
			if err != nil {
				return executionAttemptSummary{}, false, err
			}
			if len(attempts) == 0 {
				return executionAttemptSummary{}, false, nil
			}
			attemptID = attempts[len(attempts)-1]
		}
	}

	attemptPaths := executionAttemptPaths(context.RunPaths, attemptID)
	dirExists, err := storeDirExists(attemptPaths.Dir)
	if err != nil {
		return executionAttemptSummary{}, false, err
	}
	if !dirExists {
		return executionAttemptSummary{}, false, fmt.Errorf("execution attempt not found: %s", attemptID)
	}
	if !isRegularFile(attemptPaths.ResultJSON) {
		return executionAttemptSummary{}, false, fmt.Errorf("execution attempt not found: %s", attemptID)
	}
	if !resultLoaded {
		if err := readJSON(attemptPaths.ResultJSON, &result); err != nil {
			return executionAttemptSummary{}, false, err
		}
	}
	if result.AttemptID == "" {
		result.AttemptID = attemptID
	}
	return executionAttemptSummary{
		ID:     attemptID,
		Paths:  attemptPaths,
		Result: result,
	}, true, nil
}

func buildExecuteShowResponse(attempt executionAttemptSummary, logs bool) (executeShowResponse, error) {
	request, err := readJSONMap(attempt.Paths.RequestJSON)
	if err != nil {
		return executeShowResponse{}, err
	}
	result, err := readJSONMap(attempt.Paths.ResultJSON)
	if err != nil {
		return executeShowResponse{}, err
	}
	response := executeShowResponse{
		RunID:     attempt.Result.RunID,
		AttemptID: attempt.ID,
		Request:   request,
		Result:    result,
	}
	if logs {
		stdoutLog, err := readExecutionLogExcerpt(attempt.Paths.StdoutLog)
		if err != nil {
			return executeShowResponse{}, err
		}
		stderrLog, err := readExecutionLogExcerpt(attempt.Paths.StderrLog)
		if err != nil {
			return executeShowResponse{}, err
		}
		response.StdoutExcerpt = &stdoutLog.Text
		response.StderrExcerpt = &stderrLog.Text
	}
	return response, nil
}

func readJSONMap(path string) (map[string]any, error) {
	data, err := activeStore.ReadBytes(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func readExecutionLogExcerpt(path string) (logExcerpt, error) {
	data, err := activeStore.ReadBytes(path)
	if err != nil {
		if os.IsNotExist(err) {
			return logExcerpt{}, nil
		}
		return logExcerpt{}, err
	}
	truncated := false
	if len(data) > executeLogExcerptBytes {
		data = data[len(data)-executeLogExcerptBytes:]
		truncated = true
		if index := strings.IndexByte(string(data), '\n'); index >= 0 && index+1 < len(data) {
			data = data[index+1:]
		}
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return logExcerpt{Truncated: truncated}, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > executeLogExcerptLines {
		lines = lines[len(lines)-executeLogExcerptLines:]
		truncated = true
	}
	return logExcerpt{
		Text:      strings.Join(lines, "\n"),
		Truncated: truncated,
	}, nil
}

func writeExecuteStatus(stdout io.Writer, response executeStatusResponse) {
	fmt.Fprintln(stdout, "Execution status")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Prompt:")
	fmt.Fprintf(stdout, "  ready: %s\n", yesNo(response.PromptReady))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Dry-run:")
	fmt.Fprintf(stdout, "  exists: %s\n", yesNo(response.DryRun.Exists))
	fmt.Fprintf(stdout, "  path: %s\n", runArtifactRepoRel(response.RunID, response.DryRun.Path))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempts:")
	fmt.Fprintf(stdout, "  count: %d\n", response.Attempts.Count)
	if response.Attempts.LastAttemptID != "" {
		fmt.Fprintf(stdout, "  last: %s\n", response.Attempts.LastAttemptID)
	}
	if response.LastResult.Exists {
		if response.LastResult.AttemptID == "" || response.LastResult.AttemptID == response.Attempts.LastAttemptID {
			fmt.Fprintf(stdout, "  last exit code: %d\n", response.LastResult.ExitCode)
			fmt.Fprintf(stdout, "  last timed out: %t\n", response.LastResult.TimedOut)
		} else {
			fmt.Fprintf(stdout, "  last completed: %s\n", response.LastResult.AttemptID)
			fmt.Fprintf(stdout, "  last completed exit code: %d\n", response.LastResult.ExitCode)
			fmt.Fprintf(stdout, "  last completed timed out: %t\n", response.LastResult.TimedOut)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(response.RunID, response.LastResult.Path))
}

func writeExecuteShow(stdout io.Writer, attempt executionAttemptSummary, stdoutLog *logExcerpt, stderrLog *logExcerpt) {
	fmt.Fprintln(stdout, "Execution attempt")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", attempt.Result.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", attempt.ID)
	fmt.Fprintf(stdout, "  exit code: %d\n", attempt.Result.ExitCode)
	fmt.Fprintf(stdout, "  timed out: %t\n", attempt.Result.TimedOut)
	fmt.Fprintf(stdout, "  started: %s\n", attempt.Result.StartedAt)
	fmt.Fprintf(stdout, "  finished: %s\n", attempt.Result.FinishedAt)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  request: %s\n", runArtifactRepoRel(attempt.Result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", attempt.ID, "request.json"))))
	fmt.Fprintf(stdout, "  stdout: %s\n", runArtifactRepoRel(attempt.Result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", attempt.ID, "stdout.log"))))
	fmt.Fprintf(stdout, "  stderr: %s\n", runArtifactRepoRel(attempt.Result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", attempt.ID, "stderr.log"))))
	fmt.Fprintf(stdout, "  result: %s\n", runArtifactRepoRel(attempt.Result.RunID, filepath.ToSlash(filepath.Join("execute", "attempts", attempt.ID, "result.json"))))
	if stdoutLog != nil || stderrLog != nil {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Logs:")
		writeLogBlock(stdout, "stdout", stdoutLog)
		writeLogBlock(stdout, "stderr", stderrLog)
	}
}

func writeLogBlock(stdout io.Writer, name string, excerpt *logExcerpt) {
	if excerpt == nil {
		return
	}
	fmt.Fprintf(stdout, "  %s:\n", name)
	if excerpt.Text == "" {
		fmt.Fprintln(stdout, "    (empty)")
	} else {
		for _, line := range strings.Split(excerpt.Text, "\n") {
			fmt.Fprintf(stdout, "    %s\n", line)
		}
	}
	if excerpt.Truncated {
		fmt.Fprintf(stdout, "    [%s truncated to last %d lines or %d bytes]\n", name, executeLogExcerptLines, executeLogExcerptBytes)
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
