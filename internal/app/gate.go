package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/ledger"
)

const (
	gateReportSchema              = "pactum.gate_report.v1alpha1"
	validationCommandResultSchema = "pactum.validation_command_result.v1alpha1"
	gateReportArtifact            = "gate/gate-report.json"
	gateDefaultCommandTimeout     = 10 * time.Minute
)

type gateReportDocument struct {
	Schema     string               `json:"schema"`
	RunID      string               `json:"run_id"`
	CreatedAt  string               `json:"created_at"`
	Status     string               `json:"status"`
	Execution  gateExecutionReport  `json:"execution"`
	Changes    gateChangeReport     `json:"changes"`
	Scope      *gateScopeReport     `json:"scope,omitempty"`
	Validation gateValidationReport `json:"validation"`
	Summary    gateSummary          `json:"summary"`
}

type gateExecutionReport struct {
	AttemptID string `json:"attempt_id"`
	ExitCode  int    `json:"exit_code"`
	TimedOut  bool   `json:"timed_out"`
	// CompletedDespiteTimeout mirrors the attempt result: the idle watchdog
	// fired after the agent's successful terminal marker, so the execution
	// still counts as passed.
	CompletedDespiteTimeout bool   `json:"completed_despite_timeout,omitempty"`
	Result                  string `json:"result"`
}

type gateChangeReport struct {
	Status       string   `json:"status"`
	ChangedFiles []string `json:"changed_files"`
	NewFiles     []string `json:"new_files"`
	MissingFiles []string `json:"missing_files"`
	Reasons      []string `json:"reasons"`
}

type gateScopeReport struct {
	Status     string   `json:"status"`
	Undeclared []string `json:"undeclared"`
	OutOfScope []string `json:"out_of_scope"`
	Warnings   []string `json:"warnings"`
}

type gateValidationReport struct {
	CommandsAllowed bool                          `json:"commands_allowed"`
	Commands        []gateValidationCommandReport `json:"commands"`
}

type gateValidationCommandReport struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Result   string `json:"result"`
}

type gateSummary struct {
	ExecutionPassed   bool `json:"execution_passed"`
	ValidationPassed  bool `json:"validation_passed"`
	ChangesNeedReview bool `json:"changes_need_review"`
	ScopeBlocked      bool `json:"scope_blocked"`
}

type validationCommandResultDocument struct {
	Schema  string `json:"schema"`
	ID      string `json:"id"`
	Command string `json:"command"`
	processResult
}

type gateProcessError struct {
	Status string
}

func (e gateProcessError) Error() string {
	return fmt.Sprintf("gate status %s", e.Status)
}

func (a App) GateRun(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadGateContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if err := ensureGateContractApproved(context); err != nil {
		return err
	}
	manifest, err := ensureGatePromptReady(context)
	if err != nil {
		return err
	}

	attempt, resultArtifact, found, err := latestCompletedGateExecution(context, manifest.ContractSHA256)
	if err != nil {
		return err
	}
	if !found {
		hasCompleted, err := hasCompletedGateExecutionAttempt(context.RunPaths)
		if err != nil {
			return err
		}
		if !hasCompleted {
			return noExecutionAttemptError("cannot run gate: no completed execution attempts found", runID)
		}
		return noExecutionAttemptError("cannot run gate: no completed execution attempts found for current approved contract", runID)
	}

	commands := nonEmptyValidationCommands(context.Contract.Validation.Commands)
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return err
	}

	startedAt := a.nowUTC()
	if err := activeStore.MkdirAll(context.RunPaths.GateDir); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "gate_run_started", Timestamp: startedAt, RunID: runID}); err != nil {
		return err
	}

	validation := gateValidationReport{
		// Running the contract's validation commands is the gate's purpose, so
		// they always run; the field stays for report-schema compatibility.
		CommandsAllowed: true,
		Commands:        []gateValidationCommandReport{},
	}
	if len(commands) > 0 {
		if err := activeStore.MkdirAll(context.RunPaths.GateValidationDir); err != nil {
			return err
		}
		for index, command := range commands {
			id := fmt.Sprintf("command_%03d", index+1)
			result, err := a.runGateValidationCommand(context.Root, context.RunPaths, id, command, gateDefaultCommandTimeout)
			if err != nil {
				return err
			}
			validation.Commands = append(validation.Commands, result)
		}
	}
	changes := a.buildGateChangeReport(context.Root)
	scope := buildGateScopeReport(context.Contract, changes, config.OutOfScope)

	summary := gateSummary{
		ExecutionPassed:   attempt.Result.ExitCode == 0 && (!attempt.Result.TimedOut || attempt.Result.CompletedDespiteTimeout),
		ValidationPassed:  gateValidationPassed(validation.Commands),
		ChangesNeedReview: gateChangesNeedReview(changes),
		ScopeBlocked:      gateScopeBlocked(scope),
	}
	status := gateStatus(summary)
	report := gateReportDocument{
		Schema:    gateReportSchema,
		RunID:     runID,
		CreatedAt: a.nowUTC().Format(time.RFC3339),
		Status:    status,
		Execution: gateExecutionReport{
			AttemptID:               attempt.ID,
			ExitCode:                attempt.Result.ExitCode,
			TimedOut:                attempt.Result.TimedOut,
			CompletedDespiteTimeout: attempt.Result.CompletedDespiteTimeout,
			Result:                  resultArtifact,
		},
		Changes:    changes,
		Scope:      scope,
		Validation: validation,
		Summary:    summary,
	}
	if err := writeJSON(context.RunPaths.GateReportJSON, report); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "gate_run_finished", Timestamp: a.nowUTC(), RunID: runID}); err != nil {
		return err
	}

	if jsonOutput {
		// A failed gate has no safe next move — fixing and re-executing is a
		// human decision.
		next := []string{}
		if report.Status != "failed" {
			next = nextCommandsForRun(context.Paths, runID)
		}
		if err := writeJSONResponse(stdout, gateRunResponse{gateReportDocument: report, Next: next}); err != nil {
			return err
		}
	} else {
		writeGateRun(stdout, context.State, report)
	}
	if report.Status == "failed" {
		return gateProcessError{Status: report.Status}
	}
	return nil
}

// gateRunResponse is the gate report plus the next affordance; the report
// artifact on disk stays unchanged.
type gateRunResponse struct {
	gateReportDocument
	Next []string `json:"next"`
}

func (a App) GateShow(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadGateContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		suggested := fmt.Sprintf("pactum gate run %s", runID)
		return writeNotReady(stdout, jsonOutput, runID, "Gate report has not been created. Run: "+suggested, suggested)
	}

	var report gateReportDocument
	if err := readJSON(context.RunPaths.GateReportJSON, &report); err != nil {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, report)
	}
	writeGateShow(stdout, report)
	return nil
}

func (a App) loadGateContext(stdout io.Writer, runID string) (runContext, bool, error) {
	return a.loadRunContext(stdout, runID, false)
}

func ensureGateContractApproved(context runContext) error {
	_, err := verifyApprovedContract(context.RunPaths, context.Contract, context.Approval, "run gate")
	return err
}

func ensureGatePromptReady(context runContext) (promptManifest, error) {
	if !isRegularFile(context.RunPaths.PromptManifest) {
		return promptManifest{}, promptNotBuiltError("run gate", context.State.RunID)
	}
	manifest, err := readPromptManifest(context.RunPaths.PromptManifest)
	if err != nil {
		return promptManifest{}, err
	}
	if manifest.Status != "ready" {
		return promptManifest{}, promptNotBuiltError("run gate", context.State.RunID)
	}
	if context.Approval.ContractSHA256 == nil || manifest.ContractSHA256 != *context.Approval.ContractSHA256 {
		return promptManifest{}, fmt.Errorf("cannot run gate: executor prompt does not match current approved contract")
	}
	return manifest, nil
}

func latestCompletedGateExecution(context runContext, contractSHA256 string) (executionAttemptSummary, string, bool, error) {
	attemptIDs, err := listExecutionAttemptIDs(context.RunPaths.AttemptsDir)
	if err != nil {
		return executionAttemptSummary{}, "", false, err
	}
	for index := len(attemptIDs) - 1; index >= 0; index-- {
		attemptID := attemptIDs[index]
		attemptPaths := executionAttemptPaths(context.RunPaths, attemptID)
		if !isRegularFile(attemptPaths.ResultJSON) {
			continue
		}
		if !gateAttemptMatchesContract(attemptPaths.RequestJSON, contractSHA256) {
			continue
		}
		var result executionResultDocument
		if err := readJSON(attemptPaths.ResultJSON, &result); err != nil {
			return executionAttemptSummary{}, "", false, err
		}
		if result.GitGuard != nil && result.GitGuard.TerminalReason != "" {
			// The most recent matching attempt is git-guard-blocked. Do NOT fall
			// back to an older successful attempt: that would hide the blocked
			// attempt and allow a corrupted execution to proceed to the gate.
			return executionAttemptSummary{}, "", false, nil
		}
		if result.AttemptID == "" {
			result.AttemptID = attemptID
		}
		artifact := filepath.ToSlash(filepath.Join("execute", "attempts", attemptID, "result.json"))
		if gateLastResultMatches(context.RunPaths.LastResultJSON, result.AttemptID) {
			artifact = executeLastResultArtifact
		}
		return executionAttemptSummary{
			ID:     attemptID,
			Paths:  attemptPaths,
			Result: result,
		}, artifact, true, nil
	}
	return executionAttemptSummary{}, "", false, nil
}

func hasCompletedGateExecutionAttempt(runPaths contractRunPathSet) (bool, error) {
	attemptIDs, err := listExecutionAttemptIDs(runPaths.AttemptsDir)
	if err != nil {
		return false, err
	}
	for _, attemptID := range attemptIDs {
		if isRegularFile(executionAttemptPaths(runPaths, attemptID).ResultJSON) {
			return true, nil
		}
	}
	return false, nil
}

func gateAttemptMatchesContract(requestPath string, contractSHA256 string) bool {
	if !isRegularFile(requestPath) {
		return false
	}
	var request executionRequestDocument
	if err := readJSON(requestPath, &request); err != nil {
		return false
	}
	return request.ContractSHA256 == contractSHA256
}

func gateLastResultMatches(path string, attemptID string) bool {
	if !isRegularFile(path) {
		return false
	}
	var result executionResultDocument
	if err := readJSON(path, &result); err != nil {
		return false
	}
	return result.AttemptID == attemptID
}

func nonEmptyValidationCommands(commands []string) []string {
	filtered := make([]string, 0, len(commands))
	for _, command := range commands {
		if strings.TrimSpace(command) != "" {
			filtered = append(filtered, command)
		}
	}
	return filtered
}

// isUnmergedStatus reports whether the XY pair represents an unmerged entry.
// This covers any pair with U in either column and the well-known special pairs
// that git uses for unmerged states without a U: DD, AU, UD, UA, DU, AA, UU.
func isUnmergedStatus(x, y byte) bool {
	if x == 'U' || y == 'U' {
		return true
	}
	switch string([]byte{x, y}) {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}

func (a App) buildGateChangeReport(root string) gateChangeReport {
	report := gateChangeReport{
		Status:       "clean",
		ChangedFiles: []string{},
		NewFiles:     []string{},
		MissingFiles: []string{},
		Reasons:      []string{},
	}

	out, err := gitExec(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		report.Status = "changed"
		report.Reasons = []string{"git status failed: " + err.Error()}
		return report
	}
	if out == "" {
		return report
	}

	// Bucket priority constants: higher value wins when the same path appears in
	// multiple git status entries (missing > changed > new).
	const (
		bucketNew     = 1
		bucketChanged = 2
		bucketMissing = 3
	)
	buckets := make(map[string]int)
	place := func(path string, b int) {
		if buckets[path] < b {
			buckets[path] = b
		}
	}

	// classifyByXY applies the contract's ordered rules for non-rename entries:
	// (1) D → missing, (2) M/T → changed, (3) unmerged → changed,
	// (4) A/?? → new, (5) unknown → changed.
	classifyByXY := func(path string, x, y byte) {
		switch {
		case x == 'D' || y == 'D':
			place(path, bucketMissing)
		case x == 'M' || y == 'M' || x == 'T' || y == 'T':
			place(path, bucketChanged)
		case isUnmergedStatus(x, y):
			place(path, bucketChanged)
		case x == '?' && y == '?':
			place(path, bucketNew)
		case x == 'A':
			place(path, bucketNew)
		default:
			place(path, bucketChanged)
		}
	}

	// classifyDest classifies a rename/copy destination by Y only (X is R or C,
	// not a state indicator): D→missing, M/T→changed, unmerged→changed,
	// ' '→new (file newly placed at this path), unknown→changed.
	classifyDest := func(path string, x, y byte) {
		switch {
		case y == 'D':
			place(path, bucketMissing)
		case y == 'M' || y == 'T':
			place(path, bucketChanged)
		case isUnmergedStatus(x, y):
			place(path, bucketChanged)
		case y == ' ':
			place(path, bucketNew)
		default:
			place(path, bucketChanged)
		}
	}

	// Porcelain v1 with -z: each entry is "XY PATH\0"; rename/copy entries are
	// "XY DEST\0ORIG\0" (two NUL-separated tokens).
	tokens := strings.Split(out, "\x00")
	for i := 0; i < len(tokens); {
		token := tokens[i]
		i++
		if len(token) < 4 {
			continue // empty trailing token or malformed
		}
		x, y := token[0], token[1]
		path := token[3:] // skip "XY "

		// Skip git-ignored files.
		if x == '!' && y == '!' {
			continue
		}

		// Rename/copy: consume orig and classify both paths.
		if x == 'R' || x == 'C' {
			var orig string
			if i < len(tokens) {
				orig = tokens[i]
				i++
			}
			// Orig outside .heurema/ is effectively deleted from the working tree.
			if orig != "" && !strings.HasPrefix(orig, ".heurema/") {
				place(orig, bucketMissing)
			}
			// Skip dest if it is inside .heurema/.
			if strings.HasPrefix(path, ".heurema/") {
				continue
			}
			classifyDest(path, x, y)
			continue
		}

		// Skip paths under .heurema/.
		if strings.HasPrefix(path, ".heurema/") {
			continue
		}

		classifyByXY(path, x, y)
	}

	for p, b := range buckets {
		switch b {
		case bucketMissing:
			report.MissingFiles = append(report.MissingFiles, p)
		case bucketChanged:
			report.ChangedFiles = append(report.ChangedFiles, p)
		case bucketNew:
			report.NewFiles = append(report.NewFiles, p)
		}
	}

	sort.Strings(report.ChangedFiles)
	sort.Strings(report.NewFiles)
	sort.Strings(report.MissingFiles)
	if len(report.ChangedFiles)+len(report.NewFiles)+len(report.MissingFiles) > 0 {
		report.Status = "changed"
		report.Reasons = gateChangeReasons(report)
	}
	return report
}

func gateChangeReasons(report gateChangeReport) []string {
	reasons := make([]string, 0, len(report.ChangedFiles)+len(report.NewFiles)+len(report.MissingFiles))
	for _, path := range report.ChangedFiles {
		reasons = append(reasons, "changed file: "+path)
	}
	for _, path := range report.NewFiles {
		reasons = append(reasons, "new file: "+path)
	}
	for _, path := range report.MissingFiles {
		reasons = append(reasons, "missing file: "+path)
	}
	return reasons
}

func buildGateScopeReport(contract draftContract, changes gateChangeReport, enforcement string) *gateScopeReport {
	pathsInScope := nonEmptyPathGlobs(contract.PathsInScope)
	pathsOutOfScope := nonEmptyPathGlobs(contract.PathsOutOfScope)
	if len(pathsInScope) == 0 && len(pathsOutOfScope) == 0 {
		return nil
	}

	report := &gateScopeReport{
		Status:     "clean",
		Undeclared: []string{},
		OutOfScope: []string{},
		Warnings:   []string{},
	}
	for _, path := range gateScopeCandidateFiles(changes) {
		if len(pathsInScope) > 0 && !pathGlobMatchesAny(pathsInScope, path) {
			report.Undeclared = append(report.Undeclared, path)
		}
		if len(pathsOutOfScope) > 0 && pathGlobMatchesAny(pathsOutOfScope, path) {
			report.OutOfScope = append(report.OutOfScope, path)
		}
	}
	if len(report.Undeclared)+len(report.OutOfScope) > 0 {
		if enforcement == gateScopeEnforcementWarn {
			report.Status = "warnings"
		} else {
			report.Status = "blocked"
		}
		report.Warnings = gateScopeWarnings(report)
	}
	return report
}

func nonEmptyPathGlobs(patterns []string) []string {
	filtered := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		// Drop globs that normalize to nothing (e.g. "/", "./"): they match no
		// path, so keeping them would flag every changed file as undeclared.
		if pattern == "" || normalizePathGlob(pattern) == "" {
			continue
		}
		filtered = append(filtered, pattern)
	}
	return filtered
}

func gateScopeCandidateFiles(changes gateChangeReport) []string {
	seen := map[string]bool{}
	files := make([]string, 0, len(changes.ChangedFiles)+len(changes.NewFiles)+len(changes.MissingFiles))
	all := append(append(append([]string{}, changes.ChangedFiles...), changes.NewFiles...), changes.MissingFiles...)
	for _, path := range all {
		if seen[path] {
			continue
		}
		seen[path] = true
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func gateScopeWarnings(report *gateScopeReport) []string {
	warnings := make([]string, 0, len(report.Undeclared)+len(report.OutOfScope))
	for _, path := range report.Undeclared {
		warnings = append(warnings, "undeclared file: "+path)
	}
	for _, path := range report.OutOfScope {
		warnings = append(warnings, "out-of-scope file: "+path)
	}
	return warnings
}

func gateScopeBlocked(scope *gateScopeReport) bool {
	return scope != nil && scope.Status == "blocked"
}

// runShellCommandIO executes commandText as sh -c from root, directing stdout
// and stderr to the provided writers. It handles process-group management and
// timeout, returning the exit code and whether the command timed out. A non-zero
// exit code is not an error; callers inspect exitCode directly.
func runShellCommandIO(root, commandText string, stdout, stderr io.Writer, timeout time.Duration) (exitCode int, timedOut bool, _ error) {
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.Command("sh", "-c", commandText)
	setValidationCommandProcessGroup(cmd)
	cmd.Dir = root
	cmd.Env = os.Environ()
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return -1, false, fmt.Errorf("start command: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var runErr error
	select {
	case runErr = <-done:
	case <-ctx.Done():
		killValidationCommandProcessGroup(cmd)
		runErr = <-done
	}

	timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	exitCode = 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			fmt.Fprintln(stderr, runErr.Error())
		}
	}
	if timedOut && exitCode == 0 {
		exitCode = -1
	}
	return exitCode, timedOut, nil
}

func (a App) runGateValidationCommand(root string, runPaths contractRunPathSet, id string, commandText string, timeout time.Duration) (gateValidationCommandReport, error) {
	commandDir := filepath.Join(runPaths.GateValidationDir, id)
	if err := os.MkdirAll(commandDir, 0o755); err != nil {
		return gateValidationCommandReport{}, err
	}

	stdoutArtifact := filepath.ToSlash(filepath.Join("gate", "validation", id, "stdout.log"))
	stderrArtifact := filepath.ToSlash(filepath.Join("gate", "validation", id, "stderr.log"))
	resultArtifact := filepath.ToSlash(filepath.Join("gate", "validation", id, "result.json"))
	stdoutPath := filepath.Join(commandDir, "stdout.log")
	stderrPath := filepath.Join(commandDir, "stderr.log")
	resultPath := filepath.Join(commandDir, "result.json")

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return gateValidationCommandReport{}, err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return gateValidationCommandReport{}, err
	}
	defer stderrFile.Close()

	started := time.Now().UTC()
	exitCode, timedOut, err := runShellCommandIO(root, commandText, stdoutFile, stderrFile, timeout)
	if err != nil {
		return gateValidationCommandReport{}, err
	}
	finished := time.Now().UTC()

	result := validationCommandResultDocument{
		Schema:  validationCommandResultSchema,
		ID:      id,
		Command: commandText,
		processResult: processResult{
			StartedAt:      started.Format(time.RFC3339Nano),
			FinishedAt:     finished.Format(time.RFC3339Nano),
			DurationMillis: finished.Sub(started).Milliseconds(),
			ExitCode:       exitCode,
			TimedOut:       timedOut,
			Stdout:         stdoutArtifact,
			Stderr:         stderrArtifact,
		},
	}
	if err := writeJSON(resultPath, result); err != nil {
		return gateValidationCommandReport{}, err
	}
	return gateValidationCommandReport{
		ID:       id,
		Command:  commandText,
		ExitCode: exitCode,
		TimedOut: timedOut,
		Stdout:   stdoutArtifact,
		Stderr:   stderrArtifact,
		Result:   resultArtifact,
	}, nil
}

func gateValidationPassed(commands []gateValidationCommandReport) bool {
	for _, command := range commands {
		if command.ExitCode != 0 || command.TimedOut {
			return false
		}
	}
	return true
}

func gateChangesNeedReview(changes gateChangeReport) bool {
	return changes.Status == "changed" || changes.Status == "unknown"
}

func gateStatus(summary gateSummary) string {
	if !summary.ExecutionPassed || !summary.ValidationPassed || summary.ScopeBlocked {
		return "failed"
	}
	if summary.ChangesNeedReview {
		return "needs_review"
	}
	return "passed"
}

func writeGateRun(stdout io.Writer, state contractRunState, report gateReportDocument) {
	fmt.Fprintln(stdout, "Gate report created")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", report.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Execution:")
	fmt.Fprintf(stdout, "  attempt: %s\n", report.Execution.AttemptID)
	fmt.Fprintf(stdout, "  exit code: %d\n", report.Execution.ExitCode)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Changes:")
	fmt.Fprintf(stdout, "  status: %s\n", report.Changes.Status)
	fmt.Fprintf(stdout, "  changed files: %d\n", len(report.Changes.ChangedFiles))
	fmt.Fprintf(stdout, "  new files: %d\n", len(report.Changes.NewFiles))
	fmt.Fprintf(stdout, "  missing files: %d\n", len(report.Changes.MissingFiles))
	fmt.Fprintln(stdout)
	writeGateScopeSummary(stdout, report.Scope)
	fmt.Fprintln(stdout, "Validation:")
	passed, failed := gateValidationCounts(report.Validation.Commands)
	fmt.Fprintf(stdout, "  commands: %d\n", len(report.Validation.Commands))
	fmt.Fprintf(stdout, "  passed: %d\n", passed)
	fmt.Fprintf(stdout, "  failed: %d\n", failed)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Gate:")
	fmt.Fprintf(stdout, "  status: %s\n", report.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  report: %s\n", runArtifactRepoRel(report.RunID, gateReportArtifact))
}

func writeGateShow(stdout io.Writer, report gateReportDocument) {
	fmt.Fprintln(stdout, "Gate report")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", report.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Gate:")
	fmt.Fprintf(stdout, "  status: %s\n", report.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Execution:")
	fmt.Fprintf(stdout, "  exit code: %d\n", report.Execution.ExitCode)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Validation:")
	if len(report.Validation.Commands) == 0 {
		fmt.Fprintln(stdout, "  commands: 0")
	} else {
		for _, command := range report.Validation.Commands {
			fmt.Fprintf(stdout, "  %s %s\n", command.ID, command.Command)
			fmt.Fprintf(stdout, "    exit code: %d\n", command.ExitCode)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Changes:")
	writeGateShowChanges(stdout, report.Changes)
	writeGateShowScope(stdout, report.Scope)
}

func writeGateShowChanges(stdout io.Writer, changes gateChangeReport) {
	if len(changes.ChangedFiles)+len(changes.NewFiles)+len(changes.MissingFiles) == 0 {
		fmt.Fprintf(stdout, "  status: %s\n", changes.Status)
		return
	}
	for _, path := range changes.ChangedFiles {
		fmt.Fprintf(stdout, "  - changed file: %s\n", path)
	}
	for _, path := range changes.NewFiles {
		fmt.Fprintf(stdout, "  - new file: %s\n", path)
	}
	for _, path := range changes.MissingFiles {
		fmt.Fprintf(stdout, "  - missing file: %s\n", path)
	}
}

func writeGateScopeSummary(stdout io.Writer, scope *gateScopeReport) {
	if scope == nil {
		return
	}
	fmt.Fprintln(stdout, "Scope:")
	fmt.Fprintf(stdout, "  status: %s\n", scope.Status)
	fmt.Fprintf(stdout, "  undeclared: %d\n", len(scope.Undeclared))
	fmt.Fprintf(stdout, "  out of scope: %d\n", len(scope.OutOfScope))
	writeGateScopeWarnings(stdout, scope, "  ")
	fmt.Fprintln(stdout)
}

func writeGateShowScope(stdout io.Writer, scope *gateScopeReport) {
	if scope == nil {
		return
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Scope:")
	fmt.Fprintf(stdout, "  status: %s\n", scope.Status)
	if len(scope.Warnings) == 0 {
		fmt.Fprintln(stdout, "  warnings: 0")
		return
	}
	writeGateScopeWarnings(stdout, scope, "  ")
}

func writeGateScopeWarnings(stdout io.Writer, scope *gateScopeReport, indent string) {
	if len(scope.Warnings) == 0 {
		return
	}
	label := "Warnings"
	if scope.Status == "blocked" {
		label = "Violations"
	}
	fmt.Fprintf(stdout, "%s%s:\n", indent, label)
	for _, warning := range scope.Warnings {
		fmt.Fprintf(stdout, "%s  - %s\n", indent, warning)
	}
}

func gateValidationCounts(commands []gateValidationCommandReport) (int, int) {
	passed := 0
	failed := 0
	for _, command := range commands {
		if command.ExitCode == 0 && !command.TimedOut {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}
