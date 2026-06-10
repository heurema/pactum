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
	"github.com/heurema/pactum/internal/ledger"
)

const (
	contractDraftProposalSchema  = "pactum.contract_draft_proposal.v1"
	contractDrafterRequestSchema = "pactum.contract_drafter_request.v1"
	contractDrafterResultSchema  = "pactum.contract_drafter_result.v1"

	contractDrafterContextArtifact    = "contract/drafter-context.md"
	contractDrafterPromptArtifact     = "contract/drafter-prompt.md"
	contractDrafterAttemptsArtifact   = "contract/drafter-attempts"
	contractDrafterLastResultArtifact = "contract/drafter-last-result.json"
	contractDraftProposalArtifact     = "contract/draft-proposal.json"
	contractDraftProposalMDArtifact   = "contract/draft-proposal.md"
)

type contractDraftPreparation struct {
	Context runContext
	Status  clarifyStatusResponse
	// DrafterName is the registry name the drafter was invoked under; Drafter
	// is the underlying built-in's read-only descriptor with the entry's pins
	// applied.
	DrafterName string
	Drafter     agents.AgentDescriptor
	ModelSpec   agents.ModelSpec
}

type contractDraftArtifacts struct {
	DrafterPrompt  string `json:"drafter_prompt"`
	DrafterContext string `json:"drafter_context"`
	Proposal       string `json:"proposal"`
	ProposalMD     string `json:"proposal_md"`
	Contract       string `json:"contract"`
	Questions      string `json:"questions"`
	Answers        string `json:"answers"`
	Decisions      string `json:"decisions"`
	RepoContext    string `json:"repo_context"`
	SearchResults  string `json:"search_results"`
}

type contractDrafterRequestDocument struct {
	Schema    string                 `json:"schema"`
	RunID     string                 `json:"run_id"`
	AttemptID string                 `json:"attempt_id"`
	CreatedAt string                 `json:"created_at"`
	Drafter   agents.AgentDescriptor `json:"drafter"`
	Artifacts contractDraftArtifacts `json:"artifacts"`
	WouldRun  agents.DryRunCommand   `json:"would_run"`
}

type contractDrafterResultDocument struct {
	Schema    string `json:"schema"`
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Drafter   string `json:"drafter"`
	processResult
}

type contractDraftProposalBlock struct {
	Schema      string   `json:"schema"`
	InScope     []string `json:"in_scope"`
	OutOfScope  []string `json:"out_of_scope"`
	Acceptance  []string `json:"acceptance"`
	Validation  []string `json:"validation"`
	Assumptions []string `json:"assumptions"`
}

type contractDraftProposalDocument struct {
	Schema           string   `json:"schema"`
	RunID            string   `json:"run_id"`
	Status           string   `json:"status"`
	CreatedAt        string   `json:"created_at"`
	Source           string   `json:"source"`
	DrafterAttemptID string   `json:"drafter_attempt_id"`
	Drafter          string   `json:"drafter"`
	InScope          []string `json:"in_scope"`
	OutOfScope       []string `json:"out_of_scope"`
	Acceptance       []string `json:"acceptance"`
	Validation       []string `json:"validation"`
	Assumptions      []string `json:"assumptions"`
	Warnings         []string `json:"warnings,omitempty"`
	AcceptedAt       *string  `json:"accepted_at,omitempty"`
	AcceptedBy       *string  `json:"accepted_by,omitempty"`
}

type contractDraftResponse struct {
	RunID     string                        `json:"run_id"`
	RunStatus string                        `json:"run_status"`
	AttemptID string                        `json:"attempt_id"`
	Drafter   string                        `json:"drafter"`
	Result    contractDrafterResultDocument `json:"result"`
	Proposal  contractDraftProposalDocument `json:"proposal"`
	Warnings  []string                      `json:"warnings"`
}

type contractShowDraftResponse struct {
	RunID     string                        `json:"run_id"`
	RunStatus string                        `json:"run_status"`
	Proposal  contractDraftProposalDocument `json:"proposal"`
}

type contractAcceptDraftResponse struct {
	RunID         string                        `json:"run_id"`
	RunStatus     string                        `json:"run_status"`
	ApprovalReset bool                          `json:"approval_reset"`
	Approval      approvalState                 `json:"approval"`
	Proposal      contractDraftProposalDocument `json:"proposal"`
	Contract      draftContract                 `json:"contract"`
}

func (a App) ContractDraft(stdout io.Writer, liveOutput io.Writer, runID string, reviewerName string, timeout time.Duration, confirm bool, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	prep, err := a.prepareContractDrafter(context, reviewerName)
	if err != nil {
		return err
	}

	return runAgentAttemptLifecycle(a, agentAttemptLifecycle[agents.DryRunCommand, contractDrafterRequestDocument, contractDrafterResultDocument, contractDraftResponse]{
		Stdout:          stdout,
		LiveOutput:      liveOutput,
		JSONOutput:      jsonOutput,
		Confirm:         confirm,
		CancelMessage:   "contract draft cancelled",
		Root:            context.Root,
		EventsJSONL:     context.Paths.EventsJSONL,
		RunID:           runID,
		Stage:           "draft",
		AttemptsDir:     context.RunPaths.ContractDrafterAttemptsDir,
		AttemptIDPrefix: "drafter_attempt",
		LastResultJSON:  context.RunPaths.ContractDrafterLastResultJSON,
		AgentName:       prep.DrafterName,
		Agent:           prep.Drafter,
		Model:           prep.ModelSpec,
		PromptRepoPath:  contractDrafterPromptRepoPath(runID),
		ArtifactDir:     contractDrafterAttemptsArtifact,
		Timeout:         timeout,
		ReadOnly:        true,
		StartedEvent:    "contract_drafter_attempt_started",
		FinishedEvent:   "contract_drafter_attempt_finished",
		ExitKind:        "contract drafter",
		TimeoutMessage: func(timeout time.Duration) string {
			return fmt.Sprintf("contract drafter process produced no output for %s", timeout)
		},
		Prepare: func(createdAt string) (agents.DryRunCommand, error) {
			if err := writeContractDrafterPromptArtifacts(prep); err != nil {
				return agents.DryRunCommand{}, err
			}
			return agents.BuildCommand(prep.Drafter, contractDrafterPromptRepoPath(runID))
		},
		BuildRequest: func(attempt agentAttemptContext[agents.DryRunCommand]) (contractDrafterRequestDocument, error) {
			return contractDrafterRequestDocument{
				Schema:    contractDrafterRequestSchema,
				RunID:     runID,
				AttemptID: attempt.AttemptID,
				CreatedAt: attempt.CreatedAt,
				Drafter:   agentDescriptorDocument(prep.Drafter),
				Artifacts: defaultContractDraftArtifacts(),
				WouldRun:  attempt.Prepared,
			}, nil
		},
		BuildResult: func(attempt agentAttemptContext[agents.DryRunCommand], runResult agents.RunResult) contractDrafterResultDocument {
			return contractDrafterResultDocument{
				Schema:        contractDrafterResultSchema,
				RunID:         runID,
				AttemptID:     attempt.AttemptID,
				Drafter:       prep.Drafter.Name,
				processResult: processResultFromRunResult(runResult),
			}
		},
		ProcessResult: func(result contractDrafterResultDocument) processResult {
			return result.processResult
		},
		RenderRunOnly: func(stdout io.Writer, request contractDrafterRequestDocument, result contractDrafterResultDocument) {
			writeContractDraftRunOnly(stdout, request, result, prep.DrafterName, prep.ModelSpec)
		},
		AfterSuccess: func(attempt agentAttemptContext[agents.DryRunCommand], request contractDrafterRequestDocument, result contractDrafterResultDocument, now time.Time) (contractDraftResponse, error) {
			proposal, warnings, err := a.recordContractDraftProposal(context, attempt.AttemptID, prep.Drafter.Name, attempt.AttemptPaths.StdoutLog, now)
			if err != nil {
				return contractDraftResponse{}, err
			}
			return contractDraftResponse{
				RunID:     runID,
				RunStatus: context.State.Status,
				AttemptID: attempt.AttemptID,
				Drafter:   prep.Drafter.Name,
				Result:    result,
				Proposal:  proposal,
				Warnings:  warnings,
			}, nil
		},
		RenderSuccess: func(stdout io.Writer, response contractDraftResponse, request contractDrafterRequestDocument) {
			writeContractDraft(stdout, response, request, prep.DrafterName, prep.ModelSpec)
		},
	})
}

func (a App) ContractShowDraft(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.ContractDraftProposalJSON) {
		suggested := fmt.Sprintf("pactum contract draft %s --yes", runID)
		return writeNotReady(stdout, jsonOutput, runID, "Contract draft proposal has not been created. Run: "+suggested, suggested)
	}
	proposal, err := readContractDraftProposal(context.RunPaths.ContractDraftProposalJSON)
	if err != nil {
		return err
	}
	response := contractShowDraftResponse{
		RunID:     runID,
		RunStatus: context.State.Status,
		Proposal:  proposal,
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	proposalMD, err := activeStore.ReadBytes(context.RunPaths.ContractDraftProposalMD)
	if err != nil {
		proposalMD = []byte(renderContractDraftProposalMD(proposal))
	}
	writeContractShowDraft(stdout, response, string(proposalMD))
	return nil
}

func (a App) ContractAcceptDraft(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.ContractDraftProposalJSON) {
		return fmt.Errorf("contract draft proposal not found: %s", runID)
	}
	proposal, err := readContractDraftProposal(context.RunPaths.ContractDraftProposalJSON)
	if err != nil {
		return err
	}
	if proposal.Status == "accepted" {
		return fmt.Errorf("contract draft proposal already accepted: %s", runID)
	}
	revision := contractDraftRevisionFromDraftProposal(proposal)
	if !revision.hasChanges() {
		return fmt.Errorf("contract draft proposal has no contract fields to apply")
	}

	var reviseOut bytes.Buffer
	if err := a.ContractRevise(&reviseOut, runID, revision, true); err != nil {
		return err
	}
	var reviseResponse contractReviseResponse
	if err := json.Unmarshal(reviseOut.Bytes(), &reviseResponse); err != nil {
		return err
	}

	now := a.nowUTC()
	acceptedAt := now.Format(time.RFC3339)
	acceptedBy := "manual"
	proposal.Status = "accepted"
	proposal.AcceptedAt = &acceptedAt
	proposal.AcceptedBy = &acceptedBy
	if err := writeContractDraftProposalArtifacts(context.RunPaths, proposal); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "contract_draft_accepted", Timestamp: now, RunID: runID}); err != nil {
		return err
	}

	response := contractAcceptDraftResponse{
		RunID:         runID,
		RunStatus:     reviseResponse.RunStatus,
		ApprovalReset: reviseResponse.ApprovalReset,
		Approval:      reviseResponse.Approval,
		Proposal:      proposal,
		Contract:      reviseResponse.Contract,
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeContractAcceptDraft(stdout, response)
	return nil
}

func (a App) prepareContractDrafter(context runContext, reviewerName string) (contractDraftPreparation, error) {
	if strings.TrimSpace(context.Contract.Goal) == "" {
		return contractDraftPreparation{}, fmt.Errorf("cannot draft contract: contract goal is empty (set a goal first)")
	}
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return contractDraftPreparation{}, err
	}
	config, err := a.readConfig(context.Paths.Config)
	if err != nil {
		return contractDraftPreparation{}, err
	}
	// The drafter is a reviewer-role agent: an explicit --reviewer resolves a
	// registry name, an omitted one applies the cross-model rule against the
	// registry, and the entry's pins travel with the name.
	entry, err := resolveReviewerEntry(config, reviewContext{
		Root:     context.Root,
		Paths:    context.Paths,
		RunPaths: context.RunPaths,
		State:    context.State,
	}, reviewerName)
	if err != nil {
		return contractDraftPreparation{}, err
	}
	resolved, err := a.resolveAgentForRole(entry, agentRoleReviewer)
	if err != nil {
		return contractDraftPreparation{}, err
	}
	return contractDraftPreparation{
		Context:     context,
		Status:      status,
		DrafterName: resolved.Name,
		Drafter:     resolved.Agent,
		ModelSpec:   resolved.ModelSpec,
	}, nil
}

func writeContractDrafterPromptArtifacts(prep contractDraftPreparation) error {
	if err := activeStore.MkdirAll(prep.Context.RunPaths.ContractDir); err != nil {
		return err
	}
	if err := activeStore.WriteBytes(prep.Context.RunPaths.ContractDrafterContextMD, []byte(renderContractDrafterContext(prep)), 0o644); err != nil {
		return err
	}
	return activeStore.WriteBytes(prep.Context.RunPaths.ContractDrafterPromptMD, []byte(renderContractDrafterPrompt(prep.Context.State.RunID)), 0o644)
}

func (a App) recordContractDraftProposal(context runContext, attemptID string, drafter string, stdoutPath string, now time.Time) (contractDraftProposalDocument, []string, error) {
	stdoutBytes, err := activeStore.ReadBytes(stdoutPath)
	if err != nil {
		return contractDraftProposalDocument{}, nil, err
	}
	blocks, warnings := parseContractDraftProposalBlocks(string(stdoutBytes))
	if len(blocks) == 0 {
		return contractDraftProposalDocument{}, warnings, fmt.Errorf("no structured contract draft proposal found")
	}
	if len(blocks) > 1 {
		warnings = append(warnings, "additional structured contract draft proposal blocks ignored")
	}
	proposal := contractDraftProposalFromBlock(context.Root, context.State.RunID, attemptID, drafter, blocks[0], warnings, now)
	if err := writeContractDraftProposalArtifacts(context.RunPaths, proposal); err != nil {
		return contractDraftProposalDocument{}, nil, err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "contract_draft_proposed", Timestamp: now, RunID: context.State.RunID}); err != nil {
		return contractDraftProposalDocument{}, nil, err
	}
	return proposal, warnings, nil
}

func parseContractDraftProposalBlocks(output string) ([]contractDraftProposalBlock, []string) {
	jsonBlocks := extractFencedJSONBlocks(agentMessageText([]byte(output)))
	blocks := make([]contractDraftProposalBlock, 0, len(jsonBlocks))
	warnings := []string{}
	for _, raw := range jsonBlocks {
		var block contractDraftProposalBlock
		if err := json.Unmarshal([]byte(raw), &block); err != nil {
			warnings = append(warnings, "structured contract draft proposal skipped: invalid JSON")
			continue
		}
		if block.Schema != contractDraftProposalSchema {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks, warnings
}

func contractDraftProposalFromBlock(root string, runID string, attemptID string, drafter string, block contractDraftProposalBlock, warnings []string, now time.Time) contractDraftProposalDocument {
	return contractDraftProposalDocument{
		Schema:           contractDraftProposalSchema,
		RunID:            runID,
		Status:           "pending",
		CreatedAt:        now.Format(time.RFC3339),
		Source:           "drafter_attempt",
		DrafterAttemptID: attemptID,
		Drafter:          drafter,
		InScope:          cleanContractDraftProposalItems(root, block.InScope),
		OutOfScope:       cleanContractDraftProposalItems(root, block.OutOfScope),
		Acceptance:       cleanContractDraftProposalItems(root, block.Acceptance),
		Validation:       cleanContractDraftProposalItems(root, block.Validation),
		Assumptions:      cleanContractDraftProposalItems(root, block.Assumptions),
		Warnings:         append([]string{}, warnings...),
	}
}

func cleanContractDraftProposalItems(root string, values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, sanitizeRepoRootInText(root, value))
	}
	return cleaned
}

func contractDraftRevisionFromDraftProposal(proposal contractDraftProposalDocument) contractRevision {
	return contractRevision{
		AddInScope:    append([]string{}, proposal.InScope...),
		AddOutOfScope: append([]string{}, proposal.OutOfScope...),
		AddAcceptance: append([]string{}, proposal.Acceptance...),
		AddValidation: append([]string{}, proposal.Validation...),
		AddAssumption: append([]string{}, proposal.Assumptions...),
	}
}

func readContractDraftProposal(path string) (contractDraftProposalDocument, error) {
	var proposal contractDraftProposalDocument
	if err := readJSON(path, &proposal); err != nil {
		return contractDraftProposalDocument{}, err
	}
	if proposal.Schema == "" {
		proposal.Schema = contractDraftProposalSchema
	}
	if proposal.Status == "" {
		proposal.Status = "pending"
	}
	return proposal, nil
}

func writeContractDraftProposalArtifacts(runPaths contractRunPathSet, proposal contractDraftProposalDocument) error {
	if err := writeJSON(runPaths.ContractDraftProposalJSON, proposal); err != nil {
		return err
	}
	return activeStore.WriteBytes(runPaths.ContractDraftProposalMD, []byte(renderContractDraftProposalMD(proposal)), 0o644)
}

func defaultContractDraftArtifacts() contractDraftArtifacts {
	return contractDraftArtifacts{
		DrafterPrompt:  contractDrafterPromptArtifact,
		DrafterContext: contractDrafterContextArtifact,
		Proposal:       contractDraftProposalArtifact,
		ProposalMD:     contractDraftProposalMDArtifact,
		Contract:       "contract/contract.json",
		Questions:      "clarify/questions.jsonl",
		Answers:        "clarify/answers.jsonl",
		Decisions:      "clarify/decisions.jsonl",
		RepoContext:    "context/repo-context.md",
		SearchResults:  "context/search-results.json",
	}
}

func contractDrafterPromptRepoPath(runID string) string {
	return runArtifactRepoRel(runID, contractDrafterPromptArtifact)
}

func renderContractDrafterContext(prep contractDraftPreparation) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Contract Drafter Context")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Run")
	fmt.Fprintf(&b, "- Run id: %s\n", prep.Context.State.RunID)
	fmt.Fprintf(&b, "- Run status: %s\n", prep.Context.State.Status)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Contract goal")
	fmt.Fprintln(&b, valueOrNone(prep.Context.Contract.Goal))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Current contract fields")
	writeMarkdownStringList(&b, "- In scope:", prep.Context.Contract.Scope.In)
	writeMarkdownStringList(&b, "- Out of scope:", prep.Context.Contract.Scope.Out)
	writeMarkdownStringList(&b, "- Acceptance criteria:", prep.Context.Contract.AcceptanceCriteria)
	writeMarkdownStringList(&b, "- Validation commands:", prep.Context.Contract.Validation.Commands)
	writeMarkdownStringList(&b, "- Assumptions:", prep.Context.Contract.Assumptions)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Answered clarifications")
	wrote := false
	for _, question := range prep.Status.Questions {
		if strings.TrimSpace(question.Answer) == "" {
			continue
		}
		wrote = true
		blocking := ""
		if question.Blocking {
			blocking = " [blocking]"
		}
		fmt.Fprintf(&b, "- %s%s: %s\n", question.ID, blocking, question.Question)
		if question.Rationale != "" {
			fmt.Fprintf(&b, "  Rationale: %s\n", question.Rationale)
		}
		fmt.Fprintf(&b, "  Answer: %s\n", question.Answer)
	}
	if !wrote {
		fmt.Fprintln(&b, "- None")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Repository context")
	writeFileExcerpt(&b, prep.Context.RunPaths.RepoContext)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Search results")
	writeFileExcerpt(&b, prep.Context.RunPaths.SearchResults)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Drafter guidance")
	fmt.Fprintln(&b, "- Propose only additions to the contract fields listed in the prompt.")
	fmt.Fprintln(&b, "- Do not change or restate the contract goal.")
	fmt.Fprintln(&b, "- Do not answer clarification questions.")
	fmt.Fprintln(&b, "- Do not edit files.")
	fmt.Fprintln(&b, "- Treat repository map/search context as navigation hints, not semantic truth.")
	return b.String()
}

func renderContractDrafterPrompt(runID string) string {
	drafterContextPath := runArtifactRepoRel(runID, contractDrafterContextArtifact)
	contractPath := runArtifactRepoRel(runID, "contract/contract.json")
	answersPath := runArtifactRepoRel(runID, "clarify/answers.jsonl")

	var b strings.Builder
	fmt.Fprintln(&b, "# Contract Drafter Prompt")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This prompt is prepared for a read-only contract drafter agent subprocess.")
	fmt.Fprintln(&b, "Pactum will parse the structured proposal into a pending draft proposal; it will not apply it until a human runs the accept command.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintln(&b, "Propose missing contract scope, acceptance, validation, and assumption entries from the contract goal, answered clarifications, and repository context.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Inputs")
	fmt.Fprintf(&b, "- Drafter context: %s\n", drafterContextPath)
	fmt.Fprintf(&b, "- Contract draft: %s\n", contractPath)
	fmt.Fprintf(&b, "- Clarification answers: %s\n", answersPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Boundaries")
	fmt.Fprintln(&b, "- Do not change the contract goal.")
	fmt.Fprintln(&b, "- Do not answer clarification questions.")
	fmt.Fprintln(&b, "- Do not edit files.")
	fmt.Fprintln(&b, "- Do not run commands that write to the repository.")
	fmt.Fprintln(&b, "- Propose additions only; Pactum will append accepted entries through contract revision.")
	fmt.Fprintln(&b, "- Use concrete, observable acceptance criteria and runnable validation commands.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Structured proposal")
	fmt.Fprintln(&b, "Include a fenced JSON block exactly like:")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "```json")
	fmt.Fprintln(&b, "{")
	fmt.Fprintf(&b, "  \"schema\": %q,\n", contractDraftProposalSchema)
	fmt.Fprintln(&b, `  "in_scope": ["Specific work to include."],`)
	fmt.Fprintln(&b, `  "out_of_scope": ["Specific work to exclude."],`)
	fmt.Fprintln(&b, `  "acceptance": ["Observable completion criterion."],`)
	fmt.Fprintln(&b, `  "validation": ["command to run"],`)
	fmt.Fprintln(&b, `  "assumptions": ["Assumption the human should review."]`)
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Use empty arrays for fields that need no additions.")
	return b.String()
}

func renderContractDraftProposalMD(proposal contractDraftProposalDocument) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Contract Draft Proposal")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Status")
	fmt.Fprintf(&b, "- Run id: %s\n", proposal.RunID)
	fmt.Fprintf(&b, "- Status: %s\n", proposal.Status)
	fmt.Fprintf(&b, "- Source: %s\n", proposal.Source)
	fmt.Fprintf(&b, "- Drafter attempt: %s\n", proposal.DrafterAttemptID)
	fmt.Fprintf(&b, "- Drafter: %s\n", proposal.Drafter)
	if proposal.AcceptedBy != nil {
		fmt.Fprintf(&b, "- Accepted by: %s\n", *proposal.AcceptedBy)
	}
	if proposal.AcceptedAt != nil {
		fmt.Fprintf(&b, "- Accepted at: %s\n", *proposal.AcceptedAt)
	}
	fmt.Fprintln(&b)
	writeContractDraftProposalList(&b, "In scope", proposal.InScope)
	writeContractDraftProposalList(&b, "Out of scope", proposal.OutOfScope)
	writeContractDraftProposalList(&b, "Acceptance criteria", proposal.Acceptance)
	writeContractDraftProposalList(&b, "Validation commands", proposal.Validation)
	writeContractDraftProposalList(&b, "Assumptions", proposal.Assumptions)
	if len(proposal.Warnings) > 0 {
		fmt.Fprintln(&b, "## Warnings")
		for _, warning := range proposal.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	return b.String()
}

func writeContractDraftProposalList(b *strings.Builder, heading string, values []string) {
	fmt.Fprintf(b, "## %s\n", heading)
	if len(values) == 0 {
		fmt.Fprintln(b, "- None")
		fmt.Fprintln(b)
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
	fmt.Fprintln(b)
}

func writeContractDraft(stdout io.Writer, response contractDraftResponse, request contractDrafterRequestDocument, drafterName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Contract draft proposal recorded")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	writeResolved(stdout, drafterName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", response.AttemptID)
	fmt.Fprintf(stdout, "  drafter: %s\n", response.Drafter)
	fmt.Fprintf(stdout, "  command: %s\n", formatAgentCommand(request.WouldRun))
	fmt.Fprintln(stdout)
	writeContractDraftProposalSummary(stdout, response.Proposal)
	writeContractDraftWarnings(stdout, response.Warnings)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  request: %s\n", runArtifactRepoRel(response.RunID, filepath.ToSlash(filepath.Join(contractDrafterAttemptsArtifact, response.AttemptID, "request.json"))))
	fmt.Fprintf(stdout, "  result: %s\n", runArtifactRepoRel(response.RunID, filepath.ToSlash(filepath.Join(contractDrafterAttemptsArtifact, response.AttemptID, "result.json"))))
	fmt.Fprintf(stdout, "  stdout: %s\n", runArtifactRepoRel(response.RunID, response.Result.Stdout))
	fmt.Fprintf(stdout, "  stderr: %s\n", runArtifactRepoRel(response.RunID, response.Result.Stderr))
	fmt.Fprintf(stdout, "  proposal: %s\n", runArtifactRepoRel(response.RunID, contractDraftProposalArtifact))
	fmt.Fprintf(stdout, "  proposal preview: %s\n", runArtifactRepoRel(response.RunID, contractDraftProposalMDArtifact))
	fmt.Fprintf(stdout, "  last result: %s\n", runArtifactRepoRel(response.RunID, contractDrafterLastResultArtifact))
}

func writeContractDraftRunOnly(stdout io.Writer, request contractDrafterRequestDocument, result contractDrafterResultDocument, drafterName string, modelSpec agents.ModelSpec) {
	fmt.Fprintln(stdout, "Contract drafter attempt finished")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintln(stdout)
	writeResolved(stdout, drafterName, modelSpec)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Attempt:")
	fmt.Fprintf(stdout, "  id: %s\n", result.AttemptID)
	fmt.Fprintf(stdout, "  drafter: %s\n", result.Drafter)
	fmt.Fprintf(stdout, "  command: %s\n", formatAgentCommand(request.WouldRun))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Result:")
	fmt.Fprintf(stdout, "  exit code: %d\n", result.ExitCode)
	fmt.Fprintf(stdout, "  timed out: %t\n", result.TimedOut)
}

func writeContractShowDraft(stdout io.Writer, response contractShowDraftResponse, proposalMD string) {
	fmt.Fprintln(stdout, "Contract draft proposal")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, proposalMD)
	if !strings.HasSuffix(proposalMD, "\n") {
		fmt.Fprintln(stdout)
	}
}

func writeContractAcceptDraft(stdout io.Writer, response contractAcceptDraftResponse) {
	fmt.Fprintln(stdout, "Contract draft proposal accepted")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	writeApprovalSummary(stdout, response.Approval)
	if response.ApprovalReset {
		fmt.Fprintln(stdout, "  reset: true")
	}
	fmt.Fprintln(stdout)
	writeContractDraftProposalSummary(stdout, response.Proposal)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintf(stdout, "  pactum contract approve %s\n", response.RunID)
}

func writeContractDraftProposalSummary(stdout io.Writer, proposal contractDraftProposalDocument) {
	fmt.Fprintln(stdout, "Proposal:")
	fmt.Fprintf(stdout, "  status: %s\n", proposal.Status)
	fmt.Fprintf(stdout, "  in scope: %d\n", len(proposal.InScope))
	fmt.Fprintf(stdout, "  out of scope: %d\n", len(proposal.OutOfScope))
	fmt.Fprintf(stdout, "  acceptance: %d\n", len(proposal.Acceptance))
	fmt.Fprintf(stdout, "  validation: %d\n", len(proposal.Validation))
	fmt.Fprintf(stdout, "  assumptions: %d\n", len(proposal.Assumptions))
}

func writeContractDraftWarnings(stdout io.Writer, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Warnings:")
	for _, warning := range warnings {
		fmt.Fprintf(stdout, "  - %s\n", warning)
	}
}
