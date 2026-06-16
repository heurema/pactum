package loop

import "context"

// Limits controls when Run stops.
type Limits struct {
	// Max is the maximum number of rounds (callers guarantee Max > 0).
	Max int
	// Patience is the stale-streak threshold; <= 0 disables stalemate detection.
	Patience int
	// Settle is the clean-streak threshold; <= 0 disables settled detection.
	Settle int
}

// RoundContext is passed to Step on each invocation.
type RoundContext struct {
	Round int // 1-based
}

// HumanExit signals that a human must take over.
type HumanExit struct {
	Reason string
}

// RoundResult is the outcome of one Step invocation.
type RoundResult struct {
	// Clean means no blocking issues were found this round.
	Clean bool
	// Progress means work moved forward this round (e.g. a fix was applied).
	// Progress is only meaningful when Clean is false.
	Progress bool
	// Human, when non-nil, requests immediate human handoff.
	Human *HumanExit
	// Summary is an optional human-readable description of the round.
	Summary string
}

// Outcome is the result of a completed Run.
type Outcome struct {
	// Reason is one of "settled", "stalemate", "human", or "max".
	Reason string
	// Rounds is the number of rounds executed.
	Rounds int
	// Last is the final RoundResult returned by Step.
	Last RoundResult
}

// Step is the per-round callback. It is called with 1-based consecutive Round
// values. If Step returns an error, Run stops immediately and propagates it.
type Step func(ctx context.Context, rc RoundContext) (RoundResult, error)

// Run executes step repeatedly until a stop condition is met or ctx is done.
//
// Stop condition precedence (evaluated each round after streaks are updated):
//
//  1. Human != nil  → "human"  (checked before streak updates)
//  2. Settle > 0 && cleanStreak >= Settle → "settled"
//  3. Patience > 0 && staleStreak >= Patience → "stalemate"
//  4. round == Max → "max"
//
// Streak rules:
//   - cleanStreak increments on a Clean round; resets to 0 on a non-Clean round.
//   - staleStreak increments when Clean is false AND Progress is false;
//     resets when Progress is true; is left unchanged when Clean is true.
//
// Run checks ctx.Err() before each round; if the context is already done, it
// returns the context error without calling step.
func Run(ctx context.Context, limits Limits, step Step) (Outcome, error) {
	cleanStreak := 0
	staleStreak := 0

	for round := 1; round <= limits.Max; round++ {
		if err := ctx.Err(); err != nil {
			return Outcome{}, err
		}

		result, err := step(ctx, RoundContext{Round: round})
		if err != nil {
			return Outcome{}, err
		}

		// Human check before streak updates.
		if result.Human != nil {
			return Outcome{Reason: "human", Rounds: round, Last: result}, nil
		}

		// Update streaks.
		if result.Clean {
			cleanStreak++
			// staleStreak unchanged when Clean is true.
		} else {
			cleanStreak = 0
			if result.Progress {
				staleStreak = 0
			} else {
				staleStreak++
			}
		}

		// Evaluate stop conditions in precedence order.
		if limits.Settle > 0 && cleanStreak >= limits.Settle {
			return Outcome{Reason: "settled", Rounds: round, Last: result}, nil
		}
		if limits.Patience > 0 && staleStreak >= limits.Patience {
			return Outcome{Reason: "stalemate", Rounds: round, Last: result}, nil
		}
		if round == limits.Max {
			return Outcome{Reason: "max", Rounds: round, Last: result}, nil
		}
	}

	// Unreachable when Max > 0 (callers guarantee this per package contract).
	return Outcome{Reason: "max"}, nil
}
