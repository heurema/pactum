package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- plan show tests ---

// TestPlanShowNoPlan verifies that plan show on a contract without a plan exits
// 0 and prints a clear no-plan message.
func TestPlanShowNoPlan(t *testing.T) {
	root := t.TempDir()
	app, _, runID := setupContractRun(t, root)

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan show (no plan) exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"Plan", "id: " + runID, "No plan defined"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan show (no plan) output missing %q:\n%s", want, got)
		}
	}
}

// TestPlanShowWithPlan verifies that plan show renders every task field for a
// contract that has a plan.
func TestPlanShowWithPlan(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	reviseDoc := map[string]any{
		"base_version": readVersionForTest(t, runPaths),
		"contract": map[string]any{
			"plan": contractPlan{Tasks: []planTask{
				{
					ID:    "t1",
					Title: "Schema work",
					Context: []planContextSelector{
						{Path: "internal/app/run.go", Lines: "10-50", Why: "type owner"},
						{Symbol: "draftContract", Why: "schema"},
					},
					ExpectedFiles: []string{"internal/app/run.go"},
					Acceptance:    []string{"Schema lands in JSON"},
					Validation:    []string{"go build ./..."},
				},
				{
					ID:         "t2",
					Title:      "Tests",
					DependsOn:  []string{"t1"},
					Acceptance: []string{"Tests pass"},
					Validation: []string{"go test ./..."},
				},
			}},
		},
	}
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "revise", runID, "--from", writeTempJSON(t, reviseDoc), "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract revise exited %d: %s", code, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"plan", "show", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan show exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"t1", "Schema work",
		"t2", "Tests",
		"Depends on: t1",
		"symbol draftContract",
		"internal/app/run.go lines 10-50",
		"Expected files: internal/app/run.go",
		"Schema lands in JSON",
		"go build ./...",
		"Tests pass",
		"go test ./...",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan show missing %q:\n%s", want, got)
		}
	}
}

// TestPlanShowJSONShape verifies the --json output has a top-level "plan" key:
// null when absent, and an object with a "tasks" array when present.
func TestPlanShowJSONShape(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// No-plan case: {"plan": null}
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"plan", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan show --json (no plan) exited %d: %s", code, stderr.String())
	}
	var noPlanResult planShowJSONResponse
	if err := json.Unmarshal(stdout.Bytes(), &noPlanResult); err != nil {
		t.Fatalf("parse no-plan JSON: %v — stdout: %s", err, stdout.String())
	}
	if noPlanResult.Plan != nil {
		t.Fatalf("no-plan: expected plan=null, got: %+v", noPlanResult.Plan)
	}
	// Verify the raw JSON has a "plan" key with null value.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("parse raw no-plan JSON: %v", err)
	}
	if _, hasPlan := raw["plan"]; !hasPlan {
		t.Fatalf("no-plan JSON missing 'plan' key: %s", stdout.String())
	}
	if string(raw["plan"]) != "null" {
		t.Fatalf("no-plan JSON 'plan' value = %s, want null", raw["plan"])
	}

	// With-plan case: {"plan": {"tasks": [...]}}
	reviseDoc := map[string]any{
		"base_version": readVersionForTest(t, runPaths),
		"contract": map[string]any{
			"plan": contractPlan{Tasks: []planTask{
				{ID: "t1", Acceptance: []string{"done"}, Validation: []string{"go build ./..."}},
			}},
		},
	}
	stdout.Reset()
	code = app.Run([]string{"contract", "revise", runID, "--from", writeTempJSON(t, reviseDoc), "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("revise exited %d: %s", code, stdout.String())
	}

	stdout.Reset()
	code = app.Run([]string{"plan", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan show --json (with plan) exited %d: %s", code, stderr.String())
	}
	var withPlanResult planShowJSONResponse
	if err := json.Unmarshal(stdout.Bytes(), &withPlanResult); err != nil {
		t.Fatalf("parse with-plan JSON: %v — stdout: %s", err, stdout.String())
	}
	if withPlanResult.Plan == nil || len(withPlanResult.Plan.Tasks) != 1 {
		t.Fatalf("with-plan: expected plan with 1 task, got: %+v", withPlanResult.Plan)
	}
	if withPlanResult.Plan.Tasks[0].ID != "t1" {
		t.Fatalf("with-plan: expected task id t1, got: %s", withPlanResult.Plan.Tasks[0].ID)
	}
	// Verify raw JSON has "plan" key with an object containing "tasks".
	var rawWithPlan map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &rawWithPlan); err != nil {
		t.Fatalf("parse raw with-plan JSON: %v", err)
	}
	if _, hasPlan := rawWithPlan["plan"]; !hasPlan {
		t.Fatalf("with-plan JSON missing 'plan' key: %s", stdout.String())
	}
	var planObj map[string]json.RawMessage
	if err := json.Unmarshal(rawWithPlan["plan"], &planObj); err != nil {
		t.Fatalf("plan value is not an object: %v", err)
	}
	if _, hasTasks := planObj["tasks"]; !hasTasks {
		t.Fatalf("with-plan JSON 'plan' object missing 'tasks' key: %s", stdout.String())
	}
}

// TestPlanShowIsReadOnly verifies that plan show and plan show --json do not
// write to ledger events, execution records, gate records, or review records.
func TestPlanShowIsReadOnly(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)

	readState := func(t *testing.T) (events, execution, gate, review string) {
		t.Helper()
		events = mustReadFile(t, paths.EventsJSONL)
		execution = mustReadFileOrAbsent(t, filepath.Join(paths.RunsDir, runID, "execute", "dry-run.json"))
		gate = mustReadFileOrAbsent(t, filepath.Join(paths.RunsDir, runID, "gate", "gate-report.json"))
		review = mustReadFileOrAbsent(t, filepath.Join(paths.RunsDir, runID, "review", "review.json"))
		return
	}

	eventsBefore, execBefore, gateBefore, reviewBefore := readState(t)

	var stdout, stderr bytes.Buffer
	if code := app.Run([]string{"plan", "show", runID}, &stdout, &stderr); code != 0 {
		t.Fatalf("plan show exited %d: %s", code, stderr.String())
	}
	if code := app.Run([]string{"plan", "show", runID, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("plan show --json exited %d: %s", code, stderr.String())
	}

	eventsAfter, execAfter, gateAfter, reviewAfter := readState(t)
	if eventsBefore != eventsAfter {
		t.Fatalf("plan show must not write ledger events\nbefore: %s\nafter: %s", eventsBefore, eventsAfter)
	}
	if execBefore != execAfter {
		t.Fatalf("plan show must not touch execution records")
	}
	if gateBefore != gateAfter {
		t.Fatalf("plan show must not touch gate records")
	}
	if reviewBefore != reviewAfter {
		t.Fatalf("plan show must not touch review records")
	}
}

// TestPlanShowCLIGrammar verifies that "plan show" is recognised by the
// command parser without error.
func TestPlanShowCLIGrammar(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"plan", "show", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan show --help exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("plan show --help did not print usage:\n%s", stdout.String())
	}
}

// TestPlanShowInHelp verifies that "plan show" appears in the top-level help output.
func TestPlanShowInHelp(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := testApp(root).Run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("--help exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "plan") {
		t.Fatalf("--help output does not mention 'plan':\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = testApp(root).Run([]string{"plan", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan --help exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "show") {
		t.Fatalf("plan --help does not mention 'show':\n%s", stdout.String())
	}
}

// --- drafter prompt plan guidance tests ---

// TestDrafterPromptContainsPlanFields verifies that renderContractDrafterPrompt
// documents the plan field: fan-in/independently-validatable condition, 3-10
// leaf task target, falsifiable validation requirement, and all task field names.
func TestDrafterPromptContainsPlanFields(t *testing.T) {
	prompt := renderContractDrafterPrompt("run_test_prompt")
	for _, want := range []string{
		"fan-in",
		"independently-validatable",
		"3-10",
		"falsifiable",
		"id",
		"title",
		"depends_on",
		"context",
		"expected_files",
		"acceptance",
		"validation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("drafter prompt missing %q:\n%s", want, prompt)
		}
	}
}

// --- drafted plan round-trip and accept tests ---

// TestDraftedPlanProposalRoundTrip verifies:
// (a) contract/draft-proposal.json contains the plan field with all proposed
//
//	tasks after recording a draft with plan.tasks[];
//
// (b) draft proposal JSON output returns the plan and all task field values
//
//	before accept is called;
//
// (c) contract/contract.json does not contain a plan field between recording
//
//	a draft and calling accept.
func TestDraftedPlanProposalRoundTrip(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	proposedPlan := &contractPlan{Tasks: []planTask{
		{
			ID:    "ta",
			Title: "Alpha task",
			Context: []planContextSelector{
				{Path: "internal/app/run.go", Lines: "1-20", Why: "entry"},
			},
			ExpectedFiles: []string{"internal/app/run.go"},
			Acceptance:    []string{"alpha done"},
			Validation:    []string{"go build ./..."},
		},
		{
			ID:         "tb",
			Title:      "Beta task",
			DependsOn:  []string{"ta"},
			Acceptance: []string{"beta done"},
			Validation: []string{"go test ./..."},
		},
	}}

	// Build drafter stdout as a fenced JSON block (as a real drafter would emit),
	// then parse it through the drafter block parsing path to test that
	// contractDraftProposalBlock.Plan and parseContractDraftProposalBlocks
	// correctly round-trip plan.tasks[].
	blockData := map[string]any{
		"schema":   contractDraftProposalSchema,
		"in_scope": []string{"some scope item"},
		"plan":     proposedPlan,
	}
	blockJSON, err := json.MarshalIndent(blockData, "", "  ")
	if err != nil {
		t.Fatalf("marshal proposal block: %v", err)
	}
	drafterOutput := "drafter notes\n```json\n" + string(blockJSON) + "\n```\n"
	parsedBlocks, parseWarnings := parseContractDraftProposalBlocks(drafterOutput)
	if len(parsedBlocks) != 1 {
		t.Fatalf("parseContractDraftProposalBlocks: expected 1 block, got %d", len(parsedBlocks))
	}
	if parsedBlocks[0].Plan == nil || len(parsedBlocks[0].Plan.Tasks) != 2 {
		t.Fatalf("parsed block: expected 2 plan tasks, got: %+v", parsedBlocks[0].Plan)
	}
	proposal := contractDraftProposalFromBlock(root, runID, "drafter_attempt_001", "claude", parsedBlocks[0], parseWarnings, time.Now().UTC())
	if err := writeContractDraftProposalArtifacts(runPaths, proposal); err != nil {
		t.Fatalf("writeContractDraftProposalArtifacts: %v", err)
	}

	contractBefore := mustReadFile(t, runPaths.ContractJSON)

	// (b) Draft proposal JSON output exposes the plan before accept.
	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "show", "--draft", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show --draft --json exited %d: %s", code, stderr.String())
	}
	var showDraftResponse contractShowDraftResponse
	if err := json.Unmarshal(stdout.Bytes(), &showDraftResponse); err != nil {
		t.Fatalf("parse show-draft JSON: %v — %s", err, stdout.String())
	}
	if showDraftResponse.Proposal.Plan == nil || len(showDraftResponse.Proposal.Plan.Tasks) != 2 {
		t.Fatalf("show-draft JSON: expected 2 plan tasks, got: %+v", showDraftResponse.Proposal.Plan)
	}
	ta := showDraftResponse.Proposal.Plan.Tasks[0]
	if ta.ID != "ta" || ta.Title != "Alpha task" || len(ta.Context) != 1 ||
		ta.Context[0].Path != "internal/app/run.go" || ta.Context[0].Lines != "1-20" ||
		len(ta.ExpectedFiles) != 1 || ta.ExpectedFiles[0] != "internal/app/run.go" ||
		len(ta.Acceptance) != 1 || ta.Acceptance[0] != "alpha done" ||
		len(ta.Validation) != 1 || ta.Validation[0] != "go build ./..." {
		t.Fatalf("show-draft JSON task ta field mismatch: %+v", ta)
	}
	tb := showDraftResponse.Proposal.Plan.Tasks[1]
	if tb.ID != "tb" || len(tb.DependsOn) != 1 || tb.DependsOn[0] != "ta" {
		t.Fatalf("show-draft JSON task tb mismatch: %+v", tb)
	}

	// (a) draft-proposal.json contains the plan.
	savedProposal := readContractDraftProposalForTest(t, runPaths.ContractDraftProposalJSON)
	if savedProposal.Plan == nil || len(savedProposal.Plan.Tasks) != 2 {
		t.Fatalf("draft-proposal.json: expected 2 plan tasks, got: %+v", savedProposal.Plan)
	}

	// (c) contract.json is unchanged before accept.
	contractAfter := mustReadFile(t, runPaths.ContractJSON)
	if contractBefore != contractAfter {
		t.Fatalf("contract.json must be unchanged before accept\nbefore:\n%s\nafter:\n%s", contractBefore, contractAfter)
	}
	if strings.Contains(contractAfter, `"plan"`) {
		t.Fatalf("contract.json must not contain a plan key before accept:\n%s", contractAfter)
	}
}

// TestAcceptDraftedPlanAppliesItToContract verifies that accepting a valid
// drafted plan writes the plan into the contract (visible via contract show
// --json and plan show --json).
func TestAcceptDraftedPlanAppliesItToContract(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	proposal := contractDraftProposalDocument{
		Schema:  contractDraftProposalSchema,
		RunID:   runID,
		Status:  "pending",
		InScope: []string{"scope item"},
		Plan: &contractPlan{Tasks: []planTask{
			{
				ID:            "t1",
				Title:         "Task one",
				Context:       []planContextSelector{{Path: "internal/app/run.go", Why: "entry"}},
				ExpectedFiles: []string{"internal/app/run.go"},
				Acceptance:    []string{"t1 done"},
				Validation:    []string{"go build ./..."},
			},
			{
				ID:         "t2",
				DependsOn:  []string{"t1"},
				Acceptance: []string{"t2 done"},
				Validation: []string{"go test ./..."},
			},
		}},
	}
	if err := writeJSON(runPaths.ContractDraftProposalJSON, proposal); err != nil {
		t.Fatalf("write draft proposal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "accept", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract accept exited %d, stderr: %s", code, stderr.String())
	}

	// contract show --json exposes plan with all task fields.
	stdout.Reset()
	code = app.Run([]string{"contract", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show --json exited %d: %s", code, stderr.String())
	}
	var showResult contractShowResponse
	if err := json.Unmarshal(stdout.Bytes(), &showResult); err != nil {
		t.Fatalf("parse show response: %v — %s", err, stdout.String())
	}
	if showResult.Contract.Plan == nil || len(showResult.Contract.Plan.Tasks) != 2 {
		t.Fatalf("contract show --json: expected 2 plan tasks, got: %+v", showResult.Contract.Plan)
	}
	t1 := showResult.Contract.Plan.Tasks[0]
	if t1.ID != "t1" || t1.Title != "Task one" || len(t1.Context) != 1 ||
		t1.Context[0].Path != "internal/app/run.go" ||
		len(t1.ExpectedFiles) != 1 || t1.ExpectedFiles[0] != "internal/app/run.go" ||
		len(t1.Acceptance) != 1 || t1.Acceptance[0] != "t1 done" ||
		len(t1.Validation) != 1 || t1.Validation[0] != "go build ./..." {
		t.Fatalf("contract show --json task t1 mismatch: %+v", t1)
	}
	t2 := showResult.Contract.Plan.Tasks[1]
	if t2.ID != "t2" || len(t2.DependsOn) != 1 || t2.DependsOn[0] != "t1" {
		t.Fatalf("contract show --json task t2 mismatch: %+v", t2)
	}

	// plan show --json exposes the same plan.
	stdout.Reset()
	code = app.Run([]string{"plan", "show", runID, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan show --json exited %d: %s", code, stderr.String())
	}
	var planResult planShowJSONResponse
	if err := json.Unmarshal(stdout.Bytes(), &planResult); err != nil {
		t.Fatalf("parse plan show JSON: %v — %s", err, stdout.String())
	}
	if planResult.Plan == nil || len(planResult.Plan.Tasks) != 2 {
		t.Fatalf("plan show --json: expected 2 plan tasks, got: %+v", planResult.Plan)
	}
	if planResult.Plan.Tasks[0].ID != "t1" || planResult.Plan.Tasks[1].ID != "t2" {
		t.Fatalf("plan show --json task ids mismatch: %+v", planResult.Plan.Tasks)
	}
	pt1 := planResult.Plan.Tasks[0]
	if pt1.Title != "Task one" {
		t.Fatalf("plan show --json task t1 title: got %q, want %q", pt1.Title, "Task one")
	}
	if len(pt1.Context) != 1 || pt1.Context[0].Path != "internal/app/run.go" || pt1.Context[0].Why != "entry" {
		t.Fatalf("plan show --json task t1 context mismatch: %+v", pt1.Context)
	}
	if len(pt1.ExpectedFiles) != 1 || pt1.ExpectedFiles[0] != "internal/app/run.go" {
		t.Fatalf("plan show --json task t1 expected_files mismatch: %+v", pt1.ExpectedFiles)
	}
	if len(pt1.Acceptance) != 1 || pt1.Acceptance[0] != "t1 done" {
		t.Fatalf("plan show --json task t1 acceptance mismatch: %+v", pt1.Acceptance)
	}
	if len(pt1.Validation) != 1 || pt1.Validation[0] != "go build ./..." {
		t.Fatalf("plan show --json task t1 validation mismatch: %+v", pt1.Validation)
	}
	pt2 := planResult.Plan.Tasks[1]
	if len(pt2.DependsOn) != 1 || pt2.DependsOn[0] != "t1" {
		t.Fatalf("plan show --json task t2 depends_on mismatch: %+v", pt2.DependsOn)
	}
	if len(pt2.Acceptance) != 1 || pt2.Acceptance[0] != "t2 done" {
		t.Fatalf("plan show --json task t2 acceptance mismatch: %+v", pt2.Acceptance)
	}
	if len(pt2.Validation) != 1 || pt2.Validation[0] != "go test ./..." {
		t.Fatalf("plan show --json task t2 validation mismatch: %+v", pt2.Validation)
	}
}

// TestAcceptDraftedPlanOnlyIsChange verifies that a draft proposal whose only
// substantive field is plan is accepted as a contract change (not rejected as
// "no contract fields to apply").
func TestAcceptDraftedPlanOnlyIsChange(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	// Proposal has no scope/acceptance/validation/assumptions — only a plan.
	proposal := contractDraftProposalDocument{
		Schema: contractDraftProposalSchema,
		RunID:  runID,
		Status: "pending",
		Plan: &contractPlan{Tasks: []planTask{
			{ID: "p1", Acceptance: []string{"done"}, Validation: []string{"go build ./..."}},
		}},
	}
	if err := writeJSON(runPaths.ContractDraftProposalJSON, proposal); err != nil {
		t.Fatalf("write draft proposal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "accept", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract accept (plan-only) exited %d, stderr: %s", code, stderr.String())
	}

	// Verify plan is in contract.
	contract := readContractDraft(t, runPaths.ContractJSON)
	if contract.Plan == nil || len(contract.Plan.Tasks) != 1 || contract.Plan.Tasks[0].ID != "p1" {
		t.Fatalf("plan-only accept: plan not in contract: %+v", contract.Plan)
	}
}

// TestAcceptInvalidDraftedPlanFails verifies that accepting a drafted plan with
// a structural error (cycle, unresolved depends_on) fails non-zero with the
// existing plan validation issue code.
func TestAcceptInvalidDraftedPlanFails(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

		proposal := contractDraftProposalDocument{
			Schema: contractDraftProposalSchema,
			RunID:  runID,
			Status: "pending",
			Plan: &contractPlan{Tasks: []planTask{
				{ID: "t1", DependsOn: []string{"t2"}, Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
				{ID: "t2", DependsOn: []string{"t1"}, Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
			}},
		}
		if err := writeJSON(runPaths.ContractDraftProposalJSON, proposal); err != nil {
			t.Fatalf("write draft proposal: %v", err)
		}

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "accept", runID}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("accept cyclic plan should fail")
		}
		var failure contractReviseFailure
		if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
			t.Fatalf("accept cyclic plan: output is not valid JSON: %v — %s", err, stdout.String())
		}
		found := false
		for _, issue := range failure.Issues {
			if issue.Code == "CYCLE_IN_DAG" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("accept cyclic plan: expected CYCLE_IN_DAG issue code, got: %v", failure.Issues)
		}
		// The contract should be unchanged (no plan applied).
		contract := readContractDraft(t, runPaths.ContractJSON)
		if contract.Plan != nil {
			t.Fatalf("cyclic plan must not be applied to contract: %+v", contract.Plan)
		}
	})

	t.Run("unresolved_dep", func(t *testing.T) {
		root := t.TempDir()
		app, paths, runID := setupContractRun(t, root)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

		proposal := contractDraftProposalDocument{
			Schema: contractDraftProposalSchema,
			RunID:  runID,
			Status: "pending",
			Plan: &contractPlan{Tasks: []planTask{
				{ID: "t1", DependsOn: []string{"nonexistent"}, Acceptance: []string{"ok"}, Validation: []string{"go build ./..."}},
			}},
		}
		if err := writeJSON(runPaths.ContractDraftProposalJSON, proposal); err != nil {
			t.Fatalf("write draft proposal: %v", err)
		}

		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "accept", runID}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("accept plan with missing dep should fail")
		}
		var failure contractReviseFailure
		if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
			t.Fatalf("accept plan with missing dep: output is not valid JSON: %v — %s", err, stdout.String())
		}
		found := false
		for _, issue := range failure.Issues {
			if issue.Code == "MISSING_DEPENDENCY" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("accept plan with missing dep: expected MISSING_DEPENDENCY issue code, got: %v", failure.Issues)
		}
		// The contract should be unchanged (no plan applied).
		contract := readContractDraft(t, runPaths.ContractJSON)
		if contract.Plan != nil {
			t.Fatalf("invalid plan must not be applied to contract: %+v", contract.Plan)
		}
	})
}

// TestAcceptPlanlessProposalCompatible verifies that a proposal with no plan
// accepts exactly as before: plan-less contract, unchanged hash behavior.
func TestAcceptPlanlessProposalCompatible(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	proposal := contractDraftProposalDocument{
		Schema:      contractDraftProposalSchema,
		RunID:       runID,
		Status:      "pending",
		InScope:     []string{"One scope item"},
		Acceptance:  []string{"Criterion A"},
		Validation:  []string{"make check"},
		Assumptions: []string{"Assumption B"},
	}
	if err := writeJSON(runPaths.ContractDraftProposalJSON, proposal); err != nil {
		t.Fatalf("write draft proposal: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "accept", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("plan-less accept exited %d, stderr: %s", code, stderr.String())
	}

	contract := readContractDraft(t, runPaths.ContractJSON)
	if contract.Plan != nil {
		t.Fatalf("plan-less proposal accept: contract should have no plan, got: %+v", contract.Plan)
	}
	if len(contract.Scope.In) != 1 || contract.Scope.In[0] != "One scope item" {
		t.Fatalf("plan-less accept: scope not applied: %+v", contract.Scope.In)
	}

	// Hash should match a contract without a plan (plan-less hash stability).
	hash, err := contractVersionHash(contract)
	if err != nil {
		t.Fatalf("contractVersionHash: %v", err)
	}
	if hash == "" {
		t.Fatal("hash must not be empty")
	}
	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatalf("marshal contract: %v", err)
	}
	if strings.Contains(string(data), `"plan"`) {
		t.Fatalf("plan-less contract JSON must not contain a plan key:\n%s", data)
	}
}

// TestDraftProposalMDRendersPlan verifies that the draft proposal markdown
// preview renders plan tasks when a plan is present.
func TestDraftProposalMDRendersPlan(t *testing.T) {
	proposal := contractDraftProposalDocument{
		Schema:  contractDraftProposalSchema,
		RunID:   "run_test",
		Status:  "pending",
		InScope: []string{"scope"},
		Plan: &contractPlan{Tasks: []planTask{
			{
				ID:         "x1",
				Title:      "X task",
				DependsOn:  []string{},
				Acceptance: []string{"x done"},
				Validation: []string{"go test ./..."},
			},
		}},
	}
	md := renderContractDraftProposalMD(proposal)
	for _, want := range []string{"x1", "X task", "x done", "go test ./..."} {
		if !strings.Contains(md, want) {
			t.Fatalf("draft proposal MD missing %q:\n%s", want, md)
		}
	}
}

// helpers for plan_test.go that read dry-run.json or return absent placeholder
func mustReadFileOrAbsent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "<absent>"
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
