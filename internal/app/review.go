package app

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	reviewSchema           = "pactum.review.v1"
	reviewFindingSchema    = "pactum.review_finding.v1"
	reviewResolutionSchema = "pactum.review_resolution.v1"

	reviewArtifact             = "review/review.json"
	reviewFindingsArtifact     = "review/findings.jsonl"
	reviewResolutionsArtifact  = "review/resolutions.jsonl"
	reviewerContextArtifact    = "review/reviewer-context.md"
	reviewerDryRunArtifact     = "review/reviewer-dry-run.json"
	reviewerDryRunSchema       = "pactum.review_dry_run.v2"
	reviewerAttemptsArtifact   = "review/reviewer-attempts"
	reviewerLastResultArtifact = "review/reviewer-last-result.json"
	reviewerRequestSchema      = "pactum.reviewer_request.v1"
	reviewerResultSchema       = "pactum.reviewer_result.v1"
	reviewerRunSchema          = "pactum.reviewer_run.v1"
)

// reviewLens is one specialist review lens. The set below is fixed in code and
// deliberately not configurable: every review spawns one reviewer attempt per
// lens, each with a prompt holding only that lens's checklist.
type reviewLens struct {
	Key       string
	Focus     string
	Heading   string
	Checklist []string
}

var reviewLenses = []reviewLens{
	{
		Key:     "correctness",
		Focus:   "correctness",
		Heading: "Correctness",
		Checklist: []string{
			"Logic errors: off-by-one, wrong operators, inverted conditions.",
			"Edge cases: empty, nil, boundary, and concurrent inputs.",
			"Error handling: no silent failures.",
			"Resource cleanup: leaks, unclosed handles.",
			"Races and deadlocks.",
		},
	},
	{
		Key:     "implementation",
		Focus:   "implementation-vs-contract",
		Heading: "Implementation vs contract",
		Checklist: []string{
			"Does the diff achieve the contract goal?",
			"Is every in-scope item and acceptance criterion covered?",
			"Is wiring and integration complete (components registered, configs updated)?",
			"Are there missing pieces that prevent the change from working end to end?",
		},
	},
	{
		Key:     "tests",
		Focus:   "test-quality",
		Heading: "Test quality",
		Checklist: []string{
			"New code paths and error paths have tests.",
			"Fake tests: always-pass tests, hardcoded-value checks, assertions on mock behavior instead of the code under test, ignored errors, commented-out cases.",
		},
	},
	{
		Key:     "over_engineering",
		Focus:   "over-engineering",
		Heading: "Over-engineering",
		Checklist: []string{
			"Wrappers that add nothing.",
			"Factories or abstractions for a single case.",
			"Premature generalization and unused extension points.",
			"Dual implementations where the old path has no callers.",
			"Silent fallbacks that hide failures.",
		},
	},
	{
		Key:     "docs",
		Focus:   "documentation",
		Heading: "Documentation",
		Checklist: []string{
			"User-visible changes missing documentation updates.",
			"Internal-only changes need no documentation; do not flag them.",
		},
	},
}

// reviewerLensPromptArtifact is the per-member, per-lens prompt path. Panel
// members run concurrently, so each (member, lens) attempt reads its own
// prompt file; registry names are unique and the lens set is fixed, which
// makes the path collision-free within a round.
func reviewerLensPromptArtifact(member string, lens reviewLens) string {
	return fmt.Sprintf("review/reviewer-prompt-%s-%s.md", member, lens.Key)
}

func reviewerLensPromptPath(runPaths contractRunPathSet, member string, lens reviewLens) string {
	return filepath.Join(runPaths.ReviewDir, fmt.Sprintf("reviewer-prompt-%s-%s.md", member, lens.Key))
}

type reviewContext struct {
	Root     string
	Paths    artifacts.Paths
	RunPaths contractRunPathSet
	State    contractRunState
}

type reviewDocument struct {
	Schema    string          `json:"schema"`
	RunID     string          `json:"run_id"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Gate      reviewGate      `json:"gate"`
	Summary   reviewSummary   `json:"summary"`
	Approval  reviewApproval  `json:"approval"`
	Artifacts reviewArtifacts `json:"artifacts"`
}

type reviewGate struct {
	Status string `json:"status"`
	Report string `json:"report"`
}

type reviewSummary struct {
	Findings     int `json:"findings"`
	Open         int `json:"open"`
	Resolved     int `json:"resolved"`
	BlockingOpen int `json:"blocking_open"`
}

type reviewApproval struct {
	ApprovedAt *string `json:"approved_at"`
	ApprovedBy *string `json:"approved_by"`
}

type reviewArtifacts struct {
	Review            string `json:"review"`
	Findings          string `json:"findings"`
	Resolutions       string `json:"resolutions"`
	Proposals         string `json:"proposals"`
	ProposalDecisions string `json:"proposal_decisions"`
	GateReport        string `json:"gate_report"`
}

type reviewFindingInput struct {
	Message  string
	Severity string
	Category string
	File     string
	Line     int
	Blocking bool
}

// findingCore is the body shared by review findings and review proposals.
// Evidence is intentionally NOT part of the core: it belongs only to reviewer
// proposals, so accepting a proposal must not carry it into a review finding.
// Confidence is empty on manual findings; reviewer proposals always carry one
// (a missing value defaults to medium at parse time).
type findingCore struct {
	Message    string `json:"message"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Blocking   bool   `json:"blocking"`
	Confidence string `json:"confidence,omitempty"`
}

type reviewFindingFingerprint struct {
	File    string
	Line    int
	Message string
}

func fingerprintReviewFinding(core findingCore) reviewFindingFingerprint {
	return reviewFindingFingerprint{
		File:    core.File,
		Line:    core.Line,
		Message: core.Message,
	}
}

type reviewFindingRecord struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	RunID  string `json:"run_id"`
	findingCore
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Source    string `json:"source"`
}

type reviewResolutionRecord struct {
	Schema    string `json:"schema"`
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	FindingID string `json:"finding_id"`
	Outcome   string `json:"outcome,omitempty"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
	Source    string `json:"source"`
}

type reviewFindingView struct {
	reviewFindingRecord
	LatestResolution *reviewResolutionRecord `json:"latest_resolution,omitempty"`
}

type reviewStateResponse struct {
	Review            reviewDocument                 `json:"review"`
	Findings          []reviewFindingView            `json:"findings"`
	Resolutions       []reviewResolutionRecord       `json:"resolutions"`
	Proposals         []reviewProposalView           `json:"proposals"`
	ProposalDecisions []reviewProposalDecisionRecord `json:"proposal_decisions"`
	ProposalSummary   reviewProposalSummary          `json:"proposal_summary"`
}

type reviewAddFindingResponse struct {
	Finding reviewFindingRecord `json:"finding"`
	State   reviewStateResponse `json:"state"`
	Next    []string            `json:"next"`
}

type reviewResolveResponse struct {
	Resolution reviewResolutionRecord `json:"resolution"`
	State      reviewStateResponse    `json:"state"`
	Next       []string               `json:"next"`
}

// reviewStateMutationResponse is the review state plus the next affordance,
// the --json response of the mutating review commands (prepare, approve).
type reviewStateMutationResponse struct {
	reviewStateResponse
	Next []string `json:"next"`
}

type reviewerDryRunPreparation struct {
	Context           reviewContext
	Contract          draftContract
	GateReport        gateReportDocument
	Review            reviewDocument
	Findings          []reviewFindingRecord
	Resolutions       []reviewResolutionRecord
	Proposals         []reviewProposalRecord
	ProposalDecisions []reviewProposalDecisionRecord
	// ReviewerName is the registry name the reviewer was invoked under;
	// Reviewer is the underlying built-in's read-only descriptor with the
	// entry's pins applied.
	ReviewerName string
	Reviewer     agents.AgentDescriptor
	ModelSpec    agents.ModelSpec
}

type reviewerDryRunDocument struct {
	Schema    string                    `json:"schema"`
	RunID     string                    `json:"run_id"`
	CreatedAt string                    `json:"created_at"`
	Reviewer  agents.AgentDescriptor    `json:"reviewer"`
	Checks    reviewerDryRunChecks      `json:"checks"`
	Attempts  []reviewerLensAttemptPlan `json:"attempts"`
}

// reviewerLensAttemptPlan is one lens attempt of the five every review spawns:
// its prompt artifact and the exact command that would run against it.
type reviewerLensAttemptPlan struct {
	Lens      string                  `json:"lens"`
	Artifacts reviewerDryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand    `json:"would_run"`
}

type reviewerDryRunChecks struct {
	ReviewPrepared   bool `json:"review_prepared"`
	GateReportReady  bool `json:"gate_report_ready"`
	ContractApproved bool `json:"contract_approved"`
}

type reviewerDryRunArtifacts struct {
	ReviewerPrompt    string `json:"reviewer_prompt"`
	ReviewerContext   string `json:"reviewer_context"`
	Review            string `json:"review"`
	Findings          string `json:"findings"`
	Resolutions       string `json:"resolutions"`
	Proposals         string `json:"proposals"`
	ProposalDecisions string `json:"proposal_decisions"`
	GateReport        string `json:"gate_report"`
}

type reviewerRequestDocument struct {
	Schema    string                  `json:"schema"`
	RunID     string                  `json:"run_id"`
	AttemptID string                  `json:"attempt_id"`
	CreatedAt string                  `json:"created_at"`
	Reviewer  agents.AgentDescriptor  `json:"reviewer"`
	Lens      string                  `json:"lens"`
	Artifacts reviewerDryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand    `json:"would_run"`
}

type reviewerResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Reviewer  string `json:"reviewer"`
	Lens      string `json:"lens"`
	processResult
}

// reviewerRunResponse aggregates the five lens attempts a single review run
// spawns for the resolved reviewer.
type reviewerRunResponse struct {
	Schema   string                   `json:"schema"`
	RunID    string                   `json:"run_id"`
	Reviewer string                   `json:"reviewer"`
	Attempts []reviewerResultDocument `json:"attempts"`
	Next     []string                 `json:"next"`
}

// reviewPlanResponse is the reviewer dry-run plan plus the next affordance;
// the plan artifact on disk stays unchanged. Running the reviewer is
// human-approved, so the plan has no safe next.
type reviewPlanResponse struct {
	reviewerDryRunDocument
	Next []string `json:"next"`
}

func (a App) ReviewPrepare(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return gateReportMissingError("prepare review", runID)
	}

	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	if err := activeStore.MkdirAll(context.RunPaths.ReviewDir); err != nil {
		return err
	}
	if err := ensureAppendOnlyFile(context.RunPaths.ReviewFindingsJSONL); err != nil {
		return err
	}
	if err := ensureAppendOnlyFile(context.RunPaths.ReviewResolutionsJSONL); err != nil {
		return err
	}

	now := a.nowUTC()
	review := newReviewDocument(runID, gateReport.Status, now.Format(time.RFC3339))
	if isRegularFile(context.RunPaths.ReviewJSON) {
		existing, err := readReviewDocument(context.RunPaths.ReviewJSON)
		if err != nil {
			return err
		}
		review = existing
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return err
	}
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, now.Format(time.RFC3339))
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_prepared", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	state := buildReviewStateWithProposals(review, findings, resolutions, proposals, proposalDecisions)
	if jsonOutput {
		return writeJSONResponse(stdout, reviewStateMutationResponse{reviewStateResponse: state, Next: nextCommandsForRun(context.Paths, runID)})
	}
	writeReviewPrepared(stdout, state)
	return nil
}

func (a App) ReviewStatus(stdout io.Writer, runID string, jsonOutput bool) error {
	state, ok, err := a.loadPreparedReviewState(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewStatus(stdout, state)
	return nil
}

func (a App) ReviewShow(stdout io.Writer, runID string, jsonOutput bool) error {
	state, ok, err := a.loadPreparedReviewState(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewShow(stdout, state)
	return nil
}

func (a App) ReviewAddFinding(stdout io.Writer, runID string, input reviewFindingInput, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.Message) == "" {
		return fmt.Errorf("review finding message is required")
	}
	if input.File != "" && filepath.IsAbs(input.File) {
		return fmt.Errorf("review finding file must be repo-relative")
	}

	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	now := a.nowUTC()
	finding := reviewFindingRecord{
		Schema: reviewFindingSchema,
		ID:     nextReviewID("f", len(findings)+1),
		RunID:  runID,
		findingCore: findingCore{
			Message:  input.Message,
			Severity: input.Severity,
			Category: input.Category,
			File:     filepath.ToSlash(input.File),
			Line:     input.Line,
			Blocking: input.Blocking,
		},
		Status:    "open",
		CreatedAt: now.Format(time.RFC3339),
		Source:    "manual",
	}
	if err := appendJSONLine(context.RunPaths.ReviewFindingsJSONL, finding); err != nil {
		return err
	}
	findings = append(findings, finding)

	resetApproval := review.Status == "approved" || review.Approval.ApprovedAt != nil || review.Approval.ApprovedBy != nil
	if resetApproval {
		review.Approval = reviewApproval{}
	}
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, now.Format(time.RFC3339))
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if resetApproval {
		if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_approval_reset", Timestamp: now, RunID: runID}); err != nil {
			return err
		}
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_added", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	response := reviewAddFindingResponse{Finding: finding, State: buildReviewState(review, findings, resolutions), Next: nextCommandsForRun(context.Paths, runID)}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewFindingAdded(stdout, response)
	return nil
}

func (a App) ReviewResolve(stdout io.Writer, runID string, findingID string, note string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	if !hasReviewFinding(findings, findingID) {
		return fmt.Errorf("review finding not found: %s", findingID)
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	resolution := reviewResolutionRecord{
		Schema:    reviewResolutionSchema,
		ID:        nextReviewID("r", len(resolutions)+1),
		RunID:     runID,
		FindingID: findingID,
		Note:      note,
		CreatedAt: now.Format(time.RFC3339),
		Source:    "manual",
	}
	if err := appendJSONLine(context.RunPaths.ReviewResolutionsJSONL, resolution); err != nil {
		return err
	}
	resolutions = append(resolutions, resolution)

	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, now.Format(time.RFC3339))
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_resolved", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	response := reviewResolveResponse{Resolution: resolution, State: buildReviewState(review, findings, resolutions), Next: nextCommandsForRun(context.Paths, runID)}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeReviewResolved(stdout, response)
	return nil
}

func (a App) ReviewApprove(stdout io.Writer, runID string, approvedBy string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	review, err := requireReviewPrepared(context.RunPaths, runID)
	if err != nil {
		return err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return gateReportMissingError("approve review", runID)
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	if gateReport.Status == "failed" {
		return fmt.Errorf("cannot approve review: gate status is failed")
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return err
	}
	summary := summarizeReview(findings, resolutions)
	if summary.BlockingOpen > 0 {
		return fmt.Errorf("cannot approve review: blocking review findings remain")
	}

	now := a.nowUTC()
	approvedBy = normalizePrincipal(context.Root, approvedBy)
	approvedAt := now.Format(time.RFC3339)
	review.Gate.Status = gateReport.Status
	review.Summary = summary
	review.Status = "approved"
	review.UpdatedAt = approvedAt
	review.Approval = reviewApproval{ApprovedAt: &approvedAt, ApprovedBy: &approvedBy}
	review.Artifacts = defaultReviewArtifacts()
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_approved", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	state := buildReviewState(review, findings, resolutions)
	if jsonOutput {
		return writeJSONResponse(stdout, reviewStateMutationResponse{reviewStateResponse: state, Next: nextCommandsForRun(context.Paths, runID)})
	}
	writeReviewApproved(stdout, state)
	return nil
}

func (a App) ReviewPlan(stdout io.Writer, runID string, reviewerName string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	prep, err := a.prepareReviewer(context, reviewerName, "prepare reviewer plan")
	if err != nil {
		return err
	}

	now := a.nowUTC()
	createdAt := now.Format(time.RFC3339)
	plan, err := buildReviewerDryRunDocument(runID, createdAt, prep.ReviewerName, prep.Reviewer)
	if err != nil {
		return err
	}
	if err := writeReviewerDryRunArtifacts(prep, plan); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "review_dry_run_prepared", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, reviewPlanResponse{reviewerDryRunDocument: plan, Next: []string{}})
	}
	writeReviewPlan(stdout, plan, prep.ReviewerName, prep.ModelSpec)
	return nil
}

func (a App) ReviewRun(stdout io.Writer, liveOutput io.Writer, runID string, reviewerName string, timeout time.Duration, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	timeout, err = resolveIdleTimeout(context.Paths.Config, timeout)
	if err != nil {
		return err
	}
	prep, err := a.prepareReviewer(context, reviewerName, "run reviewer")
	if err != nil {
		return err
	}
	plan, err := buildReviewerDryRunDocument(runID, a.nowUTC().Format(time.RFC3339), prep.ReviewerName, prep.Reviewer)
	if err != nil {
		return err
	}
	if err := writeReviewerDryRunArtifacts(prep, plan); err != nil {
		return err
	}
	reviewer := reviewLoopReviewer{Name: prep.ReviewerName, Agent: prep.Reviewer, ModelSpec: prep.ModelSpec}
	results, runErr := a.runReviewerLensAttempts(liveOutput, runID, prep, reviewer, timeout)
	next := []string{}
	if runErr == nil {
		// Recorded reviewer output is parsed into proposals; collecting them is
		// the safe next move.
		next = []string{"pactum review proposal collect " + runID}
	}
	response := reviewerRunResponse{
		Schema:   reviewerRunSchema,
		RunID:    runID,
		Reviewer: prep.ReviewerName,
		Attempts: results,
		Next:     next,
	}
	if jsonOutput {
		if err := writeJSONResponse(stdout, response); err != nil {
			return err
		}
		return runErr
	}
	writeReviewRun(stdout, response, prep.Reviewer, prep.ReviewerName, prep.ModelSpec)
	return runErr
}

func (a App) loadPreparedReviewState(stdout io.Writer, runID string, jsonOutput bool) (reviewStateResponse, bool, error) {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return reviewStateResponse{}, false, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) || !isRegularFile(context.RunPaths.ReviewJSON) {
		suggested := fmt.Sprintf("pactum review prepare %s", runID)
		return reviewStateResponse{}, false, writeNotReady(stdout, jsonOutput, runID, "Review has not been prepared. Run: "+suggested, suggested)
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	review, err := readReviewDocument(context.RunPaths.ReviewJSON)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return reviewStateResponse{}, false, err
	}
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, "")
	return buildReviewStateWithProposals(review, findings, resolutions, proposals, proposalDecisions), true, nil
}

func (a App) loadReviewContext(stdout io.Writer, runID string) (reviewContext, bool, error) {
	base, ok, err := a.loadRunStateContext(stdout, runID, false)
	if err != nil || !ok {
		return reviewContext{}, false, err
	}
	return reviewContext{
		Root:     base.Root,
		Paths:    base.Paths,
		RunPaths: base.RunPaths,
		State:    base.State,
	}, true, nil
}

// prepareReviewer loads the reviewer inputs for a prepared, approved run.
// action names the operation for error messages, e.g. "run reviewer".
func (a App) prepareReviewer(context reviewContext, reviewerName string, action string) (reviewerDryRunPreparation, error) {
	review, err := requireReviewPrepared(context.RunPaths, context.State.RunID)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return reviewerDryRunPreparation{}, gateReportMissingError(action, context.State.RunID)
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	approval, err := readApprovalState(context.RunPaths.ApprovalJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if _, err := verifyApprovedContract(context.RunPaths, contract, approval, action); err != nil {
		return reviewerDryRunPreparation{}, err
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	// An explicit --reviewer resolves a registry name; an omitted one applies
	// the cross-model rule against the registry. The entry's pins travel with
	// the name.
	entry, err := resolveReviewerEntry(config, context, reviewerName)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	review = refreshReviewDocument(review, context.State.RunID, gateReport.Status, findings, resolutions, "")
	return reviewerDryRunPreparation{
		Context:           context,
		Contract:          contract,
		GateReport:        gateReport,
		Review:            review,
		Findings:          findings,
		Resolutions:       resolutions,
		Proposals:         proposals,
		ProposalDecisions: proposalDecisions,
		ReviewerName:      resolved.Name,
		Reviewer:          resolved.Agent,
		ModelSpec:         resolved.ModelSpec,
	}, nil
}

func (a App) prepareReviewerForAgent(context reviewContext, reviewer agents.AgentDescriptor, modelSpec agents.ModelSpec, action string) (reviewerDryRunPreparation, error) {
	review, err := requireReviewPrepared(context.RunPaths, context.State.RunID)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return reviewerDryRunPreparation{}, gateReportMissingError(action, context.State.RunID)
	}
	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	approval, err := readApprovalState(context.RunPaths.ApprovalJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if _, err := verifyApprovedContract(context.RunPaths, contract, approval, action); err != nil {
		return reviewerDryRunPreparation{}, err
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	proposals, proposalDecisions, err := readReviewProposalRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	review = refreshReviewDocument(review, context.State.RunID, gateReport.Status, findings, resolutions, "")
	return reviewerDryRunPreparation{
		Context:           context,
		Contract:          contract,
		GateReport:        gateReport,
		Review:            review,
		Findings:          findings,
		Resolutions:       resolutions,
		Proposals:         proposals,
		ProposalDecisions: proposalDecisions,
		Reviewer:          reviewer,
		ModelSpec:         modelSpec,
	}, nil
}

// latestExecutionExecutorName reads the engine recorded by the run's latest
// execution attempt. Cross-model reviewer selection compares the registry
// entries' inferred engines against it.
func latestExecutionExecutorName(context reviewContext) (string, bool) {
	attempt, ok, err := loadLatestExecutionAttempt(executeReportContext{
		RunPaths: context.RunPaths,
		State:    context.State,
	})
	if err != nil || !ok {
		return "", false
	}
	var request executionRequestDocument
	if err := readJSON(attempt.Paths.RequestJSON, &request); err != nil {
		return "", false
	}
	name := strings.TrimSpace(request.Agent.Name)
	return name, name != ""
}

func writeReviewerDryRunArtifacts(prep reviewerDryRunPreparation, plan reviewerDryRunDocument) error {
	if err := writeReviewerPromptsAndContext(prep, []string{prep.ReviewerName}); err != nil {
		return err
	}
	return writeJSON(prep.Context.RunPaths.ReviewDryRunJSON, plan)
}

// writeReviewerPromptsAndContext writes the shared reviewer context plus one
// lensed prompt per (member, lens) pair, so every concurrent attempt reads its
// own prompt file.
func writeReviewerPromptsAndContext(prep reviewerDryRunPreparation, members []string) error {
	if err := activeStore.MkdirAll(prep.Context.RunPaths.ReviewDir); err != nil {
		return err
	}
	if err := activeStore.WriteBytes(prep.Context.RunPaths.ReviewContextMD, []byte(renderReviewerContext(prep)), 0o644); err != nil {
		return err
	}
	for _, member := range members {
		for _, lens := range reviewLenses {
			path := reviewerLensPromptPath(prep.Context.RunPaths, member, lens)
			if err := activeStore.WriteBytes(path, []byte(renderReviewerPrompt(prep.Context.State.RunID, lens)), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func readReviewGateReport(path string) (gateReportDocument, error) {
	var report gateReportDocument
	if err := readJSON(path, &report); err != nil {
		return gateReportDocument{}, err
	}
	return report, nil
}

func readReviewDocument(path string) (reviewDocument, error) {
	var review reviewDocument
	if err := readJSON(path, &review); err != nil {
		return reviewDocument{}, err
	}
	return review, nil
}

func requireReviewPrepared(runPaths contractRunPathSet, runID string) (reviewDocument, error) {
	if !isRegularFile(runPaths.ReviewJSON) {
		return reviewDocument{}, reviewNotPreparedError(fmt.Sprintf("review has not been prepared: %s", runID), runID)
	}
	return readReviewDocument(runPaths.ReviewJSON)
}

func readReviewRecords(runPaths contractRunPathSet) ([]reviewFindingRecord, []reviewResolutionRecord, error) {
	findings, err := readJSONLines[reviewFindingRecord](runPaths.ReviewFindingsJSONL)
	if err != nil {
		return nil, nil, err
	}
	resolutions, err := readJSONLines[reviewResolutionRecord](runPaths.ReviewResolutionsJSONL)
	if err != nil {
		return nil, nil, err
	}
	return findings, resolutions, nil
}

func ensureAppendOnlyFile(path string) error {
	return activeStore.AppendBytes(path, nil)
}

func newReviewDocument(runID string, gateStatus string, now string) reviewDocument {
	return reviewDocument{
		Schema:    reviewSchema,
		RunID:     runID,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
		Gate: reviewGate{
			Status: gateStatus,
			Report: gateReportArtifact,
		},
		Summary:   reviewSummary{},
		Approval:  reviewApproval{},
		Artifacts: defaultReviewArtifacts(),
	}
}

func refreshReviewDocument(review reviewDocument, runID string, gateStatus string, findings []reviewFindingRecord, resolutions []reviewResolutionRecord, updatedAt string) reviewDocument {
	if review.Schema == "" {
		review.Schema = reviewSchema
	}
	review.RunID = runID
	if review.CreatedAt == "" {
		if updatedAt != "" {
			review.CreatedAt = updatedAt
		} else {
			review.CreatedAt = time.Time{}.Format(time.RFC3339)
		}
	}
	if updatedAt != "" {
		review.UpdatedAt = updatedAt
	}
	review.Gate = reviewGate{
		Status: gateStatus,
		Report: gateReportArtifact,
	}
	review.Summary = summarizeReview(findings, resolutions)
	review.Artifacts = defaultReviewArtifacts()
	review.Status = reviewStatus(review.Approval, review.Summary)
	return review
}

func defaultReviewArtifacts() reviewArtifacts {
	return reviewArtifacts{
		Review:            reviewArtifact,
		Findings:          reviewFindingsArtifact,
		Resolutions:       reviewResolutionsArtifact,
		Proposals:         reviewProposalsArtifact,
		ProposalDecisions: reviewProposalDecisionsArtifact,
		GateReport:        gateReportArtifact,
	}
}

func summarizeReview(findings []reviewFindingRecord, resolutions []reviewResolutionRecord) reviewSummary {
	latest := latestReviewResolutions(resolutions)
	summary := reviewSummary{Findings: len(findings)}
	for _, finding := range findings {
		if _, ok := latest[finding.ID]; ok {
			summary.Resolved++
			continue
		}
		summary.Open++
		if finding.Blocking {
			summary.BlockingOpen++
		}
	}
	return summary
}

func reviewStatus(approval reviewApproval, summary reviewSummary) string {
	if approval.ApprovedAt != nil && approval.ApprovedBy != nil && summary.BlockingOpen == 0 {
		return "approved"
	}
	if summary.BlockingOpen > 0 {
		return "changes_requested"
	}
	return "pending"
}

func buildReviewState(review reviewDocument, findings []reviewFindingRecord, resolutions []reviewResolutionRecord) reviewStateResponse {
	return buildReviewStateWithProposals(review, findings, resolutions, nil, nil)
}

func buildReviewStateWithProposals(review reviewDocument, findings []reviewFindingRecord, resolutions []reviewResolutionRecord, proposals []reviewProposalRecord, proposalDecisions []reviewProposalDecisionRecord) reviewStateResponse {
	latest := latestReviewResolutions(resolutions)
	views := make([]reviewFindingView, 0, len(findings))
	for _, finding := range findings {
		view := reviewFindingView{reviewFindingRecord: finding}
		view.Status = "open"
		if resolution, ok := latest[finding.ID]; ok {
			view.Status = "resolved"
			resolutionCopy := resolution
			view.LatestResolution = &resolutionCopy
		}
		views = append(views, view)
	}
	if resolutions == nil {
		resolutions = []reviewResolutionRecord{}
	}
	if proposalDecisions == nil {
		proposalDecisions = []reviewProposalDecisionRecord{}
	}
	proposalViews := buildReviewProposalViews(proposals, proposalDecisions)
	return reviewStateResponse{
		Review:            review,
		Findings:          views,
		Resolutions:       resolutions,
		Proposals:         proposalViews,
		ProposalDecisions: proposalDecisions,
		ProposalSummary:   summarizeReviewProposals(proposalViews),
	}
}

func latestReviewResolutions(resolutions []reviewResolutionRecord) map[string]reviewResolutionRecord {
	latest := make(map[string]reviewResolutionRecord, len(resolutions))
	for _, resolution := range resolutions {
		latest[resolution.FindingID] = resolution
	}
	return latest
}

func hasReviewFinding(findings []reviewFindingRecord, findingID string) bool {
	for _, finding := range findings {
		if finding.ID == findingID {
			return true
		}
	}
	return false
}

func nextReviewID(prefix string, index int) string {
	return fmt.Sprintf("%s_%03d", prefix, index)
}

func buildReviewerDryRunDocument(runID string, createdAt string, member string, reviewer agents.AgentDescriptor) (reviewerDryRunDocument, error) {
	attempts := make([]reviewerLensAttemptPlan, 0, len(reviewLenses))
	for _, lens := range reviewLenses {
		plan, err := buildReviewerLensPlan(runID, member, lens, reviewer)
		if err != nil {
			return reviewerDryRunDocument{}, err
		}
		attempts = append(attempts, plan)
	}
	return reviewerDryRunDocument{
		Schema:    reviewerDryRunSchema,
		RunID:     runID,
		CreatedAt: createdAt,
		Reviewer:  agentDescriptorDocument(reviewer),
		Checks: reviewerDryRunChecks{
			ReviewPrepared:   true,
			GateReportReady:  true,
			ContractApproved: true,
		},
		Attempts: attempts,
	}, nil
}

func buildReviewerLensPlan(runID string, member string, lens reviewLens, reviewer agents.AgentDescriptor) (reviewerLensAttemptPlan, error) {
	promptArtifact := reviewerLensPromptArtifact(member, lens)
	wouldRun, err := agents.BuildCommand(reviewer, runArtifactRepoRel(runID, promptArtifact))
	if err != nil {
		return reviewerLensAttemptPlan{}, err
	}
	return reviewerLensAttemptPlan{
		Lens: lens.Key,
		Artifacts: reviewerDryRunArtifacts{
			ReviewerPrompt:    promptArtifact,
			ReviewerContext:   reviewerContextArtifact,
			Review:            reviewArtifact,
			Findings:          reviewFindingsArtifact,
			Resolutions:       reviewResolutionsArtifact,
			Proposals:         reviewProposalsArtifact,
			ProposalDecisions: reviewProposalDecisionsArtifact,
			GateReport:        gateReportArtifact,
		},
		WouldRun: agents.DryRunCommand{
			Command: wouldRun.Command,
			Args:    append([]string{}, wouldRun.Args...),
			Stdin:   wouldRun.Stdin,
		},
	}, nil
}

func reviewerAttemptPaths(runPaths contractRunPathSet, attemptID string) attemptPathSet {
	return agentAttemptPaths(runPaths.ReviewAttemptsDir, attemptID)
}

// writeReviewerMemorySection summarizes the accepted-memory prompt boundary for
// the reviewer. When the prompt manifest records memory metadata, the selected
// freshness counts are shown; otherwise the boundary is reported as not built.
func writeReviewerMemorySection(b *strings.Builder, runPaths contractRunPathSet) {
	memory, hasManifestMemory := promptManifestMemoryForReview(runPaths.PromptManifest)
	fmt.Fprintln(b, "## Accepted memory")
	if hasManifestMemory || isRegularFile(runPaths.MemoryContextMD) {
		fmt.Fprintln(b, "- Memory context: context/memory-context.md")
	}
	if hasManifestMemory {
		fmt.Fprintf(b, "- Selected items: %d\n", memory.Selected.Total)
		fmt.Fprintf(b, "- Fresh: %d\n", memory.Selected.Fresh)
		fmt.Fprintf(b, "- Stale: %d\n", memory.Selected.Stale)
		fmt.Fprintf(b, "- Unknown: %d\n", memory.Selected.Unknown)
		fmt.Fprintln(b, "- Stale memory may be outdated and must be verified.")
	} else {
		fmt.Fprintln(b, "- Memory prompt boundary: not built")
	}
	fmt.Fprintln(b)
}

func promptManifestMemoryForReview(path string) (promptManifestMemory, bool) {
	if !isRegularFile(path) {
		return promptManifestMemory{}, false
	}
	manifest, err := readPromptManifest(path)
	if err != nil || manifest.Memory == nil {
		return promptManifestMemory{}, false
	}
	return *manifest.Memory, true
}

func renderReviewerContext(prep reviewerDryRunPreparation) string {
	var b strings.Builder
	state := buildReviewStateWithProposals(prep.Review, prep.Findings, prep.Resolutions, prep.Proposals, prep.ProposalDecisions)

	fmt.Fprintln(&b, "# Reviewer Context")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Run")
	fmt.Fprintf(&b, "- Run id: %s\n", prep.Context.State.RunID)
	fmt.Fprintf(&b, "- Run status: %s\n", prep.Context.State.Status)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract")
	fmt.Fprintf(&b, "- Goal: %s\n", prep.Contract.Goal)
	writeMarkdownStringList(&b, "- In scope:", prep.Contract.Scope.In)
	writeMarkdownStringList(&b, "- Out of scope:", prep.Contract.Scope.Out)
	writeMarkdownStringList(&b, "- Acceptance criteria:", prep.Contract.AcceptanceCriteria)
	writeMarkdownStringList(&b, "- Validation commands:", prep.Contract.Validation.Commands)
	fmt.Fprintln(&b)
	writeReviewerMemorySection(&b, prep.Context.RunPaths)
	fmt.Fprintln(&b, "## Gate report")
	fmt.Fprintf(&b, "- Gate status: %s\n", prep.GateReport.Status)
	fmt.Fprintf(&b, "- Execution attempt id: %s\n", valueOrNone(prep.GateReport.Execution.AttemptID))
	fmt.Fprintf(&b, "- Execution exit code: %d\n", prep.GateReport.Execution.ExitCode)
	fmt.Fprintln(&b, "- Validation command results:")
	if len(prep.GateReport.Validation.Commands) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, command := range prep.GateReport.Validation.Commands {
			fmt.Fprintf(&b, "  - %s: %s (exit %d, timed out: %t, result: %s)\n", command.ID, command.Command, command.ExitCode, command.TimedOut, command.Result)
		}
	}
	fmt.Fprintln(&b, "- Change summary:")
	writeMarkdownIndentedStringList(&b, "changed files", prep.GateReport.Changes.ChangedFiles)
	writeMarkdownIndentedStringList(&b, "new files", prep.GateReport.Changes.NewFiles)
	writeMarkdownIndentedStringList(&b, "missing files", prep.GateReport.Changes.MissingFiles)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Existing manual review")
	fmt.Fprintf(&b, "- Review status: %s\n", state.Review.Status)
	fmt.Fprintf(&b, "- Current findings summary: findings=%d open=%d resolved=%d blocking_open=%d\n", state.Review.Summary.Findings, state.Review.Summary.Open, state.Review.Summary.Resolved, state.Review.Summary.BlockingOpen)
	fmt.Fprintln(&b, "- Existing findings:")
	if len(state.Findings) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, finding := range state.Findings {
			fmt.Fprintf(&b, "  - %s severity=%s category=%s blocking=%t status=%s: %s\n", finding.ID, finding.Severity, finding.Category, finding.Blocking, finding.Status, finding.Message)
		}
	}
	fmt.Fprintln(&b, "- Existing resolutions:")
	if len(state.Resolutions) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, resolution := range state.Resolutions {
			note := valueOrNone(resolution.Note)
			fmt.Fprintf(&b, "  - %s finding=%s source=%s note=%s\n", resolution.ID, resolution.FindingID, resolution.Source, note)
		}
	}
	fmt.Fprintf(&b, "- Proposal summary: pending=%d accepted=%d rejected=%d\n", state.ProposalSummary.Pending, state.ProposalSummary.Accepted, state.ProposalSummary.Rejected)
	fmt.Fprintln(&b, "- Existing proposals:")
	if len(state.Proposals) == 0 {
		fmt.Fprintln(&b, "  - none")
	} else {
		for _, proposal := range state.Proposals {
			fmt.Fprintf(&b, "  - %s severity=%s category=%s blocking=%t status=%s source=%s attempt=%s: %s\n", proposal.ID, proposal.Severity, proposal.Category, proposal.Blocking, proposal.Status, proposal.Source, proposal.ReviewerAttemptID, proposal.Message)
			if proposal.File != "" {
				location := proposal.File
				if proposal.Line > 0 {
					location = fmt.Sprintf("%s:%d", location, proposal.Line)
				}
				fmt.Fprintf(&b, "    location: %s\n", location)
			}
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Artifacts")
	fmt.Fprintln(&b, "- Contract: contract/contract.json")
	fmt.Fprintln(&b, "- Gate report: gate/gate-report.json")
	fmt.Fprintln(&b, "- Review: review/review.json")
	fmt.Fprintln(&b, "- Findings: review/findings.jsonl")
	fmt.Fprintln(&b, "- Resolutions: review/resolutions.jsonl")
	fmt.Fprintln(&b, "- Proposals: review/proposals.jsonl")
	fmt.Fprintln(&b, "- Proposal decisions: review/proposal-decisions.jsonl")
	fmt.Fprintln(&b, "- Execution result: execute/last-result.json")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Reviewer guidance")
	fmt.Fprintln(&b, "- This context is not complete semantic truth.")
	fmt.Fprintln(&b, "- Use `pactum search \"<term>\"` and inspect files before proposing findings.")
	fmt.Fprintln(&b, "- Do not invent changes.")
	fmt.Fprintln(&b, "- Do not approve automatically.")
	fmt.Fprintln(&b, "- If you are not certain an issue is real after verification, do not flag it.")
	return b.String()
}

func renderReviewerPrompt(runID string, lens reviewLens) string {
	reviewerContextPath := runArtifactRepoRel(runID, reviewerContextArtifact)
	contractPath := runArtifactRepoRel(runID, "contract/contract.json")
	gateReportPath := runArtifactRepoRel(runID, gateReportArtifact)
	reviewPath := runArtifactRepoRel(runID, reviewArtifact)
	findingsPath := runArtifactRepoRel(runID, reviewFindingsArtifact)
	resolutionsPath := runArtifactRepoRel(runID, reviewResolutionsArtifact)
	proposalsPath := runArtifactRepoRel(runID, reviewProposalsArtifact)
	proposalDecisionsPath := runArtifactRepoRel(runID, reviewProposalDecisionsArtifact)

	var b strings.Builder
	fmt.Fprintln(&b, "# Reviewer Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This prompt is prepared for a reviewer agent subprocess.")
	fmt.Fprintln(&b, "Pactum captures reviewer output as artifacts and may parse optional structured proposal blocks, but it does not trust reviewer output automatically.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintln(&b, "Review the executed task against the approved Pactum contract and gate report.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Inputs")
	fmt.Fprintf(&b, "- Reviewer context: %s\n", reviewerContextPath)
	fmt.Fprintf(&b, "- Contract: %s\n", contractPath)
	fmt.Fprintf(&b, "- Gate report: %s\n", gateReportPath)
	fmt.Fprintf(&b, "- Review artifacts: %s, %s, %s, %s, %s\n", reviewPath, findingsPath, resolutionsPath, proposalsPath, proposalDecisionsPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Review boundaries")
	fmt.Fprintln(&b, "- Do not apply patches.")
	fmt.Fprintln(&b, "- Do not modify files.")
	fmt.Fprintln(&b, "- Do not approve the review.")
	fmt.Fprintln(&b, "- Do not claim semantic correctness without evidence.")
	fmt.Fprintln(&b, "- Prefer concrete findings with file/path evidence.")
	fmt.Fprintln(&b, "- Read the actual file and surrounding context before proposing a finding.")
	fmt.Fprintln(&b, "- Check whether the issue is already mitigated or already represented in existing findings/proposals.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## High-signal contract")
	fmt.Fprintln(&b, "- Report a finding only when you are certain it is real after verification.")
	fmt.Fprintln(&b, "- If you are not certain an issue is real, do not flag it. False positives erode trust and waste reviewer time.")
	fmt.Fprintln(&b, "- Report problems only. No positive observations, no praise.")
	fmt.Fprintln(&b, "- Do NOT flag:")
	fmt.Fprintln(&b, "  - Style or formatting preferences.")
	fmt.Fprintln(&b, "  - Anything the contract's validation commands already catch (the gate runs them; they are listed in the reviewer context).")
	fmt.Fprintln(&b, "  - Input-dependent hypotheticals without a concrete failure path.")
	fmt.Fprintln(&b, "  - Subjective redesign suggestions.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Review lens")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "You are the %s reviewer; other lenses are covered by other reviewers running in parallel — report only findings within your lens; do not silently expand scope.\n", lens.Focus)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "### %s\n", lens.Heading)
	for _, item := range lens.Checklist {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Verify before reporting")
	fmt.Fprintln(&b, "For every candidate finding, before emitting it:")
	fmt.Fprintln(&b, "- Read the actual code at the file and line, plus 20-30 surrounding lines.")
	fmt.Fprintln(&b, "- Check whether the issue is already mitigated elsewhere.")
	fmt.Fprintln(&b, "- Check for duplicates among existing findings and proposals.")
	fmt.Fprintln(&b, "- Classify the candidate CONFIRMED or FALSE POSITIVE.")
	fmt.Fprintln(&b, "- Report only CONFIRMED findings. Discard FALSE POSITIVE candidates.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Pre-existing issues")
	fmt.Fprintln(&b, "- Issues that were present before this change are advisory: report them as non-blocking findings.")
	fmt.Fprintln(&b, "- Never mark a pre-existing issue blocking.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Output ordering")
	fmt.Fprintln(&b, "- Findings first, ordered by severity, each with file and line.")
	fmt.Fprintln(&b, "- Open questions and assumptions after the findings.")
	fmt.Fprintln(&b, "- Summary last.")
	fmt.Fprintln(&b, "- If there are no findings, say so explicitly and name residual risks or testing gaps.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Output shape")
	fmt.Fprintln(&b, "If you report findings in prose, make them easy for a human to convert manually:")
	fmt.Fprintln(&b, "- message")
	fmt.Fprintln(&b, "- severity")
	fmt.Fprintln(&b, "- category")
	fmt.Fprintln(&b, "- file")
	fmt.Fprintln(&b, "- line")
	fmt.Fprintln(&b, "- blocking")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Optional structured finding proposals")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "If you propose findings, include a fenced JSON block exactly like:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", reviewerFindingsSchema)
	fmt.Fprintln(&b, `  "findings": [`)
	fmt.Fprintln(&b, "    {")
	fmt.Fprintln(&b, `      "message": "Explain the issue clearly.",`)
	fmt.Fprintln(&b, `      "severity": "medium",`)
	fmt.Fprintln(&b, `      "category": "quality",`)
	fmt.Fprintln(&b, `      "file": "internal/app/example.go",`)
	fmt.Fprintln(&b, `      "line": 42,`)
	fmt.Fprintln(&b, `      "blocking": true,`)
	fmt.Fprintln(&b, `      "confidence": "high",`)
	fmt.Fprintln(&b, `      "evidence": "Short evidence from reviewed artifacts."`)
	fmt.Fprintln(&b, "    }")
	fmt.Fprintln(&b, "  ]")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Rules:")
	fmt.Fprintln(&b, "- Use repo-relative file paths only.")
	fmt.Fprintln(&b, "- Do not include absolute paths.")
	fmt.Fprintf(&b, "- Use severity: %s.\n", strings.Join(reviewSeverities, ", "))
	fmt.Fprintf(&b, "- Use category: %s.\n", strings.Join(reviewCategories, ", "))
	fmt.Fprintln(&b, "- Set blocking=true for findings introduced by this change that must block a merge: correctness or security bugs, or high/critical severity.")
	fmt.Fprintln(&b, "- Set blocking=false for advisory, pre-existing, or low-severity findings; they are still recorded but do not block convergence.")
	fmt.Fprintln(&b, "- If unsure whether a confirmed finding should block, set blocking=true and explain why in evidence.")
	fmt.Fprintf(&b, "- Use confidence: %s. Confidence reflects how certain you are the finding is real after verification.\n", strings.Join(reviewConfidences, ", "))
	fmt.Fprintln(&b, "- A missing confidence defaults to medium.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Important: Pactum does not trust this output automatically. A human must accept proposals.")
	return b.String()
}

func writeMarkdownStringList(b *strings.Builder, heading string, values []string) {
	fmt.Fprintln(b, heading)
	if len(values) == 0 {
		fmt.Fprintln(b, "  - none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "  - %s\n", value)
	}
}

func writeMarkdownIndentedStringList(b *strings.Builder, label string, values []string) {
	fmt.Fprintf(b, "  - %s:\n", label)
	if len(values) == 0 {
		fmt.Fprintln(b, "    - none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "    - %s\n", value)
	}
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

func writeReviewPrepared(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review prepared")
	fmt.Fprintln(stdout)
	writeReviewRunAndGate(stdout, state)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Review:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	fmt.Fprintf(stdout, "  findings: %d\n", state.Review.Summary.Findings)
	fmt.Fprintf(stdout, "  blocking open: %d\n", state.Review.Summary.BlockingOpen)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  review: %s\n", runArtifactRepoRel(state.Review.RunID, reviewArtifact))
}

func writeReviewStatus(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review status")
	fmt.Fprintln(stdout)
	writeReviewRunAndGate(stdout, state)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Review:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	fmt.Fprintf(stdout, "  findings: %d\n", state.Review.Summary.Findings)
	fmt.Fprintf(stdout, "  open: %d\n", state.Review.Summary.Open)
	fmt.Fprintf(stdout, "  resolved: %d\n", state.Review.Summary.Resolved)
	fmt.Fprintf(stdout, "  blocking open: %d\n", state.Review.Summary.BlockingOpen)
	if state.Review.Status == "approved" {
		if state.Review.Approval.ApprovedAt != nil {
			fmt.Fprintf(stdout, "  approved at: %s\n", *state.Review.Approval.ApprovedAt)
		}
		if state.Review.Approval.ApprovedBy != nil {
			fmt.Fprintf(stdout, "  approved by: %s\n", *state.Review.Approval.ApprovedBy)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Proposals:")
	fmt.Fprintf(stdout, "  pending: %d\n", state.ProposalSummary.Pending)
	fmt.Fprintf(stdout, "  accepted: %d\n", state.ProposalSummary.Accepted)
	fmt.Fprintf(stdout, "  rejected: %d\n", state.ProposalSummary.Rejected)
}

func writeReviewShow(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.Review.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Review:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Findings:")
	if len(state.Findings) == 0 {
		fmt.Fprintln(stdout, "  none")
	} else {
		for _, finding := range state.Findings {
			blocking := ""
			if finding.Blocking {
				blocking = " [blocking]"
			}
			fmt.Fprintf(stdout, "  - %s [%s]%s %s: %s\n", finding.ID, finding.Severity, blocking, finding.Category, finding.Message)
			if finding.File != "" {
				location := finding.File
				if finding.Line > 0 {
					location = fmt.Sprintf("%s:%d", location, finding.Line)
				}
				fmt.Fprintf(stdout, "    location: %s\n", location)
			}
			if finding.Confidence != "" {
				fmt.Fprintf(stdout, "    confidence: %s\n", finding.Confidence)
			}
			fmt.Fprintf(stdout, "    status: %s\n", finding.Status)
			if finding.LatestResolution != nil && finding.LatestResolution.Note != "" {
				fmt.Fprintf(stdout, "    resolution: %s\n", finding.LatestResolution.Note)
			}
		}
	}
	fmt.Fprintln(stdout)
	writeReviewPendingProposals(stdout, state.Proposals)
}

func writeReviewFindingAdded(stdout io.Writer, response reviewAddFindingResponse) {
	fmt.Fprintln(stdout, "Review finding added")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Finding.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.State.Review.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Finding:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Finding.ID)
	fmt.Fprintf(stdout, "  severity: %s\n", response.Finding.Severity)
	fmt.Fprintf(stdout, "  category: %s\n", response.Finding.Category)
	fmt.Fprintf(stdout, "  blocking: %t\n", response.Finding.Blocking)
	fmt.Fprintf(stdout, "  status: open\n")
}

func writeReviewResolved(stdout io.Writer, response reviewResolveResponse) {
	fmt.Fprintln(stdout, "Review finding resolved")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Resolution.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.State.Review.Status)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Resolution:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Resolution.ID)
	fmt.Fprintf(stdout, "  finding: %s\n", response.Resolution.FindingID)
}

func writeReviewApproved(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Review approved")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.Review.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Status)
	if state.Review.Approval.ApprovedBy != nil {
		fmt.Fprintf(stdout, "  approved by: %s\n", *state.Review.Approval.ApprovedBy)
	}
}

func writeReviewPlan(stdout io.Writer, plan reviewerDryRunDocument, reviewerName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Reviewer plan prepared")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", plan.RunID)
	fmt.Fprintln(stdout)
	writeResolved(stdout, reviewerName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Reviewer:")
	fmt.Fprintf(stdout, "  name: %s\n", plan.Reviewer.Name)
	fmt.Fprintf(stdout, "  command: %s\n", plan.Reviewer.Command)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Checks:")
	fmt.Fprintln(stdout, "  review prepared: yes")
	fmt.Fprintln(stdout, "  gate report: ready")
	fmt.Fprintln(stdout, "  contract approved: yes")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Would run (one attempt per lens):")
	for _, attempt := range plan.Attempts {
		fmt.Fprintf(stdout, "  %s: %s\n", attempt.Lens, formatAgentCommand(attempt.WouldRun))
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	for _, attempt := range plan.Attempts {
		fmt.Fprintf(stdout, "  reviewer prompt (%s): %s\n", attempt.Lens, runArtifactRepoRel(plan.RunID, attempt.Artifacts.ReviewerPrompt))
	}
	fmt.Fprintf(stdout, "  reviewer context: %s\n", runArtifactRepoRel(plan.RunID, reviewerContextArtifact))
	fmt.Fprintf(stdout, "  plan: %s\n", runArtifactRepoRel(plan.RunID, reviewerDryRunArtifact))
}

func writeReviewRun(stdout io.Writer, response reviewerRunResponse, reviewer agents.AgentDescriptor, reviewerName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Reviewer attempts finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintln(stdout)
	writeResolved(stdout, reviewerName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Reviewer:")
	fmt.Fprintf(stdout, "  name: %s\n", reviewer.Name)
	fmt.Fprintf(stdout, "  command: %s\n", reviewer.Command)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempts (one per lens):")
	for _, result := range response.Attempts {
		suffix := ""
		if result.CompletedDespiteTimeout {
			suffix = " (completed despite timeout)"
		}
		fmt.Fprintf(stdout, "  - %s [%s] exit code %d, timed out: %t%s\n", result.AttemptID, result.Lens, result.ExitCode, result.TimedOut, suffix)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  attempts: %s\n", runArtifactRepoRel(response.RunID, reviewerAttemptsArtifact))
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(response.RunID, reviewerLastResultArtifact))
}

func writeReviewRunAndGate(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.Review.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Gate:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Gate.Status)
}
