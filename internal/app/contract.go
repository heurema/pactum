package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const approvalSchema = "pactum.approval.v1"

type contractRevision struct {
	Goal              string
	AddInScope        []string
	AddOutOfScope     []string
	AddPathInScope    []string
	AddPathOutOfScope []string
	AddAcceptance     []string
	AddValidation     []string
	AddAssumption     []string
}

// runContext is the fully-loaded state of a run directory: its resolved paths,
// run state, contract draft, and approval. Shared by the contract, prompt, and
// gate commands.
type runContext struct {
	Root     string
	Paths    artifacts.Paths
	RunPaths contractRunPathSet
	State    contractRunState
	Contract draftContract
	Approval approvalState
}

type contractShowResponse struct {
	RunID     string        `json:"run_id"`
	RunStatus string        `json:"run_status"`
	Approval  approvalState `json:"approval"`
	Contract  draftContract `json:"contract"`
}

type contractReviseResponse struct {
	RunID         string        `json:"run_id"`
	RunStatus     string        `json:"run_status"`
	ApprovalReset bool          `json:"approval_reset"`
	Approval      approvalState `json:"approval"`
	Contract      draftContract `json:"contract"`
}

type contractApproveResponse struct {
	RunID     string        `json:"run_id"`
	RunStatus string        `json:"run_status"`
	Approval  approvalState `json:"approval"`
}

func (a App) ContractShow(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	response := contractShowResponse{
		RunID:     runID,
		RunStatus: context.State.Status,
		Approval:  context.Approval,
		Contract:  context.Contract,
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	contractMD, err := os.ReadFile(context.RunPaths.ContractMD)
	if err != nil {
		return err
	}
	writeContractShow(stdout, response, string(contractMD))
	return nil
}

func (a App) ContractRevise(stdout io.Writer, runID string, revision contractRevision, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !revision.hasChanges() {
		return fmt.Errorf("no contract revisions provided")
	}

	now := a.nowUTC()
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return err
	}

	contract := context.Contract
	applyContractRevision(&contract, revision)
	applyClarificationStatusToContract(&contract, status)
	contract.Status = "draft"

	state := context.State
	state.Status = status.RunStatus
	state.UpdatedAt = now

	if err := writeContractArtifacts(context.RunPaths, contract, state.MapRunID); err != nil {
		return err
	}
	if err := writeJSON(context.RunPaths.RunJSON, state); err != nil {
		return err
	}

	approval, approvalReset, err := resetApprovalIfApproved(context.Paths, context.RunPaths, context.Root, runID, now)
	if err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "contract_revised", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := contractReviseResponse{
		RunID:         runID,
		RunStatus:     state.Status,
		ApprovalReset: approvalReset,
		Approval:      approval,
		Contract:      contract,
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeContractRevise(stdout, response)
	return nil
}

func (a App) ContractApprove(stdout io.Writer, runID string, approvedBy string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return err
	}
	if status.BlockingOpen > 0 {
		if !jsonOutput {
			writeBlockingApprovalQuestions(stdout, status)
		}
		return fmt.Errorf("cannot approve contract: blocking clarification questions remain")
	}

	now := a.nowUTC()
	contract := context.Contract
	applyClarificationStatusToContract(&contract, status)
	contract.Status = "approved"
	if err := writeContractArtifacts(context.RunPaths, contract, context.State.MapRunID); err != nil {
		return err
	}
	if err := removePromptReadinessArtifacts(context.RunPaths); err != nil {
		return err
	}
	hash, err := fileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return err
	}
	approval := approvedApprovalState(approvedBy, now, hash)
	if err := writeJSON(context.RunPaths.ApprovalJSON, approval); err != nil {
		return err
	}

	state := context.State
	state.Status = "contract_approved"
	state.UpdatedAt = now
	if err := writeJSON(context.RunPaths.RunJSON, state); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "contract_approved", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := contractApproveResponse{RunID: runID, RunStatus: state.Status, Approval: approval}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeContractApprove(stdout, response)
	return nil
}

func (a App) loadContractContext(stdout io.Writer, runID string, jsonOutput bool) (runContext, bool, error) {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return runContext{}, false, err
	}

	runDir := filepath.Join(paths.RunsDir, runID)
	info, err := os.Stat(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return runContext{}, false, fmt.Errorf("run not found: %s", runID)
		}
		return runContext{}, false, err
	}
	if !info.IsDir() {
		return runContext{}, false, fmt.Errorf("run not found: %s", runID)
	}

	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return runContext{}, false, err
	}
	contract, err := readDraftContract(runPaths.ContractJSON)
	if err != nil {
		return runContext{}, false, err
	}
	approval, err := readApprovalState(runPaths.ApprovalJSON)
	if err != nil {
		return runContext{}, false, err
	}
	return runContext{
		Root:     root,
		Paths:    paths,
		RunPaths: runPaths,
		State:    state,
		Contract: contract,
		Approval: approval,
	}, true, nil
}

func (revision contractRevision) hasChanges() bool {
	return strings.TrimSpace(revision.Goal) != "" ||
		len(revision.AddInScope) > 0 ||
		len(revision.AddOutOfScope) > 0 ||
		len(revision.AddPathInScope) > 0 ||
		len(revision.AddPathOutOfScope) > 0 ||
		len(revision.AddAcceptance) > 0 ||
		len(revision.AddValidation) > 0 ||
		len(revision.AddAssumption) > 0
}

func applyContractRevision(contract *draftContract, revision contractRevision) {
	if strings.TrimSpace(revision.Goal) != "" {
		contract.Goal = revision.Goal
	}
	contract.Scope.In = append(contract.Scope.In, revision.AddInScope...)
	contract.Scope.Out = append(contract.Scope.Out, revision.AddOutOfScope...)
	contract.PathsInScope = append(contract.PathsInScope, revision.AddPathInScope...)
	contract.PathsOutOfScope = append(contract.PathsOutOfScope, revision.AddPathOutOfScope...)
	contract.AcceptanceCriteria = append(contract.AcceptanceCriteria, revision.AddAcceptance...)
	contract.Validation.Commands = append(contract.Validation.Commands, revision.AddValidation...)
	contract.Assumptions = append(contract.Assumptions, revision.AddAssumption...)
}

func writeContractArtifacts(runPaths contractRunPathSet, contract draftContract, mapRunID string) error {
	if err := writeJSON(runPaths.ContractJSON, contract); err != nil {
		return err
	}
	searchCount := readRunSearchResultCount(runPaths.SearchResults)
	if err := os.WriteFile(runPaths.ContractMD, renderContractMDFromDraft(contract, mapRunID, searchCount), 0o644); err != nil {
		return err
	}
	return os.WriteFile(runPaths.PromptMD, renderPromptMDFromDraft(contract), 0o644)
}

func applyClarificationStatusToContract(contract *draftContract, status clarifyStatusResponse) {
	contract.Clarifications = contractClarifySet{Questions: status.Questions}
	contract.OpenQuestions = openClarificationQuestionTexts(status.Questions)
}

func readApprovalState(path string) (approvalState, error) {
	var approval approvalState
	if err := readJSON(path, &approval); err != nil {
		return approvalState{}, err
	}
	if approval.Schema == "" {
		approval.Schema = approvalSchema
	}
	return approval, nil
}

// verifyApprovedContract confirms the run's contract is approved and that the
// contract on disk still hashes to the approved hash, returning that hash.
// action names the operation for error messages, e.g. "run gate".
func verifyApprovedContract(runPaths contractRunPathSet, contract draftContract, approval approvalState, action string) (string, error) {
	if contract.Status != "approved" || approval.Status != "approved" || approval.ContractSHA256 == nil {
		return "", fmt.Errorf("cannot %s: contract is not approved", action)
	}
	hash, err := fileSHA256(runPaths.ContractJSON)
	if err != nil {
		return "", err
	}
	if hash != *approval.ContractSHA256 {
		return "", fmt.Errorf("cannot %s: approved contract hash does not match current contract", action)
	}
	return hash, nil
}

func pendingApprovalState() approvalState {
	return approvalState{
		Schema:         approvalSchema,
		Status:         "pending",
		ApprovedAt:     nil,
		ApprovedBy:     nil,
		ContractSHA256: nil,
	}
}

func approvedApprovalState(approvedBy string, approvedAt time.Time, contractSHA256 string) approvalState {
	if strings.TrimSpace(approvedBy) == "" {
		approvedBy = "manual"
	}
	approvedAtText := approvedAt.Format(time.RFC3339)
	return approvalState{
		Schema:         approvalSchema,
		Status:         "approved",
		ApprovedAt:     &approvedAtText,
		ApprovedBy:     &approvedBy,
		ContractSHA256: &contractSHA256,
	}
}

func resetApprovalIfApproved(paths artifacts.Paths, runPaths contractRunPathSet, root string, runID string, resetAt time.Time) (approvalState, bool, error) {
	approval, err := readApprovalState(runPaths.ApprovalJSON)
	if err != nil {
		return approvalState{}, false, err
	}
	if approval.Status != "approved" {
		return approval, false, nil
	}
	pending := pendingApprovalState()
	if err := writeJSON(runPaths.ApprovalJSON, pending); err != nil {
		return approvalState{}, false, err
	}
	if err := removePromptReadinessArtifacts(runPaths); err != nil {
		return approvalState{}, false, err
	}
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "contract_approval_reset", Timestamp: resetAt, RunID: runID, RepoRoot: root}); err != nil {
		return approvalState{}, false, err
	}
	return pending, true, nil
}

func writeContractShow(stdout io.Writer, response contractShowResponse, contractMD string) {
	fmt.Fprintln(stdout, "Contract")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	writeApprovalSummary(stdout, response.Approval)
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, contractMD)
	if !strings.HasSuffix(contractMD, "\n") {
		fmt.Fprintln(stdout)
	}
}

func writeContractRevise(stdout io.Writer, response contractReviseResponse) {
	fmt.Fprintln(stdout, "Contract revised")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	writeApprovalSummary(stdout, response.Approval)
	if response.ApprovalReset {
		fmt.Fprintln(stdout, "  reset: true")
	}
}

func writeContractApprove(stdout io.Writer, response contractApproveResponse) {
	fmt.Fprintln(stdout, "Contract approved")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	writeApprovalSummary(stdout, response.Approval)
}

func writeApprovalSummary(stdout io.Writer, approval approvalState) {
	fmt.Fprintln(stdout, "Approval:")
	fmt.Fprintf(stdout, "  status: %s\n", approval.Status)
	if approval.ApprovedBy != nil {
		fmt.Fprintf(stdout, "  approved by: %s\n", *approval.ApprovedBy)
	}
	if approval.ApprovedAt != nil {
		fmt.Fprintf(stdout, "  approved at: %s\n", *approval.ApprovedAt)
	}
	if approval.ContractSHA256 != nil {
		fmt.Fprintf(stdout, "  contract sha256: %s\n", *approval.ContractSHA256)
	}
}

func writeBlockingApprovalQuestions(stdout io.Writer, status clarifyStatusResponse) {
	fmt.Fprintln(stdout, "Blocking clarification questions remain")
	fmt.Fprintln(stdout)
	for _, question := range status.Questions {
		if question.Status == "open" && question.Blocking {
			fmt.Fprintf(stdout, "- %s %s\n", question.ID, question.Question)
		}
	}
}
