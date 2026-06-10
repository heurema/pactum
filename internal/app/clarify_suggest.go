package app

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	clarificationSuggestionsSchema = "pactum.clarification_suggestions.v1"

	clarifierContextArtifact    = "clarify/clarifier-context.md"
	clarifierPromptArtifact     = "clarify/clarifier-prompt.md"
	clarifierAttemptsArtifact   = "clarify/clarifier-attempts"
	clarifierLastResultArtifact = "clarify/clarifier-last-result.json"
	clarifierRequestSchema      = "pactum.clarifier_request.v1"
	clarifierResultSchema       = "pactum.clarifier_result.v1"
)

type clarifierPreparation struct {
	Context  clarifyContext
	Contract draftContract
	Status   clarifyStatusResponse
	// ClarifierName is the registry name the clarifier was invoked under;
	// Clarifier is the underlying built-in's read-only descriptor with the
	// entry's pins applied.
	ClarifierName string
	Clarifier     agents.AgentDescriptor
	ModelSpec     agents.ModelSpec
}

type clarifierArtifacts struct {
	ClarifierPrompt  string `json:"clarifier_prompt"`
	ClarifierContext string `json:"clarifier_context"`
	Questions        string `json:"questions"`
	Answers          string `json:"answers"`
	Decisions        string `json:"decisions"`
	Contract         string `json:"contract"`
	RepoContext      string `json:"repo_context"`
	SearchResults    string `json:"search_results"`
}

type clarifierRequestDocument struct {
	Schema    string                 `json:"schema"`
	RunID     string                 `json:"run_id"`
	AttemptID string                 `json:"attempt_id"`
	CreatedAt string                 `json:"created_at"`
	Clarifier agents.AgentDescriptor `json:"clarifier"`
	Artifacts clarifierArtifacts     `json:"artifacts"`
	WouldRun  agents.DryRunCommand   `json:"would_run"`
}

type clarifierResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Clarifier string `json:"clarifier"`
	processResult
}

type clarifierSuggestionsBlock struct {
	Schema    string            `json:"schema"`
	Questions []json.RawMessage `json:"questions"`
}

type clarifierSuggestionInput struct {
	Text     string `json:"text"`
	Blocking *bool  `json:"blocking"`
	// Kind classifies the question (terminology, scope, acceptance, edge_case,
	// assumption, other). It categorizes the question and is the structural basis
	// for later interrogation slices (edge-case probing, coverage signal).
	Kind              string `json:"kind"`
	Rationale         string `json:"rationale"`
	RecommendedAnswer string `json:"recommended_answer"`
	Confidence        string `json:"confidence"`
	// DependsOn lists the 1-based positions of EARLIER questions in the same
	// emitted block whose answers this question hinges on. Positions are numbered
	// per block, so Pactum resolves each one against that block's own questions
	// when the suggestion is recorded — never across into another block.
	DependsOn []int `json:"depends_on"`
}

type clarifySuggestResponse struct {
	RunID     string                        `json:"run_id"`
	RunStatus string                        `json:"run_status"`
	AttemptID string                        `json:"attempt_id"`
	Clarifier string                        `json:"clarifier"`
	Result    clarifierResultDocument       `json:"result"`
	Created   []clarificationQuestionRecord `json:"created"`
	Warnings  []string                      `json:"warnings"`
	// ApprovalReset reports that recording these suggestions regressed an
	// already-approved run back to clarifying (approval reset to pending). It is
	// omitted (false) when the run was not approved or no questions were recorded.
	ApprovalReset bool `json:"approval_reset,omitempty"`
}

func (a App) ClarifySuggest(stdout io.Writer, liveOutput io.Writer, runID string, reviewerName string, timeout time.Duration, confirm bool, jsonOutput bool) error {
	context, ok, err := a.loadClarifyContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	prep, err := a.prepareClarifier(context, reviewerName)
	if err != nil {
		return err
	}

	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[agents.DryRunCommand, clarifierRequestDocument, clarifierResultDocument, clarifySuggestResponse]{
		Stdout:          stdout,
		LiveOutput:      liveOutput,
		JSONOutput:      jsonOutput,
		Confirm:         confirm,
		CancelMessage:   "clarification suggestion cancelled",
		Root:            context.Root,
		EventsJSONL:     context.Paths.EventsJSONL,
		RunID:           runID,
		Stage:           "clarify",
		AttemptsDir:     context.RunPaths.ClarifierAttemptsDir,
		AttemptIDPrefix: "clarifier_attempt",
		LastResultJSON:  context.RunPaths.ClarifierLastResultJSON,
		AgentName:       prep.ClarifierName,
		Agent:           prep.Clarifier,
		Model:           prep.ModelSpec,
		PromptRepoPath:  clarifierPromptRepoPath(runID),
		ArtifactDir:     clarifierAttemptsArtifact,
		Timeout:         timeout,
		ReadOnly:        true,
		StartedEvent:    "clarifier_attempt_started",
		FinishedEvent:   "clarifier_attempt_finished",
		ExitKind:        "clarifier",
		TimeoutMessage: func(timeout time.Duration) string {
			return fmt.Sprintf("clarifier process produced no output for %s", timeout)
		},
		Prepare: func(createdAt string) (agents.DryRunCommand, error) {
			if err := writeClarifierPromptArtifacts(prep); err != nil {
				return agents.DryRunCommand{}, err
			}
			return agents.BuildCommand(prep.Clarifier, clarifierPromptRepoPath(runID))
		},
		BuildRequest: func(attempt agentAttemptContext[agents.DryRunCommand]) (clarifierRequestDocument, error) {
			return clarifierRequestDocument{
				Schema:    clarifierRequestSchema,
				RunID:     runID,
				AttemptID: attempt.AttemptID,
				CreatedAt: attempt.CreatedAt,
				Clarifier: agentDescriptorDocument(prep.Clarifier),
				Artifacts: defaultClarifierArtifacts(),
				WouldRun:  attempt.Prepared,
			}, nil
		},
		BuildResult: func(attempt agentAttemptContext[agents.DryRunCommand], runResult agents.RunResult) clarifierResultDocument {
			return clarifierResultDocument{
				Schema:        clarifierResultSchema,
				RunID:         runID,
				AttemptID:     attempt.AttemptID,
				Clarifier:     prep.Clarifier.Name,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result clarifierResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(stdout io.Writer, request clarifierRequestDocument, result clarifierResultDocument) {
			writeClarifySuggestRunOnly(stdout, request, result, prep.ClarifierName, prep.ModelSpec)
		},
		AfterSuccess: func(attempt agentAttemptContext[agents.DryRunCommand], request clarifierRequestDocument, result clarifierResultDocument, now time.Time) (clarifySuggestResponse, error) {
			created, warnings, status, approvalReset, err := a.recordClarifierSuggestions(context, attempt.AttemptID, attempt.AttemptPaths.StdoutLog, now)
			if err != nil {
				return clarifySuggestResponse{}, err
			}
			return clarifySuggestResponse{
				RunID:         runID,
				RunStatus:     status.RunStatus,
				AttemptID:     attempt.AttemptID,
				Clarifier:     prep.Clarifier.Name,
				Result:        result,
				Created:       created,
				Warnings:      warnings,
				ApprovalReset: approvalReset,
			}, nil
		},
		RenderSuccess: func(stdout io.Writer, response clarifySuggestResponse, request clarifierRequestDocument) {
			writeClarifySuggest(stdout, response, request, prep.ClarifierName, prep.ModelSpec)
		},
	})
}

func (a App) prepareClarifier(context clarifyContext, reviewerName string) (clarifierPreparation, error) {
	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return clarifierPreparation{}, err
	}
	if strings.TrimSpace(contract.Goal) == "" {
		return clarifierPreparation{}, fmt.Errorf("cannot suggest clarifications: contract goal is empty (set a goal first)")
	}
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return clarifierPreparation{}, err
	}
	config, err := readConfig(context.Paths.Config)
	if err != nil {
		return clarifierPreparation{}, err
	}
	// The clarifier is a reviewer-role agent: an explicit --reviewer resolves a
	// registry name, an omitted one applies the cross-model rule against the
	// registry, and the entry's pins travel with the name.
	entry, err := resolveReviewerEntry(config, reviewContext{
		Root:     context.Root,
		Paths:    context.Paths,
		RunPaths: context.RunPaths,
		State:    context.State,
	}, reviewerName)
	if err != nil {
		return clarifierPreparation{}, err
	}
	resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
	if err != nil {
		return clarifierPreparation{}, err
	}
	return clarifierPreparation{
		Context:       context,
		Contract:      contract,
		Status:        status,
		ClarifierName: resolved.Name,
		Clarifier:     resolved.Agent,
		ModelSpec:     resolved.ModelSpec,
	}, nil
}

func writeClarifierPromptArtifacts(prep clarifierPreparation) error {
	if err := activeStore.MkdirAll(prep.Context.RunPaths.ClarifyDir); err != nil {
		return err
	}
	if err := activeStore.WriteBytes(prep.Context.RunPaths.ClarifierContextMD, []byte(renderClarifierContext(prep)), 0o644); err != nil {
		return err
	}
	return activeStore.WriteBytes(prep.Context.RunPaths.ClarifierPromptMD, []byte(renderClarifierPrompt(prep.Context.State.RunID)), 0o644)
}

// recordClarifierSuggestions parses the clarifier output into open questions and,
// when any are recorded, refreshes the clarification artifacts. The returned bool
// is the approvalReset flag from refreshClarificationArtifacts so ClarifySuggest
// can surface an approved -> clarifying regression; it is false when zero
// questions are recorded (the early-return path never refreshes artifacts).
func (a App) recordClarifierSuggestions(context clarifyContext, attemptID string, stdoutPath string, now time.Time) ([]clarificationQuestionRecord, []string, clarifyStatusResponse, bool, error) {
	stdoutBytes, err := activeStore.ReadBytes(stdoutPath)
	if err != nil {
		return nil, nil, clarifyStatusResponse{}, false, err
	}
	questions, err := readClarificationQuestions(context.RunPaths.QuestionsJSONL)
	if err != nil {
		return nil, nil, clarifyStatusResponse{}, false, err
	}

	blocks, warnings := parseClarifierSuggestionBlocks(string(stdoutBytes))
	created := make([]clarificationQuestionRecord, 0)
	for _, block := range blocks {
		// positionToID maps each emitted question's 1-based position WITHIN THIS
		// block to its assigned id, or to skippedClarifierPosition when the
		// suggestion at that position was dropped during validation. Both the
		// counter and the map reset per block, so a depends_on resolves only
		// against strictly-earlier questions in the same block — matching the
		// clarifier prompt's "this same block" wording — and can never reach an
		// earlier block's question. The global q_NNN id assignment
		// (len(questions)+len(created)+1) still numbers across all blocks.
		positionToID := make(map[int]string)
		position := 0
		for _, rawQuestion := range block.Questions {
			position++
			var input clarifierSuggestionInput
			if err := json.Unmarshal(rawQuestion, &input); err != nil {
				warnings = append(warnings, "question skipped: invalid question object")
				positionToID[position] = skippedClarifierPosition
				continue
			}
			record, warning := clarificationQuestionFromSuggestion(context.Root, context.State.RunID, attemptID, len(questions)+len(created)+1, input, now)
			if warning != "" {
				warnings = append(warnings, warning)
				positionToID[position] = skippedClarifierPosition
				continue
			}
			dependsOn, dependsWarnings := resolveClarifierDependsOn(positionToID, position, record.ID, input.DependsOn)
			warnings = append(warnings, dependsWarnings...)
			record.DependsOn = dependsOn
			positionToID[position] = record.ID
			created = append(created, record)
		}
	}

	if len(created) == 0 {
		status, err := buildClarificationStatus(context.RunPaths, context.State)
		if err != nil {
			return nil, nil, clarifyStatusResponse{}, false, err
		}
		// No questions recorded: artifacts are not refreshed and approval is not
		// reset, so approvalReset is false.
		return created, warnings, status, false, nil
	}
	for _, record := range created {
		if err := appendJSONLine(context.RunPaths.QuestionsJSONL, record); err != nil {
			return nil, nil, clarifyStatusResponse{}, false, err
		}
	}
	status, approvalReset, err := a.refreshClarificationArtifacts(context, now)
	if err != nil {
		return nil, nil, clarifyStatusResponse{}, false, err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_questions_suggested", Timestamp: now, RunID: context.State.RunID}); err != nil {
		return nil, nil, clarifyStatusResponse{}, false, err
	}
	return created, warnings, status, approvalReset, nil
}

func parseClarifierSuggestionBlocks(output string) ([]clarifierSuggestionsBlock, []string) {
	jsonBlocks := extractFencedJSONBlocks(agentMessageText([]byte(output)))
	blocks := make([]clarifierSuggestionsBlock, 0, len(jsonBlocks))
	warnings := []string{}
	for _, raw := range jsonBlocks {
		var block clarifierSuggestionsBlock
		if err := json.Unmarshal([]byte(raw), &block); err != nil {
			warnings = append(warnings, "structured suggestion block skipped: invalid JSON")
			continue
		}
		if block.Schema != clarificationSuggestionsSchema {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks, warnings
}

func clarificationQuestionFromSuggestion(root string, runID string, attemptID string, index int, input clarifierSuggestionInput, now time.Time) (clarificationQuestionRecord, string) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return clarificationQuestionRecord{}, "question skipped: text is required"
	}
	if input.Blocking == nil {
		return clarificationQuestionRecord{}, "question skipped: blocking is required"
	}
	rationale := strings.TrimSpace(input.Rationale)
	if rationale == "" {
		return clarificationQuestionRecord{}, "question skipped: rationale is required"
	}
	recommendedAnswer := strings.TrimSpace(input.RecommendedAnswer)
	if recommendedAnswer == "" {
		return clarificationQuestionRecord{}, "question skipped: recommended answer is required"
	}
	confidence := strings.TrimSpace(input.Confidence)
	if !isValidClarificationConfidence(confidence) {
		return clarificationQuestionRecord{}, "question skipped: confidence must be one of high, medium, low"
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		// A missing kind defaults to the catch-all 'other', preserving v1
		// compatibility for producers that predate the kind field. A non-empty
		// kind outside the allowed set is still rejected.
		kind = "other"
	}
	if !isValidClarificationKind(kind) {
		return clarificationQuestionRecord{}, "question skipped: kind must be one of terminology, scope, acceptance, edge_case, assumption, other"
	}
	return clarificationQuestionRecord{
		Schema:             clarificationQuestionSchema,
		ID:                 nextClarificationID("q", index),
		RunID:              runID,
		Question:           sanitizeRepoRootInText(root, text),
		Blocking:           *input.Blocking,
		Kind:               kind,
		Rationale:          sanitizeRepoRootInText(root, rationale),
		RecommendedAnswer:  sanitizeRepoRootInText(root, recommendedAnswer),
		Confidence:         confidence,
		Status:             "open",
		CreatedAt:          now,
		Source:             "clarifier_attempt",
		ClarifierAttemptID: attemptID,
	}, ""
}

// skippedClarifierPosition marks an emitted question position whose suggestion
// was dropped during validation, so a later depends_on referencing it resolves
// to nothing rather than to a real question id.
const skippedClarifierPosition = ""

// resolveClarifierDependsOn turns a question's depends_on positions into the ids
// of the earlier questions they reference. positionToID holds every already-
// processed position (recorded id or skippedClarifierPosition); position is the
// 1-based position of the question being resolved. An entry is dropped, with a
// warning that keeps the question, when it is not a strictly-earlier (1 <= d <
// position) recorded position.
func resolveClarifierDependsOn(positionToID map[int]string, position int, questionID string, dependsOn []int) ([]string, []string) {
	if len(dependsOn) == 0 {
		return nil, nil
	}
	resolved := make([]string, 0, len(dependsOn))
	warnings := []string{}
	for _, d := range dependsOn {
		id, ok := positionToID[d]
		if d < 1 || d >= position || !ok || id == skippedClarifierPosition {
			warnings = append(warnings, fmt.Sprintf("%s dependency dropped: depends_on position %d is not a strictly earlier recorded question", questionID, d))
			continue
		}
		resolved = append(resolved, id)
	}
	if len(resolved) == 0 {
		return nil, warnings
	}
	return resolved, warnings
}

func isValidClarificationConfidence(confidence string) bool {
	switch confidence {
	case "high", "medium", "low":
		return true
	default:
		return false
	}
}

func isValidClarificationKind(kind string) bool {
	switch kind {
	case "terminology", "scope", "acceptance", "edge_case", "assumption", "other":
		return true
	default:
		return false
	}
}

func defaultClarifierArtifacts() clarifierArtifacts {
	return clarifierArtifacts{
		ClarifierPrompt:  clarifierPromptArtifact,
		ClarifierContext: clarifierContextArtifact,
		Questions:        "clarify/questions.jsonl",
		Answers:          "clarify/answers.jsonl",
		Decisions:        "clarify/decisions.jsonl",
		Contract:         "contract/contract.json",
		RepoContext:      "context/repo-context.md",
		SearchResults:    "context/search-results.json",
	}
}

func clarifierPromptRepoPath(runID string) string {
	return runArtifactRepoRel(runID, clarifierPromptArtifact)
}

func renderClarifierContext(prep clarifierPreparation) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Clarifier Context")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Run")
	fmt.Fprintf(&b, "- Run id: %s\n", prep.Context.State.RunID)
	fmt.Fprintf(&b, "- Run status: %s\n", prep.Context.State.Status)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract draft")
	fmt.Fprintf(&b, "- Goal: %s\n", valueOrNone(prep.Contract.Goal))
	writeMarkdownStringList(&b, "- In scope:", prep.Contract.Scope.In)
	writeMarkdownStringList(&b, "- Out of scope:", prep.Contract.Scope.Out)
	writeMarkdownStringList(&b, "- Acceptance criteria:", prep.Contract.AcceptanceCriteria)
	writeMarkdownStringList(&b, "- Validation commands:", prep.Contract.Validation.Commands)
	writeMarkdownStringList(&b, "- Assumptions:", prep.Contract.Assumptions)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Existing clarifications")
	fmt.Fprintf(&b, "- Total: %d\n", prep.Status.Total)
	fmt.Fprintf(&b, "- Open: %d\n", prep.Status.Open)
	fmt.Fprintf(&b, "- Blocking open: %d\n", prep.Status.BlockingOpen)
	fmt.Fprintf(&b, "- Converged: %t\n", prep.Status.Converged)
	fmt.Fprintln(&b, "- Coverage by dimension (a zero-question dimension is unprobed, not necessarily settled):")
	for _, coverage := range prep.Status.Coverage {
		fmt.Fprintf(&b, "  - %s: total %d, answered %d, open %d, blocking open %d\n", coverage.Kind, coverage.Total, coverage.Answered, coverage.Open, coverage.BlockingOpen)
	}
	if len(prep.Status.Questions) == 0 {
		fmt.Fprintln(&b, "- Questions: none")
	} else {
		fmt.Fprintln(&b, "- Questions:")
		for _, question := range prep.Status.Questions {
			fmt.Fprintf(&b, "  - %s blocking=%t status=%s: %s\n", question.ID, question.Blocking, question.Status, question.Question)
			if question.Kind != "" {
				fmt.Fprintf(&b, "    kind: %s\n", question.Kind)
			}
			if question.Rationale != "" {
				fmt.Fprintf(&b, "    rationale: %s\n", question.Rationale)
			}
			if question.RecommendedAnswer != "" {
				confidence := question.Confidence
				if confidence == "" {
					confidence = "unknown"
				}
				fmt.Fprintf(&b, "    recommended answer (confidence %s): %s\n", confidence, question.RecommendedAnswer)
			}
			if len(question.DependsOn) > 0 {
				fmt.Fprintf(&b, "    depends on: %s\n", strings.Join(question.DependsOn, ", "))
			}
			if question.Blocked {
				fmt.Fprintln(&b, "    blocked: waiting on unanswered prerequisites")
			}
			if question.Answer != "" {
				fmt.Fprintf(&b, "    answer: %s\n", question.Answer)
			}
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Repository context")
	writeFileExcerpt(&b, prep.Context.RunPaths.RepoContext)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Search results")
	writeFileExcerpt(&b, prep.Context.RunPaths.SearchResults)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Clarifier guidance")
	fmt.Fprintln(&b, "- Propose only questions that need a human answer to improve the contract.")
	fmt.Fprintln(&b, "- Do not answer existing or proposed questions.")
	fmt.Fprintln(&b, "- Do not modify files.")
	fmt.Fprintln(&b, "- Avoid duplicates of existing clarification questions.")
	fmt.Fprintln(&b, "- Treat repository map/search context as navigation hints, not semantic truth.")
	return b.String()
}

func writeFileExcerpt(b *strings.Builder, path string) {
	data, err := activeStore.ReadBytes(path)
	if err != nil {
		fmt.Fprintf(b, "Unavailable: %v\n", err)
		return
	}
	const maxBytes = 32 * 1024
	truncated := len(data) > maxBytes
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	b.Write(data)
	if !strings.HasSuffix(string(data), "\n") {
		fmt.Fprintln(b)
	}
	if truncated {
		fmt.Fprintf(b, "\n[Truncated to %d bytes]\n", maxBytes)
	}
}

func renderClarifierPrompt(runID string) string {
	clarifierContextPath := runArtifactRepoRel(runID, clarifierContextArtifact)
	contractPath := runArtifactRepoRel(runID, "contract/contract.json")
	questionsPath := runArtifactRepoRel(runID, "clarify/questions.jsonl")
	answersPath := runArtifactRepoRel(runID, "clarify/answers.jsonl")

	var b strings.Builder
	fmt.Fprintln(&b, "# Clarifier Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This prompt is prepared for a read-only clarifier agent subprocess.")
	fmt.Fprintln(&b, "Pactum will parse structured question suggestions into open clarification questions for the human to answer.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintln(&b, "Propose human-answerable clarification questions for the Pactum run contract.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Inputs")
	fmt.Fprintf(&b, "- Clarifier context: %s\n", clarifierContextPath)
	fmt.Fprintf(&b, "- Contract draft: %s\n", contractPath)
	fmt.Fprintf(&b, "- Existing questions: %s\n", questionsPath)
	fmt.Fprintf(&b, "- Existing answers: %s\n", answersPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Boundaries")
	fmt.Fprintln(&b, "- Do not answer any clarification question.")
	fmt.Fprintln(&b, "- Do not edit files.")
	fmt.Fprintln(&b, "- Do not draft or revise the contract.")
	fmt.Fprintln(&b, "- Do not run commands that write to the repository.")
	fmt.Fprintln(&b, "- Mark blocking=true when execution should not continue safely without the answer.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Explore first, escalate sparingly")
	fmt.Fprintln(&b, "- Try to resolve every candidate question yourself first: read the contract draft, the repository context, and the search results, and search the repository for the answer.")
	fmt.Fprintln(&b, "- If the repository or contract already answers it, do NOT ask — fold the finding into the rationale and the recommended answer instead.")
	fmt.Fprintln(&b, "- Escalate only questions that genuinely need a human decision: product intent, priorities, trade-offs, external constraints, or genuinely ambiguous requirements that the repo cannot settle.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Challenge vague terminology")
	fmt.Fprintln(&b, "- Read the contract goal, scope, and acceptance criteria for vague or overloaded domain terms — words that could denote more than one concrete thing in this repository.")
	fmt.Fprintln(&b, "- Do NOT silently pick a meaning. Ask which concrete concept is intended, and name the candidate interpretations you are choosing between in the question text and recommended answer.")
	fmt.Fprintln(&b, "- Anchor the challenge on the repository's actual concepts and identifiers (types, functions, files, commands surfaced by the repository context and search results), so the human chooses among real options rather than abstractions.")
	fmt.Fprintln(&b, "- Tag every such question kind=terminology.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Probe edge cases")
	fmt.Fprintln(&b, "- Do NOT merely say 'consider edge cases'. For each in-scope behavior and acceptance criterion, INVENT specific concrete scenarios the contract is silent on, then ask how it should behave in each.")
	fmt.Fprintln(&b, "- Derive scenarios from these categories: empty/missing/zero/duplicate/extreme inputs; error and failure paths; partial or interrupted operations; concurrency and ordering; resource and size limits; and other 'what about X' cases the contract does not address.")
	fmt.Fprintln(&b, "- Name the specific invented scenario in the question text (not an abstract category) and give a recommended answer describing how the contract should handle it.")
	fmt.Fprintln(&b, "- Prefer the scenarios most likely to change scope, acceptance, or implementation; skip ones the contract or repository already settles.")
	fmt.Fprintln(&b, "- Tag every such question kind=edge_case.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Classify every question")
	fmt.Fprintln(&b, "- EVERY question must carry a kind from: terminology, scope, acceptance, edge_case, assumption, other.")
	fmt.Fprintln(&b, "- Use terminology for a vague/overloaded-term challenge (above), scope for what is in or out of scope, acceptance for how completion is verified, edge_case for boundary or failure conditions, assumption for an unstated premise you need confirmed, and other when none fits.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Cover the material dimensions")
	fmt.Fprintln(&b, "- Before concluding, consider each material dimension — scope, acceptance, terminology, edge_case, assumption — and make sure none is left unprobed merely because you stopped early.")
	fmt.Fprintln(&b, "- The clarifier context reports coverage by dimension; treat a dimension with zero questions as a prompt to check whether the contract or repository genuinely settles it.")
	fmt.Fprintln(&b, "- Do NOT manufacture questions to fill a dimension the contract or repository already settles — explore-first still applies; a dimension can legitimately need no question.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Recommended answers")
	fmt.Fprintln(&b, "- EVERY question must carry a specific recommended answer: your best-guess resolution, phrased so the human can apply it directly as the contract change (confirm or adjust it, not author it from scratch).")
	fmt.Fprintln(&b, "- EVERY question must carry a confidence of high, medium, or low, reflecting how sure you are the recommended answer is correct.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Order and dependencies")
	fmt.Fprintln(&b, "- Order the questions foundational-first: ask the decisions that constrain other answers before the questions they constrain.")
	fmt.Fprintln(&b, "- When a question's framing or answer hinges on an earlier question in this block, set its depends_on to that earlier question's 1-based position in the questions array (positions count from 1, top to bottom).")
	fmt.Fprintln(&b, "- depends_on may reference only strictly-earlier positions; omit it (or leave it empty) for a foundational question.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Structured suggestions")
	fmt.Fprintln(&b, "Include a fenced JSON block exactly like:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", clarificationSuggestionsSchema)
	fmt.Fprintln(&b, `  "questions": [`)
	fmt.Fprintln(&b, "    {")
	fmt.Fprintln(&b, `      "text": "What should the human clarify?",`)
	fmt.Fprintln(&b, `      "blocking": true,`)
	fmt.Fprintln(&b, `      "kind": "terminology",`)
	fmt.Fprintln(&b, `      "rationale": "Why this answer changes scope or implementation choices, and what the repo already told you.",`)
	fmt.Fprintln(&b, `      "recommended_answer": "Your best-guess resolution, phrased so it is directly usable as the contract change.",`)
	fmt.Fprintln(&b, `      "confidence": "high",`)
	fmt.Fprintln(&b, `      "depends_on": []`)
	fmt.Fprintln(&b, "    }")
	fmt.Fprintln(&b, "  ]")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "kind must be one of: terminology, scope, acceptance, edge_case, assumption, other (use other when none fits).")
	fmt.Fprintln(&b, "confidence must be one of: high, medium, low.")
	fmt.Fprintln(&b, "depends_on (optional) lists the 1-based positions of earlier questions in this same block that must be answered first; omit or leave it empty for a foundational question.")
	fmt.Fprintln(&b, "If no clarification is needed, return the same schema with an empty questions array.")
	return b.String()
}

func writeClarifySuggest(stdout io.Writer, response clarifySuggestResponse, request clarifierRequestDocument, clarifierName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Clarification suggestions recorded")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	if response.ApprovalReset {
		writeApprovalResetWarning(stdout, response.RunID, response.RunStatus)
	}
	fmt.Fprintln(stdout)
	writeResolved(stdout, clarifierName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", response.AttemptID)
	fmt.Fprintf(stdout, "  clarifier: %s\n", response.Clarifier)
	fmt.Fprintf(stdout, "  command: %s\n", formatAgentCommand(request.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Questions:")
	fmt.Fprintf(stdout, "  created: %d\n", len(response.Created))
	for _, question := range response.Created {
		blocking := ""
		if question.Blocking {
			blocking = " [blocking]"
		}
		fmt.Fprintf(stdout, "  - %s%s %s\n", question.ID, blocking, question.Question)
	}
	if len(response.Warnings) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Warnings:")
		for _, warning := range response.Warnings {
			fmt.Fprintf(stdout, "  - %s\n", warning)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  request: %s\n", runArtifactRepoRel(response.RunID, filepath.ToSlash(filepath.Join(clarifierAttemptsArtifact, response.AttemptID, "request.json"))))
	fmt.Fprintf(stdout, "  result: %s\n", runArtifactRepoRel(response.RunID, filepath.ToSlash(filepath.Join(clarifierAttemptsArtifact, response.AttemptID, "result.json"))))
	fmt.Fprintf(stdout, "  stdout: %s\n", runArtifactRepoRel(response.RunID, response.Result.Stdout))
	fmt.Fprintf(stdout, "  stderr: %s\n", runArtifactRepoRel(response.RunID, response.Result.Stderr))
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(response.RunID, clarifierLastResultArtifact))
}

func writeClarifySuggestRunOnly(stdout io.Writer, request clarifierRequestDocument, result clarifierResultDocument, clarifierName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Clarifier attempt finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintln(stdout)
	writeResolved(stdout, clarifierName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", result.AttemptID)
	fmt.Fprintf(stdout, "  clarifier: %s\n", result.Clarifier)
	fmt.Fprintf(stdout, "  command: %s\n", formatAgentCommand(request.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Result:")
	fmt.Fprintf(stdout, "  exit code: %d\n", result.ExitCode)
	fmt.Fprintf(stdout, "  timed out: %t\n", result.TimedOut)
}
