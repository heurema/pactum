package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	runSchema      = "pactum.run.v1alpha1"
	contractSchema = "pactum.contract.v1alpha1"
)

type contractRunState struct {
	Schema    string               `json:"schema"`
	RunID     string               `json:"run_id"`
	Status    string               `json:"status"`
	Task      string               `json:"task"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	RepoRoot  string               `json:"repo_root"`
	Workspace string               `json:"workspace"`
	Artifacts contractRunArtifacts `json:"artifacts"`
}

type contractRunArtifacts struct {
	Task            string `json:"task"`
	RepoContext     string `json:"repo_context"`
	ExecutorContext string `json:"executor_context"`
	ContractJSON    string `json:"contract_json"`
	ContractMD      string `json:"contract_md"`
	Prompt          string `json:"prompt"`
	PromptManifest  string `json:"prompt_manifest"`
	Approval        string `json:"approval"`
}

type draftContract struct {
	Schema             string             `json:"schema"`
	RunID              string             `json:"run_id"`
	Status             string             `json:"status"`
	Goal               string             `json:"goal"`
	Scope              draftContractScope `json:"scope"`
	PathsInScope       []string           `json:"paths_in_scope,omitempty"`
	PathsOutOfScope    []string           `json:"paths_out_of_scope,omitempty"`
	AcceptanceCriteria []string           `json:"acceptance_criteria"`
	Validation         draftValidation    `json:"validation"`
	Assumptions        []string           `json:"assumptions"`
	OpenQuestions      []string           `json:"open_questions"`
	Clarifications     contractClarifySet `json:"clarifications,omitempty"`
	MemoryContext      draftMemoryContext `json:"memory_context"`
}

type draftContractScope struct {
	In  []string `json:"in"`
	Out []string `json:"out"`
}

type draftValidation struct {
	Commands []string `json:"commands"`
}

type draftMemoryContext struct {
	UsedItems []string `json:"used_items"`
}

type approvalState struct {
	Schema         string  `json:"schema"`
	Status         string  `json:"status"`
	ApprovedAt     *string `json:"approved_at"`
	ApprovedBy     *string `json:"approved_by"`
	ContractSHA256 *string `json:"contract_sha256"`
}

func (a App) createContractOnlyRun(root string, task string) (contractRunState, error) {
	paths := artifacts.New(root)

	createdAt := a.nowUTC()
	runID, runDir, err := reserveContractRunDir(createdAt, paths.RunsDir)
	if err != nil {
		return contractRunState{}, err
	}
	runPaths := contractRunPaths(runDir)
	if err := ensureDirs([]string{
		runPaths.ContextDir,
		runPaths.ClarifyDir,
		runPaths.ContractDir,
		runPaths.ExecuteDir,
		runPaths.ReviewDir,
		runPaths.MemoryDir,
		runPaths.LedgerDir,
	}); err != nil {
		return contractRunState{}, err
	}

	state := contractRunState{
		Schema:    runSchema,
		RunID:     runID,
		Status:    "contract_draft",
		Task:      task,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		RepoRoot:  ".",
		Workspace: artifacts.WorkspaceRel,
		Artifacts: contractRunArtifacts{
			Task:            "task.md",
			RepoContext:     "context/repo-context.md",
			ExecutorContext: "context/executor-context.md",
			ContractJSON:    "contract/contract.json",
			ContractMD:      "contract/contract.md",
			Prompt:          "contract/prompt.md",
			PromptManifest:  "contract/prompt-manifest.json",
			Approval:        "contract/approval.json",
		},
	}

	contract := draftContractFor(runID, task)
	memorySelection, err := buildAcceptedMemorySelection(paths, runID, task, "task", defaultMemorySelectionLimit, createdAt.Format(time.RFC3339))
	if err != nil {
		return contractRunState{}, err
	}
	files := map[string][]byte{
		runPaths.TaskMD:              renderTaskMD(task, createdAt),
		runPaths.RepoContext:         renderRepoContext(createdAt),
		runPaths.MemoryContextMD:     []byte(renderMemoryContextMD(memorySelection)),
		runPaths.QuestionsJSONL:      nil,
		runPaths.AnswersJSONL:        nil,
		runPaths.DecisionsJSONL:      nil,
		runPaths.UsageJSONL:          nil,
		runPaths.ContractMD:          renderContractMDFromDraft(contract),
		runPaths.PromptMD:            renderPromptMDFromDraft(contract),
		runPaths.MemorySelectionJSON: mustMarshalJSON(memorySelection),
	}
	files[runPaths.ContractJSON] = mustMarshalJSON(contract)
	files[runPaths.ApprovalJSON] = mustMarshalJSON(pendingApprovalState())
	files[runPaths.RunJSON] = mustMarshalJSON(state)

	for _, path := range sortedKeys(files) {
		if err := activeStore.WriteBytes(path, files[path], 0o644); err != nil {
			return contractRunState{}, err
		}
	}

	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "run_created", Timestamp: createdAt, RunID: runID}); err != nil {
		return contractRunState{}, err
	}
	if err := ledger.Append(activeStore, paths.EventsJSONL, ledger.Event{Type: "contract_draft_created", Timestamp: createdAt, RunID: runID}); err != nil {
		return contractRunState{}, err
	}

	return state, nil
}

type contractRunPathSet struct {
	ContextDir  string
	ClarifyDir  string
	ContractDir string
	ExecuteDir  string
	GateDir     string
	ReviewDir   string
	MemoryDir   string
	LedgerDir   string
	UsageJSONL  string

	RunJSON string
	TaskMD  string

	RepoContext         string
	MemoryContextMD     string
	MemorySelectionJSON string

	ExecutorContext string

	QuestionsJSONL string
	AnswersJSONL   string
	DecisionsJSONL string

	ClarifierContextMD      string
	ClarifierPromptMD       string
	ClarifierAttemptsDir    string
	ClarifierLastResultJSON string
	ClarifyLoopSummaryJSON  string

	ContractJSON   string
	ContractMD     string
	PromptMD       string
	PromptManifest string
	ApprovalJSON   string

	ContractDrafterContextMD      string
	ContractDrafterPromptMD       string
	ContractDrafterAttemptsDir    string
	ContractDrafterLastResultJSON string
	ContractDraftProposalJSON     string
	ContractDraftProposalMD       string

	ContractReviewDir              string
	ContractReviewAttemptsDir      string
	ContractReviewLastResultJSON   string
	ContractReviewFindingsJSONL    string
	ContractReviewResolutionsJSONL string

	ContractFixerDir            string
	ContractFixerPromptMD       string
	ContractFixerAttemptsDir    string
	ContractFixerLastResultJSON string

	DryRunJSON     string
	AttemptsDir    string
	LastResultJSON string

	GateReportJSON    string
	GateValidationDir string

	MemoryCandidateJSON  string
	MemoryCandidateMD    string
	MemoryAcceptanceJSON string

	ReviewJSON                   string
	ReviewFindingsJSONL          string
	ReviewResolutionsJSONL       string
	ReviewProposalsJSONL         string
	ReviewProposalDecisionsJSONL string
	ReviewContextMD              string
	ReviewDryRunJSON             string
	ReviewAttemptsDir            string
	ReviewLastResultJSON         string
	ReviewCriticAttemptsDir      string
	ReviewCriticLastResultJSON   string
	ReviewLoopSummaryJSON        string
	ReviewFixDir                 string
	ReviewFixContextMD           string
	ReviewFixPromptMD            string
	ReviewFixDryRunJSON          string
	ReviewFixAttemptsDir         string
	ReviewFixLastResultJSON      string

	TasksDir string
}

func contractRunPaths(runDir string) contractRunPathSet {
	contextDir := filepath.Join(runDir, "context")
	clarifyDir := filepath.Join(runDir, "clarify")
	contractDir := filepath.Join(runDir, "contract")
	executeDir := filepath.Join(runDir, "execute")
	gateDir := filepath.Join(runDir, "gate")
	reviewDir := filepath.Join(runDir, "review")
	return contractRunPathSet{
		ContextDir:                     contextDir,
		ClarifyDir:                     clarifyDir,
		ContractDir:                    contractDir,
		ExecuteDir:                     executeDir,
		GateDir:                        gateDir,
		ReviewDir:                      reviewDir,
		MemoryDir:                      filepath.Join(runDir, "memory"),
		LedgerDir:                      filepath.Join(runDir, "ledger"),
		UsageJSONL:                     filepath.Join(runDir, "ledger", "usage.jsonl"),
		RunJSON:                        filepath.Join(runDir, "run.json"),
		TaskMD:                         filepath.Join(runDir, "task.md"),
		RepoContext:                    filepath.Join(contextDir, "repo-context.md"),
		MemoryContextMD:                filepath.Join(contextDir, "memory-context.md"),
		MemorySelectionJSON:            filepath.Join(contextDir, "memory-selection.json"),
		ExecutorContext:                filepath.Join(contextDir, "executor-context.md"),
		QuestionsJSONL:                 filepath.Join(clarifyDir, "questions.jsonl"),
		AnswersJSONL:                   filepath.Join(clarifyDir, "answers.jsonl"),
		DecisionsJSONL:                 filepath.Join(clarifyDir, "decisions.jsonl"),
		ClarifierContextMD:             filepath.Join(clarifyDir, "clarifier-context.md"),
		ClarifierPromptMD:              filepath.Join(clarifyDir, "clarifier-prompt.md"),
		ClarifierAttemptsDir:           filepath.Join(clarifyDir, "clarifier-attempts"),
		ClarifierLastResultJSON:        filepath.Join(clarifyDir, "clarifier-last-result.json"),
		ClarifyLoopSummaryJSON:         filepath.Join(clarifyDir, "loop-summary.json"),
		ContractJSON:                   filepath.Join(contractDir, "contract.json"),
		ContractMD:                     filepath.Join(contractDir, "contract.md"),
		PromptMD:                       filepath.Join(contractDir, "prompt.md"),
		PromptManifest:                 filepath.Join(contractDir, "prompt-manifest.json"),
		ApprovalJSON:                   filepath.Join(contractDir, "approval.json"),
		ContractDrafterContextMD:       filepath.Join(contractDir, "drafter-context.md"),
		ContractDrafterPromptMD:        filepath.Join(contractDir, "drafter-prompt.md"),
		ContractDrafterAttemptsDir:     filepath.Join(contractDir, "drafter-attempts"),
		ContractDrafterLastResultJSON:  filepath.Join(contractDir, "drafter-last-result.json"),
		ContractDraftProposalJSON:      filepath.Join(contractDir, "draft-proposal.json"),
		ContractDraftProposalMD:        filepath.Join(contractDir, "draft-proposal.md"),
		ContractReviewDir:              filepath.Join(contractDir, "reviewer"),
		ContractReviewAttemptsDir:      filepath.Join(contractDir, "reviewer", "attempts"),
		ContractReviewLastResultJSON:   filepath.Join(contractDir, "reviewer", "last-result.json"),
		ContractReviewFindingsJSONL:    filepath.Join(contractDir, "reviewer", "findings.jsonl"),
		ContractReviewResolutionsJSONL: filepath.Join(contractDir, "reviewer", "resolutions.jsonl"),
		ContractFixerDir:               filepath.Join(contractDir, "fixer"),
		ContractFixerPromptMD:          filepath.Join(contractDir, "fixer", "fixer-prompt.md"),
		ContractFixerAttemptsDir:       filepath.Join(contractDir, "fixer", "attempts"),
		ContractFixerLastResultJSON:    filepath.Join(contractDir, "fixer", "last-result.json"),
		DryRunJSON:                     filepath.Join(executeDir, "dry-run.json"),
		AttemptsDir:                    filepath.Join(executeDir, "attempts"),
		LastResultJSON:                 filepath.Join(executeDir, "last-result.json"),
		GateReportJSON:                 filepath.Join(gateDir, "gate-report.json"),
		GateValidationDir:              filepath.Join(gateDir, "validation"),
		MemoryCandidateJSON:            filepath.Join(runDir, "memory", "memory-candidate.json"),
		MemoryCandidateMD:              filepath.Join(runDir, "memory", "memory-candidate.md"),
		MemoryAcceptanceJSON:           filepath.Join(runDir, "memory", "memory-acceptance.json"),
		ReviewJSON:                     filepath.Join(reviewDir, "review.json"),
		ReviewFindingsJSONL:            filepath.Join(reviewDir, "findings.jsonl"),
		ReviewResolutionsJSONL:         filepath.Join(reviewDir, "resolutions.jsonl"),
		ReviewProposalsJSONL:           filepath.Join(reviewDir, "proposals.jsonl"),
		ReviewProposalDecisionsJSONL:   filepath.Join(reviewDir, "proposal-decisions.jsonl"),
		ReviewContextMD:                filepath.Join(reviewDir, "reviewer-context.md"),
		ReviewDryRunJSON:               filepath.Join(reviewDir, "reviewer-dry-run.json"),
		ReviewAttemptsDir:              filepath.Join(reviewDir, "reviewer-attempts"),
		ReviewLastResultJSON:           filepath.Join(reviewDir, "reviewer-last-result.json"),
		ReviewCriticAttemptsDir:        filepath.Join(reviewDir, "critic-attempts"),
		ReviewCriticLastResultJSON:     filepath.Join(reviewDir, "critic-last-result.json"),
		ReviewLoopSummaryJSON:          filepath.Join(reviewDir, "loop-summary.json"),
		ReviewFixDir:                   filepath.Join(reviewDir, "fix"),
		ReviewFixContextMD:             filepath.Join(reviewDir, "fix", "fixer-context.md"),
		ReviewFixPromptMD:              filepath.Join(reviewDir, "fix", "fixer-prompt.md"),
		ReviewFixDryRunJSON:            filepath.Join(reviewDir, "fix", "fixer-dry-run.json"),
		ReviewFixAttemptsDir:           filepath.Join(reviewDir, "fix", "attempts"),
		ReviewFixLastResultJSON:        filepath.Join(reviewDir, "fix", "last-result.json"),
		TasksDir:                       filepath.Join(executeDir, "tasks"),
	}
}

func reserveContractRunDir(createdAt time.Time, runsDir string) (string, string, error) {
	if err := activeStore.MkdirAll(runsDir); err != nil {
		return "", "", err
	}

	base := "run_" + createdAt.Format("20060102_150405")
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s_%02d", base, suffix)
		}
		path := filepath.Join(runsDir, candidate)
		if err := activeStore.Mkdir(path); err == nil {
			return candidate, path, nil
		} else if os.IsExist(err) {
			continue
		} else {
			return "", "", err
		}
	}
}

func renderTaskMD(task string, generatedAt time.Time) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Task")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, task)
	fmt.Fprintln(&buffer)
	fmt.Fprintf(&buffer, "Generated: %s\n", generatedAt.Format(time.RFC3339))
	return buffer.Bytes()
}

func renderRepoContext(generatedAt time.Time) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Repository Context")
	fmt.Fprintln(&buffer)
	fmt.Fprintf(&buffer, "Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	fmt.Fprintln(&buffer, "Accepted memory context: context/memory-context.md")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "Notes:")
	fmt.Fprintln(&buffer, "- Pactum has not yet done agentic clarification.")
	fmt.Fprintln(&buffer, "- This is deterministic context assembled from the run task description.")
	return buffer.Bytes()
}

func draftContractFor(runID string, task string) draftContract {
	return draftContract{
		Schema: contractSchema,
		RunID:  runID,
		Status: "draft",
		Goal:   task,
		Scope: draftContractScope{
			In:  []string{},
			Out: []string{},
		},
		AcceptanceCriteria: []string{},
		Validation: draftValidation{
			Commands: []string{},
		},
		Assumptions:   []string{},
		OpenQuestions: []string{},
		Clarifications: contractClarifySet{
			Questions: []clarifyQuestionStatus{},
		},
		MemoryContext: draftMemoryContext{
			UsedItems: []string{},
		},
	}
}

func renderContractMDFromDraft(contract draftContract) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Contract Draft")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Goal")
	writeMarkdownValue(&buffer, contract.Goal)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Current status")
	fmt.Fprintf(&buffer, "Contract status: %s\n", contract.Status)
	fmt.Fprintln(&buffer, "Manual clarification, contract approval, prompt build, and agent execution are available through staged Pactum commands.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Clarifications")
	if len(contract.Clarifications.Questions) == 0 {
		fmt.Fprintln(&buffer, "- None")
	} else {
		for _, question := range contract.Clarifications.Questions {
			blocking := ""
			if question.Blocking {
				blocking = " [blocking]"
			}
			fmt.Fprintf(&buffer, "- %s%s — %s\n", question.ID, blocking, question.Question)
			if question.Rationale != "" {
				fmt.Fprintf(&buffer, "  Rationale: %s\n", question.Rationale)
			}
			if question.Answer != "" {
				fmt.Fprintf(&buffer, "  Answer: %s\n", question.Answer)
			} else {
				fmt.Fprintln(&buffer, "  Answer: pending")
			}
		}
	}
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "In scope", contract.Scope.In)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Out of scope", contract.Scope.Out)
	writePathScopeSections(&buffer, contract)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Acceptance criteria", contract.AcceptanceCriteria)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Validation commands", contract.Validation.Commands)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Assumptions", contract.Assumptions)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Open questions")
	if len(contract.OpenQuestions) == 0 {
		fmt.Fprintln(&buffer, "- None")
	} else {
		for _, question := range contract.OpenQuestions {
			fmt.Fprintf(&buffer, "- %s\n", question)
		}
	}
	return buffer.Bytes()
}

func renderPromptMDFromDraft(contract draftContract) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Executor Prompt")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "This is a contract-draft placeholder. Run `pactum prompt build` after the contract is approved to build the executor prompt for `pactum execute`.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Goal")
	writeMarkdownValue(&buffer, contract.Goal)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "In scope", contract.Scope.In)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Out of scope", contract.Scope.Out)
	writePathScopeSections(&buffer, contract)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Acceptance criteria", contract.AcceptanceCriteria)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Validation commands", contract.Validation.Commands)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Project context")
	fmt.Fprintln(&buffer, "- Accepted memory context: context/memory-context.md")
	return buffer.Bytes()
}

func writeMarkdownValue(buffer *bytes.Buffer, value string) {
	if strings.TrimSpace(value) == "" {
		fmt.Fprintln(buffer, "TBD")
		return
	}
	fmt.Fprintln(buffer, value)
}

func writeMarkdownListSection(buffer *bytes.Buffer, heading string, values []string) {
	fmt.Fprintf(buffer, "## %s\n", heading)
	if len(values) == 0 {
		fmt.Fprintln(buffer, "TBD")
		return
	}
	for _, value := range values {
		fmt.Fprintf(buffer, "- %s\n", value)
	}
}

func writePathScopeSections(buffer *bytes.Buffer, contract draftContract) {
	if len(contract.PathsInScope) == 0 && len(contract.PathsOutOfScope) == 0 {
		return
	}
	fmt.Fprintln(buffer)
	if len(contract.PathsInScope) > 0 {
		writeMarkdownListSection(buffer, "Paths in scope", contract.PathsInScope)
		fmt.Fprintln(buffer)
	}
	if len(contract.PathsOutOfScope) > 0 {
		writeMarkdownListSection(buffer, "Paths out of scope", contract.PathsOutOfScope)
	}
}

func runArtifactRepoRel(runID string, artifactPath string) string {
	return filepath.ToSlash(filepath.Join(artifacts.WorkspaceRel, "runs", runID, artifactPath))
}

func toRepoRel(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func mustMarshalJSON(value any) []byte {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
