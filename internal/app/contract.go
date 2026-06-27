package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const approvalSchema = "pactum.approval.v1alpha1"

// contractReviseIssue is a single validation failure in a revise request.
type contractReviseIssue struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// contractReviseFailure is the structured failure body emitted when revise
// input is invalid or the version/approval check rejects the request.
type contractReviseFailure struct {
	OK                bool                  `json:"ok"`
	ContractUnchanged bool                  `json:"contract_unchanged"`
	Issues            []contractReviseIssue `json:"issues"`
}

// contractPartialUpdate holds a validated partial contract update.
// A nil pointer means the field was absent in the --from document (untouched).
// A non-nil pointer (even to an empty slice) means present (replace wholesale).
type contractPartialUpdate struct {
	Goal               *string
	ScopeIn            *[]string
	ScopeOut           *[]string
	PathsInScope       *[]string
	PathsOutOfScope    *[]string
	AcceptanceCriteria *[]string
	ValidationCommands *[]string
	Assumptions        *[]string
}

// contractReviseFailureError is returned after writing a structured failure JSON
// to stdout, signaling app.Run to exit 1 without additional error output.
type contractReviseFailureError struct{}

func (contractReviseFailureError) Error() string { return "contract revise failed" }

// approvalResetGateError is returned by contractReviseWithUpdate when a
// content-changing revise on an approved contract is blocked because
// --allow-approval-reset was not passed.
type approvalResetGateError struct{}

func (approvalResetGateError) Error() string { return "approval reset required" }

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

// runStateBase is the common base shared by every load*Context helper: the
// resolved workspace root and paths, the run's artifact path set, and the
// decoded run state. It is the result of the run-context loading prefix
// (requireWorkspace -> run-not-found check -> readContractRunState) that each
// load*Context function used to inline.
type runStateBase struct {
	Root     string
	Paths    artifacts.Paths
	RunPaths contractRunPathSet
	State    contractRunState
}

type contractShowResponse struct {
	RunID     string        `json:"run_id"`
	RunStatus string        `json:"run_status"`
	Approval  approvalState `json:"approval"`
	Version   string        `json:"version,omitempty"`
	Contract  draftContract `json:"contract"`
}

type contractReviseResponse struct {
	OK                   bool          `json:"ok"`
	RunID                string        `json:"run_id"`
	RunStatus            string        `json:"run_status"`
	BaseVersion          string        `json:"base_version"`
	NewVersion           string        `json:"new_version"`
	Changed              bool          `json:"changed"`
	Deduped              []string      `json:"deduped,omitempty"`
	ApprovalReset        bool          `json:"approval_reset,omitempty"`
	PreviousApprovalHash *string       `json:"previous_approval_hash,omitempty"`
	AttemptsOrphaned     int           `json:"attempts_orphaned,omitempty"`
	Contract             draftContract `json:"contract"`
	Next                 []string      `json:"next"`
}

type contractApproveResponse struct {
	RunID     string                        `json:"run_id"`
	RunStatus string                        `json:"run_status"`
	Approval  approvalState                 `json:"approval"`
	Waivers   []contractReviewWaivedFinding `json:"waivers,omitempty"`
	Next      []string                      `json:"next"`
}

func (a App) ContractShow(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	version, err := storeFileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return err
	}
	response := contractShowResponse{
		RunID:     runID,
		RunStatus: context.State.Status,
		Approval:  context.Approval,
		Version:   version,
		Contract:  context.Contract,
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	contractMD, err := activeStore.ReadBytes(context.RunPaths.ContractMD)
	if err != nil {
		return err
	}
	writeContractShow(stdout, response, string(contractMD))
	return nil
}

func (a App) ContractRevise(stdout io.Writer, runID string, input []byte, allowApprovalReset bool, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}

	baseVersion, update, issues := parseContractReviseInput(input)
	if len(issues) > 0 {
		failure := contractReviseFailure{OK: false, ContractUnchanged: true, Issues: issues}
		if err := writeReviseFailure(stdout, failure); err != nil {
			return err
		}
		return contractReviseFailureError{}
	}

	currentVersion, err := storeFileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return err
	}
	if baseVersion != currentVersion {
		issue := contractReviseIssue{
			Field:   "base_version",
			Code:    "STALE_VERSION",
			Message: fmt.Sprintf("base_version %q does not match current version; re-read contract show --json and retry", baseVersion),
		}
		failure := contractReviseFailure{OK: false, ContractUnchanged: true, Issues: []contractReviseIssue{issue}}
		if err := writeReviseFailure(stdout, failure); err != nil {
			return err
		}
		return contractReviseFailureError{}
	}

	result, err := a.contractReviseWithUpdate(context, update, allowApprovalReset)
	if err != nil {
		var gateErr approvalResetGateError
		if errors.As(err, &gateErr) {
			issue := contractReviseIssue{
				Field:   "contract",
				Code:    "APPROVAL_RESET_REQUIRED",
				Message: "contract is approved; pass --allow-approval-reset to revise and reset approval",
			}
			failure := contractReviseFailure{OK: false, ContractUnchanged: true, Issues: []contractReviseIssue{issue}}
			if err := writeReviseFailure(stdout, failure); err != nil {
				return err
			}
			return contractReviseFailureError{}
		}
		return err
	}
	result.BaseVersion = baseVersion

	if jsonOutput {
		return writeJSONResponse(stdout, result)
	}
	writeContractRevise(stdout, result)
	return nil
}

// contractReviseWithUpdate applies a validated partial update to the contract
// and writes all affected artifacts. It is used by both the CLI (ContractRevise)
// and the internal accept path (ContractAcceptDraft), which bypasses the
// base_version check by passing allowApprovalReset=true.
func (a App) contractReviseWithUpdate(context runContext, update contractPartialUpdate, allowApprovalReset bool) (contractReviseResponse, error) {
	currentVersion, err := storeFileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return contractReviseResponse{}, err
	}

	now := a.nowUTC()
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return contractReviseResponse{}, err
	}

	contract := context.Contract
	deduped := applyContractPartialUpdate(&contract, update)
	applyClarificationStatusToContract(&contract, status)

	newVersion, err := contractVersionHash(contract)
	if err != nil {
		return contractReviseResponse{}, err
	}

	if newVersion == currentVersion {
		return contractReviseResponse{
			OK:          true,
			RunID:       context.State.RunID,
			RunStatus:   context.State.Status,
			BaseVersion: currentVersion,
			NewVersion:  currentVersion,
			Changed:     false,
			Deduped:     deduped,
			Contract:    contract,
			Next:        nextCommandsForRun(context.Paths, context.State.RunID),
		}, nil
	}

	contract.Status = "draft"

	if context.Approval.Status == "approved" && !allowApprovalReset {
		return contractReviseResponse{}, approvalResetGateError{}
	}

	state := context.State
	state.Status = status.RunStatus
	state.UpdatedAt = now

	if err := writeContractArtifacts(context.RunPaths, contract); err != nil {
		return contractReviseResponse{}, err
	}
	if err := writeJSON(context.RunPaths.RunJSON, state); err != nil {
		return contractReviseResponse{}, err
	}

	_, approvalReset, prevHash, err := resetApprovalIfApproved(context.Paths, context.RunPaths, context.Root, context.State.RunID, now)
	if err != nil {
		return contractReviseResponse{}, err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "contract_revised", Timestamp: now, RunID: context.State.RunID}); err != nil {
		return contractReviseResponse{}, err
	}

	var attemptsOrphaned int
	if approvalReset {
		attemptsOrphaned = countExecutionAttempts(context.RunPaths.AttemptsDir)
	}

	writtenVersion, err := storeFileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return contractReviseResponse{}, err
	}

	return contractReviseResponse{
		OK:                   true,
		RunID:                context.State.RunID,
		RunStatus:            state.Status,
		BaseVersion:          currentVersion,
		NewVersion:           writtenVersion,
		Changed:              true,
		Deduped:              deduped,
		ApprovalReset:        approvalReset,
		PreviousApprovalHash: prevHash,
		AttemptsOrphaned:     attemptsOrphaned,
		Contract:             contract,
		Next:                 nextCommandsForRun(context.Paths, context.State.RunID),
	}, nil
}

func (a App) ContractApprove(stdout io.Writer, runID string, approvedBy string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if _, err := readConfig(context.Paths.Config); err != nil {
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
		return blockingClarificationsOpenError("approve contract", runID)
	}

	// Contract-review blocking findings guard. Enforced when reviewers are currently
	// configured OR when a prior review run already produced findings.jsonl — so that
	// removing reviewers from the config after a blocking run cannot bypass the guard.
	// Fail-closed if the artifact is absent, unreadable, or malformed.
	var waivers []contractReviewWaivedFinding
	if configured, cfgErr := contractReviewersConfigured(context.Paths.Config); cfgErr != nil {
		return fmt.Errorf("cannot approve contract: cannot read reviewer config: %w", cfgErr)
	} else if configured || isRegularFile(context.RunPaths.ContractReviewFindingsJSONL) {
		var guardErr error
		waivers, guardErr = checkContractReviewFindingsApprovalGuard(context.RunPaths)
		if guardErr != nil {
			return guardErr
		}
	}

	now := a.nowUTC()
	contract := context.Contract
	applyClarificationStatusToContract(&contract, status)
	contract.Status = "approved"
	if err := writeContractArtifacts(context.RunPaths, contract); err != nil {
		return err
	}
	if err := removePromptReadinessArtifacts(context.RunPaths); err != nil {
		return err
	}
	hash, err := storeFileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return err
	}
	approval := approvedApprovalState(normalizePrincipal(context.Root, approvedBy), now, hash)
	if err := writeJSON(context.RunPaths.ApprovalJSON, approval); err != nil {
		return err
	}

	state := context.State
	state.Status = "contract_approved"
	state.UpdatedAt = now
	if err := writeJSON(context.RunPaths.RunJSON, state); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "contract_approved", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	response := contractApproveResponse{RunID: runID, RunStatus: state.Status, Approval: approval, Waivers: waivers, Next: nextCommandsForRun(context.Paths, runID)}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeContractApprove(stdout, response)
	return nil
}

// loadRunStateContext performs the run-context loading prefix shared by every
// load*Context helper: it resolves the workspace, rejects a missing run dir
// with "run not found: <id>", and decodes the run state. ok is true only on
// full success; a requireWorkspace failure or !ok returns the zero base with
// ok=false and requireWorkspace's err, and any later error propagates.
func (a App) loadRunStateContext(stdout io.Writer, runID string, jsonOutput bool) (runStateBase, bool, error) {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return runStateBase{}, false, err
	}

	runDir := filepath.Join(paths.RunsDir, runID)
	runDirExists, err := storeDirExists(runDir)
	if err != nil {
		return runStateBase{}, false, err
	}
	if !runDirExists {
		return runStateBase{}, false, runNotFoundError(runID)
	}

	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return runStateBase{}, false, err
	}
	return runStateBase{Root: root, Paths: paths, RunPaths: runPaths, State: state}, true, nil
}

// loadRunContext loads the full runContext (base plus the draft contract and
// approval state). It is the shared tail of loadMemoryContext,
// loadContractContext, and loadGateContext, which differ only in the jsonOutput
// argument forwarded to requireWorkspace.
func (a App) loadRunContext(stdout io.Writer, runID string, jsonOutput bool) (runContext, bool, error) {
	base, ok, err := a.loadRunStateContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return runContext{}, false, err
	}
	contract, err := readDraftContract(base.RunPaths.ContractJSON)
	if err != nil {
		return runContext{}, false, err
	}
	approval, err := readApprovalState(base.RunPaths.ApprovalJSON)
	if err != nil {
		return runContext{}, false, err
	}
	return runContext{
		Root:     base.Root,
		Paths:    base.Paths,
		RunPaths: base.RunPaths,
		State:    base.State,
		Contract: contract,
		Approval: approval,
	}, true, nil
}

func (a App) loadContractContext(stdout io.Writer, runID string, jsonOutput bool) (runContext, bool, error) {
	return a.loadRunContext(stdout, runID, jsonOutput)
}

// parseContractReviseInput parses and validates the --from document, collecting
// all validation issues at once. On success it returns the base_version string
// and the validated partial update; on failure it returns empty values and a
// non-empty issues slice that the caller should render as a structured failure.
func parseContractReviseInput(input []byte) (baseVersion string, update contractPartialUpdate, issues []contractReviseIssue) {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(input, &outer); err != nil {
		issues = append(issues, contractReviseIssue{
			Field:   "",
			Code:    "INVALID_JSON",
			Message: "input is not valid JSON: " + err.Error(),
		})
		return "", contractPartialUpdate{}, issues
	}

	// run_id, run_status, and approval appear in contract show --json output.
	// Accept them silently so that output can be fed directly to --from.
	showOnlyFields := map[string]bool{"run_id": true, "run_status": true, "approval": true}
	knownOuter := []string{"base_version", "version", "contract"}
	for k := range outer {
		if k != "base_version" && k != "version" && k != "contract" && !showOnlyFields[k] {
			msg := fmt.Sprintf("unknown field %q", k)
			if suggestion := didYouMean(k, knownOuter); suggestion != "" {
				msg += fmt.Sprintf("; did you mean %q?", suggestion)
			}
			issues = append(issues, contractReviseIssue{Field: k, Code: "UNKNOWN_FIELD", Message: msg})
		}
	}

	// Accept "version" as an alias for "base_version" so that contract show --json
	// output can be fed directly to --from after editing the contract sub-object.
	rawBV, hasBV := outer["base_version"]
	if !hasBV {
		rawBV, hasBV = outer["version"]
	}
	if !hasBV {
		issues = append(issues, contractReviseIssue{
			Field:   "base_version",
			Code:    "MISSING_BASE_VERSION",
			Message: "base_version is required; read contract show --json to obtain the current version",
		})
	} else {
		var bv any
		if err := json.Unmarshal(rawBV, &bv); err != nil || bv == nil {
			issues = append(issues, contractReviseIssue{
				Field:   "base_version",
				Code:    "NULL_NOT_ALLOWED",
				Message: "base_version must be a non-null string",
			})
		} else if _, ok := bv.(string); !ok {
			issues = append(issues, contractReviseIssue{
				Field:   "base_version",
				Code:    "INVALID_TYPE",
				Message: "base_version must be a string",
			})
		} else {
			baseVersion = bv.(string)
		}
	}

	rawContract, hasContract := outer["contract"]
	if !hasContract {
		issues = append(issues, contractReviseIssue{
			Field:   "contract",
			Code:    "MISSING_FIELD",
			Message: "contract is required",
		})
		return baseVersion, contractPartialUpdate{}, issues
	}
	var contractNull any
	if err := json.Unmarshal(rawContract, &contractNull); err != nil || contractNull == nil {
		issues = append(issues, contractReviseIssue{
			Field:   "contract",
			Code:    "NULL_NOT_ALLOWED",
			Message: "contract must be a non-null object",
		})
		return baseVersion, contractPartialUpdate{}, issues
	}

	var contractFields map[string]json.RawMessage
	if err := json.Unmarshal(rawContract, &contractFields); err != nil {
		issues = append(issues, contractReviseIssue{
			Field:   "contract",
			Code:    "INVALID_TYPE",
			Message: "contract must be an object",
		})
		return baseVersion, contractPartialUpdate{}, issues
	}

	showOnlyContractFields := map[string]bool{
		"schema":         true,
		"run_id":         true,
		"status":         true,
		"open_questions": true,
		"clarifications": true,
		"memory_context": true,
	}
	knownContract := []string{
		"goal",
		"scope",
		"paths_in_scope",
		"paths_out_of_scope",
		"acceptance_criteria",
		"validation",
		"assumptions",
	}
	for k := range contractFields {
		known := false
		for _, kc := range knownContract {
			if k == kc {
				known = true
				break
			}
		}
		if !known && !showOnlyContractFields[k] {
			msg := fmt.Sprintf("unknown contract field %q", k)
			if suggestion := didYouMean(k, knownContract); suggestion != "" {
				msg += fmt.Sprintf("; did you mean %q?", suggestion)
			}
			issues = append(issues, contractReviseIssue{Field: "contract." + k, Code: "UNKNOWN_FIELD", Message: msg})
		}
	}

	if raw, ok := contractFields["goal"]; ok {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil || v == nil {
			issues = append(issues, contractReviseIssue{
				Field:   "contract.goal",
				Code:    "NULL_NOT_ALLOWED",
				Message: "goal must be a non-null string",
			})
		} else if s, ok := v.(string); !ok {
			issues = append(issues, contractReviseIssue{
				Field:   "contract.goal",
				Code:    "INVALID_TYPE",
				Message: "goal must be a string",
			})
		} else {
			update.Goal = &s
		}
	}

	if raw, ok := contractFields["scope"]; ok {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil || v == nil {
			issues = append(issues, contractReviseIssue{
				Field:   "contract.scope",
				Code:    "NULL_NOT_ALLOWED",
				Message: "scope must be a non-null object",
			})
		} else {
			var scopeFields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &scopeFields); err != nil {
				issues = append(issues, contractReviseIssue{
					Field:   "contract.scope",
					Code:    "INVALID_TYPE",
					Message: "scope must be an object",
				})
			} else {
				for k := range scopeFields {
					if k != "in" && k != "out" {
						msg := fmt.Sprintf("unknown scope field %q", k)
						if suggestion := didYouMean(k, []string{"in", "out"}); suggestion != "" {
							msg += fmt.Sprintf("; did you mean %q?", suggestion)
						}
						issues = append(issues, contractReviseIssue{Field: "contract.scope." + k, Code: "UNKNOWN_FIELD", Message: msg})
					}
				}
				if rawIn, ok := scopeFields["in"]; ok {
					if list, fieldIssues := parseStringList(rawIn, "contract.scope.in"); len(fieldIssues) > 0 {
						issues = append(issues, fieldIssues...)
					} else {
						update.ScopeIn = &list
					}
				}
				if rawOut, ok := scopeFields["out"]; ok {
					if list, fieldIssues := parseStringList(rawOut, "contract.scope.out"); len(fieldIssues) > 0 {
						issues = append(issues, fieldIssues...)
					} else {
						update.ScopeOut = &list
					}
				}
			}
		}
	}

	for _, field := range []struct {
		key  string
		path string
		dest **[]string
	}{
		{"paths_in_scope", "contract.paths_in_scope", &update.PathsInScope},
		{"paths_out_of_scope", "contract.paths_out_of_scope", &update.PathsOutOfScope},
		{"acceptance_criteria", "contract.acceptance_criteria", &update.AcceptanceCriteria},
		{"assumptions", "contract.assumptions", &update.Assumptions},
	} {
		if raw, ok := contractFields[field.key]; ok {
			if list, fieldIssues := parseStringList(raw, field.path); len(fieldIssues) > 0 {
				issues = append(issues, fieldIssues...)
			} else {
				*field.dest = &list
			}
		}
	}

	if raw, ok := contractFields["validation"]; ok {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil || v == nil {
			issues = append(issues, contractReviseIssue{
				Field:   "contract.validation",
				Code:    "NULL_NOT_ALLOWED",
				Message: "validation must be a non-null object",
			})
		} else {
			var validationFields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &validationFields); err != nil {
				issues = append(issues, contractReviseIssue{
					Field:   "contract.validation",
					Code:    "INVALID_TYPE",
					Message: "validation must be an object",
				})
			} else {
				for k := range validationFields {
					if k != "commands" {
						msg := fmt.Sprintf("unknown validation field %q", k)
						if suggestion := didYouMean(k, []string{"commands"}); suggestion != "" {
							msg += fmt.Sprintf("; did you mean %q?", suggestion)
						}
						issues = append(issues, contractReviseIssue{Field: "contract.validation." + k, Code: "UNKNOWN_FIELD", Message: msg})
					}
				}
				if rawCmds, ok := validationFields["commands"]; ok {
					if list, fieldIssues := parseStringList(rawCmds, "contract.validation.commands"); len(fieldIssues) > 0 {
						issues = append(issues, fieldIssues...)
					} else {
						update.ValidationCommands = &list
					}
				}
			}
		}
	}

	return baseVersion, update, issues
}

// parseStringList parses a JSON value as a []string, rejecting null and
// non-array types and collecting all element-level issues.
func parseStringList(raw json.RawMessage, field string) ([]string, []contractReviseIssue) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return nil, []contractReviseIssue{{
			Field:   field,
			Code:    "NULL_NOT_ALLOWED",
			Message: field + " must be a non-null array (use [] to clear)",
		}}
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, []contractReviseIssue{{
			Field:   field,
			Code:    "INVALID_TYPE",
			Message: field + " must be an array",
		}}
	}
	result := make([]string, 0, len(list))
	var issues []contractReviseIssue
	for i, elem := range list {
		var s string
		if err := json.Unmarshal(elem, &s); err != nil {
			issues = append(issues, contractReviseIssue{
				Field:   fmt.Sprintf("%s[%d]", field, i),
				Code:    "INVALID_TYPE",
				Message: fmt.Sprintf("%s[%d] must be a string", field, i),
			})
			continue
		}
		result = append(result, s)
	}
	return result, issues
}

// applyContractPartialUpdate replaces fields in contract for each non-nil
// pointer in update, stable-deduping lists and reporting dropped duplicates.
func applyContractPartialUpdate(contract *draftContract, update contractPartialUpdate) []string {
	var dropped []string
	if update.Goal != nil {
		contract.Goal = *update.Goal
	}
	if update.ScopeIn != nil {
		deduped, dups := stableDedup(*update.ScopeIn)
		contract.Scope.In = deduped
		dropped = append(dropped, prefixedDropped("scope.in", dups)...)
	}
	if update.ScopeOut != nil {
		deduped, dups := stableDedup(*update.ScopeOut)
		contract.Scope.Out = deduped
		dropped = append(dropped, prefixedDropped("scope.out", dups)...)
	}
	if update.PathsInScope != nil {
		deduped, dups := stableDedup(*update.PathsInScope)
		contract.PathsInScope = deduped
		dropped = append(dropped, prefixedDropped("paths_in_scope", dups)...)
	}
	if update.PathsOutOfScope != nil {
		deduped, dups := stableDedup(*update.PathsOutOfScope)
		contract.PathsOutOfScope = deduped
		dropped = append(dropped, prefixedDropped("paths_out_of_scope", dups)...)
	}
	if update.AcceptanceCriteria != nil {
		deduped, dups := stableDedup(*update.AcceptanceCriteria)
		contract.AcceptanceCriteria = deduped
		dropped = append(dropped, prefixedDropped("acceptance_criteria", dups)...)
	}
	if update.ValidationCommands != nil {
		deduped, dups := stableDedup(*update.ValidationCommands)
		contract.Validation.Commands = deduped
		dropped = append(dropped, prefixedDropped("validation.commands", dups)...)
	}
	if update.Assumptions != nil {
		deduped, dups := stableDedup(*update.Assumptions)
		contract.Assumptions = deduped
		dropped = append(dropped, prefixedDropped("assumptions", dups)...)
	}
	return dropped
}

func prefixedDropped(field string, dups []string) []string {
	out := make([]string, len(dups))
	for i, d := range dups {
		out[i] = field + ": " + d
	}
	return out
}

// stableDedup returns (deduped, dropped): unique items in first-seen order,
// and the exact values that were dropped.
func stableDedup(items []string) ([]string, []string) {
	seen := make(map[string]bool, len(items))
	deduped := make([]string, 0, len(items))
	var dropped []string
	for _, item := range items {
		if seen[item] {
			dropped = append(dropped, item)
		} else {
			seen[item] = true
			deduped = append(deduped, item)
		}
	}
	return deduped, dropped
}

// contractVersionHash computes the SHA256 that storeFileSHA256 would return
// after writing contract with writeJSON, enabling a no-op check without disk IO.
func contractVersionHash(contract draftContract) (string, error) {
	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// countExecutionAttempts returns the number of attempt subdirectories under
// attemptsDir. It returns 0 when the directory does not exist.
func countExecutionAttempts(attemptsDir string) int {
	entries, err := activeStore.ReadDir(attemptsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}

// didYouMean returns the closest known string within Levenshtein distance 3,
// or "" if nothing is close enough.
func didYouMean(unknown string, known []string) string {
	best, bestDist := "", 4
	for _, k := range known {
		if d := levenshtein(unknown, k); d < bestDist {
			best, bestDist = k, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = del
			if ins < curr[j] {
				curr[j] = ins
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func writeReviseFailure(stdout io.Writer, failure contractReviseFailure) error {
	return writeJSONResponse(stdout, failure)
}

func writeContractArtifacts(runPaths contractRunPathSet, contract draftContract) error {
	if err := writeJSON(runPaths.ContractJSON, contract); err != nil {
		return err
	}
	if err := activeStore.WriteBytes(runPaths.ContractMD, renderContractMDFromDraft(contract), 0o644); err != nil {
		return err
	}
	return activeStore.WriteBytes(runPaths.PromptMD, renderPromptMDFromDraft(contract), 0o644)
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
		return "", contractNotApprovedError(action, contract.RunID)
	}
	hash, err := storeFileSHA256(runPaths.ContractJSON)
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
	approvedAtText := approvedAt.Format(time.RFC3339)
	return approvalState{
		Schema:         approvalSchema,
		Status:         "approved",
		ApprovedAt:     &approvedAtText,
		ApprovedBy:     &approvedBy,
		ContractSHA256: &contractSHA256,
	}
}

// resetApprovalIfApproved resets the approval to pending if it is currently
// approved, returning the new approval state, whether a reset occurred, and the
// previous contract_sha256 (nil when no reset).
func resetApprovalIfApproved(paths artifacts.Paths, runPaths contractRunPathSet, root string, runID string, resetAt time.Time) (approvalState, bool, *string, error) {
	approval, err := readApprovalState(runPaths.ApprovalJSON)
	if err != nil {
		return approvalState{}, false, nil, err
	}
	if approval.Status != "approved" {
		return approval, false, nil, nil
	}
	prevHash := approval.ContractSHA256
	pending := pendingApprovalState()
	if err := writeJSON(runPaths.ApprovalJSON, pending); err != nil {
		return approvalState{}, false, nil, err
	}
	if err := removePromptReadinessArtifacts(runPaths); err != nil {
		return approvalState{}, false, nil, err
	}
	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "contract_approval_reset", Timestamp: resetAt, RunID: runID}); err != nil {
		return approvalState{}, false, nil, err
	}
	return pending, true, prevHash, nil
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
	fmt.Fprintln(stdout, "Version:")
	fmt.Fprintf(stdout, "  base: %s\n", response.BaseVersion)
	fmt.Fprintf(stdout, "  new: %s\n", response.NewVersion)
	fmt.Fprintf(stdout, "  changed: %t\n", response.Changed)
	if response.ApprovalReset {
		fmt.Fprintln(stdout, "  approval reset: true")
	}
	if len(response.Deduped) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Deduped:")
		for _, d := range response.Deduped {
			fmt.Fprintf(stdout, "  - %s\n", d)
		}
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
	if len(response.Waivers) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "WARNING: %d operator-resolved blocking finding(s) accepted:\n", len(response.Waivers))
		for _, w := range response.Waivers {
			fmt.Fprintf(stdout, "  - %s (%s): %s (by: %s)\n", w.FindingID, w.Fingerprint, w.Reason, w.By)
		}
	}
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
