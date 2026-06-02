package app

import (
	"fmt"
	"io"
	"os"
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

	reviewArtifact            = "review/review.json"
	reviewFindingsArtifact    = "review/findings.jsonl"
	reviewResolutionsArtifact = "review/resolutions.jsonl"
	reviewerContextArtifact   = "review/reviewer-context.md"
	reviewerPromptArtifact    = "review/reviewer-prompt.md"
	reviewerDryRunArtifact    = "review/reviewer-dry-run.json"
	reviewerDryRunSchema      = "pactum.review_dry_run.v1"
)

type reviewContext struct {
	Root     string
	Paths    artifacts.Paths
	RunDir   string
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
	Review      string `json:"review"`
	Findings    string `json:"findings"`
	Resolutions string `json:"resolutions"`
	GateReport  string `json:"gate_report"`
}

type reviewFindingInput struct {
	Message  string
	Severity string
	Category string
	File     string
	Line     int
	Blocking bool
}

type reviewFindingRecord struct {
	Schema    string `json:"schema"`
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
	Category  string `json:"category"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Blocking  bool   `json:"blocking"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Source    string `json:"source"`
}

type reviewResolutionRecord struct {
	Schema    string `json:"schema"`
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	FindingID string `json:"finding_id"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
	Source    string `json:"source"`
}

type reviewFindingView struct {
	Schema           string                  `json:"schema"`
	ID               string                  `json:"id"`
	RunID            string                  `json:"run_id"`
	Message          string                  `json:"message"`
	Severity         string                  `json:"severity"`
	Category         string                  `json:"category"`
	File             string                  `json:"file,omitempty"`
	Line             int                     `json:"line,omitempty"`
	Blocking         bool                    `json:"blocking"`
	Status           string                  `json:"status"`
	CreatedAt        string                  `json:"created_at"`
	Source           string                  `json:"source"`
	LatestResolution *reviewResolutionRecord `json:"latest_resolution,omitempty"`
}

type reviewStateResponse struct {
	Review      reviewDocument           `json:"review"`
	Findings    []reviewFindingView      `json:"findings"`
	Resolutions []reviewResolutionRecord `json:"resolutions"`
}

type reviewAddFindingResponse struct {
	Finding reviewFindingRecord `json:"finding"`
	State   reviewStateResponse `json:"state"`
}

type reviewResolveResponse struct {
	Resolution reviewResolutionRecord `json:"resolution"`
	State      reviewStateResponse    `json:"state"`
}

type reviewerDryRunPreparation struct {
	Context     reviewContext
	Contract    draftContract
	GateReport  gateReportDocument
	Review      reviewDocument
	Findings    []reviewFindingRecord
	Resolutions []reviewResolutionRecord
	Reviewer    agents.AgentDescriptor
}

type reviewerDryRunDocument struct {
	Schema    string                  `json:"schema"`
	RunID     string                  `json:"run_id"`
	CreatedAt string                  `json:"created_at"`
	Reviewer  reviewerDryRunAgent     `json:"reviewer"`
	Checks    reviewerDryRunChecks    `json:"checks"`
	Artifacts reviewerDryRunArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand    `json:"would_run"`
}

type reviewerDryRunAgent struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Input   string   `json:"input"`
}

type reviewerDryRunChecks struct {
	ReviewPrepared   bool `json:"review_prepared"`
	GateReportReady  bool `json:"gate_report_ready"`
	ContractApproved bool `json:"contract_approved"`
}

type reviewerDryRunArtifacts struct {
	ReviewerPrompt  string `json:"reviewer_prompt"`
	ReviewerContext string `json:"reviewer_context"`
	Review          string `json:"review"`
	Findings        string `json:"findings"`
	Resolutions     string `json:"resolutions"`
	GateReport      string `json:"gate_report"`
}

func (a App) ReviewPrepare(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return fmt.Errorf("cannot prepare review: gate report not found")
	}

	gateReport, err := readReviewGateReport(context.RunPaths.GateReportJSON)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(context.RunPaths.ReviewDir, 0o755); err != nil {
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
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, now.Format(time.RFC3339))
	if err := writeJSON(context.RunPaths.ReviewJSON, review); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_prepared", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	state := buildReviewState(review, findings, resolutions)
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewPrepared(stdout, state)
	return nil
}

func (a App) ReviewStatus(stdout io.Writer, runID string, jsonOutput bool) error {
	state, ok, err := a.loadPreparedReviewState(stdout, runID)
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
	state, ok, err := a.loadPreparedReviewState(stdout, runID)
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
		Schema:    reviewFindingSchema,
		ID:        nextReviewID("f", len(findings)+1),
		RunID:     runID,
		Message:   input.Message,
		Severity:  input.Severity,
		Category:  input.Category,
		File:      filepath.ToSlash(input.File),
		Line:      input.Line,
		Blocking:  input.Blocking,
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
		if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_approval_reset", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
			return err
		}
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_added", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := reviewAddFindingResponse{Finding: finding, State: buildReviewState(review, findings, resolutions)}
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
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_finding_resolved", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := reviewResolveResponse{Resolution: resolution, State: buildReviewState(review, findings, resolutions)}
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
		return fmt.Errorf("cannot approve review: gate report not found")
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
	if strings.TrimSpace(approvedBy) == "" {
		approvedBy = "manual"
	}
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
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_approved", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	state := buildReviewState(review, findings, resolutions)
	if jsonOutput {
		return writeJSONResponse(stdout, state)
	}
	writeReviewApproved(stdout, state)
	return nil
}

func (a App) ReviewDryRun(stdout io.Writer, runID string, reviewerName string, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	prep, err := a.prepareReviewerDryRun(context, reviewerName)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	createdAt := now.Format(time.RFC3339)
	plan := buildReviewerDryRunDocument(runID, createdAt, prep.Reviewer)
	if err := os.MkdirAll(context.RunPaths.ReviewDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(context.RunPaths.ReviewContextMD, []byte(renderReviewerContext(prep)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(context.RunPaths.ReviewPromptMD, []byte(renderReviewerPrompt(runID)), 0o644); err != nil {
		return err
	}
	if err := writeJSON(context.RunPaths.ReviewDryRunJSON, plan); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "review_dry_run_prepared", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	if jsonOutput {
		return writeJSONResponse(stdout, plan)
	}
	writeReviewDryRun(stdout, plan)
	return nil
}

func (a App) loadPreparedReviewState(stdout io.Writer, runID string) (reviewStateResponse, bool, error) {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return reviewStateResponse{}, false, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) || !isRegularFile(context.RunPaths.ReviewJSON) {
		writeReviewNotPrepared(stdout, runID)
		return reviewStateResponse{}, false, nil
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
	review = refreshReviewDocument(review, runID, gateReport.Status, findings, resolutions, "")
	return buildReviewState(review, findings, resolutions), true, nil
}

func (a App) loadReviewContext(stdout io.Writer, runID string) (reviewContext, bool, error) {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return reviewContext{}, false, err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return reviewContext{}, false, nil
	}

	paths := artifacts.New(root)
	runDir := filepath.Join(paths.RunsDir, runID)
	info, err := os.Stat(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return reviewContext{}, false, fmt.Errorf("run not found: %s", runID)
		}
		return reviewContext{}, false, err
	}
	if !info.IsDir() {
		return reviewContext{}, false, fmt.Errorf("run not found: %s", runID)
	}

	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return reviewContext{}, false, err
	}
	return reviewContext{
		Root:     root,
		Paths:    paths,
		RunDir:   runDir,
		RunPaths: runPaths,
		State:    state,
	}, true, nil
}

func (a App) prepareReviewerDryRun(context reviewContext, reviewerName string) (reviewerDryRunPreparation, error) {
	review, err := requireReviewPrepared(context.RunPaths, context.State.RunID)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if !isRegularFile(context.RunPaths.GateReportJSON) {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot prepare reviewer dry-run: gate report not found")
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
	if contract.Status != "approved" || approval.Status != "approved" || approval.ContractSHA256 == nil {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot prepare reviewer dry-run: contract is not approved")
	}
	hash, err := fileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if hash != *approval.ContractSHA256 {
		return reviewerDryRunPreparation{}, fmt.Errorf("cannot prepare reviewer dry-run: contract is not approved")
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	reviewer, err := a.agentRegistry().ResolveReviewer(reviewerName)
	if err != nil {
		return reviewerDryRunPreparation{}, err
	}
	if reviewer.Input != agents.InputPromptFile {
		return reviewerDryRunPreparation{}, fmt.Errorf("unsupported agent input mode: %s", reviewer.Input)
	}

	review = refreshReviewDocument(review, context.State.RunID, gateReport.Status, findings, resolutions, "")
	return reviewerDryRunPreparation{
		Context:     context,
		Contract:    contract,
		GateReport:  gateReport,
		Review:      review,
		Findings:    findings,
		Resolutions: resolutions,
		Reviewer:    reviewer,
	}, nil
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
		return reviewDocument{}, fmt.Errorf("review has not been prepared: %s", runID)
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
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
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
		Review:      reviewArtifact,
		Findings:    reviewFindingsArtifact,
		Resolutions: reviewResolutionsArtifact,
		GateReport:  gateReportArtifact,
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
	latest := latestReviewResolutions(resolutions)
	views := make([]reviewFindingView, 0, len(findings))
	for _, finding := range findings {
		view := reviewFindingView{
			Schema:    finding.Schema,
			ID:        finding.ID,
			RunID:     finding.RunID,
			Message:   finding.Message,
			Severity:  finding.Severity,
			Category:  finding.Category,
			File:      finding.File,
			Line:      finding.Line,
			Blocking:  finding.Blocking,
			Status:    "open",
			CreatedAt: finding.CreatedAt,
			Source:    finding.Source,
		}
		if resolution, ok := latest[finding.ID]; ok {
			view.Status = "resolved"
			resolutionCopy := resolution
			view.LatestResolution = &resolutionCopy
		}
		views = append(views, view)
	}
	return reviewStateResponse{
		Review:      review,
		Findings:    views,
		Resolutions: resolutions,
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

func buildReviewerDryRunDocument(runID string, createdAt string, reviewer agents.AgentDescriptor) reviewerDryRunDocument {
	agentArgs := append([]string{}, reviewer.Args...)
	wouldRunArgs := append([]string{}, reviewer.Args...)
	wouldRunArgs = append(wouldRunArgs, "--", runArtifactRepoRel(runID, reviewerPromptArtifact))
	return reviewerDryRunDocument{
		Schema:    reviewerDryRunSchema,
		RunID:     runID,
		CreatedAt: createdAt,
		Reviewer: reviewerDryRunAgent{
			Name:    reviewer.Name,
			Command: reviewer.Command,
			Args:    agentArgs,
			Input:   reviewer.Input,
		},
		Checks: reviewerDryRunChecks{
			ReviewPrepared:   true,
			GateReportReady:  true,
			ContractApproved: true,
		},
		Artifacts: reviewerDryRunArtifacts{
			ReviewerPrompt:  reviewerPromptArtifact,
			ReviewerContext: reviewerContextArtifact,
			Review:          reviewArtifact,
			Findings:        reviewFindingsArtifact,
			Resolutions:     reviewResolutionsArtifact,
			GateReport:      gateReportArtifact,
		},
		WouldRun: agents.DryRunCommand{
			Command: reviewer.Command,
			Args:    wouldRunArgs,
		},
	}
}

func renderReviewerContext(prep reviewerDryRunPreparation) string {
	var b strings.Builder
	state := buildReviewState(prep.Review, prep.Findings, prep.Resolutions)

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
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Artifacts")
	fmt.Fprintln(&b, "- Contract: contract/contract.json")
	fmt.Fprintln(&b, "- Gate report: gate/gate-report.json")
	fmt.Fprintln(&b, "- Review: review/review.json")
	fmt.Fprintln(&b, "- Findings: review/findings.jsonl")
	fmt.Fprintln(&b, "- Resolutions: review/resolutions.jsonl")
	fmt.Fprintln(&b, "- Execution result: execute/last-result.json")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Reviewer guidance")
	fmt.Fprintln(&b, "- This context is not complete semantic truth.")
	fmt.Fprintln(&b, "- Use `pactum search \"<term>\"` and inspect files before proposing findings.")
	fmt.Fprintln(&b, "- Do not invent changes.")
	fmt.Fprintln(&b, "- Do not approve automatically.")
	fmt.Fprintln(&b, "- If uncertain, propose a blocking finding that asks for clarification.")
	return b.String()
}

func renderReviewerPrompt(runID string) string {
	reviewerContextPath := runArtifactRepoRel(runID, reviewerContextArtifact)
	contractPath := runArtifactRepoRel(runID, "contract/contract.json")
	gateReportPath := runArtifactRepoRel(runID, gateReportArtifact)
	reviewPath := runArtifactRepoRel(runID, reviewArtifact)
	findingsPath := runArtifactRepoRel(runID, reviewFindingsArtifact)
	resolutionsPath := runArtifactRepoRel(runID, reviewResolutionsArtifact)

	var b strings.Builder
	fmt.Fprintln(&b, "# Reviewer Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This prompt is prepared for a future reviewer agent.")
	fmt.Fprintln(&b, "Pactum does not execute reviewer agents in this milestone.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintln(&b, "Review the executed task against the approved Pactum contract and gate report.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Inputs")
	fmt.Fprintf(&b, "- Reviewer context: %s\n", reviewerContextPath)
	fmt.Fprintf(&b, "- Contract: %s\n", contractPath)
	fmt.Fprintf(&b, "- Gate report: %s\n", gateReportPath)
	fmt.Fprintf(&b, "- Review artifacts: %s, %s, %s\n", reviewPath, findingsPath, resolutionsPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Review boundaries")
	fmt.Fprintln(&b, "- Do not apply patches.")
	fmt.Fprintln(&b, "- Do not modify files.")
	fmt.Fprintln(&b, "- Do not approve the review.")
	fmt.Fprintln(&b, "- Do not claim semantic correctness without evidence.")
	fmt.Fprintln(&b, "- Prefer concrete findings with file/path evidence.")
	fmt.Fprintln(&b, "- If uncertain, recommend a blocking manual finding.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Expected future output")
	fmt.Fprintln(&b, "Future reviewer output should be converted into manual review findings:")
	fmt.Fprintln(&b, "- message")
	fmt.Fprintln(&b, "- severity")
	fmt.Fprintln(&b, "- category")
	fmt.Fprintln(&b, "- file")
	fmt.Fprintln(&b, "- line")
	fmt.Fprintln(&b, "- blocking")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This PR does not parse reviewer output.")
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

func writeReviewNotPrepared(stdout io.Writer, runID string) {
	fmt.Fprintf(stdout, "Review has not been prepared. Run: pactum review prepare %s\n", runID)
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
		return
	}
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
		fmt.Fprintf(stdout, "    status: %s\n", finding.Status)
		if finding.LatestResolution != nil && finding.LatestResolution.Note != "" {
			fmt.Fprintf(stdout, "    resolution: %s\n", finding.LatestResolution.Note)
		}
	}
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

func writeReviewDryRun(stdout io.Writer, plan reviewerDryRunDocument) {
	fmt.Fprintln(stdout, "Reviewer dry-run prepared")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", plan.RunID)
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
	fmt.Fprintln(stdout, "Would run:")
	fmt.Fprintf(stdout, "  %s\n", formatAgentCommand(plan.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  reviewer prompt: %s\n", runArtifactRepoRel(plan.RunID, reviewerPromptArtifact))
	fmt.Fprintf(stdout, "  reviewer context: %s\n", runArtifactRepoRel(plan.RunID, reviewerContextArtifact))
	fmt.Fprintf(stdout, "  dry run: %s\n", runArtifactRepoRel(plan.RunID, reviewerDryRunArtifact))
}

func writeReviewRunAndGate(stdout io.Writer, state reviewStateResponse) {
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", state.Review.RunID)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Gate:")
	fmt.Fprintf(stdout, "  status: %s\n", state.Review.Gate.Status)
}
