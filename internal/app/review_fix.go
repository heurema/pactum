package app

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
)

const (
	reviewFixDirArtifact        = "review/fix"
	reviewFixContextArtifact    = "review/fix/fixer-context.md"
	reviewFixPromptArtifact     = "review/fix/fixer-prompt.md"
	reviewFixDryRunArtifact     = "review/fix/fixer-dry-run.json"
	reviewFixAttemptsArtifact   = "review/fix/attempts"
	reviewFixLastResultArtifact = "review/fix/last-result.json"
	reviewFixDryRunSchema       = "pactum.review_fix_dry_run.v1"
	reviewFixRequestSchema      = "pactum.review_fix_request.v1"
	reviewFixResultSchema       = "pactum.review_fix_result.v1"
	reviewFixOutcomesSchema     = "pactum.review_fix_outcomes.v1"
)

type reviewFixPreparation struct {
	Context     reviewContext
	Contract    draftContract
	Review      reviewDocument
	Findings    []reviewFindingRecord
	Resolutions []reviewResolutionRecord
	// FixerName is the registry name the fixer was invoked under; Fixer is the
	// underlying built-in's descriptor with the entry's pins applied.
	FixerName string
	Fixer     agents.AgentDescriptor
	ModelSpec agents.ModelSpec
}

type reviewFixDryRunDocument struct {
	Schema    string                 `json:"schema"`
	RunID     string                 `json:"run_id"`
	CreatedAt string                 `json:"created_at"`
	Fixer     agents.AgentDescriptor `json:"fixer"`
	Checks    reviewFixChecks        `json:"checks"`
	Artifacts reviewFixArtifacts     `json:"artifacts"`
	WouldRun  agents.DryRunCommand   `json:"would_run"`
}

type reviewFixChecks struct {
	ReviewPrepared   bool `json:"review_prepared"`
	FindingsReady    bool `json:"findings_ready"`
	ContractApproved bool `json:"contract_approved"`
}

type reviewFixArtifacts struct {
	FixerPrompt  string `json:"fixer_prompt"`
	FixerContext string `json:"fixer_context"`
	Review       string `json:"review"`
	Findings     string `json:"findings"`
	Resolutions  string `json:"resolutions"`
	Contract     string `json:"contract"`
}

type reviewFixRequestDocument struct {
	Schema    string                 `json:"schema"`
	RunID     string                 `json:"run_id"`
	AttemptID string                 `json:"attempt_id"`
	CreatedAt string                 `json:"created_at"`
	Fixer     agents.AgentDescriptor `json:"fixer"`
	Artifacts reviewFixArtifacts     `json:"artifacts"`
	WouldRun  agents.DryRunCommand   `json:"would_run"`
}

type reviewFixResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Fixer     string `json:"fixer"`
	processResult
}

func (a App) ReviewFix(stdout io.Writer, liveOutput io.Writer, runID string, agentName string, timeout time.Duration, confirm bool, jsonOutput bool) error {
	context, ok, err := a.loadReviewContext(stdout, runID)
	if err != nil || !ok {
		return err
	}
	prep, err := a.prepareReviewFixer(context, agentName)
	if err != nil {
		return err
	}

	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[reviewFixDryRunDocument, reviewFixRequestDocument, reviewFixResultDocument, struct{}]{
		Stdout:           stdout,
		LiveOutput:       liveOutput,
		JSONOutput:       jsonOutput,
		Confirm:          confirm,
		CancelMessage:    "review fix cancelled",
		Root:             context.Root,
		EventsJSONL:      context.Paths.EventsJSONL,
		RunID:            runID,
		Stage:            "fix",
		AttemptsDir:      context.RunPaths.ReviewFixAttemptsDir,
		AttemptIDPrefix:  "attempt",
		LastResultJSON:   context.RunPaths.ReviewFixLastResultJSON,
		AgentName:        prep.FixerName,
		Agent:            prep.Fixer,
		Model:            prep.ModelSpec,
		PromptRepoPath:   reviewFixPromptRepoPath(runID),
		ArtifactDir:      reviewFixAttemptsArtifact,
		Timeout:          timeout,
		WritePathAllowed: contractWritePathAllowed(prep.Contract),
		StartedEvent:     "review_fix_attempt_started",
		FinishedEvent:    "review_fix_attempt_finished",
		ExitKind:         "review fix",
		TimeoutMessage: func(timeout time.Duration) string {
			return fmt.Sprintf("review fix process produced no output for %s", timeout)
		},
		Prepare: func(createdAt string) (reviewFixDryRunDocument, error) {
			return ensureReviewFixDryRunArtifacts(prep, createdAt)
		},
		BuildRequest: func(context agentAttemptContext[reviewFixDryRunDocument]) (reviewFixRequestDocument, error) {
			return reviewFixRequestDocument{
				Schema:    reviewFixRequestSchema,
				RunID:     runID,
				AttemptID: context.AttemptID,
				CreatedAt: context.CreatedAt,
				Fixer:     agentDescriptorDocument(prep.Fixer),
				Artifacts: context.Prepared.Artifacts,
				WouldRun:  context.Prepared.WouldRun,
			}, nil
		},
		BuildResult: func(context agentAttemptContext[reviewFixDryRunDocument], runResult agents.RunResult) reviewFixResultDocument {
			return reviewFixResultDocument{
				Schema:        reviewFixResultSchema,
				RunID:         runID,
				AttemptID:     context.AttemptID,
				Fixer:         prep.Fixer.Name,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result reviewFixResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(stdout io.Writer, request reviewFixRequestDocument, result reviewFixResultDocument) {
			writeReviewFixRun(stdout, request, result, prep.FixerName, prep.ModelSpec)
		},
	})
}

func (a App) prepareReviewFixer(context reviewContext, agentName string) (reviewFixPreparation, error) {
	review, err := requireReviewPrepared(context.RunPaths, context.State.RunID)
	if err != nil {
		return reviewFixPreparation{}, err
	}
	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return reviewFixPreparation{}, err
	}
	approval, err := readApprovalState(context.RunPaths.ApprovalJSON)
	if err != nil {
		return reviewFixPreparation{}, err
	}
	if _, err := verifyApprovedContract(context.RunPaths, contract, approval, "run review fix"); err != nil {
		return reviewFixPreparation{}, err
	}
	findings, resolutions, err := readReviewRecords(context.RunPaths)
	if err != nil {
		return reviewFixPreparation{}, err
	}
	if len(findings) == 0 {
		return reviewFixPreparation{}, fmt.Errorf("cannot run review fix: no review findings found")
	}
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return reviewFixPreparation{}, err
	}
	// The fixer is an executor-stage agent: an explicit --agent resolves a
	// registry name, an omitted one defaults to the first registry entry, and
	// the entry's pins travel with the name.
	entry, err := resolveExecutorEntry(config, agentName)
	if err != nil {
		return reviewFixPreparation{}, err
	}
	resolved, err := a.resolveAgentForRole(entry, agentRoleExecutor)
	if err != nil {
		return reviewFixPreparation{}, err
	}

	review = refreshReviewDocument(review, context.State.RunID, review.Gate.Status, findings, resolutions, "")
	return reviewFixPreparation{
		Context:     context,
		Contract:    contract,
		Review:      review,
		Findings:    findings,
		Resolutions: resolutions,
		FixerName:   resolved.Name,
		Fixer:       resolved.Agent,
		ModelSpec:   resolved.ModelSpec,
	}, nil
}

func ensureReviewFixDryRunArtifacts(prep reviewFixPreparation, createdAt string) (reviewFixDryRunDocument, error) {
	expected, err := buildReviewFixDryRunDocument(prep.Context.State.RunID, createdAt, prep.Fixer)
	if err != nil {
		return reviewFixDryRunDocument{}, err
	}
	// The fixer prompt and context inline the current findings and contract, both
	// of which are mutable between attempts, so always regenerate them rather than
	// reusing a prior attempt's artifacts — reuse would feed the fixer a stale
	// finding set.
	if err := writeReviewFixDryRunArtifacts(prep, expected); err != nil {
		return reviewFixDryRunDocument{}, err
	}
	return expected, nil
}

func writeReviewFixDryRunArtifacts(prep reviewFixPreparation, plan reviewFixDryRunDocument) error {
	if err := activeStore.MkdirAll(prep.Context.RunPaths.ReviewFixDir); err != nil {
		return err
	}
	if err := activeStore.WriteBytes(prep.Context.RunPaths.ReviewFixContextMD, []byte(renderReviewFixContext(prep)), 0o644); err != nil {
		return err
	}
	if err := activeStore.WriteBytes(prep.Context.RunPaths.ReviewFixPromptMD, []byte(renderReviewFixPrompt(prep)), 0o644); err != nil {
		return err
	}
	return writeJSON(prep.Context.RunPaths.ReviewFixDryRunJSON, plan)
}

func buildReviewFixDryRunDocument(runID string, createdAt string, fixer agents.AgentDescriptor) (reviewFixDryRunDocument, error) {
	wouldRun, err := agents.BuildCommand(fixer, reviewFixPromptRepoPath(runID))
	if err != nil {
		return reviewFixDryRunDocument{}, err
	}
	return reviewFixDryRunDocument{
		Schema:    reviewFixDryRunSchema,
		RunID:     runID,
		CreatedAt: createdAt,
		Fixer: agents.AgentDescriptor{
			Name:    fixer.Name,
			Command: fixer.Command,
			Args:    append([]string{}, fixer.Args...),
			Input:   fixer.Input,
		},
		Checks: reviewFixChecks{
			ReviewPrepared:   true,
			FindingsReady:    true,
			ContractApproved: true,
		},
		Artifacts: reviewFixArtifacts{
			FixerPrompt:  reviewFixPromptArtifact,
			FixerContext: reviewFixContextArtifact,
			Review:       reviewArtifact,
			Findings:     reviewFindingsArtifact,
			Resolutions:  reviewResolutionsArtifact,
			Contract:     "contract/contract.json",
		},
		WouldRun: agents.DryRunCommand{
			Command: wouldRun.Command,
			Args:    append([]string{}, wouldRun.Args...),
			Stdin:   wouldRun.Stdin,
		},
	}, nil
}

func reviewFixPromptRepoPath(runID string) string {
	return runArtifactRepoRel(runID, reviewFixPromptArtifact)
}

func renderReviewFixContext(prep reviewFixPreparation) string {
	var b strings.Builder
	state := buildReviewState(prep.Review, prep.Findings, prep.Resolutions)

	fmt.Fprintln(&b, "# Review Fixer Context")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Run")
	fmt.Fprintf(&b, "- Run id: %s\n", prep.Context.State.RunID)
	fmt.Fprintf(&b, "- Run status: %s\n", prep.Context.State.Status)
	fmt.Fprintln(&b)
	writeReviewFixContractSection(&b, prep.Contract)
	fmt.Fprintln(&b)
	writeReviewFixFindingsSection(&b, state)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Artifacts")
	fmt.Fprintln(&b, "- Contract: contract/contract.json")
	fmt.Fprintln(&b, "- Review: review/review.json")
	fmt.Fprintln(&b, "- Findings: review/findings.jsonl")
	fmt.Fprintln(&b, "- Resolutions: review/resolutions.jsonl")
	fmt.Fprintln(&b, "- Gate report: gate/gate-report.json")
	fmt.Fprintln(&b, "- Execution result: execute/last-result.json")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Fixer guidance")
	fmt.Fprintln(&b, "- Source files are the source of truth.")
	fmt.Fprintln(&b, "- Use `pactum search \"<term>\"` and inspect current source files before relying on this context.")
	fmt.Fprintln(&b, "- For each current review finding, trace the finding to the code.")
	fmt.Fprintln(&b, "- If a finding is valid, fix it in place within the approved contract scope.")
	fmt.Fprintln(&b, "- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.")
	fmt.Fprintln(&b, "- Do not approve the review or mutate review findings/resolutions/proposals.")
	fmt.Fprintln(&b, "- Do not modify generated `.heurema` artifacts.")
	return b.String()
}

func renderReviewFixPrompt(prep reviewFixPreparation) string {
	fixerContextPath := runArtifactRepoRel(prep.Context.State.RunID, reviewFixContextArtifact)
	contractPath := runArtifactRepoRel(prep.Context.State.RunID, "contract/contract.json")
	reviewPath := runArtifactRepoRel(prep.Context.State.RunID, reviewArtifact)
	findingsPath := runArtifactRepoRel(prep.Context.State.RunID, reviewFindingsArtifact)
	resolutionsPath := runArtifactRepoRel(prep.Context.State.RunID, reviewResolutionsArtifact)

	var b strings.Builder
	fmt.Fprintln(&b, "# Review Fix Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This prompt is prepared for a write-enabled executor agent subprocess.")
	fmt.Fprintln(&b, "Pactum captures the fix attempt artifacts and may parse the required structured outcome block.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintln(&b, "Address the current run's review findings against the approved Pactum contract.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Inputs")
	fmt.Fprintf(&b, "- Fixer context: %s\n", fixerContextPath)
	fmt.Fprintf(&b, "- Contract: %s\n", contractPath)
	fmt.Fprintf(&b, "- Review artifacts: %s, %s, %s\n", reviewPath, findingsPath, resolutionsPath)
	fmt.Fprintln(&b)
	writeReviewFixContractSection(&b, prep.Contract)
	fmt.Fprintln(&b)
	writeReviewFixFindingsSection(&b, buildReviewState(prep.Review, prep.Findings, prep.Resolutions))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Fix boundaries")
	fmt.Fprintln(&b, "- Trace each finding to the relevant code before acting.")
	fmt.Fprintln(&b, "- Fix valid findings in place.")
	fmt.Fprintln(&b, "- For false positives, explain a concrete rebuttal instead of changing code.")
	fmt.Fprintln(&b, "- Keep changes inside the approved contract and review-finding scope.")
	fmt.Fprintln(&b, "- Do not edit `.heurema` artifacts.")
	fmt.Fprintln(&b, "- Do not run `pactum review approve`, `pactum review resolve`, or any review loop command.")
	fmt.Fprintln(&b)
	writeHouseStyleSection(&b)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "The reviewer will re-check your fixes against the discipline rules above.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Output shape")
	fmt.Fprintln(&b, "Your final output MUST include exactly one fenced `json` block with this shape:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", reviewFixOutcomesSchema)
	fmt.Fprintln(&b, `  "outcomes": [`)
	fmt.Fprintln(&b, "    {")
	fmt.Fprintln(&b, `      "finding_id": "f_001",`)
	fmt.Fprintln(&b, `      "outcome": "fixed",`)
	fmt.Fprintln(&b, `      "note": "What changed and where, or the concrete rebuttal/blocker."`)
	fmt.Fprintln(&b, "    }")
	fmt.Fprintln(&b, "  ]")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Rules:")
	fmt.Fprintln(&b, "- Include exactly one outcome entry for every blocking finding listed above with status open.")
	fmt.Fprintln(&b, "- Do NOT edit code for advisory (non-blocking) findings, and do NOT emit outcomes for them; they are context only.")
	fmt.Fprintln(&b, "- Use outcome fixed when you changed code to address a valid blocking finding.")
	fmt.Fprintln(&b, "- Use outcome rebutted when the blocking finding is a false positive; note must contain the concrete rebuttal.")
	fmt.Fprintln(&b, "- Use outcome blocked when concrete missing information or state prevents a fix.")
	fmt.Fprintln(&b, "- Do not include advisory or resolved findings in the outcomes list.")
	return b.String()
}

func writeReviewFixContractSection(b *strings.Builder, contract draftContract) {
	fmt.Fprintln(b, "## Approved contract")
	fmt.Fprintf(b, "- Goal: %s\n", contract.Goal)
	writeMarkdownStringList(b, "- In scope:", contract.Scope.In)
	writeMarkdownStringList(b, "- Out of scope:", contract.Scope.Out)
	writeMarkdownStringList(b, "- Acceptance criteria:", contract.AcceptanceCriteria)
	writeMarkdownStringList(b, "- Validation commands:", contract.Validation.Commands)
}

func writeReviewFixFindingsSection(b *strings.Builder, state reviewStateResponse) {
	fmt.Fprintln(b, "## Current review findings")
	fmt.Fprintf(b, "- Summary: findings=%d open=%d resolved=%d blocking_open=%d\n", state.Review.Summary.Findings, state.Review.Summary.Open, state.Review.Summary.Resolved, state.Review.Summary.BlockingOpen)

	// Partition open findings so the fixer acts only on blocking work. Blocking
	// findings must each get a fix-outcome; advisory (non-blocking) findings are
	// context only and must never be edited or carry an outcome.
	var blocking, advisory, resolved []reviewFindingView
	for _, finding := range state.Findings {
		switch {
		case finding.Status == "resolved":
			resolved = append(resolved, finding)
		case finding.Blocking:
			blocking = append(blocking, finding)
		default:
			advisory = append(advisory, finding)
		}
	}

	fmt.Fprintln(b, "- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):")
	writeReviewFixFindingList(b, blocking)
	fmt.Fprintln(b, "- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):")
	writeReviewFixFindingList(b, advisory)
	if len(resolved) > 0 {
		fmt.Fprintln(b, "- Resolved findings (already addressed — context only):")
		writeReviewFixFindingList(b, resolved)
	}
}

func writeReviewFixFindingList(b *strings.Builder, findings []reviewFindingView) {
	if len(findings) == 0 {
		fmt.Fprintln(b, "  - none")
		return
	}
	for _, finding := range findings {
		fmt.Fprintf(b, "  - %s severity=%s category=%s blocking=%t status=%s: %s\n", finding.ID, finding.Severity, finding.Category, finding.Blocking, finding.Status, finding.Message)
		if finding.File != "" {
			location := finding.File
			if finding.Line > 0 {
				location = fmt.Sprintf("%s:%d", location, finding.Line)
			}
			fmt.Fprintf(b, "    location: %s\n", location)
		}
		if finding.LatestResolution != nil {
			fmt.Fprintf(b, "    latest resolution: %s\n", valueOrNone(finding.LatestResolution.Note))
		}
	}
}

func writeReviewFixRun(stdout io.Writer, request reviewFixRequestDocument, result reviewFixResultDocument, fixerName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Review fix attempt finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintln(stdout)
	writeResolved(stdout, fixerName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Fixer:")
	fmt.Fprintf(stdout, "  name: %s\n", request.Fixer.Name)
	fmt.Fprintf(stdout, "  command: %s\n", request.Fixer.Command)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", result.AttemptID)
	fmt.Fprintf(stdout, "  exit code: %d\n", result.ExitCode)
	fmt.Fprintf(stdout, "  timed out: %t\n", result.TimedOut)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  request: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join(reviewFixAttemptsArtifact, result.AttemptID, "request.json"))))
	fmt.Fprintf(stdout, "  result: %s\n", runArtifactRepoRel(result.RunID, filepath.ToSlash(filepath.Join(reviewFixAttemptsArtifact, result.AttemptID, "result.json"))))
	fmt.Fprintf(stdout, "  stdout: %s\n", runArtifactRepoRel(result.RunID, result.Stdout))
	fmt.Fprintf(stdout, "  stderr: %s\n", runArtifactRepoRel(result.RunID, result.Stderr))
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(result.RunID, reviewFixLastResultArtifact))
}
