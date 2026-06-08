package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/heurema/pactum/internal/artifacts"
	"github.com/heurema/pactum/internal/ledger"
)

const (
	clarificationQuestionSchema = "pactum.clarification_question.v1"
	clarificationAnswerSchema   = "pactum.clarification_answer.v1"
	clarificationDecisionSchema = "pactum.clarification_decision.v1"
)

type clarificationQuestionRecord struct {
	Schema             string    `json:"schema"`
	ID                 string    `json:"id"`
	RunID              string    `json:"run_id"`
	Question           string    `json:"question"`
	Blocking           bool      `json:"blocking"`
	Rationale          string    `json:"rationale,omitempty"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	Source             string    `json:"source"`
	ClarifierAttemptID string    `json:"clarifier_attempt_id,omitempty"`
}

type clarificationAnswerRecord struct {
	Schema     string    `json:"schema"`
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	QuestionID string    `json:"question_id"`
	Answer     string    `json:"answer"`
	CreatedAt  time.Time `json:"created_at"`
	Source     string    `json:"source"`
}

type clarificationDecisionRecord struct {
	Schema     string    `json:"schema"`
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	QuestionID string    `json:"question_id"`
	Decision   string    `json:"decision"`
	CreatedAt  time.Time `json:"created_at"`
	Source     string    `json:"source"`
}

type contractClarifySet struct {
	Questions []clarifyQuestionStatus `json:"questions"`
}

type clarifyQuestionStatus struct {
	ID        string `json:"id"`
	Question  string `json:"question"`
	Blocking  bool   `json:"blocking"`
	Rationale string `json:"rationale,omitempty"`
	Status    string `json:"status"`
	Answer    string `json:"answer,omitempty"`
}

type clarifyStatusResponse struct {
	RunID        string                  `json:"run_id"`
	RunStatus    string                  `json:"run_status"`
	Total        int                     `json:"total"`
	Answered     int                     `json:"answered"`
	Open         int                     `json:"open"`
	BlockingOpen int                     `json:"blocking_open"`
	Questions    []clarifyQuestionStatus `json:"questions"`
}

type clarifyAskResponse struct {
	RunID     string                      `json:"run_id"`
	RunStatus string                      `json:"run_status"`
	Question  clarificationQuestionRecord `json:"question"`
}

type clarifyAnswerResponse struct {
	RunID     string                      `json:"run_id"`
	RunStatus string                      `json:"run_status"`
	Answer    clarificationAnswerRecord   `json:"answer"`
	Decision  clarificationDecisionRecord `json:"decision"`
}

func (a App) ClarifyAsk(stdout io.Writer, runID string, question string, blocking bool, jsonOutput bool) error {
	context, ok, err := a.loadClarifyContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}

	now := a.nowUTC()
	questions, err := readClarificationQuestions(context.RunPaths.QuestionsJSONL)
	if err != nil {
		return err
	}
	record := clarificationQuestionRecord{
		Schema:    clarificationQuestionSchema,
		ID:        nextClarificationID("q", len(questions)+1),
		RunID:     runID,
		Question:  question,
		Blocking:  blocking,
		Status:    "open",
		CreatedAt: now,
		Source:    "manual",
	}
	if err := appendJSONLine(context.RunPaths.QuestionsJSONL, record); err != nil {
		return err
	}
	status, err := a.refreshClarificationArtifacts(context, now)
	if err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_question_added", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := clarifyAskResponse{RunID: runID, RunStatus: status.RunStatus, Question: record}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeClarifyAskResponse(stdout, response)
	return nil
}

func (a App) ClarifyAnswer(stdout io.Writer, runID string, questionID string, answer string, jsonOutput bool) error {
	context, ok, err := a.loadClarifyContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	questions, err := readClarificationQuestions(context.RunPaths.QuestionsJSONL)
	if err != nil {
		return err
	}
	if !hasClarificationQuestion(questions, questionID) {
		return fmt.Errorf("question not found: %s", questionID)
	}
	answers, err := readClarificationAnswers(context.RunPaths.AnswersJSONL)
	if err != nil {
		return err
	}
	decisions, err := readClarificationDecisions(context.RunPaths.DecisionsJSONL)
	if err != nil {
		return err
	}

	now := a.nowUTC()
	answerRecord := clarificationAnswerRecord{
		Schema:     clarificationAnswerSchema,
		ID:         nextClarificationID("a", len(answers)+1),
		RunID:      runID,
		QuestionID: questionID,
		Answer:     answer,
		CreatedAt:  now,
		Source:     "manual",
	}
	decisionRecord := clarificationDecisionRecord{
		Schema:     clarificationDecisionSchema,
		ID:         nextClarificationID("d", len(decisions)+1),
		RunID:      runID,
		QuestionID: questionID,
		Decision:   answer,
		CreatedAt:  now,
		Source:     "manual_answer",
	}
	// TODO: Later revisions may reject duplicate answers or model answer revisions explicitly.
	if err := appendJSONLine(context.RunPaths.AnswersJSONL, answerRecord); err != nil {
		return err
	}
	if err := appendJSONLine(context.RunPaths.DecisionsJSONL, decisionRecord); err != nil {
		return err
	}
	status, err := a.refreshClarificationArtifacts(context, now)
	if err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_answer_recorded", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}
	if err := ledger.Append(activeStore, context.Paths.EventsJSONL, ledger.Event{Type: "clarification_decision_recorded", Timestamp: now, RunID: runID, RepoRoot: context.Root}); err != nil {
		return err
	}

	response := clarifyAnswerResponse{RunID: runID, RunStatus: status.RunStatus, Answer: answerRecord, Decision: decisionRecord}
	if jsonOutput {
		return writeJSONResponse(stdout, response)
	}
	writeClarifyAnswerResponse(stdout, response)
	return nil
}

func (a App) ClarifyStatus(stdout io.Writer, runID string, jsonOutput bool) error {
	context, ok, err := a.loadClarifyContext(stdout, runID, jsonOutput)
	if err != nil || !ok {
		return err
	}
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return err
	}
	if context.State.Status == "contract_approved" && status.BlockingOpen == 0 {
		status.RunStatus = "contract_approved"
	}
	if jsonOutput {
		return writeJSONResponse(stdout, status)
	}
	writeClarifyStatus(stdout, status)
	return nil
}

type clarifyContext struct {
	Root     string
	Paths    artifacts.Paths
	RunPaths contractRunPathSet
	State    contractRunState
}

func (a App) loadClarifyContext(stdout io.Writer, runID string, jsonOutput bool) (clarifyContext, bool, error) {
	root, paths, ok, err := a.requireWorkspace(stdout, jsonOutput)
	if err != nil || !ok {
		return clarifyContext{}, false, err
	}

	runDir := filepath.Join(paths.RunsDir, runID)
	runDirExists, err := storeDirExists(runDir)
	if err != nil {
		return clarifyContext{}, false, err
	}
	if !runDirExists {
		return clarifyContext{}, false, fmt.Errorf("run not found: %s", runID)
	}
	runPaths := contractRunPaths(runDir)
	state, err := readContractRunState(runPaths.RunJSON)
	if err != nil {
		return clarifyContext{}, false, err
	}
	return clarifyContext{Root: root, Paths: paths, RunPaths: runPaths, State: state}, true, nil
}

func (a App) refreshClarificationArtifacts(context clarifyContext, updatedAt time.Time) (clarifyStatusResponse, error) {
	status, err := buildClarificationStatus(context.RunPaths, context.State)
	if err != nil {
		return clarifyStatusResponse{}, err
	}

	state := context.State
	state.Status = status.RunStatus
	state.UpdatedAt = updatedAt
	if _, _, err := resetApprovalIfApproved(context.Paths, context.RunPaths, context.Root, context.State.RunID, updatedAt); err != nil {
		return clarifyStatusResponse{}, err
	}
	if err := writeJSON(context.RunPaths.RunJSON, state); err != nil {
		return clarifyStatusResponse{}, err
	}

	contract, err := readDraftContract(context.RunPaths.ContractJSON)
	if err != nil {
		return clarifyStatusResponse{}, err
	}
	applyClarificationStatusToContract(&contract, status)
	contract.Status = "draft"
	if err := writeContractArtifacts(context.RunPaths, contract, state.MapRunID); err != nil {
		return clarifyStatusResponse{}, err
	}
	status.RunStatus = state.Status
	return status, nil
}

func buildClarificationStatus(runPaths contractRunPathSet, state contractRunState) (clarifyStatusResponse, error) {
	questions, err := readClarificationQuestions(runPaths.QuestionsJSONL)
	if err != nil {
		return clarifyStatusResponse{}, err
	}
	answers, err := readClarificationAnswers(runPaths.AnswersJSONL)
	if err != nil {
		return clarifyStatusResponse{}, err
	}
	latestAnswers := latestAnswersByQuestion(answers)

	response := clarifyStatusResponse{
		RunID:     state.RunID,
		RunStatus: state.Status,
		Questions: []clarifyQuestionStatus{},
	}
	for _, question := range questions {
		answer, answered := latestAnswers[question.ID]
		questionStatus := "open"
		answerText := ""
		if answered {
			questionStatus = "answered"
			answerText = answer.Answer
			response.Answered++
		} else {
			response.Open++
			if question.Blocking {
				response.BlockingOpen++
			}
		}
		response.Questions = append(response.Questions, clarifyQuestionStatus{
			ID:        question.ID,
			Question:  question.Question,
			Blocking:  question.Blocking,
			Rationale: question.Rationale,
			Status:    questionStatus,
			Answer:    answerText,
		})
	}
	response.Total = len(response.Questions)
	if response.BlockingOpen > 0 {
		response.RunStatus = "clarifying"
	} else {
		response.RunStatus = "contract_draft"
	}
	return response, nil
}

func readContractRunState(path string) (contractRunState, error) {
	var state contractRunState
	return state, readJSON(path, &state)
}

func readDraftContract(path string) (draftContract, error) {
	var contract draftContract
	return contract, readJSON(path, &contract)
}

func readClarificationQuestions(path string) ([]clarificationQuestionRecord, error) {
	return readJSONLines[clarificationQuestionRecord](path)
}

func readClarificationAnswers(path string) ([]clarificationAnswerRecord, error) {
	return readJSONLines[clarificationAnswerRecord](path)
}

func readClarificationDecisions(path string) ([]clarificationDecisionRecord, error) {
	return readJSONLines[clarificationDecisionRecord](path)
}

func readJSONLines[T any](path string) ([]T, error) {
	file, err := activeStore.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []T{}, nil
		}
		return nil, err
	}
	defer file.Close()

	records := []T{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func appendJSONLine(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return activeStore.AppendBytes(path, data)
}

func writeJSONLines[T any](path string, values []T) error {
	var buffer strings.Builder
	encoder := json.NewEncoder(&buffer)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return err
		}
	}
	return activeStore.WriteBytes(path, []byte(buffer.String()), 0o644)
}

func nextClarificationID(prefix string, index int) string {
	return fmt.Sprintf("%s_%03d", prefix, index)
}

func hasClarificationQuestion(questions []clarificationQuestionRecord, questionID string) bool {
	for _, question := range questions {
		if question.ID == questionID {
			return true
		}
	}
	return false
}

func latestAnswersByQuestion(answers []clarificationAnswerRecord) map[string]clarificationAnswerRecord {
	latest := make(map[string]clarificationAnswerRecord)
	for _, answer := range answers {
		latest[answer.QuestionID] = answer
	}
	return latest
}

func openClarificationQuestionTexts(questions []clarifyQuestionStatus) []string {
	open := []string{}
	for _, question := range questions {
		if question.Status == "open" {
			open = append(open, question.Question)
		}
	}
	return open
}

func readRunSearchResultCount(path string) int {
	var results runSearchResults
	if err := readJSON(path, &results); err != nil {
		return 0
	}
	return len(results.Results)
}

func writeClarifyAskResponse(stdout io.Writer, response clarifyAskResponse) {
	fmt.Fprintln(stdout, "Clarification question added")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Question:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Question.ID)
	fmt.Fprintf(stdout, "  blocking: %t\n", response.Question.Blocking)
	fmt.Fprintf(stdout, "  status: %s\n", response.Question.Status)
	fmt.Fprintf(stdout, "  text: %s\n", response.Question.Question)
}

func writeClarifyAnswerResponse(stdout io.Writer, response clarifyAnswerResponse) {
	fmt.Fprintln(stdout, "Clarification answer recorded")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", response.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", response.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Answer:")
	fmt.Fprintf(stdout, "  id: %s\n", response.Answer.ID)
	fmt.Fprintf(stdout, "  question: %s\n", response.Answer.QuestionID)
}

func writeClarifyStatus(stdout io.Writer, status clarifyStatusResponse) {
	fmt.Fprintln(stdout, "Clarification status")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Run:")
	fmt.Fprintf(stdout, "  id: %s\n", status.RunID)
	fmt.Fprintf(stdout, "  status: %s\n", status.RunStatus)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Questions:")
	fmt.Fprintf(stdout, "  total: %d\n", status.Total)
	fmt.Fprintf(stdout, "  answered: %d\n", status.Answered)
	fmt.Fprintf(stdout, "  open: %d\n", status.Open)
	fmt.Fprintf(stdout, "  blocking open: %d\n", status.BlockingOpen)
	if status.Open > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Open questions:")
		for _, question := range status.Questions {
			if question.Status != "open" {
				continue
			}
			blocking := ""
			if question.Blocking {
				blocking = " [blocking]"
			}
			fmt.Fprintf(stdout, "  - %s%s %s\n", question.ID, blocking, question.Question)
		}
	}
}
