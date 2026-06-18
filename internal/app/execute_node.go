package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	searchpkg "github.com/heurema/pactum/internal/search"
)

// contextPackSliceCap and contextPackTotalCap bound the size of a context pack.
const (
	contextPackSliceCap = 64 * 1024  // 64 KB per selector slice
	contextPackTotalCap = 512 * 1024 // 512 KB total pack
)

// contextPackResult is the outcome of building a context pack for one plan task.
type contextPackResult struct {
	Path   string // repo-relative path: execute/context/<taskID>.md
	SHA256 string
}

// buildContextPack resolves a task's context[] selectors into a bounded markdown
// evidence pack, writes it to execute/context/<taskID>.md using os.WriteFile (not
// through the ACP write path), and returns its repo-relative path and SHA-256.
// File content for selectors is read with os.Lstat/os.ReadFile from repo root.
func buildContextPack(root string, task planTask, contract draftContract, runPaths contractRunPathSet) (contextPackResult, error) {
	paths := artifacts.New(root)
	dbPath := paths.SearchSQLite

	contextDir := filepath.Join(runPaths.ExecuteDir, "context")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return contextPackResult{}, err
	}

	var buf bytes.Buffer
	assembleContextPack(root, dbPath, &buf, task, contract)

	data := buf.Bytes()
	if len(data) > contextPackTotalCap {
		data = truncatePackBytes(data)
	}

	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])

	// Use only the base component to prevent task IDs with path separators or
	// ".." from writing outside execute/context/.
	safeID := filepath.Base(filepath.Clean(task.ID))
	if safeID == "." || safeID == ".." {
		safeID = "task"
	}
	packPath := filepath.Join(contextDir, safeID+".md")
	if err := os.WriteFile(packPath, data, 0o644); err != nil {
		return contextPackResult{}, err
	}

	rel, err := filepath.Rel(root, packPath)
	if err != nil {
		rel = packPath
	}
	return contextPackResult{Path: filepath.ToSlash(rel), SHA256: shaHex}, nil
}

// assembleContextPack writes the full pack body into buf.
func assembleContextPack(root, dbPath string, buf *bytes.Buffer, task planTask, contract draftContract) {
	if task.Title != "" {
		fmt.Fprintf(buf, "# Context pack: %s — %s\n\n", task.ID, task.Title)
	} else {
		fmt.Fprintf(buf, "# Context pack: %s\n\n", task.ID)
	}

	// Constitution constraints
	fmt.Fprintf(buf, "## Goal\n\n%s\n\n", contract.Goal)
	if in := contract.Scope.In; len(in) > 0 {
		fmt.Fprintf(buf, "**Scope (in):** %s\n\n", strings.Join(in, "; "))
	}
	if out := contract.Scope.Out; len(out) > 0 {
		fmt.Fprintf(buf, "**Scope (out):** %s\n\n", strings.Join(out, "; "))
	}
	if pis := contract.PathsInScope; len(pis) > 0 {
		fmt.Fprintf(buf, "**Paths in scope:** %s\n\n", strings.Join(pis, ", "))
	}

	// Evidence selectors — stop early if the buffer is already at the cap.
	if len(task.Context) > 0 {
		fmt.Fprintf(buf, "## Evidence\n\n")
		for i, sel := range task.Context {
			writeContextSelector(root, dbPath, buf, i+1, sel)
			if buf.Len() >= contextPackTotalCap {
				fmt.Fprintf(buf, "\n> [evidence truncated — use `pactum search` to access remaining selectors]\n\n")
				break
			}
		}
	}

	// Task acceptance
	if len(task.Acceptance) > 0 {
		fmt.Fprintf(buf, "## Acceptance\n\n")
		for _, ac := range task.Acceptance {
			fmt.Fprintf(buf, "- %s\n", ac)
		}
		fmt.Fprintln(buf)
	}

	// Frozen validation commands
	if len(task.Validation) > 0 {
		fmt.Fprintf(buf, "## Validation\n\n```\n")
		for _, cmd := range task.Validation {
			fmt.Fprintf(buf, "%s\n", cmd)
		}
		fmt.Fprintf(buf, "```\n\n")
	}
}

func writeContextSelector(root, dbPath string, buf *bytes.Buffer, n int, sel planContextSelector) {
	if sel.Why != "" {
		fmt.Fprintf(buf, "### Selector %d — %s\n\n", n, sel.Why)
	} else {
		fmt.Fprintf(buf, "### Selector %d\n\n", n)
	}
	switch {
	case sel.Symbol != "":
		writeSymbolSelector(root, dbPath, buf, sel)
	case sel.Path != "":
		writePathSelector(root, buf, sel)
	default:
		fmt.Fprintf(buf, "> Note: empty selector (no path or symbol). Use `pactum search` to locate content.\n\n")
	}
}

func writePathSelector(root string, buf *bytes.Buffer, sel planContextSelector) {
	absPath := filepath.Join(root, sel.Path)
	// Reject paths that escape the workspace root after cleaning (e.g. "../outside").
	cleanRoot := filepath.Clean(root)
	if !strings.HasPrefix(filepath.Clean(absPath)+string(filepath.Separator), cleanRoot+string(filepath.Separator)) {
		fmt.Fprintf(buf, "> Note: path `%s` escapes the workspace and cannot be read. Use `pactum search %q` to locate it.\n\n", sel.Path, filepath.Base(sel.Path))
		return
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(buf, "> Note: file not found: `%s`. Use `pactum search %q` to locate it.\n\n", sel.Path, filepath.Base(sel.Path))
		} else {
			fmt.Fprintf(buf, "> Note: cannot stat `%s`: %s. Use `pactum search %q` to locate it.\n\n", sel.Path, err, filepath.Base(sel.Path))
		}
		return
	}
	if !info.Mode().IsRegular() {
		fmt.Fprintf(buf, "> Note: `%s` is not a regular file. Use `pactum search %q` to locate it.\n\n", sel.Path, filepath.Base(sel.Path))
		return
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Fprintf(buf, "> Note: cannot read `%s`: %s. Use `pactum search %q` to locate it.\n\n", sel.Path, err, filepath.Base(sel.Path))
		return
	}
	if isBinaryContent(content) {
		fmt.Fprintf(buf, "> Note: `%s` appears to be binary. Use `pactum search %q` to locate it.\n\n", sel.Path, filepath.Base(sel.Path))
		return
	}

	lineText := string(content)
	if sel.Lines != "" {
		start, end, err := parseLineRange(sel.Lines)
		lines := strings.Split(lineText, "\n")
		if err != nil || start < 1 || end < start || end > len(lines) {
			if err == nil {
				err = fmt.Errorf("range %s out of bounds (file has %d lines)", sel.Lines, len(lines))
			}
			fmt.Fprintf(buf, "> Note: invalid line range for `%s`: %s. Use `pactum search %q` to locate it.\n\n", sel.Path, err, filepath.Base(sel.Path))
			return
		}
		tag := fmt.Sprintf("%s:%d-%d", sel.Path, start, end)
		writeSlice(buf, tag, strings.Join(lines[start-1:end], "\n"))
	} else {
		writeSlice(buf, sel.Path, lineText)
	}
}

func writeSymbolSelector(root, dbPath string, buf *bytes.Buffer, sel planContextSelector) {
	if dbPath == "" {
		fmt.Fprintf(buf, "> Note: no search index available to resolve symbol `%s`. Use `pactum search --symbol %q`.\n\n", sel.Symbol, sel.Symbol)
		return
	}
	results, err := searchpkg.Query(dbPath, searchpkg.QueryOptions{Symbol: sel.Symbol, Limit: 10})
	if err != nil {
		if searchpkg.IsMissingIndex(err) || searchpkg.IsStaleIndex(err) {
			fmt.Fprintf(buf, "> Note: search index unavailable (run `pactum map refresh`). Cannot resolve symbol `%s`. Use `pactum search --symbol %q`.\n\n", sel.Symbol, sel.Symbol)
		} else {
			fmt.Fprintf(buf, "> Note: symbol lookup failed for `%s`: %s. Use `pactum search --symbol %q`.\n\n", sel.Symbol, err, sel.Symbol)
		}
		return
	}
	hits := results.Results
	if len(hits) == 0 {
		fmt.Fprintf(buf, "> Note: symbol `%s` not found in the code index. Use `pactum search --symbol %q` to locate it.\n\n", sel.Symbol, sel.Symbol)
		return
	}
	if len(hits) > 1 {
		fmt.Fprintf(buf, "> Note: symbol `%s` matched %d definitions (all shown, ordered by path/range). Use `pactum search --symbol %q` for the full list.\n\n", sel.Symbol, len(hits), sel.Symbol)
	}
	for _, hit := range hits {
		if !hit.HasRange() {
			fmt.Fprintf(buf, "> Note: symbol `%s` in `%s` has no line range. Use `pactum search --symbol %q`.\n\n", sel.Symbol, hit.Path, sel.Symbol)
			continue
		}
		tag := hit.Address()
		absPath := filepath.Join(root, hit.Path)
		content, err := os.ReadFile(absPath)
		if err != nil {
			fmt.Fprintf(buf, "> Note: cannot read `%s` for symbol `%s`: %s. Use `pactum search --symbol %q`.\n\n", hit.Path, sel.Symbol, err, sel.Symbol)
			continue
		}
		lines := strings.Split(string(content), "\n")
		s, e := hit.StartLine, hit.EndLine
		if s < 1 || e < s || e > len(lines) {
			fmt.Fprintf(buf, "> Note: symbol `%s` range %d-%d out of bounds for `%s`. Use `pactum search --symbol %q`.\n\n", sel.Symbol, s, e, hit.Path, sel.Symbol)
			continue
		}
		writeSlice(buf, tag, strings.Join(lines[s-1:e], "\n"))
	}
}

// writeSlice writes a fenced code block tagged with addr. When the slice exceeds
// contextPackSliceCap the content is trimmed and an explicit truncation marker is
// appended so the cut is never silent.
func writeSlice(buf *bytes.Buffer, addr, content string) {
	if len(content) > contextPackSliceCap {
		trimmed := content[:contextPackSliceCap]
		if idx := strings.LastIndex(trimmed, "\n"); idx > 0 {
			trimmed = trimmed[:idx]
		}
		fmt.Fprintf(buf, "```%s\n%s\n```\n\n", addr, trimmed)
		fmt.Fprintf(buf, "> [slice truncated — use `pactum search` for full content]\n\n")
		return
	}
	fmt.Fprintf(buf, "```%s\n%s\n```\n\n", addr, content)
}

// truncatePackBytes caps data at contextPackTotalCap and appends an explicit note.
func truncatePackBytes(data []byte) []byte {
	cut := data[:contextPackTotalCap]
	if idx := bytes.LastIndex(cut, []byte("\n")); idx > 0 {
		cut = cut[:idx+1]
	}
	return append(cut, []byte("\n> [pack truncated — use `pactum search` to access remaining content]\n")...)
}

func parseLineRange(s string) (start, end int, err error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("line range %q: expected start-end", s)
	}
	start, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("line range %q: %w", s, err)
	}
	end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("line range %q: %w", s, err)
	}
	return start, end, nil
}

// isBinaryContent reports whether data contains a null byte in its first 512 bytes.
func isBinaryContent(data []byte) bool {
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	return bytes.IndexByte(check, 0) >= 0
}

// ---------------------------------------------------------------------------
// Per-task frozen-validation runner
// ---------------------------------------------------------------------------

// taskValidationCommandResult is the captured outcome of one frozen validation
// command.
type taskValidationCommandResult struct {
	Command  string
	ExitCode int
	TimedOut bool
	Stdout   string
	Stderr   string
}

// taskValidationResult summarizes a task's full validation run.
type taskValidationResult struct {
	Passed   bool
	Commands []taskValidationCommandResult
}

// runTaskValidation runs the task's frozen validation commands using the gate's
// mechanism (sh -c from root, process-group kill on timeout) and reports pass
// when every command exits 0 without timeout.
func runTaskValidation(root string, commands []string, timeout time.Duration) (taskValidationResult, error) {
	var results []taskValidationCommandResult
	for _, cmd := range commands {
		r, err := runValidationCmd(root, cmd, timeout)
		if err != nil {
			return taskValidationResult{}, err
		}
		results = append(results, r)
	}
	passed := true
	for _, r := range results {
		if r.ExitCode != 0 || r.TimedOut {
			passed = false
			break
		}
	}
	return taskValidationResult{Passed: passed, Commands: results}, nil
}

// runValidationCmd executes one validation command using the shared shell runner
// (sh -c, cwd=root, process-group kill on timeout). Stdout and stderr are
// captured to memory buffers.
func runValidationCmd(root, commandText string, timeout time.Duration) (taskValidationCommandResult, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	exitCode, timedOut, err := runShellCommandIO(root, commandText, &stdoutBuf, &stderrBuf, timeout)
	if err != nil {
		return taskValidationCommandResult{}, err
	}
	return taskValidationCommandResult{
		Command:  commandText,
		ExitCode: exitCode,
		TimedOut: timedOut,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	}, nil
}

// ---------------------------------------------------------------------------
// Baseline-red checker
// ---------------------------------------------------------------------------

// baselineCommandResult classifies one validation command run against the
// unchanged working tree.
type baselineCommandResult struct {
	Command        string
	ExitCode       int
	TimedOut       bool
	Stdout         string
	Stderr         string
	Shape          string // "test" | "other"
	Status         string // "red" (non-zero) | "green" (exit 0 without timeout)
	Recommendation string // "proceed" | "block" | "signal"
}

// baselineCheckResult is the per-task baseline classification.
type baselineCheckResult struct {
	Commands       []baselineCommandResult
	Recommendation string // conservative aggregation: "proceed" | "block" | "signal"
}

// runBaselineCheck runs the task's validation commands against the unchanged
// working tree and classifies each result. It delegates to runTaskValidation so
// each command is executed exactly once. Classification is pure: no task state
// is mutated and no recommendation is acted upon (that is slice 4b).
func runBaselineCheck(root string, commands []string, timeout time.Duration) (baselineCheckResult, error) {
	v, err := runTaskValidation(root, commands, timeout)
	if err != nil {
		return baselineCheckResult{}, err
	}
	var cmdResults []baselineCommandResult
	for _, r := range v.Commands {
		cmdResults = append(cmdResults, classifyBaselineCommand(r))
	}
	return baselineCheckResult{
		Commands:       cmdResults,
		Recommendation: aggregateBaselineRecommendation(cmdResults),
	}, nil
}

func classifyBaselineCommand(r taskValidationCommandResult) baselineCommandResult {
	shape := "other"
	if isTestShaped(r.Command) {
		shape = "test"
	}
	status := "red"
	if r.ExitCode == 0 && !r.TimedOut {
		status = "green"
	}
	return baselineCommandResult{
		Command:        r.Command,
		ExitCode:       r.ExitCode,
		TimedOut:       r.TimedOut,
		Stdout:         r.Stdout,
		Stderr:         r.Stderr,
		Shape:          shape,
		Status:         status,
		Recommendation: baselineCommandRecommendation(shape, status),
	}
}

func baselineCommandRecommendation(shape, status string) string {
	if shape == "test" && status == "green" {
		// Already-green test validation — hard block candidate. This also catches
		// the `go test -run NoSuchTest` exit-0 trap where no tests matched.
		return "block"
	}
	if shape == "other" && status == "green" {
		return "signal"
	}
	return "proceed"
}

// aggregateBaselineRecommendation applies the conservative rule across all
// command results: block wins when any test-shaped command is already green;
// otherwise signal if any non-test command is green; otherwise proceed.
func aggregateBaselineRecommendation(cmds []baselineCommandResult) string {
	for _, c := range cmds {
		if c.Recommendation == "block" {
			return "block"
		}
	}
	for _, c := range cmds {
		if c.Recommendation == "signal" {
			return "signal"
		}
	}
	return "proceed"
}

// isTestShaped reports whether commandText contains a test-runner invocation
// (as opposed to a build, lint, or other non-test command).
func isTestShaped(commandText string) bool {
	for _, marker := range testShapedMarkers {
		if strings.Contains(commandText, marker) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// pactum plan context
// ---------------------------------------------------------------------------

// planContextTaskResult is the per-task outcome of pactum plan context.
type planContextTaskResult struct {
	TaskID          string               `json:"task_id"`
	ContextPackPath string               `json:"context_pack"`
	ContextPackSHA  string               `json:"context_pack_sha256"`
	Baseline        baselineCheckResult  `json:"baseline"`
	ValidationNow   taskValidationResult `json:"validation_now"`
}

// planContextResponse is the JSON output of pactum plan context.
type planContextResponse struct {
	RunID string                  `json:"run_id"`
	Tasks []planContextTaskResult `json:"tasks"`
}

// PlanContext builds per-task context packs and runs baseline validation checks
// for every task in the contract's plan. Results are written to
// execute/context/<taskID>.md and printed as a summary.
func (a App) PlanContext(stdout io.Writer, runID string, jsonOutput bool) error {
	root, _, ok, err := a.requireWorkspace(stdout, false)
	if err != nil || !ok {
		return err
	}

	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, runID)
	runDirExists, err := storeDirExists(runDir)
	if err != nil {
		return err
	}
	if !runDirExists {
		return runNotFoundError(runID)
	}
	runPaths := contractRunPaths(runDir)
	contract, err := readDraftContract(runPaths.ContractJSON)
	if err != nil {
		return err
	}
	if contract.Plan == nil || len(contract.Plan.Tasks) == 0 {
		if jsonOutput {
			return writeJSONResponse(stdout, planContextResponse{RunID: runID, Tasks: []planContextTaskResult{}})
		}
		fmt.Fprintf(stdout, "Run %s has no plan tasks.\n", runID)
		return nil
	}

	var taskResults []planContextTaskResult
	for _, task := range contract.Plan.Tasks {
		pack, err := buildContextPack(root, task, contract, runPaths)
		if err != nil {
			return fmt.Errorf("context pack for task %s: %w", task.ID, err)
		}
		// Run validation once; derive both the baseline classification and the
		// pass/fail summary from the same execution to avoid running commands twice.
		baseline, err := runBaselineCheck(root, task.Validation, gateDefaultCommandTimeout)
		if err != nil {
			return fmt.Errorf("baseline check for task %s: %w", task.ID, err)
		}
		taskResults = append(taskResults, planContextTaskResult{
			TaskID:          task.ID,
			ContextPackPath: pack.Path,
			ContextPackSHA:  pack.SHA256,
			Baseline:        baseline,
			ValidationNow:   taskValidationResultFromBaseline(baseline),
		})
	}

	if jsonOutput {
		return writeJSONResponse(stdout, planContextResponse{RunID: runID, Tasks: taskResults})
	}
	writePlanContext(stdout, runID, taskResults)
	return nil
}

func writePlanContext(stdout io.Writer, runID string, tasks []planContextTaskResult) {
	fmt.Fprintf(stdout, "Plan context prepared\n\n")
	fmt.Fprintf(stdout, "Run: %s\n\n", runID)
	for _, t := range tasks {
		fmt.Fprintf(stdout, "Task: %s\n", t.TaskID)
		fmt.Fprintf(stdout, "  context pack: %s\n", t.ContextPackPath)
		fmt.Fprintf(stdout, "  baseline:     %s\n", t.Baseline.Recommendation)
		fmt.Fprintf(stdout, "  validation:   %s\n", boolStatus(t.ValidationNow.Passed))
		fmt.Fprintln(stdout)
	}
}

func boolStatus(b bool) string {
	if b {
		return "pass"
	}
	return "fail"
}

// taskValidationResultFromBaseline derives a taskValidationResult from a
// baselineCheckResult so the same execution can satisfy both needs.
func taskValidationResultFromBaseline(b baselineCheckResult) taskValidationResult {
	passed := true
	cmds := make([]taskValidationCommandResult, 0, len(b.Commands))
	for _, c := range b.Commands {
		if c.ExitCode != 0 || c.TimedOut {
			passed = false
		}
		cmds = append(cmds, taskValidationCommandResult{
			Command:  c.Command,
			ExitCode: c.ExitCode,
			TimedOut: c.TimedOut,
			Stdout:   c.Stdout,
			Stderr:   c.Stderr,
		})
	}
	return taskValidationResult{Passed: passed, Commands: cmds}
}

// testShapedMarkers are substrings that identify test-runner invocations. The
// list covers the most common ecosystems; build/lint commands (go build, make,
// golangci-lint, etc.) are absent.
var testShapedMarkers = []string{
	"go test",
	"npm test",
	"npm run test",
	"yarn test",
	"yarn run test",
	"pytest",
	"python -m pytest",
	"cargo test",
	"ruby -Itest",
	"rake test",
	"mvn test",
	"gradle test",
	"dotnet test",
	"jest ",
	"vitest ",
	"mocha ",
}
