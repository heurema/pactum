package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/codeindex"
	searchpkg "github.com/heurema/pactum/internal/search"
)

// nodeTestTimeout is a short timeout for validation commands used in unit tests.
const nodeTestTimeout = 30 * time.Second

// ---------------------------------------------------------------------------
// Context-pack builder tests
// ---------------------------------------------------------------------------

func TestContextPackPathLines(t *testing.T) {
	root := t.TempDir()
	fileContent := "line1\nline2\nline3\nline4\nline5\n"
	mustWriteFile(t, filepath.Join(root, "foo.go"), fileContent)
	runPaths := makeNodeTestRunPaths(t, root)

	task := planTask{
		ID:    "task_001",
		Title: "example",
		Context: []planContextSelector{
			{Path: "foo.go", Lines: "2-4", Why: "key lines"},
		},
		Acceptance: []string{"it works"},
		Validation: []string{"go build ./..."},
	}
	contract := nodeTestContract("do the thing")

	result, err := buildContextPack(root, task, contract, runPaths)
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}

	data := mustReadFile(t, filepath.Join(root, filepath.FromSlash(result.Path)))
	if !strings.Contains(data, "foo.go:2-4") {
		t.Errorf("pack missing path:range tag:\n%s", data)
	}
	if !strings.Contains(data, "line2") || !strings.Contains(data, "line3") || !strings.Contains(data, "line4") {
		t.Errorf("pack missing expected lines:\n%s", data)
	}
	// line1 and line5 are outside the range
	if strings.Contains(data, "line1\n") || strings.Contains(data, "line5\n") {
		t.Errorf("pack contains out-of-range lines:\n%s", data)
	}
	if !strings.Contains(data, "key lines") {
		t.Errorf("pack missing selector why:\n%s", data)
	}
	if !strings.Contains(data, "it works") {
		t.Errorf("pack missing acceptance:\n%s", data)
	}
	if !strings.Contains(data, "go build ./...") {
		t.Errorf("pack missing validation command:\n%s", data)
	}
	if !strings.Contains(data, "do the thing") {
		t.Errorf("pack missing contract goal:\n%s", data)
	}
	if result.SHA256 == "" {
		t.Error("SHA256 must not be empty")
	}
}

func TestContextPackDeterministic(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha.go"), "package alpha\n\nfunc Foo() {}\n")
	runPaths := makeNodeTestRunPaths(t, root)

	task := planTask{
		ID:         "task_det",
		Context:    []planContextSelector{{Path: "alpha.go", Why: "check determinism"}},
		Acceptance: []string{"same bytes"},
		Validation: []string{"go test ./..."},
	}
	contract := nodeTestContract("determinism check")

	r1, err := buildContextPack(root, task, contract, runPaths)
	if err != nil {
		t.Fatalf("first buildContextPack: %v", err)
	}
	// Remove the pack so the second call rewrites it
	assertNoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(r1.Path))))

	r2, err := buildContextPack(root, task, contract, runPaths)
	if err != nil {
		t.Fatalf("second buildContextPack: %v", err)
	}
	if r1.SHA256 != r2.SHA256 {
		t.Errorf("SHA256 not deterministic: %s vs %s", r1.SHA256, r2.SHA256)
	}
	if mustReadFile(t, filepath.Join(root, filepath.FromSlash(r1.Path))) !=
		mustReadFile(t, filepath.Join(root, filepath.FromSlash(r2.Path))) {
		t.Error("pack bytes not identical on second run")
	}
}

func TestContextPackMissingFile(t *testing.T) {
	root := t.TempDir()
	runPaths := makeNodeTestRunPaths(t, root)

	task := planTask{
		ID:      "task_miss",
		Context: []planContextSelector{{Path: "does_not_exist.go", Why: "missing"}},
	}
	result, err := buildContextPack(root, task, nodeTestContract("x"), runPaths)
	if err != nil {
		t.Fatalf("buildContextPack with missing file should not error: %v", err)
	}
	data := mustReadFile(t, filepath.Join(root, filepath.FromSlash(result.Path)))
	if !strings.Contains(data, "Note:") {
		t.Errorf("pack should contain a note for missing file:\n%s", data)
	}
	if !strings.Contains(data, "pactum search") {
		t.Errorf("pack should contain pactum search pointer:\n%s", data)
	}
}

func TestContextPackBinaryFile(t *testing.T) {
	root := t.TempDir()
	// A null byte in the first 512 bytes marks the file binary (isBinaryContent).
	mustWriteFile(t, filepath.Join(root, "blob.bin"), "abc\x00def\n")
	runPaths := makeNodeTestRunPaths(t, root)

	task := planTask{
		ID:      "task_bin",
		Context: []planContextSelector{{Path: "blob.bin", Why: "binary evidence"}},
	}
	result, err := buildContextPack(root, task, nodeTestContract("x"), runPaths)
	if err != nil {
		t.Fatalf("buildContextPack with binary file should not error: %v", err)
	}
	data := mustReadFile(t, filepath.Join(root, filepath.FromSlash(result.Path)))
	if !strings.Contains(data, "binary") {
		t.Errorf("pack should note the file is binary:\n%s", data)
	}
	if !strings.Contains(data, "pactum search") {
		t.Errorf("pack should contain a pactum search pointer for the binary file:\n%s", data)
	}
}

func TestContextPackInvalidLineRange(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "short.go"), "line1\nline2\n")
	runPaths := makeNodeTestRunPaths(t, root)

	cases := []struct {
		name  string
		lines string
	}{
		{"out of bounds", "5-10"},
		{"bad format", "abc-def"},
		{"reversed", "3-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := planTask{
				ID:      "task_range",
				Context: []planContextSelector{{Path: "short.go", Lines: tc.lines}},
			}
			result, err := buildContextPack(root, task, nodeTestContract("x"), runPaths)
			if err != nil {
				t.Fatalf("buildContextPack: %v", err)
			}
			data := mustReadFile(t, filepath.Join(root, filepath.FromSlash(result.Path)))
			if !strings.Contains(data, "Note:") {
				t.Errorf("pack should contain a note for invalid range %q:\n%s", tc.lines, data)
			}
			if !strings.Contains(data, "pactum search") {
				t.Errorf("pack should contain pactum search pointer:\n%s", data)
			}
		})
	}
}

func TestContextPackSymbolNoMatch(t *testing.T) {
	root := t.TempDir()
	dbPath := buildNodeTestIndex(t, nil)
	runPaths := makeNodeTestRunPaths(t, root)

	// Override the search DB for this test by pointing the pack builder's root
	// to a temp workspace that has the test DB at the expected path.
	workRoot := buildNodeTestWorkspaceWithDB(t, dbPath)
	runPaths = makeNodeTestRunPaths(t, workRoot)

	task := planTask{
		ID:      "task_nosym",
		Context: []planContextSelector{{Symbol: "NoSuchSymbolXYZ", Why: "test"}},
	}
	result, err := buildContextPack(workRoot, task, nodeTestContract("x"), runPaths)
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}
	data := mustReadFile(t, filepath.Join(workRoot, filepath.FromSlash(result.Path)))
	if !strings.Contains(data, "not found") {
		t.Errorf("pack should say symbol not found:\n%s", data)
	}
	if !strings.Contains(data, "pactum search") {
		t.Errorf("pack should contain pactum search pointer:\n%s", data)
	}
}

func TestContextPackSymbolSingleMatch(t *testing.T) {
	// Build an index with one symbol that maps to a real file in root.
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc DoWork() {}\nfunc helper() {}\n")

	items := []codeindex.Item{
		{Path: "main.go", Kind: "go_func", Language: "go", Name: "DoWork", Signature: "func DoWork()", StartLine: 3, EndLine: 3},
	}
	dbPath := buildNodeTestIndex(t, items)
	workRoot := buildNodeTestWorkspaceWithDB(t, dbPath)
	// Copy the test file into workRoot so the pack can read it
	mustWriteFile(t, filepath.Join(workRoot, "main.go"), "package main\n\nfunc DoWork() {}\nfunc helper() {}\n")
	runPaths := makeNodeTestRunPaths(t, workRoot)

	task := planTask{
		ID:      "task_sym1",
		Context: []planContextSelector{{Symbol: "DoWork", Why: "implementation"}},
	}
	result, err := buildContextPack(workRoot, task, nodeTestContract("x"), runPaths)
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}
	data := mustReadFile(t, filepath.Join(workRoot, filepath.FromSlash(result.Path)))
	if !strings.Contains(data, "main.go:3-3") {
		t.Errorf("pack missing symbol address tag:\n%s", data)
	}
	if !strings.Contains(data, "DoWork") {
		t.Errorf("pack missing symbol content:\n%s", data)
	}
	if !strings.Contains(data, "implementation") {
		t.Errorf("pack missing why:\n%s", data)
	}
}

func TestContextPackSymbolMultipleMatches(t *testing.T) {
	root := t.TempDir()
	// Two files with the same symbol name — pack must note ambiguity and include both.
	mustWriteFile(t, filepath.Join(root, "pkg1/pkg1.go"), "package pkg1\n\nfunc Run() {}\n")
	mustWriteFile(t, filepath.Join(root, "pkg2/pkg2.go"), "package pkg2\n\nfunc Run() {}\n")

	items := []codeindex.Item{
		{Path: "pkg1/pkg1.go", Kind: "go_func", Language: "go", Name: "Run", StartLine: 3, EndLine: 3},
		{Path: "pkg2/pkg2.go", Kind: "go_func", Language: "go", Name: "Run", StartLine: 3, EndLine: 3},
	}
	dbPath := buildNodeTestIndex(t, items)
	workRoot := buildNodeTestWorkspaceWithDB(t, dbPath)
	mustWriteFile(t, filepath.Join(workRoot, "pkg1/pkg1.go"), "package pkg1\n\nfunc Run() {}\n")
	mustWriteFile(t, filepath.Join(workRoot, "pkg2/pkg2.go"), "package pkg2\n\nfunc Run() {}\n")
	runPaths := makeNodeTestRunPaths(t, workRoot)

	task := planTask{
		ID:      "task_multi",
		Context: []planContextSelector{{Symbol: "Run", Why: "both packages"}},
	}
	result, err := buildContextPack(workRoot, task, nodeTestContract("x"), runPaths)
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}
	data := mustReadFile(t, filepath.Join(workRoot, filepath.FromSlash(result.Path)))
	// Both addresses must appear in deterministic order (path ASC)
	if !strings.Contains(data, "pkg1/pkg1.go:3-3") {
		t.Errorf("pack missing first symbol:\n%s", data)
	}
	if !strings.Contains(data, "pkg2/pkg2.go:3-3") {
		t.Errorf("pack missing second symbol:\n%s", data)
	}
	// Ambiguity note
	if !strings.Contains(data, "2 definitions") {
		t.Errorf("pack should note ambiguity:\n%s", data)
	}
	idx1 := strings.Index(data, "pkg1/pkg1.go:3-3")
	idx2 := strings.Index(data, "pkg2/pkg2.go:3-3")
	if idx1 < 0 || idx2 < 0 || idx1 >= idx2 {
		t.Errorf("pack addresses not in deterministic path order:\n%s", data)
	}
}

func TestContextPackSliceTruncation(t *testing.T) {
	root := t.TempDir()
	// Write a file larger than contextPackSliceCap
	big := strings.Repeat("x", contextPackSliceCap+100)
	mustWriteFile(t, filepath.Join(root, "big.go"), big)
	runPaths := makeNodeTestRunPaths(t, root)

	task := planTask{
		ID:      "task_trunc",
		Context: []planContextSelector{{Path: "big.go"}},
	}
	result, err := buildContextPack(root, task, nodeTestContract("x"), runPaths)
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}
	data := mustReadFile(t, filepath.Join(root, filepath.FromSlash(result.Path)))
	if !strings.Contains(data, "truncated") {
		t.Errorf("pack missing truncation marker:\n%s", data)
	}
	if !strings.Contains(data, "pactum search") {
		t.Errorf("pack missing pactum search pointer after truncation:\n%s", data)
	}
}

func TestContextPackPackTotalCap(t *testing.T) {
	root := t.TempDir()
	// Two files that together exceed the total cap
	half := strings.Repeat("y", contextPackTotalCap/2+1024)
	mustWriteFile(t, filepath.Join(root, "a.go"), half)
	mustWriteFile(t, filepath.Join(root, "b.go"), half)
	runPaths := makeNodeTestRunPaths(t, root)

	task := planTask{
		ID: "task_total",
		Context: []planContextSelector{
			{Path: "a.go"},
			{Path: "b.go"},
		},
	}
	result, err := buildContextPack(root, task, nodeTestContract("x"), runPaths)
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}
	data := mustReadFile(t, filepath.Join(root, filepath.FromSlash(result.Path)))
	if len(data) > contextPackTotalCap+512 {
		t.Errorf("pack exceeds total cap: len=%d", len(data))
	}
	if !strings.Contains(data, "truncated") {
		t.Errorf("pack missing truncation marker:\n%s", data[:min(500, len(data))])
	}
}

// ---------------------------------------------------------------------------
// Per-task validation runner tests
// ---------------------------------------------------------------------------

func TestTaskValidationPass(t *testing.T) {
	root := t.TempDir()
	result, err := runTaskValidation(root, []string{"true", "echo ok"}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runTaskValidation: %v", err)
	}
	if !result.Passed {
		t.Errorf("all-zero-exit commands should pass")
	}
	if len(result.Commands) != 2 {
		t.Fatalf("expected 2 command results, got %d", len(result.Commands))
	}
	for _, cmd := range result.Commands {
		if cmd.ExitCode != 0 {
			t.Errorf("command %q exit code = %d, want 0", cmd.Command, cmd.ExitCode)
		}
	}
}

func TestTaskValidationFail(t *testing.T) {
	root := t.TempDir()
	result, err := runTaskValidation(root, []string{"true", "false", "true"}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runTaskValidation: %v", err)
	}
	if result.Passed {
		t.Error("validation with a failing command must not pass")
	}
	if len(result.Commands) != 3 {
		t.Fatalf("expected 3 command results, got %d", len(result.Commands))
	}
	if result.Commands[1].ExitCode == 0 {
		t.Errorf("second command should have non-zero exit code")
	}
}

func TestTaskValidationCapturesOutput(t *testing.T) {
	root := t.TempDir()
	result, err := runTaskValidation(root, []string{`echo hello-stdout && echo hello-stderr >&2`}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runTaskValidation: %v", err)
	}
	cmd := result.Commands[0]
	if !strings.Contains(cmd.Stdout, "hello-stdout") {
		t.Errorf("stdout not captured: %q", cmd.Stdout)
	}
	if !strings.Contains(cmd.Stderr, "hello-stderr") {
		t.Errorf("stderr not captured: %q", cmd.Stderr)
	}
}

func TestTaskValidationFailReportsDetails(t *testing.T) {
	root := t.TempDir()
	result, err := runTaskValidation(root, []string{`echo failure-marker >&2 && exit 42`}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runTaskValidation: %v", err)
	}
	if result.Passed {
		t.Error("exit-42 command must not pass")
	}
	cmd := result.Commands[0]
	if cmd.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", cmd.ExitCode)
	}
	if !strings.Contains(cmd.Stderr, "failure-marker") {
		t.Errorf("stderr missing failure marker: %q", cmd.Stderr)
	}
}

func TestTaskValidationTimeout(t *testing.T) {
	// A command that exceeds the timeout must be reported as timed_out and not passed.
	root := t.TempDir()
	result, err := runTaskValidation(root, []string{"sleep 60"}, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("runTaskValidation: %v", err)
	}
	if result.Passed {
		t.Error("timed-out validation must not pass")
	}
	if len(result.Commands) != 1 {
		t.Fatalf("expected 1 command result, got %d", len(result.Commands))
	}
	if !result.Commands[0].TimedOut {
		t.Error("expected timed_out = true")
	}
}

// ---------------------------------------------------------------------------
// Baseline-red checker tests
// ---------------------------------------------------------------------------

func TestBaselineRedTestValidation(t *testing.T) {
	// A test-shaped command that fails → proceed (the test is a real guard).
	root := t.TempDir()
	result, err := runBaselineCheck(root, []string{"go test -run TestFoo && false"}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Commands))
	}
	cmd := result.Commands[0]
	if cmd.Shape != "test" {
		t.Errorf("shape = %q, want test", cmd.Shape)
	}
	if cmd.Status != "red" {
		t.Errorf("status = %q, want red", cmd.Status)
	}
	if cmd.Recommendation != "proceed" {
		t.Errorf("recommendation = %q, want proceed", cmd.Recommendation)
	}
	if result.Recommendation != "proceed" {
		t.Errorf("task recommendation = %q, want proceed", result.Recommendation)
	}
}

func TestBaselineGreenTestValidation(t *testing.T) {
	// A test-shaped command that exits 0 → block (already passing before the work).
	root := t.TempDir()
	result, err := runBaselineCheck(root, []string{"go test --help > /dev/null 2>&1 || true"}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	cmd := result.Commands[0]
	if cmd.Shape != "test" {
		t.Errorf("shape = %q, want test", cmd.Shape)
	}
	if cmd.Status != "green" {
		t.Errorf("status = %q, want green (command must exit 0)", cmd.Status)
	}
	if cmd.Recommendation != "block" {
		t.Errorf("recommendation = %q, want block", cmd.Recommendation)
	}
	if result.Recommendation != "block" {
		t.Errorf("task recommendation = %q, want block", result.Recommendation)
	}
}

func TestBaselineGreenGoTestNoMatch(t *testing.T) {
	// Reproduce the go test -run NoSuchTest exit-0 trap: a test-shaped command that
	// exits 0 because no tests match the filter. The checker must classify it as block.
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module baseline_test_nosuchtestpkg\n\ngo 1.21\n")
	mustWriteFile(t, filepath.Join(root, "example_test.go"),
		"package baseline_test_nosuchtestpkg\n\nimport \"testing\"\n\nfunc TestActuallyExists(t *testing.T) {}\n")

	// go test -run TestNotPresent exits 0 even though no tests matched.
	result, err := runBaselineCheck(root, []string{"go test -run TestNotPresent ."}, 2*time.Minute)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	cmd := result.Commands[0]
	if cmd.ExitCode != 0 {
		t.Skipf("go test -run TestNotPresent exited %d (want 0 to demonstrate the trap); skipping trap verification", cmd.ExitCode)
	}
	if cmd.Shape != "test" {
		t.Errorf("shape = %q, want test", cmd.Shape)
	}
	if cmd.Status != "green" {
		t.Errorf("status = %q, want green", cmd.Status)
	}
	if cmd.Recommendation != "block" {
		t.Errorf("recommendation = %q, want block (exit-0 trap must be blocked)", cmd.Recommendation)
	}
}

func TestBaselineGreenBuildLint(t *testing.T) {
	// A build/lint command that exits 0 → advisory signal, not a hard block.
	root := t.TempDir()
	result, err := runBaselineCheck(root, []string{"echo build-ok"}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	cmd := result.Commands[0]
	if cmd.Shape != "other" {
		t.Errorf("shape = %q, want other", cmd.Shape)
	}
	if cmd.Status != "green" {
		t.Errorf("status = %q, want green", cmd.Status)
	}
	if cmd.Recommendation != "signal" {
		t.Errorf("recommendation = %q, want signal", cmd.Recommendation)
	}
	if result.Recommendation != "signal" {
		t.Errorf("task recommendation = %q, want signal", result.Recommendation)
	}
}

func TestBaselineAggregationBlockWinsOverSignal(t *testing.T) {
	// When a test command is already green and a lint command is also green,
	// the task-level recommendation must be block (conservative wins).
	root := t.TempDir()
	cmds := []string{
		"echo lint-ok",                           // other + green → signal
		"go test --help >/dev/null 2>&1 || true", // test + green → block
	}
	result, err := runBaselineCheck(root, cmds, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	if len(result.Commands) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Commands))
	}
	if result.Recommendation != "block" {
		t.Errorf("task recommendation = %q, want block", result.Recommendation)
	}
}

func TestBaselineAggregationProceedWhenAllTestsRed(t *testing.T) {
	// All test-shaped commands fail → proceed.
	root := t.TempDir()
	cmds := []string{
		"go test -run Foo && false", // test + red
		"go test -run Bar && false", // test + red
	}
	result, err := runBaselineCheck(root, cmds, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	if result.Recommendation != "proceed" {
		t.Errorf("task recommendation = %q, want proceed", result.Recommendation)
	}
}

func TestBaselineAggregationSignalWhenOnlyBuildGreen(t *testing.T) {
	// No test commands; only a green build command → signal.
	root := t.TempDir()
	result, err := runBaselineCheck(root, []string{"echo build-ok"}, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	if result.Recommendation != "signal" {
		t.Errorf("task recommendation = %q, want signal", result.Recommendation)
	}
}

func TestBaselineAggregationProceedWhenAllRed(t *testing.T) {
	// All commands fail → proceed (nothing blocks or signals).
	root := t.TempDir()
	cmds := []string{"false", "false"}
	result, err := runBaselineCheck(root, cmds, nodeTestTimeout)
	if err != nil {
		t.Fatalf("runBaselineCheck: %v", err)
	}
	if result.Recommendation != "proceed" {
		t.Errorf("task recommendation = %q, want proceed", result.Recommendation)
	}
}

// ---------------------------------------------------------------------------
// PlanContext integration test
// ---------------------------------------------------------------------------

func TestPlanContextWritesPackAndReturnsJSON(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	contractUpdate := map[string]any{
		"plan": contractPlan{Tasks: []planTask{{
			ID:    "t1",
			Title: "Example",
			Context: []planContextSelector{
				{Path: "README.md", Why: "always present"},
			},
			Acceptance: []string{"it works"},
			Validation: []string{"true"},
		}}},
	}
	fromFile := writeReviseDocForTest(t, runPaths, contractUpdate)
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", fromFile}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d: %s", code, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"plan", "context", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan context exited %d, stderr: %s", code, stderr.String())
	}

	var response planContextResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("parse plan context JSON: %v\noutput: %s", err, stdout.String())
	}
	if len(response.Tasks) != 1 {
		t.Fatalf("expected 1 task result, got %d", len(response.Tasks))
	}
	task := response.Tasks[0]
	if task.TaskID != "t1" {
		t.Errorf("task ID = %q, want t1", task.TaskID)
	}
	if task.ContextPackPath == "" {
		t.Error("context pack path must not be empty")
	}
	if task.ContextPackSHA == "" {
		t.Error("context pack SHA256 must not be empty")
	}
	// The context pack file must have been written to disk.
	packPath := filepath.Join(root, filepath.FromSlash(task.ContextPackPath))
	if _, err := os.Stat(packPath); os.IsNotExist(err) {
		t.Errorf("context pack file not created at %s", packPath)
	}
	// Baseline and ValidationNow must be present and consistent.
	if task.Baseline.Recommendation == "" {
		t.Error("baseline recommendation must not be empty")
	}
	// The validation command "true" exits 0, so validation must pass.
	if !task.ValidationNow.Passed {
		t.Error("validation with 'true' command must pass")
	}
}

// ---------------------------------------------------------------------------
// Execute loop config gate tests
// ---------------------------------------------------------------------------

func TestConfigExecuteLoopMaxIsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  execute:\n    loop:\n      max: 5\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := readConfig(path)
	if err != nil {
		t.Fatalf("execute.loop.max should be valid: %v", err)
	}
	if cfg.Pipeline.Execute.Loop == nil || cfg.Pipeline.Execute.Loop.Max != 5 {
		t.Fatalf("execute.loop.max not loaded correctly: %+v", cfg.Pipeline.Execute.Loop)
	}
}

func TestConfigExecuteLoopPatienceRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  execute:\n    loop:\n      max: 3\n      patience: 2\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := readConfig(path)
	if err == nil {
		t.Fatal("execute.loop.patience should be rejected at config load")
	}
	if !strings.Contains(err.Error(), "patience") {
		t.Errorf("error should mention patience: %v", err)
	}
}

func TestConfigExecuteLoopSettleDefaultIsValid(t *testing.T) {
	// settle: 1 is the fixed default for execute and must be accepted at config load.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  execute:\n    loop:\n      max: 3\n      settle: 1\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := readConfig(path)
	if err != nil {
		t.Fatalf("execute.loop.settle=1 (the fixed default) should be valid: %v", err)
	}
}

func TestConfigExecuteLoopSettleRejected(t *testing.T) {
	// settle values above the fixed default of 1 are non-default and must be rejected.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  execute:\n    loop:\n      max: 3\n      settle: 2\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := readConfig(path)
	if err == nil {
		t.Fatal("execute.loop.settle=2 (non-default) should be rejected at config load")
	}
	if !strings.Contains(err.Error(), "settle") {
		t.Errorf("error should mention settle: %v", err)
	}
}

func TestConfigExecuteLoopMaxZeroIsValid(t *testing.T) {
	// max: 0 means the loop block is present but max is unset (treated as absent).
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	contents := "version: v1alpha1\nagents:\n  - name: claude\n    model: claude-opus-4-8\npipeline:\n  execute:\n    loop:\n      max: 0\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := readConfig(path)
	if err != nil {
		t.Fatalf("execute.loop with max=0 should be valid: %v", err)
	}
}

// ---------------------------------------------------------------------------
// isTestShaped unit test
// ---------------------------------------------------------------------------

func TestIsTestShaped(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"go test ./...", true},
		{"go test -run TestFoo ./internal/app", true},
		{"go test -run 'Plan|Context' ./...", true},
		{"npm test", true},
		{"pytest tests/", true},
		{"cargo test", true},
		{"go build ./...", false},
		{"make check", false},
		{"golangci-lint run", false},
		{"echo hello", false},
		{"go vet ./...", false},
	}
	for _, tc := range cases {
		got := isTestShaped(tc.cmd)
		if got != tc.want {
			t.Errorf("isTestShaped(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeNodeTestRunPaths constructs a contractRunPathSet with an execute dir
// rooted in root. No actual run is created; only the dir structure is set up.
func makeNodeTestRunPaths(t *testing.T, root string) contractRunPathSet {
	t.Helper()
	runDir := filepath.Join(root, ".heurema", "pactum", "runs", "run_node_test")
	assertNoError(t, os.MkdirAll(runDir, 0o755))
	return contractRunPaths(runDir)
}

// nodeTestContract returns a minimal contract for context-pack tests.
func nodeTestContract(goal string) draftContract {
	return draftContract{
		Goal:  goal,
		Scope: draftContractScope{In: []string{"internal/**"}, Out: []string{}},
	}
}

// buildNodeTestIndex creates a search index with the given code items.
func buildNodeTestIndex(t *testing.T, items []codeindex.Item) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "search.sqlite")
	if err := searchpkg.Rebuild(dbPath, searchpkg.IndexInput{CodeItems: items}); err != nil {
		t.Fatalf("Rebuild search index: %v", err)
	}
	return dbPath
}

// buildNodeTestWorkspaceWithDB creates a temp root with the search DB placed at
// the path that artifacts.New(root).SearchSQLite expects.
func buildNodeTestWorkspaceWithDB(t *testing.T, dbPath string) string {
	t.Helper()
	root := t.TempDir()
	dest := filepath.Join(root, ".heurema", "pactum", "map", "search.sqlite")
	assertNoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read test DB: %v", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatalf("write test DB: %v", err)
	}
	return root
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
