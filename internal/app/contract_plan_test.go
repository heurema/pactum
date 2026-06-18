package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPlanDAGHashStability verifies that adding the Plan field to draftContract
// does not change the hash of plan-less contracts. A plan-less contract and a
// contract with Plan.Tasks == [] must both produce the same SHA-256 as the
// pre-slice plan-less contract (no "plan" key in the serialized JSON).
func TestPlanDAGHashStability(t *testing.T) {
	contract := draftContract{
		Schema: contractSchema,
		RunID:  "run_hash_stable",
		Status: "draft",
		Goal:   "hash stability test",
		Scope: draftContractScope{
			In:  []string{},
			Out: []string{},
		},
		AcceptanceCriteria: []string{},
		Validation:         draftValidation{Commands: []string{}},
		Assumptions:        []string{},
		OpenQuestions:      []string{},
		Clarifications: contractClarifySet{
			Questions: []clarifyQuestionStatus{},
		},
		MemoryContext: draftMemoryContext{
			UsedItems: []string{},
		},
	}

	// Plan-less: hash H1, no "plan" key in JSON.
	h1, err := contractVersionHash(contract)
	if err != nil {
		t.Fatalf("contractVersionHash: %v", err)
	}

	// Frozen golden: the SHA-256 of the canonical plan-less contract above,
	// captured when the plan field landed. Because plan is omitempty, a
	// plan-less contract must serialize byte-for-byte as it did before the
	// field existed. Comparing only h2/h3 to a live-computed h1 would let a
	// serialization drift move all four together and still pass; anchoring h1
	// to a literal catches that — a change here means every previously
	// approved contract's version token silently broke.
	const goldenPlanlessHash = "2a429978596f383a8c9c1fce0a84bbe28d4d99ac7cda14b707022c042a86fc4b"
	if h1 != goldenPlanlessHash {
		t.Fatalf("plan-less contract hash drifted:\n  got:    %s\n  golden: %s", h1, goldenPlanlessHash)
	}
	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(data, []byte(`"plan"`)) {
		t.Fatalf("plan-less contract JSON must not contain a plan key:\n%s", data)
	}

	// Plan with nil Tasks: normalizes to nil → same hash.
	c2 := contract
	c2.Plan = &contractPlan{Tasks: nil}
	normalizeDraftContractPlan(&c2)
	if c2.Plan != nil {
		t.Fatal("Plan{Tasks:nil} must normalize to nil")
	}
	h2, err := contractVersionHash(c2)
	if err != nil {
		t.Fatalf("contractVersionHash(Plan{Tasks:nil}): %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash mismatch: plan-less=%s, Plan{Tasks:nil}=%s", h1, h2)
	}

	// Plan with empty Tasks slice: normalizes to nil → same hash.
	c3 := contract
	c3.Plan = &contractPlan{Tasks: []planTask{}}
	normalizeDraftContractPlan(&c3)
	if c3.Plan != nil {
		t.Fatal("Plan{Tasks:[]} must normalize to nil")
	}
	h3, err := contractVersionHash(c3)
	if err != nil {
		t.Fatalf("contractVersionHash(Plan{Tasks:[]}): %v", err)
	}
	if h1 != h3 {
		t.Fatalf("hash mismatch: plan-less=%s, Plan{Tasks:[]}=%s", h1, h3)
	}
}

// TestPlanDAGValid verifies that a valid plan is accepted, preserved through
// contract show and show --json, and survives a subsequent revise round-trip.
func TestPlanDAGValid(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	validPlan := contractPlan{
		Tasks: []planTask{
			{
				ID:    "t1",
				Title: "First task",
				Context: []planContextSelector{
					{Path: "internal/app/run.go", Lines: "60-100", Why: "contract shape"},
					{Symbol: "draftContract", Why: "schema owner"},
				},
				ExpectedFiles: []string{"internal/app/run.go"},
				Acceptance:    []string{"plan schema added"},
				Validation:    []string{"go test ./internal/app/..."},
			},
			{
				ID:         "t2",
				Title:      "Second task",
				DependsOn:  []string{"t1"},
				Acceptance: []string{"tests pass"},
				Validation: []string{"go test ./..."},
			},
		},
	}

	// Revise contract with the valid plan.
	reviseDoc := map[string]any{
		"base_version": readVersionForTest(t, runPaths),
		"contract":     map[string]any{"plan": validPlan},
	}
	revisePath := writeTempJSON(t, reviseDoc)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", revisePath, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d, stdout: %s, stderr: %s", code, stdout.String(), stderr.String())
	}

	var revResult contractReviseResponse
	if err := json.Unmarshal(stdout.Bytes(), &revResult); err != nil {
		t.Fatalf("parse revise response: %v — stdout: %s", err, stdout.String())
	}
	if !revResult.OK || !revResult.Changed {
		t.Fatalf("expected ok+changed, got: %+v", revResult)
	}
	if revResult.Contract.Plan == nil || len(revResult.Contract.Plan.Tasks) != 2 {
		t.Fatalf("revise response: expected 2 plan tasks, got: %+v", revResult.Contract.Plan)
	}

	// contract show --json: plan fully present.
	stdout.Reset()
	code = app.Run([]string{"contract", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show --json exited %d", code)
	}
	var showResult contractShowResponse
	if err := json.Unmarshal(stdout.Bytes(), &showResult); err != nil {
		t.Fatalf("parse show response: %v", err)
	}
	if showResult.Contract.Plan == nil || len(showResult.Contract.Plan.Tasks) != 2 {
		t.Fatalf("show --json: expected 2 plan tasks")
	}
	t1 := showResult.Contract.Plan.Tasks[0]
	if t1.ID != "t1" || t1.Title != "First task" {
		t.Fatalf("show --json: task t1 mismatch: %+v", t1)
	}
	// Every plan field must survive the JSON round-trip — not just id/title.
	if len(t1.Context) != 2 ||
		t1.Context[0].Path != "internal/app/run.go" || t1.Context[0].Lines != "60-100" || t1.Context[0].Why != "contract shape" ||
		t1.Context[1].Symbol != "draftContract" || t1.Context[1].Why != "schema owner" {
		t.Fatalf("show --json: task t1 context mismatch: %+v", t1.Context)
	}
	if len(t1.ExpectedFiles) != 1 || t1.ExpectedFiles[0] != "internal/app/run.go" {
		t.Fatalf("show --json: task t1 expected_files mismatch: %+v", t1.ExpectedFiles)
	}
	if len(t1.Acceptance) != 1 || t1.Acceptance[0] != "plan schema added" {
		t.Fatalf("show --json: task t1 acceptance mismatch: %+v", t1.Acceptance)
	}
	if len(t1.Validation) != 1 || t1.Validation[0] != "go test ./internal/app/..." {
		t.Fatalf("show --json: task t1 validation mismatch: %+v", t1.Validation)
	}
	t2 := showResult.Contract.Plan.Tasks[1]
	if t2.ID != "t2" || len(t2.DependsOn) != 1 || t2.DependsOn[0] != "t1" {
		t.Fatalf("show --json: task t2 mismatch: %+v", t2)
	}
	if len(t2.Acceptance) != 1 || t2.Acceptance[0] != "tests pass" ||
		len(t2.Validation) != 1 || t2.Validation[0] != "go test ./..." {
		t.Fatalf("show --json: task t2 acceptance/validation mismatch: %+v", t2)
	}

	// contract show (plain text): all task fields present.
	stdout.Reset()
	code = app.Run([]string{"contract", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show exited %d", code)
	}
	showText := stdout.String()
	for _, want := range []string{
		"t1", "First task",
		"t2", "Second task",
		"Depends on: t1",       // unambiguous: the rendered depends_on line, not a bare id
		"symbol draftContract", // context symbol selector preserved
		"internal/app/run.go lines 60-100",
		"Expected files: internal/app/run.go",
		"plan schema added",
		"go test ./internal/app/...",
		"tests pass",
		"go test ./...",
	} {
		if !strings.Contains(showText, want) {
			t.Fatalf("contract show plain text missing %q:\n%s", want, showText)
		}
	}

	// Round-trip: revise an unrelated field and verify plan is preserved.
	reviseDoc2 := map[string]any{
		"base_version": readVersionForTest(t, runPaths),
		"contract":     map[string]any{"goal": "updated goal"},
	}
	stdout.Reset()
	code = app.Run([]string{"contract", "revise", runID, "--from", writeTempJSON(t, reviseDoc2), "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("round-trip revise exited %d: %s", code, stdout.String())
	}
	var result2 contractReviseResponse
	if err := json.Unmarshal(stdout.Bytes(), &result2); err != nil {
		t.Fatalf("parse round-trip response: %v", err)
	}
	if result2.Contract.Plan == nil || len(result2.Contract.Plan.Tasks) != 2 {
		t.Fatalf("plan not preserved through round-trip: %+v", result2.Contract.Plan)
	}
	if result2.Contract.Plan.Tasks[0].ID != "t1" || result2.Contract.Plan.Tasks[1].ID != "t2" {
		t.Fatalf("plan task ids not preserved: %+v", result2.Contract.Plan.Tasks)
	}
}

func TestPlanDAGDuplicateID(t *testing.T) {
	tasks := []planTask{
		{ID: "t1", Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
		{ID: "t1", Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
	}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, nil)
		assertPlanLoadError(t, err, "DUPLICATE_TASK_ID", "duplicate")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, nil)
		assertReviseIssue(t, failure, "DUPLICATE_TASK_ID")
	})
}

func TestPlanDAGMissingDep(t *testing.T) {
	tasks := []planTask{
		{ID: "t1", DependsOn: []string{"nonexistent"}, Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
	}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, nil)
		assertPlanLoadError(t, err, "MISSING_DEPENDENCY", "nonexistent")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, nil)
		assertReviseIssue(t, failure, "MISSING_DEPENDENCY")
	})
}

func TestPlanDAGCycle(t *testing.T) {
	tasks := []planTask{
		{ID: "t1", DependsOn: []string{"t2"}, Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
		{ID: "t2", DependsOn: []string{"t1"}, Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
	}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, nil)
		assertPlanLoadError(t, err, "CYCLE_IN_DAG", "cycle")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, nil)
		assertReviseIssue(t, failure, "CYCLE_IN_DAG")
	})
}

func TestPlanDAGScopeViolation(t *testing.T) {
	pathsInScope := []string{"internal/app/**"}
	tasks := []planTask{
		{
			ID:            "t1",
			ExpectedFiles: []string{"cmd/main.go"}, // outside internal/app/**
			Acceptance:    []string{"ok"},
			Validation:    []string{"go build ./..."},
		},
	}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, pathsInScope)
		assertPlanLoadError(t, err, "EXPECTED_FILE_OUT_OF_SCOPE", "cmd/main.go")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, pathsInScope)
		assertReviseIssue(t, failure, "EXPECTED_FILE_OUT_OF_SCOPE")
	})
}

// TestPlanDAGScopeRevisionInvalidatesExistingPlan guards the validation gate:
// plan validation must run on every revise, not only when the update touches
// the plan. A revise that narrows paths_in_scope so an already-stored plan's
// expected_files fall out of scope must be rejected, never persisted.
func TestPlanDAGScopeRevisionInvalidatesExistingPlan(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	revise := func(t *testing.T, partial map[string]any) (int, contractReviseFailure) {
		t.Helper()
		doc := map[string]any{
			"base_version": readVersionForTest(t, runPaths),
			"contract":     partial,
		}
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "revise", runID, "--from", writeTempJSON(t, doc), "--json"}, &stdout, &stderr)
		var failure contractReviseFailure
		_ = json.Unmarshal(stdout.Bytes(), &failure)
		if code != 0 {
			return code, failure
		}
		return code, failure
	}

	// Establish a scope that contains the plan's expected_files.
	if code, _ := revise(t, map[string]any{"paths_in_scope": []string{"internal/app/**"}}); code != 0 {
		t.Fatalf("set paths_in_scope exited %d", code)
	}
	// Store a valid plan whose expected_files live inside the current scope.
	validPlan := contractPlan{Tasks: []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{"internal/app/run.go"},
		Acceptance:    []string{"ok"},
		Validation:    []string{"go test ./internal/app/..."},
	}}}
	if code, _ := revise(t, map[string]any{"plan": validPlan}); code != 0 {
		t.Fatalf("store valid plan exited %d (expected acceptance)", code)
	}

	// Now narrow scope so the existing plan's expected_files fall out of scope,
	// WITHOUT touching the plan field. This must be rejected.
	code, failure := revise(t, map[string]any{"paths_in_scope": []string{"docs/**"}})
	if code == 0 {
		t.Fatal("expected scope revise to be rejected because it invalidates the existing plan")
	}
	assertReviseIssue(t, failure, "EXPECTED_FILE_OUT_OF_SCOPE")
}

func TestPlanDAGEmptyAcceptance(t *testing.T) {
	tasks := []planTask{
		{ID: "t1", Acceptance: []string{}, Validation: []string{"go build ./..."}},
	}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, nil)
		assertPlanLoadError(t, err, "EMPTY_ACCEPTANCE", "acceptance")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, nil)
		assertReviseIssue(t, failure, "EMPTY_ACCEPTANCE")
	})
}

func TestPlanDAGEmptyValidation(t *testing.T) {
	tasks := []planTask{
		{ID: "t1", Acceptance: []string{"ok"}, Validation: []string{}},
	}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, nil)
		assertPlanLoadError(t, err, "EMPTY_VALIDATION", "validation")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, nil)
		assertReviseIssue(t, failure, "EMPTY_VALIDATION")
	})
}

func TestPlanDAGEmptyID(t *testing.T) {
	tasks := []planTask{
		{ID: "", Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
	}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, nil)
		assertPlanLoadError(t, err, "EMPTY_TASK_ID", "empty")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, nil)
		assertReviseIssue(t, failure, "EMPTY_TASK_ID")
	})
}

// TestPlanDAGVacuousValidation verifies that a task with non-empty
// expected_files is rejected when all validation commands are unscoped.
func TestPlanDAGVacuousValidation(t *testing.T) {
	tasks := []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{"internal/app/foo.go"},
		Acceptance:    []string{"ok"},
		Validation:    []string{"go build ./...", "make check"}, // neither scoped to internal/app/foo.go
	}}

	t.Run("load", func(t *testing.T) {
		_, err := readDraftContractFromTasks(t, tasks, nil)
		assertPlanLoadError(t, err, "VACUOUS_VALIDATION", "scoped")
	})

	t.Run("revise", func(t *testing.T) {
		failure := reviseWithBadPlan(t, tasks, nil)
		assertReviseIssue(t, failure, "VACUOUS_VALIDATION")
	})
}

// TestPlanDAGScopedValidationAccepted verifies that a task with expected_files
// is accepted when at least one validation command is scoped to a file.
func TestPlanDAGScopedValidationAccepted(t *testing.T) {
	tasks := []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{"internal/app/foo.go"},
		Acceptance:    []string{"ok"},
		// Rule (a): cmd contains the full path.
		Validation: []string{"go test ./internal/app -run TestFoo"},
	}}

	_, err := readDraftContractFromTasks(t, tasks, nil)
	if err != nil {
		t.Fatalf("expected acceptance but got: %v", err)
	}
}

// TestPlanDAGMixedScopedAndGlobalValidationAccepted verifies that having one
// scoped command alongside global commands satisfies the requirement.
func TestPlanDAGMixedScopedAndGlobalValidationAccepted(t *testing.T) {
	tasks := []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{"internal/app/plan.go"},
		Acceptance:    []string{"ok"},
		// Rule (b): cmd contains directory prefix "internal/app" which has a '/'.
		Validation: []string{"go build ./...", "go test ./internal/app/..."},
	}}

	_, err := readDraftContractFromTasks(t, tasks, nil)
	if err != nil {
		t.Fatalf("expected acceptance but got: %v", err)
	}
}

// TestPlanDAGEmptyExpectedFilesExempt verifies that tasks with no expected_files
// are not subject to VACUOUS_VALIDATION regardless of validation commands.
func TestPlanDAGEmptyExpectedFilesExempt(t *testing.T) {
	tasks := []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{},
		Acceptance:    []string{"ok"},
		Validation:    []string{"go build ./..."}, // global-only, but exempted
	}}

	_, err := readDraftContractFromTasks(t, tasks, nil)
	if err != nil {
		t.Fatalf("expected acceptance but got: %v", err)
	}
}

// TestPlanDAGRootLevelFileScopedByFullPath verifies rule (a): a root-level file
// like "go.mod" has no directory prefix, so only the full-path rule applies.
func TestPlanDAGRootLevelFileScopedByFullPath(t *testing.T) {
	tasks := []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{"go.mod"},
		Acceptance:    []string{"ok"},
		Validation:    []string{"cat go.mod"}, // contains "go.mod" as substring
	}}

	_, err := readDraftContractFromTasks(t, tasks, nil)
	if err != nil {
		t.Fatalf("expected acceptance but got: %v", err)
	}
}

// TestPlanDAGRootLevelFileUnscopedRejected verifies that a root-level file with
// only global validation commands is rejected with VACUOUS_VALIDATION.
func TestPlanDAGRootLevelFileUnscopedRejected(t *testing.T) {
	tasks := []planTask{{
		ID:            "t1",
		ExpectedFiles: []string{"go.mod"},
		Acceptance:    []string{"ok"},
		Validation:    []string{"go build ./..."}, // does not contain "go.mod"
	}}

	_, err := readDraftContractFromTasks(t, tasks, nil)
	assertPlanLoadError(t, err, "VACUOUS_VALIDATION", "scoped")
}

// TestIsValidationCommandScopedToPath exercises the scoping algorithm directly.
func TestIsValidationCommandScopedToPath(t *testing.T) {
	cases := []struct {
		cmd  string
		path string
		want bool
	}{
		// Rule (a): full path match
		{"go test ./internal/app/foo.go", "internal/app/foo.go", true},
		{"cat internal/app/foo.go", "internal/app/foo.go", true},
		// Rule (b): directory prefix with '/'
		// foo_test.go contains "internal/app" as substring → scoped via rule (b)
		{"go test ./internal/app/foo_test.go", "internal/app/foo.go", true},
		{"go test ./internal/app/...", "internal/app/foo.go", true},
		{"go test ./internal/...", "internal/app/foo.go", false}, // "internal" has no '/'
		{"go test internal/app", "internal/app/foo.go", true},
		// Rule (c): wildcard patterns
		{"go test ./internal/app/...", "internal/app/sub/foo.go", true},
		// Root-level file: only rule (a)
		{"cat go.mod", "go.mod", true},
		{"go build ./...", "go.mod", false},
		// Deep path
		{"go test ./a/b/c/...", "a/b/c/d/e.go", true},
		{"go test ./a/b/...", "a/b/c/d/e.go", true},
		{"go test a/b/c", "a/b/c/d/e.go", true},
		// Normalize ./
		{"go test internal/app/foo.go", "./internal/app/foo.go", true},
	}
	for _, tc := range cases {
		norm := normalizeExpectedFilePath(tc.path)
		got := isValidationCommandScopedToPath(tc.cmd, norm)
		if got != tc.want {
			t.Errorf("isValidationCommandScopedToPath(%q, %q) = %v, want %v", tc.cmd, norm, got, tc.want)
		}
	}
}

// --- helpers ---

// readDraftContractFromTasks writes a draftContract with the given plan tasks
// (and optional pathsInScope) to a temp file and calls readDraftContract.
func readDraftContractFromTasks(t *testing.T, tasks []planTask, pathsInScope []string) (draftContract, error) {
	t.Helper()
	contract := minimalTestDraftContract()
	contract.PathsInScope = pathsInScope
	contract.Plan = &contractPlan{Tasks: tasks}
	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	data = append(data, '\n')
	path := filepath.Join(t.TempDir(), "contract.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	return readDraftContract(path)
}

// reviseWithBadPlan sets up a contract run, optionally sets pathsInScope, then
// tries to revise with the given tasks and returns the structured failure.
func reviseWithBadPlan(t *testing.T, tasks []planTask, pathsInScope []string) contractReviseFailure {
	t.Helper()
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// If scope is needed, set paths_in_scope first.
	if len(pathsInScope) > 0 {
		scopeDoc := map[string]any{
			"base_version": readVersionForTest(t, runPaths),
			"contract":     map[string]any{"paths_in_scope": pathsInScope},
		}
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "revise", runID, "--from", writeTempJSON(t, scopeDoc), "--json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("set paths_in_scope exited %d: %s", code, stdout.String())
		}
	}

	reviseDoc := map[string]any{
		"base_version": readVersionForTest(t, runPaths),
		"contract":     map[string]any{"plan": contractPlan{Tasks: tasks}},
	}
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", writeTempJSON(t, reviseDoc), "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit for bad plan, stdout: %s", stdout.String())
	}
	var failure contractReviseFailure
	if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
		t.Fatalf("parse failure response: %v — stdout: %s", err, stdout.String())
	}
	return failure
}

// assertPlanLoadError verifies that err is non-nil and contains the given
// error code (as a substring) and keyword.
func assertPlanLoadError(t *testing.T, err error, code, keyword string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", code)
	}
	msg := err.Error()
	if !strings.Contains(strings.ToUpper(msg), strings.ToUpper(code)) && !strings.Contains(msg, keyword) {
		t.Fatalf("expected error mentioning %q or %q, got: %s", code, keyword, msg)
	}
}

// assertReviseIssue verifies that the failure contains an issue with the given code.
func assertReviseIssue(t *testing.T, failure contractReviseFailure, code string) {
	t.Helper()
	if failure.OK {
		t.Fatalf("expected failure.OK=false")
	}
	if !failure.ContractUnchanged {
		t.Fatalf("expected failure.ContractUnchanged=true")
	}
	for _, issue := range failure.Issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("expected issue code %q, got: %+v", code, failure.Issues)
}

// minimalTestDraftContract returns a draftContract with all required fields
// populated, suitable for plan validation tests.
func minimalTestDraftContract() draftContract {
	return draftContract{
		Schema: contractSchema,
		RunID:  "run_plan_test",
		Status: "draft",
		Goal:   "plan validation test",
		Scope: draftContractScope{
			In:  []string{},
			Out: []string{},
		},
		AcceptanceCriteria: []string{},
		Validation:         draftValidation{Commands: []string{}},
		Assumptions:        []string{},
		OpenQuestions:      []string{},
		Clarifications: contractClarifySet{
			Questions: []clarifyQuestionStatus{},
		},
		MemoryContext: draftMemoryContext{
			UsedItems: []string{},
		},
	}
}

// readVersionForTest reads the current SHA-256 version of the contract.json.
func readVersionForTest(t *testing.T, runPaths contractRunPathSet) string {
	t.Helper()
	version, err := storeFileSHA256(runPaths.ContractJSON)
	if err != nil {
		t.Fatalf("read contract version: %v", err)
	}
	return version
}

// writeTempJSON marshals v to JSON and writes it to a temp file, returning the path.
func writeTempJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	path := filepath.Join(t.TempDir(), fmt.Sprintf("doc_%d.json", len(data)))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp JSON: %v", err)
	}
	return path
}
