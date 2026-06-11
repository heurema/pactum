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

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
	"github.com/heurema/pactum/internal/projectmap"
)

const (
	gateReportSchema              = "pactum.gate_report.v1"
	validationCommandResultSchema = "pactum.validation_command_result.v1"
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
			return fmt.Errorf("cannot run gate: no completed execution attempts found")
		}
		return fmt.Errorf("cannot run gate: no completed execution attempts found for current approved contract")
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
	changes := a.buildGateChangeReport(context.Root, context.Paths)
	scope := buildGateScopeReport(context.Contract, changes, config.Gate.ScopeEnforcement)

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
		if err := writeJSONResponse(stdout, report); err != nil {
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
		return promptManifest{}, fmt.Errorf("cannot run gate: executor prompt has not been built")
	}
	manifest, err := readPromptManifest(context.RunPaths.PromptManifest)
	if err != nil {
		return promptManifest{}, err
	}
	if manifest.Status != "ready" {
		return promptManifest{}, fmt.Errorf("cannot run gate: executor prompt has not been built")
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

func (a App) buildGateChangeReport(root string, paths artifacts.Paths) gateChangeReport {
	report := gateChangeReport{
		Status:       "clean",
		ChangedFiles: []string{},
		NewFiles:     []string{},
		MissingFiles: []string{},
		Reasons:      []string{},
	}

	expected, err := readHashRecords(paths.HashesJSONL)
	if os.IsNotExist(err) {
		expected = []projectmap.HashRecord{}
	} else if err != nil {
		report.Status = "unknown"
		report.Reasons = append(report.Reasons, "cannot read project map hashes: "+err.Error())
		return report
	}
	config, err := readConfig(paths.Config)
	if err != nil {
		report.Status = "unknown"
		report.Reasons = append(report.Reasons, "cannot read project map config: "+err.Error())
		return report
	}
	current, err := projectmap.Scan(root, projectmap.ScanOptions{
		MaxFileBytes:  int64(config.Map.MaxFileBytes),
		CodeIndexMode: config.Map.CodeIndex,
	})
	if err != nil {
		report.Status = "unknown"
		report.Reasons = append(report.Reasons, "cannot scan repository: "+err.Error())
		return report
	}

	expectedByPath := make(map[string]string, len(expected))
	for _, record := range expected {
		if strings.HasPrefix(record.Path, artifacts.WorkspaceRel+"/") {
			continue
		}
		expectedByPath[record.Path] = record.SHA256
		fullPath := filepath.Join(root, filepath.FromSlash(record.Path))
		if !filesystemRegularFile(fullPath) {
			report.MissingFiles = append(report.MissingFiles, record.Path)
			continue
		}
		hash, err := fileSHA256(fullPath)
		if err != nil {
			report.Status = "unknown"
			report.Reasons = append(report.Reasons, "cannot hash file: "+record.Path+": "+err.Error())
			return report
		}
		if hash != record.SHA256 {
			report.ChangedFiles = append(report.ChangedFiles, record.Path)
		}
	}
	for _, record := range current.Hashes {
		if strings.HasPrefix(record.Path, artifacts.WorkspaceRel+"/") {
			continue
		}
		if _, ok := expectedByPath[record.Path]; !ok {
			report.NewFiles = append(report.NewFiles, record.Path)
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
	files := make([]string, 0, len(changes.ChangedFiles)+len(changes.NewFiles))
	for _, path := range append(append([]string{}, changes.ChangedFiles...), changes.NewFiles...) {
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

	fields := strings.Fields(commandText)
	if len(fields) == 0 {
		return gateValidationCommandReport{}, fmt.Errorf("validation command %s is empty", id)
	}

	started := time.Now().UTC()
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	command := exec.CommandContext(ctx, fields[0], fields[1:]...)
	command.Dir = root
	command.Env = os.Environ()
	command.Stdout = stdoutFile
	command.Stderr = stderrFile

	runErr := command.Run()
	finished := time.Now().UTC()

	exitCode := 0
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
			fmt.Fprintln(stderrFile, runErr.Error())
		}
	}
	if timedOut && exitCode == 0 {
		exitCode = -1
	}

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
