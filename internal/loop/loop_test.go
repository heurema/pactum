package loop_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/heurema/pactum/internal/loop"
)

// TestDepsIsolation asserts that internal/loop imports only standard-library
// packages (no sibling internal packages, no external modules).
func TestDepsIsolation(t *testing.T) {
	const modulePath = "github.com/heurema/pactum"
	const pkg = modulePath + "/internal/loop"

	depsOut, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}

	stdOut, err := exec.Command("go", "list", "std").Output()
	if err != nil {
		t.Fatalf("go list std: %v", err)
	}
	stdlib := make(map[string]bool)
	for _, p := range strings.Split(strings.TrimSpace(string(stdOut)), "\n") {
		stdlib[p] = true
	}

	internalPrefix := modulePath + "/internal/"

	for _, dep := range strings.Split(strings.TrimSpace(string(depsOut)), "\n") {
		if dep == "" || dep == pkg {
			continue
		}
		if strings.HasPrefix(dep, internalPrefix) {
			t.Errorf("internal/loop imports sibling internal package: %s", dep)
		}
		if !stdlib[dep] {
			t.Errorf("internal/loop imports non-stdlib dependency: %s", dep)
		}
	}
}

// roundSpec describes the outcome a step should produce on one call.
type roundSpec struct {
	clean    bool
	progress bool
	human    *loop.HumanExit
	err      error
}

// makeStep returns a Step that iterates through specs in round order, plus a
// pointer to the call count so callers can verify how many rounds ran.
func makeStep(specs []roundSpec) (loop.Step, *int) {
	count := 0
	return func(_ context.Context, rc loop.RoundContext) (loop.RoundResult, error) {
		count++
		idx := rc.Round - 1
		if idx < 0 || idx >= len(specs) {
			panic("step called with unexpected round")
		}
		s := specs[idx]
		if s.err != nil {
			return loop.RoundResult{}, s.err
		}
		return loop.RoundResult{
			Clean:    s.clean,
			Progress: s.progress,
			Human:    s.human,
		}, nil
	}, &count
}

func TestRunSettled(t *testing.T) {
	// Settle=2: two consecutive clean rounds trigger "settled".
	step, count := makeStep([]roundSpec{
		{clean: true},
		{clean: true},
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 10, Settle: 2}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "settled" {
		t.Fatalf("expected settled, got %q", out.Reason)
	}
	if out.Rounds != 2 {
		t.Fatalf("expected 2 rounds, got %d", out.Rounds)
	}
	if *count != 2 {
		t.Fatalf("expected step called 2 times, got %d", *count)
	}
}

func TestRunSettledResetOnNonClean(t *testing.T) {
	// Clean streak resets on a non-clean round; requires 2 more clean rounds after.
	step, count := makeStep([]roundSpec{
		{clean: true},           // cleanStreak=1
		{clean: false},          // cleanStreak=0
		{clean: true},           // cleanStreak=1
		{clean: true},           // cleanStreak=2 → settled
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 10, Settle: 2}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "settled" {
		t.Fatalf("expected settled, got %q", out.Reason)
	}
	if out.Rounds != 4 {
		t.Fatalf("expected 4 rounds, got %d", out.Rounds)
	}
	if *count != 4 {
		t.Fatalf("expected step called 4 times, got %d", *count)
	}
}

func TestRunStalemate(t *testing.T) {
	// Patience=2: two consecutive dirty/no-progress rounds trigger "stalemate".
	step, count := makeStep([]roundSpec{
		{clean: false, progress: false}, // staleStreak=1
		{clean: false, progress: false}, // staleStreak=2 → stalemate
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 10, Patience: 2}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "stalemate" {
		t.Fatalf("expected stalemate, got %q", out.Reason)
	}
	if out.Rounds != 2 {
		t.Fatalf("expected 2 rounds, got %d", out.Rounds)
	}
	if *count != 2 {
		t.Fatalf("expected step called 2 times, got %d", *count)
	}
}

func TestRunStalemateResetOnProgress(t *testing.T) {
	// Progress resets stale streak; need 2 more stale rounds after progress.
	step, count := makeStep([]roundSpec{
		{clean: false, progress: false}, // staleStreak=1
		{clean: false, progress: true},  // staleStreak=0 (progress)
		{clean: false, progress: false}, // staleStreak=1
		{clean: false, progress: false}, // staleStreak=2 → stalemate
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 10, Patience: 2}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "stalemate" {
		t.Fatalf("expected stalemate, got %q", out.Reason)
	}
	if out.Rounds != 4 {
		t.Fatalf("expected 4 rounds, got %d", out.Rounds)
	}
	_ = count
}

func TestRunStalemateUnchangedOnClean(t *testing.T) {
	// Clean rounds leave staleStreak unchanged (neither increment nor reset).
	// staleStreak starts at 1 (from round 1), round 2 is clean (staleStreak stays 1),
	// round 3 is dirty/no-progress (staleStreak becomes 2 → stalemate with Patience=2).
	step, count := makeStep([]roundSpec{
		{clean: false, progress: false}, // staleStreak=1
		{clean: true},                   // staleStreak unchanged (=1)
		{clean: false, progress: false}, // staleStreak=2 → stalemate
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 10, Patience: 2}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "stalemate" {
		t.Fatalf("expected stalemate (stale streak accumulates through clean rounds), got %q", out.Reason)
	}
	if out.Rounds != 3 {
		t.Fatalf("expected 3 rounds, got %d", out.Rounds)
	}
	if *count != 3 {
		t.Fatalf("expected step called 3 times, got %d", *count)
	}
}

func TestRunMax(t *testing.T) {
	// No stop condition fires; Max terminates the loop.
	step, count := makeStep([]roundSpec{
		{clean: false, progress: false},
		{clean: false, progress: false},
		{clean: false, progress: false},
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 3, Patience: 100}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "max" {
		t.Fatalf("expected max, got %q", out.Reason)
	}
	if out.Rounds != 3 {
		t.Fatalf("expected 3 rounds, got %d", out.Rounds)
	}
	if *count != 3 {
		t.Fatalf("expected step called 3 times, got %d", *count)
	}
}

func TestRunHuman(t *testing.T) {
	// Human exit fires immediately regardless of position.
	human := &loop.HumanExit{Reason: "needs-review"}
	step, count := makeStep([]roundSpec{
		{clean: false, progress: false},
		{clean: false, progress: false, human: human},
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 10, Patience: 100}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "human" {
		t.Fatalf("expected human, got %q", out.Reason)
	}
	if out.Rounds != 2 {
		t.Fatalf("expected 2 rounds, got %d", out.Rounds)
	}
	if out.Last.Human != human {
		t.Fatalf("expected Human to be the HumanExit we passed")
	}
	if *count != 2 {
		t.Fatalf("expected step called 2 times, got %d", *count)
	}
}

func TestRunHumanBeforeSettled(t *testing.T) {
	// Human takes precedence over settled even on the same round.
	human := &loop.HumanExit{Reason: "override"}
	step, _ := makeStep([]roundSpec{
		{clean: true, human: human},
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 10, Settle: 1}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "human" {
		t.Fatalf("human must win over settled on the same round, got %q", out.Reason)
	}
}

func TestRunErrorPropagation(t *testing.T) {
	// A step error is returned immediately; no further step calls.
	injected := errors.New("step failed")
	calls := 0
	step := func(_ context.Context, rc loop.RoundContext) (loop.RoundResult, error) {
		calls++
		if rc.Round == 2 {
			return loop.RoundResult{}, injected
		}
		return loop.RoundResult{Clean: false}, nil
	}
	_, err := loop.Run(context.Background(), loop.Limits{Max: 10}, step)
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected step called exactly 2 times (round 1 ok, round 2 error), got %d", calls)
	}
}

func TestRunContextCancellation(t *testing.T) {
	// If ctx is already cancelled before a round, Run returns the context error
	// without calling step.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	calls := 0
	step := func(_ context.Context, _ loop.RoundContext) (loop.RoundResult, error) {
		calls++
		return loop.RoundResult{}, nil
	}

	_, err := loop.Run(ctx, loop.Limits{Max: 5}, step)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("step must not be called when context is already done; called %d times", calls)
	}
}

func TestRunContextCancelledBetweenRounds(t *testing.T) {
	// Context is cancelled after round 1; round 2 is skipped.
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	step := func(_ context.Context, _ loop.RoundContext) (loop.RoundResult, error) {
		calls++
		cancel() // cancel after first call
		return loop.RoundResult{Clean: false}, nil
	}
	_, err := loop.Run(ctx, loop.Limits{Max: 5}, step)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected step called 1 time before cancel; called %d", calls)
	}
}

func TestRunSettledVsMax(t *testing.T) {
	// When Settle fires on the last round, "settled" wins over "max".
	step, _ := makeStep([]roundSpec{
		{clean: true},
		{clean: true},
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 2, Settle: 2}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "settled" {
		t.Fatalf("settled must win over max on same round, got %q", out.Reason)
	}
}

func TestRunStalemateVsMax(t *testing.T) {
	// When Patience fires on the last round, "stalemate" wins over "max".
	step, _ := makeStep([]roundSpec{
		{clean: false, progress: false},
		{clean: false, progress: false},
	})
	out, err := loop.Run(context.Background(), loop.Limits{Max: 2, Patience: 2}, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Reason != "stalemate" {
		t.Fatalf("stalemate must win over max on same round, got %q", out.Reason)
	}
}

func TestRunRoundNumbers(t *testing.T) {
	// Verify that Step is called with 1-based consecutive round numbers.
	var rounds []int
	step := func(_ context.Context, rc loop.RoundContext) (loop.RoundResult, error) {
		rounds = append(rounds, rc.Round)
		return loop.RoundResult{Clean: false, Progress: false}, nil
	}
	loop.Run(context.Background(), loop.Limits{Max: 4}, step) //nolint:errcheck
	if len(rounds) != 4 {
		t.Fatalf("expected 4 calls, got %d", len(rounds))
	}
	for i, r := range rounds {
		if r != i+1 {
			t.Fatalf("round[%d]=%d, expected %d", i, r, i+1)
		}
	}
}
