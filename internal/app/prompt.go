package app

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const promptManifestSchema = "pactum.executor_prompt.v1"

type promptManifest struct {
	Schema         string                  `json:"schema"`
	RunID          string                  `json:"run_id"`
	BuiltAt        string                  `json:"built_at"`
	Status         string                  `json:"status"`
	ContractSHA256 string                  `json:"contract_sha256"`
	ApprovalStatus string                  `json:"approval_status"`
	ApprovedBy     string                  `json:"approved_by"`
	ApprovedAt     string                  `json:"approved_at"`
	MapRunID       string                  `json:"map_run_id"`
	Artifacts      promptManifestArtifacts `json:"artifacts"`
	Checks         promptManifestChecks    `json:"checks"`
}

type promptManifestArtifacts struct {
	Prompt          string `json:"prompt"`
	ExecutorContext string `json:"executor_context"`
	Contract        string `json:"contract"`
	Approval        string `json:"approval"`
	RepoContext     string `json:"repo_context"`
	SearchResults   string `json:"search_results"`
}

type promptManifestChecks struct {
	ContractApproved            bool `json:"contract_approved"`
	ContractHashMatchesApproval bool `json:"contract_hash_matches_approval"`
	ProjectMapFresh             bool `json:"project_map_fresh"`
	BlockingClarificationsOpen  int  `json:"blocking_clarifications_open"`
}

type promptBuildResponse struct {
	RunID     string         `json:"run_id"`
	RunStatus string         `json:"run_status"`
	Manifest  promptManifest `json:"manifest"`
}

type promptShowResponse struct {
	RunID     string         `json:"run_id"`
	RunStatus string         `json:"run_status"`
	Manifest  promptManifest `json:"manifest"`
	Prompt    string         `json:"prompt"`
}

type promptShowNotBuiltResponse struct {
	RunID            string `json:"run_id"`
	RunStatus        string `json:"run_status"`
	Ready            bool   `json:"ready"`
	Message          string `json:"message"`
	SuggestedCommand string `json:"suggested_command"`
}

func (a App) PromptBuild(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}

	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return err
	}
	if status.BlockingOpen > 0 {
		return fmt.Errorf("cannot build executor prompt: blocking clarification questions remain")
	}
	if context.Approval.Status != "approved" || context.Approval.ContractSHA256 == nil {
		return fmt.Errorf("cannot build executor prompt: contract is not approved")
	}
	hash, err := fileSHA256(context.RunPaths.ContractJSON)
	if err != nil {
		return err
	}
	if hash != *context.Approval.ContractSHA256 {
		return fmt.Errorf("cannot build executor prompt: approved contract hash does not match current contract")
	}

	report, err := a.workspaceStatus(context.Root)
	if err != nil {
		return err
	}
	if report.ProjectMap.Status != "fresh" {
		if !jsonOutput {
			fmt.Fprintln(stdout, "Project map is stale")
			fmt.Fprintln(stdout)
			fmt.Fprintln(stdout, "Suggested:")
			fmt.Fprintln(stdout, "  pactum map refresh")
		}
		return fmt.Errorf("cannot build executor prompt: project map is stale")
	}

	now := a.nowUTC()
	manifest := buildPromptManifest(context, hash, report.ProjectMap.RunID, now)
	searchResults, err := readRunSearchResults(context.RunPaths.SearchResults)
	if err != nil {
		return err
	}
	decisions, err := readClarificationDecisions(context.RunPaths.DecisionsJSONL)
	if err != nil {
		return err
	}

	if err := os.WriteFile(context.RunPaths.ExecutorContext, renderExecutorContext(context.State, report.ProjectMap.RunID, hash, searchResults, decisions), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(context.RunPaths.PromptMD, renderApprovedPromptMD(context.Contract, context.State.RunID, hash), 0o644); err != nil {
		return err
	}
	if err := writeJSON(context.RunPaths.PromptManifest, manifest); err != nil {
		return err
	}
	if err := ensurePromptArtifactRefs(context.RunPaths, context.State); err != nil {
		return err
	}
	if err := ledger.Append(context.Paths.EventsJSONL, ledger.Event{Type: "executor_prompt_built", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := promptBuildResponse{RunID: runID, RunStatus: context.State.Status, Manifest: manifest}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writePromptBuild(stdout, response)
	return nil
}

func (a App) PromptShow(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadContractContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	if !isRegularFile(context.RunPaths.PromptManifest) {
		message := "Executor prompt has not been built. Run: pactum prompt build " + runID
		if jsonOutput {
			return writeJSONResponse(stdout, promptShowNotBuiltResponse{
				RunID:            runID,
				RunStatus:        context.State.Status,
				Ready:            false,
				Message:          message,
				SuggestedCommand: "pactum prompt build " + runID,
			})
		}
		fmt.Fprintf(stdout, "Executor prompt has not been built. Run: pactum prompt build %s\n", runID)
		return nil
	}

	manifest, err := readPromptManifest(context.RunPaths.PromptManifest)
	if err != nil {
		return err
	}
	prompt, err := os.ReadFile(context.RunPaths.PromptMD)
	if err != nil {
		return err
	}
	response := promptShowResponse{
		RunID:     runID,
		RunStatus: context.State.Status,
		Manifest:  manifest,
		Prompt:    string(prompt),
	}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writePromptShow(stdout, response)
	return nil
}

func buildPromptManifest(context contractContext, contractSHA256 string, mapRunID string, builtAt time.Time) promptManifest {
	approvedBy := ""
	if context.Approval.ApprovedBy != nil {
		approvedBy = *context.Approval.ApprovedBy
	}
	approvedAt := ""
	if context.Approval.ApprovedAt != nil {
		approvedAt = *context.Approval.ApprovedAt
	}
	return promptManifest{
		Schema:         promptManifestSchema,
		RunID:          context.State.RunID,
		BuiltAt:        builtAt.Format(time.RFC3339),
		Status:         "ready",
		ContractSHA256: contractSHA256,
		ApprovalStatus: "approved",
		ApprovedBy:     approvedBy,
		ApprovedAt:     approvedAt,
		MapRunID:       mapRunID,
		Artifacts: promptManifestArtifacts{
			Prompt:          "contract/prompt.md",
			ExecutorContext: "context/executor-context.md",
			Contract:        "contract/contract.json",
			Approval:        "contract/approval.json",
			RepoContext:     "context/repo-context.md",
			SearchResults:   "context/search-results.json",
		},
		Checks: promptManifestChecks{
			ContractApproved:            true,
			ContractHashMatchesApproval: true,
			ProjectMapFresh:             true,
			BlockingClarificationsOpen:  0,
		},
	}
}

func readPromptManifest(path string) (promptManifest, error) {
	var manifest promptManifest
	return manifest, readJSONFile(path, &manifest)
}

func readRunSearchResults(path string) (runSearchResults, error) {
	var results runSearchResults
	err := readJSONFile(path, &results)
	return results, err
}

func ensurePromptArtifactRefs(runPaths contractRunPathSet, state contractRunState) error {
	updated := false
	if state.Artifacts.ExecutorContext == "" {
		state.Artifacts.ExecutorContext = "context/executor-context.md"
		updated = true
	}
	if state.Artifacts.PromptManifest == "" {
		state.Artifacts.PromptManifest = "contract/prompt-manifest.json"
		updated = true
	}
	if !updated {
		return nil
	}
	return writeJSON(runPaths.RunJSON, state)
}

func removePromptReadinessArtifacts(runPaths contractRunPathSet) error {
	if err := os.Remove(runPaths.PromptManifest); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func renderExecutorContext(state contractRunState, mapRunID string, contractSHA256 string, searchResults runSearchResults, decisions []clarificationDecisionRecord) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Executor Context")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Run")
	fmt.Fprintf(&buffer, "- Run id: %s\n", state.RunID)
	fmt.Fprintf(&buffer, "- Run status: %s\n", state.Status)
	fmt.Fprintf(&buffer, "- Map run id: %s\n", mapRunID)
	fmt.Fprintf(&buffer, "- Contract hash: %s\n", contractSHA256)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Project map")
	fmt.Fprintf(&buffer, "- Repo map: %s\n", filepathToSlash(artifacts.WorkspaceRel+"/map/repo-map.md"))
	fmt.Fprintf(&buffer, "- LLM map pointer: %s\n", filepathToSlash(artifacts.WorkspaceRel+"/map/llms.txt"))
	fmt.Fprintf(&buffer, "- Search index: %s\n", filepathToSlash(artifacts.WorkspaceRel+"/map/search.sqlite"))
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Retrieval guidance")
	fmt.Fprintln(&buffer, "- Use `pactum search \"<term>\"` before adding new code.")
	fmt.Fprintln(&buffer, "- Prefer existing code items when applicable.")
	fmt.Fprintln(&buffer, "- If ownership is unclear, stop and ask for clarification.")
	fmt.Fprintln(&buffer, "- Do not rely on this map as complete semantic truth.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Relevant search results")
	if len(searchResults.Results) == 0 {
		fmt.Fprintln(&buffer, "- None")
	} else {
		for _, result := range searchResults.Results {
			titleLabel := "title"
			if result.Kind == "code_item" {
				titleLabel = "name"
			}
			fmt.Fprintf(&buffer, "- kind: %s\n", result.Kind)
			fmt.Fprintf(&buffer, "  path: %s\n", result.Path)
			if result.Title != "" {
				fmt.Fprintf(&buffer, "  %s: %s\n", titleLabel, result.Title)
			}
			if result.Language != "" {
				fmt.Fprintf(&buffer, "  language: %s\n", result.Language)
			}
			if result.CodeKind != "" {
				fmt.Fprintf(&buffer, "  code kind: %s\n", result.CodeKind)
			}
		}
	}
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Clarification decisions")
	if len(decisions) == 0 {
		fmt.Fprintln(&buffer, "- None")
	} else {
		for _, decision := range decisions {
			fmt.Fprintf(&buffer, "- %s: %s\n", decision.QuestionID, decision.Decision)
		}
	}
	return buffer.Bytes()
}

func renderApprovedPromptMD(contract draftContract, runID string, contractSHA256 string) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Executor Prompt")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "This prompt is prepared from an approved Pactum contract.")
	fmt.Fprintln(&buffer, "Pactum does not execute agents in this milestone.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Contract status")
	fmt.Fprintf(&buffer, "- Run: %s\n", runID)
	fmt.Fprintln(&buffer, "- Approval: approved")
	fmt.Fprintf(&buffer, "- Contract hash: %s\n", contractSHA256)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Goal")
	writeMarkdownValue(&buffer, contract.Goal)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "In scope", contract.Scope.In)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Out of scope", contract.Scope.Out)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Acceptance criteria", contract.AcceptanceCriteria)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Validation commands", contract.Validation.Commands)
	fmt.Fprintln(&buffer)
	writeMarkdownListSection(&buffer, "Assumptions", contract.Assumptions)
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
			fmt.Fprintf(&buffer, "- %s%s %s\n", question.ID, blocking, question.Question)
			decision := question.Answer
			if strings.TrimSpace(decision) == "" {
				decision = "pending"
			}
			fmt.Fprintf(&buffer, "  Decision: %s\n", decision)
		}
	}
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Project context")
	fmt.Fprintln(&buffer, "- Executor context: context/executor-context.md")
	fmt.Fprintf(&buffer, "- Repo map: %s\n", artifacts.WorkspaceRel+"/map/repo-map.md")
	fmt.Fprintln(&buffer, "- Search results: context/search-results.json")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Instructions for future executor")
	fmt.Fprintln(&buffer, "- Follow the approved contract.")
	fmt.Fprintln(&buffer, "- Do not implement out-of-scope work.")
	fmt.Fprintln(&buffer, "- Search before creating new code.")
	fmt.Fprintln(&buffer, "- Prefer existing code items when applicable.")
	fmt.Fprintln(&buffer, "- If the contract is ambiguous, stop and request clarification.")
	fmt.Fprintln(&buffer, "- Run listed validation commands when execution becomes available.")
	return buffer.Bytes()
}

func writePromptBuild(stdout io.Writer, response promptBuildResponse) {
	fmt.Fprintln(stdout, "Executor prompt built")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Checks:")
	fmt.Fprintln(stdout, "  contract approved: yes")
	fmt.Fprintln(stdout, "  contract hash: ok")
	fmt.Fprintln(stdout, "  project map: fresh")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  prompt: %s\n", runArtifactRepoRel(response.RunID, response.Manifest.Artifacts.Prompt))
	fmt.Fprintf(stdout, "  executor context: %s\n", runArtifactRepoRel(response.RunID, response.Manifest.Artifacts.ExecutorContext))
	fmt.Fprintf(stdout, "  prompt manifest: %s\n", runArtifactRepoRel(response.RunID, "contract/prompt-manifest.json"))
}

func writePromptShow(stdout io.Writer, response promptShowResponse) {
	fmt.Fprintln(stdout, "Executor Prompt")
	fmt.Fprintf(stdout, "Run: %s\n", response.RunID)
	fmt.Fprintf(stdout, "Status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, response.Prompt)
	if !strings.HasSuffix(response.Prompt, "\n") {
		fmt.Fprintln(stdout)
	}
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
