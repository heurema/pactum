package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
	searchpkg "github.com/heurema/pactum/internal/search"
)

const maxRepoMapContextBytes = 20000

type contractRunState struct {
	Schema    string               `json:"schema"`
	RunID     string               `json:"run_id"`
	Status    string               `json:"status"`
	Task      string               `json:"task"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	RepoRoot  string               `json:"repo_root"`
	Workspace string               `json:"workspace"`
	MapRunID  string               `json:"map_run_id"`
	Artifacts contractRunArtifacts `json:"artifacts"`
}

type contractRunArtifacts struct {
	Task          string `json:"task"`
	RepoContext   string `json:"repo_context"`
	SearchResults string `json:"search_results"`
	ContractJSON  string `json:"contract_json"`
	ContractMD    string `json:"contract_md"`
	Prompt        string `json:"prompt"`
	Approval      string `json:"approval"`
}

type runSearchResults struct {
	Query    string             `json:"query"`
	Results  []searchpkg.Result `json:"results"`
	Warnings []string           `json:"warnings,omitempty"`
}

type draftContract struct {
	Schema             string             `json:"schema"`
	RunID              string             `json:"run_id"`
	Status             string             `json:"status"`
	Goal               string             `json:"goal"`
	Scope              draftContractScope `json:"scope"`
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
	Status     string  `json:"status"`
	ApprovedAt *string `json:"approved_at"`
	ApprovedBy *string `json:"approved_by"`
}

func (a App) RunContract(stdout io.Writer, task string, contractOnly bool, jsonOutput bool) error {
	root, workspace, err := a.resolveStatusRoot()
	if err != nil {
		return err
	}
	if workspace == "" {
		fmt.Fprintln(stdout, "Pactum is not initialized. Run: pactum init")
		return nil
	}
	if !contractOnly {
		fmt.Fprintln(stdout, "Execution is not implemented yet. Use --contract-only.")
		return nil
	}

	result, err := a.createContractOnlyRun(root, task)
	if err != nil {
		return err
	}
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	writeRunCreated(stdout, result)
	return nil
}

func (a App) createContractOnlyRun(root string, task string) (contractRunState, error) {
	paths := artifacts.New(root)
	report, err := a.workspaceStatus(root)
	if err != nil {
		return contractRunState{}, err
	}

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
		Schema:    "pactum.run.v1",
		RunID:     runID,
		Status:    "contract_draft",
		Task:      task,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		RepoRoot:  ".",
		Workspace: artifacts.WorkspaceRel,
		MapRunID:  report.ProjectMap.RunID,
		Artifacts: contractRunArtifacts{
			Task:          "task.md",
			RepoContext:   "context/repo-context.md",
			SearchResults: "context/search-results.json",
			ContractJSON:  "contract/contract.json",
			ContractMD:    "contract/contract.md",
			Prompt:        "contract/prompt.md",
			Approval:      "contract/approval.json",
		},
	}

	searchResults := buildRunSearchResults(paths, report.ProjectMap, task)
	files := map[string][]byte{
		runPaths.TaskMD:         renderTaskMD(task, createdAt),
		runPaths.RepoContext:    renderRepoContext(root, paths, report.ProjectMap.RunID, createdAt),
		runPaths.ContractMD:     renderContractMD(task, report.ProjectMap.RunID, len(searchResults.Results)),
		runPaths.PromptMD:       renderPromptMD(task),
		runPaths.QuestionsJSONL: nil,
		runPaths.AnswersJSONL:   nil,
		runPaths.DecisionsJSONL: nil,
	}
	files[runPaths.SearchResults] = mustMarshalJSON(searchResults)
	files[runPaths.ContractJSON] = mustMarshalJSON(draftContractFor(runID, task))
	files[runPaths.ApprovalJSON] = mustMarshalJSON(approvalState{Status: "pending"})
	files[runPaths.RunJSON] = mustMarshalJSON(state)

	for _, path := range sortedKeys(files) {
		if err := os.WriteFile(path, files[path], 0o644); err != nil {
			return contractRunState{}, err
		}
	}

	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "run_created", Timestamp: createdAt, RunID: runID, RepoRoot: root}); err != nil {
		return contractRunState{}, err
	}
	if err := ledger.Append(paths.EventsJSONL, ledger.Event{Type: "contract_draft_created", Timestamp: createdAt, RunID: runID, RepoRoot: root}); err != nil {
		return contractRunState{}, err
	}

	return state, nil
}

type contractRunPathSet struct {
	ContextDir  string
	ClarifyDir  string
	ContractDir string
	ExecuteDir  string
	ReviewDir   string
	MemoryDir   string
	LedgerDir   string

	RunJSON string
	TaskMD  string

	RepoContext   string
	SearchResults string

	QuestionsJSONL string
	AnswersJSONL   string
	DecisionsJSONL string

	ContractJSON string
	ContractMD   string
	PromptMD     string
	ApprovalJSON string
}

func contractRunPaths(runDir string) contractRunPathSet {
	contextDir := filepath.Join(runDir, "context")
	clarifyDir := filepath.Join(runDir, "clarify")
	contractDir := filepath.Join(runDir, "contract")
	return contractRunPathSet{
		ContextDir:     contextDir,
		ClarifyDir:     clarifyDir,
		ContractDir:    contractDir,
		ExecuteDir:     filepath.Join(runDir, "execute"),
		ReviewDir:      filepath.Join(runDir, "review"),
		MemoryDir:      filepath.Join(runDir, "memory"),
		LedgerDir:      filepath.Join(runDir, "ledger"),
		RunJSON:        filepath.Join(runDir, "run.json"),
		TaskMD:         filepath.Join(runDir, "task.md"),
		RepoContext:    filepath.Join(contextDir, "repo-context.md"),
		SearchResults:  filepath.Join(contextDir, "search-results.json"),
		QuestionsJSONL: filepath.Join(clarifyDir, "questions.jsonl"),
		AnswersJSONL:   filepath.Join(clarifyDir, "answers.jsonl"),
		DecisionsJSONL: filepath.Join(clarifyDir, "decisions.jsonl"),
		ContractJSON:   filepath.Join(contractDir, "contract.json"),
		ContractMD:     filepath.Join(contractDir, "contract.md"),
		PromptMD:       filepath.Join(contractDir, "prompt.md"),
		ApprovalJSON:   filepath.Join(contractDir, "approval.json"),
	}
}

func reserveContractRunDir(createdAt time.Time, runsDir string) (string, string, error) {
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return "", "", err
	}

	base := "run_" + createdAt.Format("20060102_150405")
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s_%02d", base, suffix)
		}
		path := filepath.Join(runsDir, candidate)
		if err := os.Mkdir(path, 0o755); err == nil {
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

func renderRepoContext(root string, paths artifacts.Paths, mapRunID string, generatedAt time.Time) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Repository Context")
	fmt.Fprintln(&buffer)
	fmt.Fprintf(&buffer, "Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	fmt.Fprintf(&buffer, "Map run: %s\n", mapRunID)
	fmt.Fprintf(&buffer, "Repo map path: %s\n", toRepoRel(root, paths.RepoMap))
	fmt.Fprintf(&buffer, "LLMS path: %s\n", toRepoRel(root, paths.LLMS))
	fmt.Fprintf(&buffer, "Search index path: %s\n\n", toRepoRel(root, paths.SearchSQLite))
	fmt.Fprintln(&buffer, "Notes:")
	fmt.Fprintln(&buffer, "- Pactum has not yet done agentic clarification.")
	fmt.Fprintln(&buffer, "- This is deterministic context assembled from existing map artifacts.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Project map")
	fmt.Fprintln(&buffer)

	repoMap, err := os.ReadFile(paths.RepoMap)
	if err != nil {
		fmt.Fprintf(&buffer, "Project map is unavailable at %s.\n", toRepoRel(root, paths.RepoMap))
		return buffer.Bytes()
	}
	repoMap = sanitizeRepoMapForRunContext(root, repoMap)
	if len(repoMap) <= maxRepoMapContextBytes {
		buffer.Write(repoMap)
		if !bytes.HasSuffix(repoMap, []byte("\n")) {
			fmt.Fprintln(&buffer)
		}
		return buffer.Bytes()
	}
	buffer.Write(repoMap[:maxRepoMapContextBytes])
	if !bytes.HasSuffix(repoMap[:maxRepoMapContextBytes], []byte("\n")) {
		fmt.Fprintln(&buffer)
	}
	fmt.Fprintf(&buffer, "\n[Truncated to %d bytes from %s]\n", maxRepoMapContextBytes, toRepoRel(root, paths.RepoMap))
	return buffer.Bytes()
}

func sanitizeRepoMapForRunContext(root string, repoMap []byte) []byte {
	sanitized := bytes.ReplaceAll(repoMap, []byte(root), []byte("."))
	slashRoot := filepath.ToSlash(root)
	if slashRoot != root {
		sanitized = bytes.ReplaceAll(sanitized, []byte(slashRoot), []byte("."))
	}
	return sanitized
}

func buildRunSearchResults(paths artifacts.Paths, mapStatus projectMapStatus, task string) runSearchResults {
	result := runSearchResults{Query: task, Results: []searchpkg.Result{}}
	if mapStatus.Status == "stale" {
		result.Warnings = append(result.Warnings, "Search index is stale. Run: pactum map refresh.")
		return result
	}
	if mapStatus.SearchIndex != "ready" {
		result.Warnings = append(result.Warnings, "Search index is missing. Run: pactum map refresh.")
		return result
	}
	response, err := searchpkg.Query(paths.SearchSQLite, searchpkg.QueryOptions{
		Query: task,
		Limit: 10,
		Kind:  searchpkg.KindAny,
	})
	if err != nil {
		if searchpkg.IsMissingIndex(err) {
			result.Warnings = append(result.Warnings, "Search index is missing. Run: pactum map refresh.")
			return result
		}
		result.Warnings = append(result.Warnings, "Search failed: "+err.Error())
		return result
	}
	result.Results = response.Results
	if result.Results == nil {
		result.Results = []searchpkg.Result{}
	}
	return result
}

func draftContractFor(runID string, task string) draftContract {
	return draftContract{
		Schema: "pactum.contract.v1",
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
			Questions: []contractClarifyQuestion{},
		},
		MemoryContext: draftMemoryContext{
			UsedItems: []string{},
		},
	}
}

func renderContractMD(task string, mapRunID string, searchResults int) []byte {
	contract := draftContractFor("", task)
	return renderContractMDFromDraft(contract, mapRunID, searchResults)
}

func renderContractMDFromDraft(contract draftContract, mapRunID string, searchResults int) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Contract Draft")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Goal")
	fmt.Fprintln(&buffer, contract.Goal)
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Current status")
	fmt.Fprintln(&buffer, "Draft only. Manual clarification is available; approval and agent execution are not implemented yet.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Relevant repository context")
	fmt.Fprintf(&buffer, "- Map run: %s\n", mapRunID)
	fmt.Fprintf(&buffer, "- Repo map: %s\n", filepath.ToSlash(filepath.Join(artifacts.WorkspaceRel, "map", "repo-map.md")))
	fmt.Fprintf(&buffer, "- Search results: context/search-results.json (%d result(s))\n", searchResults)
	fmt.Fprintln(&buffer)
	if len(contract.Clarifications.Questions) > 0 {
		fmt.Fprintln(&buffer, "## Clarifications")
		fmt.Fprintln(&buffer)
		for _, question := range contract.Clarifications.Questions {
			blocking := ""
			if question.Blocking {
				blocking = " [blocking]"
			}
			fmt.Fprintf(&buffer, "- %s%s — %s\n", question.ID, blocking, question.Question)
			if question.Answer != "" {
				fmt.Fprintf(&buffer, "  Answer: %s\n", question.Answer)
			} else {
				fmt.Fprintln(&buffer, "  Answer: pending")
			}
		}
		fmt.Fprintln(&buffer)
	}
	fmt.Fprintln(&buffer, "## In scope")
	fmt.Fprintln(&buffer, "TBD")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Out of scope")
	fmt.Fprintln(&buffer, "TBD")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "## Acceptance criteria")
	fmt.Fprintln(&buffer, "TBD")
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

func renderPromptMD(task string) []byte {
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# Executor Prompt")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "This prompt is not executable yet. Manual clarification is available, but approval and agent execution are not implemented.")
	fmt.Fprintln(&buffer)
	fmt.Fprintln(&buffer, "Task:")
	fmt.Fprintln(&buffer, task)
	return buffer.Bytes()
}

func writeRunCreated(stdout io.Writer, result contractRunState) {
	fmt.Fprintln(stdout, "Run created")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", result.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", result.Status)
	fmt.Fprintf(stdout, "  task: %s\n", result.Task)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Artifacts:")
	fmt.Fprintf(stdout, "  task: %s\n", runArtifactRepoRel(result.RunID, result.Artifacts.Task))
	fmt.Fprintf(stdout, "  context: %s\n", runArtifactRepoRel(result.RunID, result.Artifacts.RepoContext))
	fmt.Fprintf(stdout, "  contract: %s\n", runArtifactRepoRel(result.RunID, result.Artifacts.ContractMD))
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Next:")
	fmt.Fprintln(stdout, "  Review the generated contract draft.")
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
