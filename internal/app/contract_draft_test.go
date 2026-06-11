package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

func TestContractDraftRecordsProposalWithoutApplying(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
	readmeBefore := mustReadFile(t, filepath.Join(root, "README.md"))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"clarify", "add", runID, "Should acceptance stay human-gated?", "--blocking"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify add exited %d, stderr: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"clarify", "answer", runID, "q_001", "Yes, keep acceptance as a separate human step."}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("clarify answer exited %d, stderr: %s", code, stderr.String())
	}
	contractBefore := mustReadFile(t, runPaths.ContractJSON)
	answersBefore := mustReadFile(t, runPaths.AnswersJSONL)
	decisionsBefore := mustReadFile(t, runPaths.DecisionsJSONL)

	writeExecutionAttemptForTest(t, runPaths, runID, "attempt_001", mustResolveExecutorForTest(t, agents.BuiltinCodex))
	app = configureHelperContractDrafters(t, app, paths, agents.BuiltinCodex, agents.BuiltinClaude)

	t.Setenv("PACTUM_CONTRACT_DRAFTER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_DRAFTER_EXPECTED_CWD", root)

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "draft", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract draft exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Contract draft proposal recorded",
		"agent: claude",
		"drafter: claude",
		"proposal: .heurema/pactum/runs/" + runID + "/contract/draft-proposal.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("contract draft output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "stdin_has_contract_drafter_prompt=") || strings.Contains(got, "drafter-stderr-line") {
		t.Fatalf("agent output leaked into stdout:\n%s", got)
	}
	if got := stderr.String(); !strings.Contains(got, "cwd_is_repo=true") || !strings.Contains(got, "stdin_has_contract_drafter_prompt=true") || !strings.Contains(got, "drafter-stderr-line") {
		t.Fatalf("live drafter output missing from stderr:\n%s", got)
	}

	if got := mustReadFile(t, runPaths.ContractJSON); got != contractBefore {
		t.Fatalf("contract draft should not apply proposal before acceptance\nbefore:\n%s\nafter:\n%s", contractBefore, got)
	}
	if got := mustReadFile(t, runPaths.AnswersJSONL); got != answersBefore {
		t.Fatalf("contract draft should not answer questions")
	}
	if got := mustReadFile(t, runPaths.DecisionsJSONL); got != decisionsBefore {
		t.Fatalf("contract draft should not write clarification decisions")
	}
	if got := mustReadFile(t, filepath.Join(root, "README.md")); got != readmeBefore {
		t.Fatalf("contract draft should not edit repository files")
	}

	proposal := readContractDraftProposalForTest(t, runPaths.ContractDraftProposalJSON)
	if proposal.Status != "pending" || proposal.Drafter != agents.BuiltinClaude || proposal.DrafterAttemptID != "drafter_attempt_001" {
		t.Fatalf("unexpected proposal metadata: %#v", proposal)
	}
	if len(proposal.InScope) != 1 || proposal.InScope[0] != "Add a contract draft command that records a proposal without applying it." {
		t.Fatalf("unexpected in-scope proposal: %#v", proposal.InScope)
	}
	if len(proposal.Acceptance) != 1 || !strings.Contains(proposal.Acceptance[0], "does not mutate contract/contract.json") {
		t.Fatalf("unexpected acceptance proposal: %#v", proposal.Acceptance)
	}
	if len(proposal.Validation) != 1 || proposal.Validation[0] != "make check" {
		t.Fatalf("unexpected validation proposal: %#v", proposal.Validation)
	}

	contextMD := mustReadFile(t, runPaths.ContractDrafterContextMD)
	if !strings.Contains(contextMD, "Answer: Yes, keep acceptance as a separate human step.") {
		t.Fatalf("drafter context missing answered clarification:\n%s", contextMD)
	}
	attemptPaths := contractDrafterAttemptPaths(runPaths, "drafter_attempt_001")
	assertFile(t, attemptPaths.RequestJSON)
	assertFile(t, attemptPaths.StdoutLog)
	assertFile(t, attemptPaths.StderrLog)
	assertFile(t, attemptPaths.ResultJSON)
	assertFile(t, runPaths.ContractDrafterLastResultJSON)

	var request contractDrafterRequestDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.RequestJSON)), &request))
	if request.Schema != contractDrafterRequestSchema || request.Drafter.Name != agents.BuiltinClaude || request.WouldRun.Stdin != contractDrafterPromptRepoPath(runID) {
		t.Fatalf("unexpected drafter request: %#v", request)
	}
	var result contractDrafterResultDocument
	assertNoError(t, json.Unmarshal([]byte(mustReadFile(t, attemptPaths.ResultJSON)), &result))
	if result.Schema != contractDrafterResultSchema || result.Drafter != agents.BuiltinClaude || result.ExitCode != 0 || result.TimedOut {
		t.Fatalf("unexpected drafter result: %#v", result)
	}
	if result.Stdout != "contract/drafter-attempts/drafter_attempt_001/stdout.log" || result.Stderr != "contract/drafter-attempts/drafter_attempt_001/stderr.log" {
		t.Fatalf("unexpected result log paths: %#v", result)
	}

	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	for _, want := range []string{"contract_drafter_attempt_started", "contract_drafter_attempt_finished", "contract_draft_proposed"} {
		if indexOfEvent(eventTypes, want) == -1 {
			t.Fatalf("events missing %s:\n%v", want, eventTypes)
		}
	}
}

func TestContractShowDraftAndAcceptAppliesProposalThroughRevision(t *testing.T) {
	root := t.TempDir()
	app, paths, runID := setupContractRun(t, root)
	runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))

	var stdout, stderr bytes.Buffer
	code := app.Run([]string{"contract", "approve", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract approve exited %d, stderr: %s", code, stderr.String())
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "contract_approved" {
		t.Fatalf("pre-draft status = %q, want contract_approved", state.Status)
	}

	app = configureHelperContractDrafters(t, app, paths, "helper")
	t.Setenv("PACTUM_CONTRACT_DRAFTER_HELPER_PROCESS", "1")
	t.Setenv("PACTUM_CONTRACT_DRAFTER_EXPECTED_CWD", root)

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "draft", runID, "--reviewer", "helper", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract draft --json exited %d, stderr: %s", code, stderr.String())
	}
	var draftResponse contractDraftResponse
	assertNoError(t, json.Unmarshal(stdout.Bytes(), &draftResponse))
	// The proposal records the engine inferred from the helper entry's model.
	if draftResponse.RunStatus != "contract_approved" || draftResponse.Proposal.Status != "pending" || draftResponse.Proposal.Drafter != "claude" {
		t.Fatalf("unexpected draft json: %#v", draftResponse)
	}
	if strings.Contains(stdout.String(), "Contract draft proposal recorded") || strings.Contains(stdout.String(), "Resolved:") {
		t.Fatalf("json output should not include human output:\n%s", stdout.String())
	}
	if contract := readContractDraft(t, runPaths.ContractJSON); contract.Goal != "add sqlite cache" || len(contract.Scope.In) != 0 {
		t.Fatalf("draft should not mutate approved contract before acceptance: %#v", contract)
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "show", "--draft", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract show-draft exited %d, stderr: %s", code, stderr.String())
	}
	for _, want := range []string{
		"Contract draft proposal",
		"## In scope",
		"Add a contract draft command that records a proposal without applying it.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("show-draft output missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = app.Run([]string{"contract", "accept", runID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("contract accept-draft exited %d, stderr: %s", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Contract draft proposal accepted") || !strings.Contains(got, "pactum contract approve "+runID) {
		t.Fatalf("accept-draft output mismatch:\n%s", got)
	}
	contract := readContractDraft(t, runPaths.ContractJSON)
	if contract.Goal != "add sqlite cache" {
		t.Fatalf("accept-draft must not change goal: %#v", contract)
	}
	if len(contract.Scope.In) != 1 || contract.Scope.In[0] != "Add a contract draft command that records a proposal without applying it." {
		t.Fatalf("accept-draft did not apply in scope: %#v", contract.Scope.In)
	}
	if len(contract.Scope.Out) != 1 || len(contract.AcceptanceCriteria) != 1 || len(contract.Validation.Commands) != 1 || len(contract.Assumptions) != 1 {
		t.Fatalf("accept-draft did not apply all proposed fields: %#v", contract)
	}
	if state := readRunState(t, runPaths.RunJSON); state.Status != "contract_draft" {
		t.Fatalf("post-accept status = %q, want contract_draft", state.Status)
	}
	if approval := readApproval(t, runPaths.ApprovalJSON); approval.Status != "pending" || approval.ContractSHA256 != nil {
		t.Fatalf("accept-draft should reset approval for separate human approval: %#v", approval)
	}
	proposal := readContractDraftProposalForTest(t, runPaths.ContractDraftProposalJSON)
	if proposal.Status != "accepted" || proposal.AcceptedAt == nil || proposal.AcceptedBy == nil || *proposal.AcceptedBy != "manual" {
		t.Fatalf("proposal should be marked accepted: %#v", proposal)
	}

	eventTypes := ledgerEventTypes(t, paths.EventsJSONL)
	for _, want := range []string{"contract_revised", "contract_approval_reset", "contract_draft_accepted"} {
		if indexOfEvent(eventTypes, want) == -1 {
			t.Fatalf("events missing %s:\n%v", want, eventTypes)
		}
	}
}

// TestContractAcceptDraftRecordsExplicitBy covers --by on contract accept: the
// trimmed principal is persisted as the proposal's accepted_by, and repo-root
// paths are sanitized before they land in the proposal artifact.
func TestContractAcceptDraftRecordsExplicitBy(t *testing.T) {
	acceptDraftWithBy := func(t *testing.T, root string, by string) (contractDraftProposalDocument, string) {
		app, paths, runID := setupContractRun(t, root)
		runPaths := contractRunPaths(filepath.Join(paths.RunsDir, runID))
		assertNoError(t, writeJSON(runPaths.ContractDraftProposalJSON, contractDraftProposalDocument{
			Schema:  contractDraftProposalSchema,
			RunID:   runID,
			Status:  "pending",
			InScope: []string{"Apply one proposed scope item."},
		}))
		var stdout, stderr bytes.Buffer
		code := app.Run([]string{"contract", "accept", runID, "--by", by}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("contract accept --by exited %d, stderr: %s", code, stderr.String())
		}
		return readContractDraftProposalForTest(t, runPaths.ContractDraftProposalJSON), runPaths.ContractDraftProposalJSON
	}

	t.Run("trims the principal", func(t *testing.T) {
		proposal, _ := acceptDraftWithBy(t, t.TempDir(), "  bob  ")
		if proposal.Status != "accepted" || proposal.AcceptedBy == nil || *proposal.AcceptedBy != "bob" {
			t.Fatalf("accepted_by mismatch: %#v", proposal)
		}
	})

	t.Run("sanitizes repo-root paths", func(t *testing.T) {
		root := t.TempDir()
		proposal, proposalPath := acceptDraftWithBy(t, root, root+"/agent")
		if proposal.AcceptedBy == nil || *proposal.AcceptedBy == "" || strings.Contains(*proposal.AcceptedBy, root) {
			t.Fatalf("accepted_by not sanitized: %#v", proposal)
		}
		assertDoesNotContainRoot(t, "contract/draft-proposal.json", mustReadFile(t, proposalPath), root)
	})
}

func configureHelperContractDrafters(t *testing.T, app App, paths artifacts.Paths, names ...string) App {
	t.Helper()
	registerTestAgents(t, paths, names...)
	app.AgentRegistry = testAgentRegistry(testHelperDescriptors(names, "TestContractDrafterHelperProcess")...)
	return app
}

func TestContractDrafterHelperProcess(t *testing.T) {
	if os.Getenv("PACTUM_CONTRACT_DRAFTER_HELPER_PROCESS") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cwd error: %v\n", err)
		os.Exit(2)
	}
	expectedCWD := os.Getenv("PACTUM_CONTRACT_DRAFTER_EXPECTED_CWD")
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	if resolved, err := filepath.EvalSymlinks(expectedCWD); err == nil {
		expectedCWD = resolved
	}
	fmt.Printf("cwd_is_repo=%t\n", cwd == expectedCWD)
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("stdin_has_contract_drafter_prompt=%t\n", strings.Contains(string(stdin), "# Contract Drafter Prompt"))
	fmt.Print(contractDrafterStructuredOutput())
	fmt.Fprintln(os.Stderr, "drafter-stderr-line")
	os.Exit(0)
}

func contractDrafterStructuredOutput() string {
	block := map[string]any{
		"schema": contractDraftProposalSchema,
		"in_scope": []string{
			"Add a contract draft command that records a proposal without applying it.",
		},
		"out_of_scope": []string{
			"Do not let the drafter change the contract goal or answer clarification questions.",
		},
		"acceptance": []string{
			"Running contract draft records a pending proposal and does not mutate contract/contract.json.",
		},
		"validation": []string{
			"make check",
		},
		"assumptions": []string{
			"Accepted draft fields are appended through the existing contract revision path.",
		},
	}
	data, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		panic(err)
	}
	return "drafter notes\n```json\n" + string(data) + "\n```\n"
}

func readContractDraftProposalForTest(t *testing.T, path string) contractDraftProposalDocument {
	t.Helper()
	proposal, err := readContractDraftProposal(path)
	assertNoError(t, err)
	return proposal
}
