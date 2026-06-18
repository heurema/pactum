package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
)

// planReviewFinding is one structured finding from a plan reviewer.
type planReviewFinding struct {
	Agent       string `json:"agent"`
	Lens        string `json:"lens"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // "blocking" or "suggestion"
}

// planReviewJSONResponse is the --json output document for pactum plan review.
type planReviewJSONResponse struct {
	NoPlan      bool                `json:"no_plan"`
	NoReviewers bool                `json:"no_reviewers"`
	Findings    []planReviewFinding `json:"findings"`
}

// planReviewFindingsError signals that plan review completed with findings.
// The caller is responsible for having already written the output; app.go
// handles this sentinel by exiting 1 without an extra error line.
type planReviewFindingsError struct{}

func (planReviewFindingsError) Error() string { return "plan review: findings reported" }

// planReviewerArtifacts is the artifacts section of a plan reviewer request.
type planReviewerArtifacts struct {
	ReviewerPrompt string `json:"reviewer_prompt"`
}

// planReviewerAttemptPlan holds the prepared data for one plan reviewer attempt.
type planReviewerAttemptPlan struct {
	Artifacts planReviewerArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand  `json:"would_run"`
}

// planReviewerRequestDocument is the ACP request for a plan reviewer attempt.
type planReviewerRequestDocument struct {
	Schema    string                 `json:"schema"`
	RunID     string                 `json:"run_id"`
	AttemptID string                 `json:"attempt_id"`
	CreatedAt string                 `json:"created_at"`
	Reviewer  agents.AgentDescriptor `json:"reviewer"`
	Artifacts planReviewerArtifacts  `json:"artifacts"`
	WouldRun  agents.DryRunCommand   `json:"would_run"`
}

// planReviewerResultDocument is the ACP result for a plan reviewer attempt.
type planReviewerResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Reviewer  string `json:"reviewer"`
	processResult
}

const (
	planReviewerRequestSchema = "pactum.plan_reviewer_request.v1alpha1"
	planReviewerResultSchema  = "pactum.plan_reviewer_result.v1alpha1"
)

// planReviewLenses is the fixed set of review lenses for plan review.
var planReviewLenses = []string{
	"granularity",
	"dependency-correctness",
	"testability",
	"non-vacuity",
	"scope-fidelity",
}

// planReviewLensDescriptions maps each lens key to its checklist description.
var planReviewLensDescriptions = map[string]string{
	"granularity":            "Tasks are focused and decomposable, not monolithic. Each task has a clear, bounded scope with verifiable acceptance criteria.",
	"dependency-correctness": "The depends_on graph is correct and complete: no missing or spurious edges. A valid topological execution order exists.",
	"testability":            "Acceptance criteria and validation commands are verifiable by automated tools. No vague criteria like 'it works' or 'looks good'.",
	"non-vacuity":            "When expected_files is set, at least one validation command is scoped to those files. Repository-wide commands alone are not sufficient.",
	"scope-fidelity":         "Expected files stay within paths_in_scope. Task descriptions and acceptance criteria are consistent with the contract goal.",
}

// planReviewFindingInput is the per-finding shape the reviewer emits inside the
// pactum.reviewer_findings.v1alpha1 block. It captures the lens alongside the
// shared proposal fields.
type planReviewFindingInput struct {
	Message  string `json:"message"`
	Blocking *bool  `json:"blocking"`
	Evidence string `json:"evidence"`
	Lens     string `json:"lens"`
}

// PlanReview runs a single-pass reviewer panel over the contract's plan DAG.
// It writes plan-review/<agent>/{prompt.txt,attempt-N.txt} and plan-review/findings.json.
// Exit 1 if findings, exit 0 if none.
func (a App) PlanReview(stdout io.Writer, stderr io.Writer, runID string, timeout time.Duration, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}

	if context.Contract.Plan == nil || len(context.Contract.Plan.Tasks) == 0 {
		if jsonOutput {
			return writeJSONResponse(stdout, planReviewJSONResponse{NoPlan: true, Findings: []planReviewFinding{}})
		}
		fmt.Fprintln(stdout, "No plan defined for this contract. Nothing to review.")
		return nil
	}

	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return err
	}

	if len(config.Pipeline.PlanReview.By) == 0 {
		if jsonOutput {
			return writeJSONResponse(stdout, planReviewJSONResponse{NoReviewers: true, Findings: []planReviewFinding{}})
		}
		fmt.Fprintln(stdout, "No plan_review reviewers configured. Nothing to review.")
		return nil
	}

	reviewers, err := a.resolvePlanReviewers(config)
	if err != nil {
		return err
	}

	resolvedTimeout := timeout
	if resolvedTimeout == 0 {
		resolvedTimeout = defaultIdleTimeout
	}

	planReviewDir := context.RunPaths.PlanReviewDir
	if err := activeStore.MkdirAll(planReviewDir); err != nil {
		return err
	}

	var allFindings []planReviewFinding
	for _, reviewer := range reviewers {
		reviewerDir := filepath.Join(planReviewDir, reviewer.Name)
		if err := activeStore.MkdirAll(reviewerDir); err != nil {
			return err
		}

		promptText := renderPlanReviewerPrompt(context.Contract)
		promptArtifact := planReviewerPromptArtifact(reviewer.Name)
		promptAbsPath := filepath.Join(reviewerDir, "prompt.txt")
		if err := activeStore.WriteBytes(promptAbsPath, []byte(promptText), 0o644); err != nil {
			return err
		}

		findings, err := a.runPlanReviewerWithRetry(stderr, runID, context, reviewer, reviewerDir, promptArtifact, resolvedTimeout)
		if err != nil {
			return err
		}
		allFindings = append(allFindings, findings...)
	}

	if allFindings == nil {
		allFindings = []planReviewFinding{}
	}

	findingsJSON, err := json.MarshalIndent(allFindings, "", "  ")
	if err != nil {
		return err
	}
	if err := activeStore.WriteBytes(context.RunPaths.PlanReviewFindingsJSON, append(findingsJSON, '\n'), 0o644); err != nil {
		return err
	}

	if jsonOutput {
		if err := writeJSONResponse(stdout, planReviewJSONResponse{Findings: allFindings}); err != nil {
			return err
		}
		if len(allFindings) > 0 {
			return planReviewFindingsError{}
		}
		return nil
	}

	if len(allFindings) == 0 {
		fmt.Fprintln(stdout, "Plan review complete. No findings.")
		return nil
	}

	fmt.Fprintf(stdout, "Plan review complete. %d finding(s):\n\n", len(allFindings))
	for _, f := range allFindings {
		fmt.Fprintf(stdout, "  [%s] %s (%s/%s)\n", f.Severity, f.Title, f.Agent, f.Lens)
		if f.Description != "" {
			fmt.Fprintf(stdout, "       %s\n", f.Description)
		}
	}
	return planReviewFindingsError{}
}

func (a App) resolvePlanReviewers(config configFile) ([]reviewLoopReviewer, error) {
	reviewers := make([]reviewLoopReviewer, 0, len(config.Pipeline.PlanReview.By))
	for _, name := range config.Pipeline.PlanReview.By {
		entry, err := findRegistryEntry(config, name)
		if err != nil {
			return nil, err
		}
		resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
		if err != nil {
			return nil, err
		}
		reviewers = append(reviewers, reviewLoopReviewer{
			Name:      resolved.Name,
			Agent:     resolved.Agent,
			ModelSpec: resolved.ModelSpec,
		})
	}
	return reviewers, nil
}

func planReviewerPromptArtifact(reviewerName string) string {
	return filepath.ToSlash(filepath.Join("plan-review", reviewerName, "prompt.txt"))
}

// runPlanReviewerWithRetry runs one plan reviewer attempt and, on parse-miss
// (non-empty stdout with no valid findings block), runs one corrective retry.
// Returns the parsed findings (may be empty) or an error on transport failure.
func (a App) runPlanReviewerWithRetry(
	liveOutput io.Writer,
	runID string,
	context runContext,
	reviewer reviewLoopReviewer,
	reviewerDir string,
	promptArtifact string,
	timeout time.Duration,
) ([]planReviewFinding, error) {
	attemptsDir := filepath.Join(reviewerDir, "reviewer-attempts")

	result1, err := a.runOnePlanReviewerAttempt(liveOutput, runID, context, reviewer, reviewerDir, attemptsDir, promptArtifact, timeout)
	if err != nil {
		return nil, err
	}

	stdoutBytes1, readErr := activeStore.ReadBytes(filepath.Join(attemptsDir, result1.AttemptID, "stdout.log"))
	if readErr != nil {
		return nil, fmt.Errorf("plan reviewer %s stdout unreadable: %w", reviewer.Name, readErr)
	}
	if err := activeStore.WriteBytes(filepath.Join(reviewerDir, "attempt-1.txt"), stdoutBytes1, 0o644); err != nil {
		return nil, err
	}

	blocks1, _ := parseReviewerFindingBlocks(string(stdoutBytes1))
	if len(blocks1) > 0 {
		return planFindingsFromBlocks(blocks1, reviewer.Name), nil
	}

	// No valid block (empty or prose-only stdout): corrective retry.
	corrText := renderPlanReviewerCorrectivePrompt(runID, reviewer.Name)
	corrArtifact := filepath.ToSlash(filepath.Join("plan-review", reviewer.Name, "corrective-prompt.txt"))
	if err := activeStore.WriteBytes(filepath.Join(reviewerDir, "corrective-prompt.txt"), []byte(corrText), 0o644); err != nil {
		return nil, err
	}

	result2, err := a.runOnePlanReviewerAttempt(liveOutput, runID, context, reviewer, reviewerDir, attemptsDir, corrArtifact, timeout)
	if err != nil {
		return nil, err
	}

	stdoutBytes2, readErr := activeStore.ReadBytes(filepath.Join(attemptsDir, result2.AttemptID, "stdout.log"))
	if readErr != nil {
		return nil, fmt.Errorf("plan reviewer %s corrective stdout unreadable: %w", reviewer.Name, readErr)
	}
	if err := activeStore.WriteBytes(filepath.Join(reviewerDir, "attempt-2.txt"), stdoutBytes2, 0o644); err != nil {
		return nil, err
	}

	blocks2, _ := parseReviewerFindingBlocks(string(stdoutBytes2))
	if len(blocks2) == 0 {
		return nil, fmt.Errorf("plan reviewer %s: response missing required pactum.reviewer_findings.v1alpha1 block after corrective retry", reviewer.Name)
	}
	return planFindingsFromBlocks(blocks2, reviewer.Name), nil
}

// runOnePlanReviewerAttempt runs a single plan reviewer ACP agent attempt and
// returns the parsed result document.
func (a App) runOnePlanReviewerAttempt(
	liveOutput io.Writer,
	runID string,
	context runContext,
	reviewer reviewLoopReviewer,
	reviewerDir string,
	attemptsDir string,
	promptArtifact string,
	timeout time.Duration,
) (planReviewerResultDocument, error) {
	var stdout bytes.Buffer
	err := runAgentAttemptLifecycle(a, agentAttemptLifecycle[planReviewerAttemptPlan, planReviewerRequestDocument, planReviewerResultDocument, struct{}]{
		Stdout:          &stdout,
		LiveOutput:      liveOutput,
		JSONOutput:      true,
		Root:            context.Root,
		EventsJSONL:     context.Paths.EventsJSONL,
		RunID:           runID,
		Stage:           "plan_review",
		AttemptsDir:     attemptsDir,
		AttemptIDPrefix: "plan_reviewer_attempt",
		LastResultJSON:  filepath.Join(reviewerDir, "last-result.json"),
		AgentName:       reviewer.Name,
		Agent:           reviewer.Agent,
		Model:           reviewer.ModelSpec,
		PromptRepoPath:  runArtifactRepoRel(runID, promptArtifact),
		ArtifactDir:     filepath.ToSlash(filepath.Join("plan-review", reviewer.Name, "reviewer-attempts")),
		Timeout:         timeout,
		ReadOnly:        true,
		StartedEvent:    "plan_reviewer_attempt_started",
		FinishedEvent:   "plan_reviewer_attempt_finished",
		ExitKind:        "plan reviewer",
		TimeoutMessage: func(d time.Duration) string {
			return fmt.Sprintf("plan reviewer process produced no output for %s", d)
		},
		Prepare: func(_ string) (planReviewerAttemptPlan, error) {
			wouldRun, err := agents.BuildACPWouldRun(reviewer.Agent.Name, reviewer.ModelSpec, true)
			if err != nil {
				return planReviewerAttemptPlan{}, err
			}
			return planReviewerAttemptPlan{
				Artifacts: planReviewerArtifacts{ReviewerPrompt: promptArtifact},
				WouldRun:  wouldRun,
			}, nil
		},
		BuildRequest: func(ctx agentAttemptContext[planReviewerAttemptPlan]) (planReviewerRequestDocument, error) {
			return planReviewerRequestDocument{
				Schema:    planReviewerRequestSchema,
				RunID:     runID,
				AttemptID: ctx.AttemptID,
				CreatedAt: ctx.CreatedAt,
				Reviewer:  agentDescriptorDocument(reviewer.Agent),
				Artifacts: ctx.Prepared.Artifacts,
				WouldRun:  ctx.Prepared.WouldRun,
			}, nil
		},
		BuildResult: func(ctx agentAttemptContext[planReviewerAttemptPlan], runResult agents.RunResult) planReviewerResultDocument {
			return planReviewerResultDocument{
				Schema:        planReviewerResultSchema,
				RunID:         runID,
				AttemptID:     ctx.AttemptID,
				Reviewer:      reviewer.Agent.Name,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result planReviewerResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(out io.Writer, _ planReviewerRequestDocument, result planReviewerResultDocument) {
			fmt.Fprintf(out, "plan reviewer attempt %s exit code %d\n", result.AttemptID, result.ExitCode)
		},
	})

	var result planReviewerResultDocument
	if stdout.Len() > 0 {
		_ = json.Unmarshal(stdout.Bytes(), &result)
	}
	return result, err
}

// planFindingsFromBlocks converts parsed reviewer finding blocks to planReviewFinding slice.
func planFindingsFromBlocks(blocks []reviewerFindingBlock, agentName string) []planReviewFinding {
	var findings []planReviewFinding
	for _, block := range blocks {
		for _, rawFinding := range *block.Findings {
			var input planReviewFindingInput
			if err := json.Unmarshal(rawFinding, &input); err != nil {
				continue
			}
			if strings.TrimSpace(input.Message) == "" {
				continue
			}
			if !isValidPlanReviewLens(input.Lens) {
				continue
			}
			severity := "suggestion"
			if input.Blocking != nil && *input.Blocking {
				severity = "blocking"
			}
			findings = append(findings, planReviewFinding{
				Agent:       agentName,
				Lens:        input.Lens,
				Title:       input.Message,
				Description: input.Evidence,
				Severity:    severity,
			})
		}
	}
	return findings
}

// isValidPlanReviewLens reports whether lens is one of the known plan review lens keys.
func isValidPlanReviewLens(lens string) bool {
	for _, l := range planReviewLenses {
		if l == lens {
			return true
		}
	}
	return false
}

// renderPlanReviewerPrompt builds the single prompt sent to each plan reviewer.
// It includes the contract goal, all plan tasks, and the five review lenses.
func renderPlanReviewerPrompt(contract draftContract) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Plan Review")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "You are reviewing the task plan (DAG) for a software change contract.")
	fmt.Fprintln(&b, "Evaluate the plan against all five lenses below and report any findings.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "**Goal**: %s\n\n", valueOrNone(contract.Goal))
	writeMarkdownStringList(&b, "**Paths in scope**:", contract.PathsInScope)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Plan Tasks")
	fmt.Fprintln(&b)
	if contract.Plan != nil {
		for _, task := range contract.Plan.Tasks {
			fmt.Fprintf(&b, "### %s\n\n", task.ID)
			if task.Title != "" {
				fmt.Fprintf(&b, "**Title**: %s\n\n", task.Title)
			}
			if len(task.DependsOn) > 0 {
				writeMarkdownStringList(&b, "**Depends on**:", task.DependsOn)
				fmt.Fprintln(&b)
			}
			writeMarkdownStringList(&b, "**Expected files**:", task.ExpectedFiles)
			fmt.Fprintln(&b)
			writeMarkdownStringList(&b, "**Acceptance**:", task.Acceptance)
			fmt.Fprintln(&b)
			writeMarkdownStringList(&b, "**Validation**:", task.Validation)
			fmt.Fprintln(&b)
		}
	}
	fmt.Fprintln(&b, "## Review Lenses")
	fmt.Fprintln(&b)
	for _, key := range planReviewLenses {
		desc := planReviewLensDescriptions[key]
		fmt.Fprintf(&b, "- **%s**: %s\n", key, desc)
	}
	fmt.Fprintln(&b)
	writePlanReviewerOutputFormat(&b)
	return b.String()
}

// renderPlanReviewerCorrectivePrompt builds the corrective prompt for a
// parse-miss: directs the reviewer to re-read the original prompt and produce
// a valid findings block.
func renderPlanReviewerCorrectivePrompt(runID string, reviewerName string) string {
	promptPath := runArtifactRepoRel(runID, planReviewerPromptArtifact(reviewerName))
	var b strings.Builder
	fmt.Fprintln(&b, "# Corrective Plan Review Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Your previous response did not include a valid `pactum.reviewer_findings.v1alpha1` JSON block.")
	fmt.Fprintln(&b, "Findings expressed only in prose are not recoverable; re-review the plan.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Re-read the original plan review prompt at %s and produce a valid findings block.\n", promptPath)
	fmt.Fprintln(&b)
	writePlanReviewerOutputFormat(&b)
	return b.String()
}

// writePlanReviewerOutputFormat writes the output format instructions for a
// plan reviewer, including the required pactum.reviewer_findings.v1alpha1 block.
func writePlanReviewerOutputFormat(b *strings.Builder) {
	fmt.Fprintln(b, "## Output")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "State your analysis in prose. If you find issues, include a structured findings block:")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "```json")
	fmt.Fprintln(b, "{")
	fmt.Fprintf(b, "  \"schema\": %q,\n", reviewerFindingsSchema)
	fmt.Fprintln(b, `  "findings": [`)
	fmt.Fprintln(b, "    {")
	fmt.Fprintf(b, "      \"lens\": %q,\n", planReviewLenses[0])
	fmt.Fprintln(b, `      "message": "Describe the plan issue clearly.",`)
	fmt.Fprintln(b, `      "blocking": true,`)
	fmt.Fprintln(b, `      "evidence": "Quote or cite the task field that shows the issue."`)
	fmt.Fprintln(b, "    }")
	fmt.Fprintln(b, "  ]")
	fmt.Fprintln(b, "}")
	fmt.Fprintln(b, "```")
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Rules:")
	fmt.Fprintf(b, "- Set `lens` to one of: %s.\n", strings.Join(planReviewLenses, ", "))
	fmt.Fprintln(b, "- Set blocking=true for plan defects that must be fixed before execution.")
	fmt.Fprintln(b, "- Set blocking=false for advisory suggestions.")
	fmt.Fprintln(b, "- Always include exactly one findings block. Use an empty array when there are no issues.")
}
