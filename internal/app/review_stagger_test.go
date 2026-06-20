package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heurema/pactum/internal/agents"
	"github.com/heurema/pactum/internal/artifacts"
)

// staggerTransport is an agents.Transport double for the review-stagger tests.
// It records each attempt's prompt path the moment Run is entered (the launch
// order the scheduler produced) and writes the empty reviewer logs the real
// transports would, so the round parses to a clean pass. A per-attempt behavior
// closure (keyed off the prompt path) lets a test gate the lead attempt: fire
// its first-output callback, block it, or let it finish without output.
type staggerTransport struct {
	mu       sync.Mutex
	order    []string
	stdout   string
	behavior func(req agents.RunRequest)
	failFor  string
}

func (tr *staggerTransport) Run(req agents.RunRequest) (agents.RunResult, error) {
	tr.mu.Lock()
	tr.order = append(tr.order, req.PromptRepoPath)
	tr.mu.Unlock()
	artifactDir := req.ArtifactDir
	if artifactDir == "" {
		artifactDir = "execute/attempts"
	}
	attemptDir := filepath.Join(req.RepoRoot, artifacts.WorkspaceRel, "runs", req.RunID, filepath.FromSlash(artifactDir), req.AttemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return agents.RunResult{}, err
	}
	stdout := tr.stdout
	if stdout == "" {
		stdout = reviewerStructuredOutput([]map[string]any{})
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stdout.log"), []byte(stdout), 0o644); err != nil {
		return agents.RunResult{}, err
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr.log"), nil, 0o644); err != nil {
		return agents.RunResult{}, err
	}
	if tr.behavior != nil {
		tr.behavior(req)
	}
	if tr.failFor != "" && req.PromptRepoPath == tr.failFor {
		return agents.RunResult{}, fmt.Errorf("lens transport failure")
	}
	return agents.RunResult{
		ExitCode:   0,
		StartedAt:  "2026-05-31T18:40:12Z",
		FinishedAt: "2026-05-31T18:40:13Z",
		StdoutPath: artifactDir + "/" + req.AttemptID + "/stdout.log",
		StderrPath: artifactDir + "/" + req.AttemptID + "/stderr.log",
	}, nil
}

func (tr *staggerTransport) launchCount() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return len(tr.order)
}

func (tr *staggerTransport) firstLaunch() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.order) == 0 {
		return ""
	}
	return tr.order[0]
}

// waitForLaunchCount blocks until the transport has entered want attempts, or
// fails after 5 seconds — well under any production hold timeout, so a release
// observed here cannot have come from the 60s hold fallback.
func waitForLaunchCount(t *testing.T, tr *staggerTransport, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for tr.launchCount() < want {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d launches, have %d", want, tr.launchCount())
		}
		time.Sleep(time.Millisecond)
	}
}

func reviewerLeadPromptRepoPath(runID string, member string) string {
	return runArtifactRepoRel(runID, reviewerLensPromptArtifact(member, reviewLenses[0]))
}

// TestReviewRunStaggersClaudeGroupUntilFirstOutput pins the core behavior: a
// multi-attempt Claude group launches exactly one lead, holds the rest, and
// releases them the moment the lead streams its first output.
func TestReviewRunStaggersClaudeGroupUntilFirstOutput(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})

	leadEntered := make(chan struct{})
	fireOutput := make(chan struct{})
	completeLead := make(chan struct{})
	leadPrompt := reviewerLeadPromptRepoPath(runID, "claude")

	tr := &staggerTransport{behavior: func(req agents.RunRequest) {
		if req.PromptRepoPath != leadPrompt {
			return
		}
		close(leadEntered)
		<-fireOutput
		req.OnFirstOutput()
		<-completeLead
	}}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"})
	}()

	<-leadEntered
	// Until the lead streams output, only the lead has launched; the four held
	// lens attempts stay queued.
	if got := tr.launchCount(); got != 1 {
		t.Fatalf("before first output: launched %d attempts, want only the lead", got)
	}
	close(fireOutput)
	// Releasing within the 5s deadline (vs the 60s hold) proves first output —
	// not the timeout fallback — triggered the release.
	waitForLaunchCount(t, tr, len(reviewLenses))
	close(completeLead)
	assertNoError(t, <-done)

	if got := tr.launchCount(); got != len(reviewLenses) {
		t.Fatalf("final launch count = %d, want %d", got, len(reviewLenses))
	}
	if got := tr.firstLaunch(); got != leadPrompt {
		t.Fatalf("lead was not the first launched attempt: first=%q want=%q", got, leadPrompt)
	}
	if live := stderr.String(); !strings.Contains(live, "review stagger: holding 4 claude") || !strings.Contains(live, "review stagger: releasing 4 held claude attempt(s) (lead streamed first output)") {
		t.Fatalf("hold/release live output missing:\n%s", live)
	}
	// Byte-compatible recorded attempts: five lens attempts under sequential IDs.
	for i := 1; i <= len(reviewLenses); i++ {
		assertFile(t, reviewerAttemptPaths(runPaths, fmt.Sprintf("reviewer_attempt_%03d", i)).ResultJSON)
	}
}

// TestReviewRunReleasesStaggeredClaudeGroupConcurrently pins that the held
// attempts are released concurrently, not serialized: once the lead streams
// output, every held attempt is in-flight inside the transport at the same
// moment. Each held attempt blocks on a barrier that only opens when all of
// them have arrived, so a one-at-a-time release loop would never satisfy the
// barrier and the test would fail rather than pass on the final launch count
// alone.
func TestReviewRunReleasesStaggeredClaudeGroupConcurrently(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})

	leadPrompt := reviewerLeadPromptRepoPath(runID, "claude")
	heldCount := len(reviewLenses) - 1
	allHeldInFlight := make(chan struct{})
	proceed := make(chan struct{})
	var heldMu sync.Mutex
	heldInFlight := 0

	tr := &staggerTransport{behavior: func(req agents.RunRequest) {
		if req.PromptRepoPath == leadPrompt {
			req.OnFirstOutput() // release the held attempts the moment the lead "streams"
			return
		}
		heldMu.Lock()
		heldInFlight++
		reached := heldInFlight == heldCount
		heldMu.Unlock()
		if reached {
			close(allHeldInFlight)
		}
		// Hold every held attempt inside the transport until the test has confirmed
		// they are all concurrently in-flight (or has given up); a serialized
		// release would deadlock the first held attempt here instead.
		<-proceed
	}}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"})
	}()

	serialized := false
	select {
	case <-allHeldInFlight:
	case <-time.After(5 * time.Second):
		serialized = true
	}
	close(proceed) // unblock the held attempts whether the barrier opened or timed out
	assertNoError(t, <-done)
	if serialized {
		t.Fatal("held attempts were not all in-flight at once: release was serialized")
	}
	if got := tr.launchCount(); got != len(reviewLenses) {
		t.Fatalf("final launch count = %d, want %d", got, len(reviewLenses))
	}
}

// TestReviewRunReleasesStaggeredClaudeGroupOnLeadCompletion covers the
// early-termination release: a lead that finishes before producing any visible
// output must release the held attempts immediately, not wait for the hold
// timeout.
func TestReviewRunReleasesStaggeredClaudeGroupOnLeadCompletion(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})

	leadEntered := make(chan struct{})
	completeLead := make(chan struct{})
	leadPrompt := reviewerLeadPromptRepoPath(runID, "claude")

	tr := &staggerTransport{behavior: func(req agents.RunRequest) {
		if req.PromptRepoPath != leadPrompt {
			return
		}
		close(leadEntered)
		<-completeLead // finish only when the test allows; never fire output
	}}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"})
	}()

	<-leadEntered
	if got := tr.launchCount(); got != 1 {
		t.Fatalf("before lead completion: launched %d attempts, want only the lead", got)
	}
	close(completeLead) // lead returns without ever streaming output
	waitForLaunchCount(t, tr, len(reviewLenses))
	assertNoError(t, <-done)

	if live := stderr.String(); !strings.Contains(live, "review stagger: releasing 4 held claude attempt(s) (lead finished before output)") {
		t.Fatalf("completion release live output missing:\n%s", live)
	}
}

// TestReviewRunReleasesStaggeredClaudeGroupOnHoldTimeout covers the timeout
// release: a silent lead that neither streams nor finishes must not serialize
// the panel past the hold timeout. The hold is shrunk so the test does not wait
// a real minute.
func TestReviewRunReleasesStaggeredClaudeGroupOnHoldTimeout(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})

	leadEntered := make(chan struct{})
	completeLead := make(chan struct{})
	leadPrompt := reviewerLeadPromptRepoPath(runID, "claude")

	tr := &staggerTransport{behavior: func(req agents.RunRequest) {
		if req.PromptRepoPath != leadPrompt {
			return
		}
		close(leadEntered)
		<-completeLead // stays silent and unfinished across the hold window
	}}
	app.AgentTransport = tr
	app.reviewStaggerHold = 80 * time.Millisecond

	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"})
	}()

	<-leadEntered
	// The lead never fires output and never returns until the test ends, so the
	// only path to launching the held attempts is the hold timeout.
	waitForLaunchCount(t, tr, len(reviewLenses))
	if live := stderr.String(); !strings.Contains(live, "review stagger: releasing 4 held claude attempt(s) (hold timeout elapsed)") {
		t.Fatalf("timeout release live output missing:\n%s", live)
	}
	close(completeLead)
	assertNoError(t, <-done)
}

// TestReviewRunSharesStaggerGroupAcrossRegistryNames pins q_002: two registry
// names resolving to the same Claude model and effort form one group with one
// lead — not one lead each.
func TestReviewRunSharesStaggerGroupAcrossRegistryNames(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths,
		agentRegistryEntry{Name: "claude-a", Model: "claude-sonnet-4", Effort: "high"},
		agentRegistryEntry{Name: "claude-b", Model: "claude-sonnet-4", Effort: "high"},
	)
	setReviewPanelConfig(t, paths, "claude-a", "claude-b")

	leadEntered := make(chan struct{})
	fireOutput := make(chan struct{})
	completeLead := make(chan struct{})
	leadPrompt := reviewerLeadPromptRepoPath(runID, "claude-a")

	tr := &staggerTransport{behavior: func(req agents.RunRequest) {
		if req.PromptRepoPath != leadPrompt {
			return
		}
		close(leadEntered)
		<-fireOutput
		req.OnFirstOutput()
		<-completeLead
	}}
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{})
	}()

	total := 2 * len(reviewLenses)
	<-leadEntered
	// One shared group means one lead: if claude-b staggered independently its
	// own lead would already have launched (count 2).
	if got := tr.launchCount(); got != 1 {
		t.Fatalf("before first output: launched %d attempts, want a single shared lead", got)
	}
	close(fireOutput)
	waitForLaunchCount(t, tr, total)
	close(completeLead)
	assertNoError(t, <-done)

	if got := tr.launchCount(); got != total {
		t.Fatalf("final launch count = %d, want %d", got, total)
	}
	if live := stderr.String(); !strings.Contains(live, "review stagger: holding 9 claude") {
		t.Fatalf("shared group should hold 9 of 10 attempts:\n%s", live)
	}
}

// TestReviewRunLaunchesCodexGroupWithoutStagger pins that a Codex group never
// holds: every lens attempt launches at once and no stagger line is printed.
func TestReviewRunLaunchesCodexGroupWithoutStagger(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, _ := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "codex", Model: "gpt-5", Effort: "high"})

	tr := &staggerTransport{}
	app.AgentTransport = tr
	// A short hold would matter only if a stagger were (wrongly) applied.
	app.reviewStaggerHold = 80 * time.Millisecond

	var stdout, stderr bytes.Buffer
	if err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "codex"}); err != nil {
		t.Fatalf("review run errored: %v\nstderr: %s", err, stderr.String())
	}
	if got := tr.launchCount(); got != len(reviewLenses) {
		t.Fatalf("codex launch count = %d, want %d", got, len(reviewLenses))
	}
	if strings.Contains(stderr.String(), "review stagger:") {
		t.Fatalf("codex group must not emit a stagger hold line:\n%s", stderr.String())
	}
}

// TestGroupReviewerLensTasks pins the grouping and the stagger decision: same
// Claude model+effort across two names → one staggered group with the first
// reviewer's first lens as lead; Codex → its own immediate group; a
// single-attempt group never staggers.
func TestGroupReviewerLensTasks(t *testing.T) {
	claudeA := reviewLoopReviewer{Name: "claude-a", Agent: agents.AgentDescriptor{Name: agents.BuiltinClaude}, ModelSpec: agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}}
	claudeB := reviewLoopReviewer{Name: "claude-b", Agent: agents.AgentDescriptor{Name: agents.BuiltinClaude}, ModelSpec: agents.ModelSpec{Model: "claude-sonnet-4", Effort: "high"}}
	codex := reviewLoopReviewer{Name: "codex", Agent: agents.AgentDescriptor{Name: agents.BuiltinCodex}, ModelSpec: agents.ModelSpec{Model: "gpt-5"}}

	groups := groupReviewerLensTasks([]reviewLoopReviewer{claudeA, claudeB, codex})
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (one per normalized engine/model/effort)", len(groups))
	}

	claudeGroup := groups[0]
	if !claudeGroup.claude || len(claudeGroup.tasks) != 2*len(reviewLenses) {
		t.Fatalf("claude group mismatch: claude=%t tasks=%d", claudeGroup.claude, len(claudeGroup.tasks))
	}
	if !claudeGroup.staggered() {
		t.Fatal("a multi-attempt claude group must stagger")
	}
	lead := claudeGroup.tasks[0]
	if lead.reviewerIndex != 0 || lead.lensIndex != 0 {
		t.Fatalf("lead = reviewer %d lens %d, want the first reviewer's first lens", lead.reviewerIndex, lead.lensIndex)
	}

	codexGroup := groups[1]
	if codexGroup.claude || codexGroup.staggered() {
		t.Fatalf("codex group must launch immediately: claude=%t staggered=%t", codexGroup.claude, codexGroup.staggered())
	}

	single := reviewerLensGroup{claude: true, tasks: []reviewerLensTask{{reviewer: claudeA, lens: reviewLenses[0]}}}
	if single.staggered() {
		t.Fatal("a single-attempt claude group must launch immediately, not stagger")
	}
}

// TestReviewRunSurfacesFailedLensAttemptInStaggeredGroup pins the partial-failure
// path: a single lens attempt failing inside a staggered group is recorded as a
// skipped lens and the round continues with the remaining successful attempts.
func TestReviewRunSurfacesFailedLensAttemptInStaggeredGroup(t *testing.T) {
	root := t.TempDir()
	app, paths, runID, runPaths := setupApprovedPreparedReview(t, root, "passed")
	setAgentRegistryConfig(t, paths, agentRegistryEntry{Name: "claude", Model: "claude-sonnet-4", Effort: "high"})

	// The lead succeeds and streams output; one held lens fails after release.
	failingPrompt := runArtifactRepoRel(runID, reviewerLensPromptArtifact("claude", reviewLenses[2]))
	tr := &staggerTransport{behavior: func(req agents.RunRequest) {
		if req.OnFirstOutput != nil {
			req.OnFirstOutput()
		}
	}}
	tr.failFor = failingPrompt
	app.AgentTransport = tr

	var stdout, stderr bytes.Buffer
	if err := app.ReviewRun(&stdout, &stderr, runID, reviewRunOptions{Reviewer: "claude"}); err != nil {
		t.Fatalf("partial lens failure must not abort the round, got: %v", err)
	}
	summary := readReviewLoopSummary(t, runPaths.ReviewLoopSummaryJSON)
	if len(summary.Rounds) != 1 {
		t.Fatalf("partial failure should still complete one round: %#v", summary)
	}
	round := summary.Rounds[0]
	if len(round.SkippedLenses) != 1 {
		t.Fatalf("skipped lenses = %d, want 1: %#v", len(round.SkippedLenses), round)
	}
	s := round.SkippedLenses[0]
	if s.Reviewer != "claude" || s.Lens != reviewLenses[2].Key || !strings.Contains(s.Reason, "lens transport failure") {
		t.Fatalf("skipped lens mismatch: %#v", s)
	}
	// The 4 succeeding lens attempts should still be recorded in the round.
	if got := len(round.ReviewerAttemptIDs); got != len(reviewLenses)-1 {
		t.Fatalf("attempt IDs = %d, want %d: %#v", got, len(reviewLenses)-1, round)
	}
}
