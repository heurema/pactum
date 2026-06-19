# Memory Candidate

## Run
- Run id: run_20260619_155159
- Source: deterministic

## Contract
- Goal: Add an absolute per-attempt WALL-CLOCK CAP to the ACP agent transport so an agent attempt can never hang indefinitely, even when it trickles output. Today the only bound is an IDLE watchdog (internal/agents/acp_transport.go: startIdleTimeout) that resets on every streamed token and every inbound protocol callback, so an agent that trickles 1-3 KB then stalls keeps resetting the idle timer forever. Observed live (2026-06-19, slice-4b code-review): all five codex-xhigh reviewer attempts stuck mid-generation on a large diff for ~5 hours, the 25-minute idle timeout never fired, the loop sat on round 1, and ~17 codex-acp child processes leaked because the kill path (which reaps the whole process group) was never triggered.

The fix is a hard total-duration ceiling that fires regardless of output activity. The process-group kill machinery already exists (setProcessGroup + killProcessGroup via Setpgid); it just needs to be triggered by a wall-clock timer, not only the idle watchdog.

In scope:
1. Add a wall-clock cap timer in the ACP transport (acp_transport.go) that starts at attempt start and fires after a fixed total duration measured from `started`, INDEPENDENT of the idle activity channel — it must NOT reset on streamed output or inbound callbacks. When it fires, it cancels the run context (same cancel the idle path uses) so the existing killProcessGroup path reaps the whole adapter+agent process tree.
2. Make the wall-clock cap timeout a DISTINCT, loud outcome, separate from the idle timeout: a wall-clock-capped attempt must be distinguishable in the RunResult / attempt record from (a) a normal completion, (b) an idle timeout, and (c) a transport error. Record a distinct reason/flag (e.g. wall_clock_timeout) so run-records and callers can tell "hung past the hard ceiling" from "went idle". Keep the existing idle `timed_out` semantics unchanged.
3. Resolve the cap value the same shape as the idle timeout is resolved today (a built-in default, overridable): pick a generous built-in default that comfortably exceeds a legitimate long attempt but bounds a true hang (the idle default is 25m; the wall-clock cap should be a larger total ceiling). Do not change the idle-timeout default or its resolution. Plumb the cap through the attempt request the same way Timeout is plumbed.
4. Apply at the transport layer so the cap protects ALL attempt types that go through ACP — reviewer, executor, fixer, drafter — not only reviewers.
5. Surface a wall-clock-capped reviewer lens loudly through the EXISTING skipped-lens mechanism in the review round (a wall-clock-capped lens attempt is recorded as skipped with a reason that names the wall-clock cap, and the round continues with the lenses/reviewers that succeeded); do not let a single hung lens silently disappear.

Out of scope (do NOT do these here):
- The automatic fallback/retry to a reliable emitter (claude/opus) when a reviewer attempt is capped — that is a separate follow-on slice. This slice only bounds and loudly reports the hang.
- Any change to idle-timeout semantics, the idle default, or the activity/reset behavior.
- Reviewer panel composition, lens selection, or reviewer-findings capture/parse (the shipped #196 behavior).
- Graceful reviewer degradation for death/rate-limit (a separate backlog item).
- Reintroducing any plan-DAG concept (removed).

Tests (helper-process fixtures; do NOT invoke real agents):
- A helper attempt that trickles output past the idle window but exceeds the wall-clock cap is killed at the cap and recorded with the distinct wall_clock_timeout reason — proving the idle watchdog alone would not have fired.
- An attempt that completes within the wall-clock cap is unaffected and records a normal completion.
- The existing idle-timeout behavior is unchanged (an attempt that goes idle still records the idle timeout, not wall_clock_timeout) — lock it with a test.
- The process tree is reaped on a wall-clock cap (no leaked child).
- A reviewer round with one lens that hits the wall-clock cap records that lens as skipped with the cap reason and still converges on the remaining successful lenses.

Validation: go test ./internal/agents ./internal/app, go test ./..., go build ./..., make check.
- In scope:
  - Add a separate ACP wall-clock cap duration alongside the existing idle Timeout plumbing, with a built-in default larger than the 25m idle default and an explicit override path that does not reinterpret the existing idle --timeout.
  - Start the ACP wall-clock cap once per attempt from the recorded start time; it must not reset on streamed output, protocol callbacks, tool activity, or any idle-watchdog activity tick.
  - When the wall-clock cap fires, cancel the ACP run context so the existing process-group kill/reap path handles the adapter and child agent process tree.
  - Propagate a distinct wall-clock timeout outcome through agents.RunResult, app processResult, per-stage attempt result JSON, and last-result JSON so callers can distinguish it from normal completion, idle timeout, and transport errors.
  - Apply the cap through the shared ACP transport/lifecycle path so executor, reviewer, review fixer, clarifier, contract drafter, and contract reviewer attempts are protected.
  - Surface a wall-clock-capped review lens through the existing skipped_lenses mechanism with a reason that names the wall-clock cap, while continuing the round when at least one lens succeeds.
  - Add deterministic helper/fake-process tests for the transport and review-loop behavior; do not invoke live external agents.
- Out of scope:
  - Do not add automatic fallback or retry to claude/opus or any other reliable emitter after a wall-clock cap.
  - Do not change idle-timeout duration defaults, idle activity/reset behavior, or completed_despite_timeout semantics.
  - Do not change reviewer panel composition, lens selection, reviewer output parsing, or reviewer findings capture behavior.
  - Do not add graceful degradation behavior for death, rate-limit, network, or provider errors beyond the wall-clock capped lens handling requested here.
  - Do not reintroduce any plan-DAG behavior.
  - Do not add tests that run real ACP agents or require external agent authentication.
- Acceptance criteria:
  - A helper ACP attempt that emits periodic output or protocol activity past the idle window is killed at the wall-clock cap and records a distinct wall_clock_timeout-style outcome without relying on stderr text parsing.
  - A helper ACP attempt that completes before the wall-clock cap records normal completion and does not set the wall-clock timeout outcome.
  - An attempt that goes idle before the wall-clock cap still records the existing idle timed_out outcome and does not record the wall-clock timeout outcome.
  - Wall-clock cap expiry uses the existing process-group kill/reap machinery so a helper adapter that spawns a child process leaves no live child process after the attempt finishes.
  - All ACP-backed attempt request paths carry the resolved wall-clock cap to agents.RunRequest without removing or changing the existing idle Timeout field.
  - Reviewer loop output and summary JSON record a wall-clock-capped lens in skipped_lenses with a reason naming the wall-clock cap, and the round still processes remaining successful lenses.
  - If every reviewer lens in a round fails or is capped, the existing all-failed hard-error behavior remains loud rather than silently converging.
  - Human and JSON result surfaces distinguish wall-clock cap from generic ACP context cancellation or transport error.
  - When no wall-clock cap override is configured, all ACP attempt requests carry a wall-clock cap equal to the built-in default duration; a unit or helper test directly checks that the resolved RunRequest wall-clock cap field equals the built-in default when no override is present.
  - When an explicit wall-clock cap override is configured, ACP attempt requests carry the overridden duration rather than the built-in default; a unit or helper test directly checks that the resolved RunRequest wall-clock cap field matches the configured override value.
  - The wall-clock cap override path enforces a positive non-zero floor: supplying a zero or negative duration is rejected at configuration load or clamped to the built-in default, so an operator misconfiguration cannot silently disable the absolute ceiling and reintroduce indefinite hangs; this behavior is verified by a unit test.
- Validation commands:
  - go test ./internal/agents ./internal/app
  - go test ./...
  - go build ./...
  - make check

## Outcome
- Gate status: needs_review
- Review status: approved
- Execution exit code: 0
- Validation passed: true
- Changes need review: true

## Changes
- Changed files:
  - docs/agents.md
  - docs/backlog.md
  - internal/agents/acp_transport.go
  - internal/agents/runner.go
  - internal/agents/types.go
  - internal/app/agent_attempt.go
  - internal/app/agent_attempt_timeout_test.go
  - internal/app/agent_attempt_transport_test.go
  - internal/app/clarify_loop.go
  - internal/app/clarify_round.go
  - internal/app/config.go
  - internal/app/config_test.go
  - internal/app/contract_draft.go
  - internal/app/contract_review.go
  - internal/app/execute.go
  - internal/app/process.go
  - internal/app/review_fix.go
  - internal/app/review_loop.go
- New files:
  - internal/agents/acp_transport_wallclock_test.go
  - internal/agents/acp_transport_wallclock_unix_test.go
- Missing files: none

## Clarifications
- None

## Review Decisions
- f_001 [medium] resolved internal/agents/acp_transport.go:82: The wall-clock timer is armed before the ACP adapter has actually started, so a valid positive cap can expire before the recorded attempt start and be reported as a generic transport start failure instead of wall_clock_timeout.
  Resolution: Moved `startWallClockTimeout` to after `cmd.Start()` in `acp_transport.go`. The wall-clock timer is now armed only after the adapter process is running, so the ceiling cannot fire before the process exists and be reported as a transport start failure.
- f_002 [medium] resolved internal/agents/acp_transport.go:82: The wall-clock timer is armed before the recorded attempt start and before the adapter process is started, so it is not measured from the `started` timestamp as required. With a very small positive configured cap, the context can be canceled before `cmd.Start`; the early return path then bypasses result construction and loses the distinct `wall_clock_timeout` outcome.
  Resolution: Same move as f_001 fix. The timer arm is now after `started := time.Now().UTC()` and after `cmd.Start()` succeeds. If `cmd.Start()` fails (early return `RunResult{}`), the timer was never armed so there is no risk of a cancelled context producing a misleading wall_clock_timeout outcome on the start-error path.
- f_003 [medium] resolved internal/agents/acp_transport_wallclock_test.go:73: The required ACP helper tests are incomplete: the new wall-clock transport test only covers a silent non-responsive helper and idle distinction, but does not cover periodic output/protocol activity past the idle window, normal ACP completion within the cap, or process-tree reaping of a spawned child.
  Resolution: Added three tests to `acp_transport_wallclock_test.go` and `acp_transport_wallclock_unix_test.go`: (1) `TestWallClockCapFiresWhileIdleTimerKeptAlive` — goroutine ticks the idle-activity channel at 20ms intervals keeping the 500ms idle timer alive while the 100ms wall-clock cap fires; (2) `TestACPTransportFastExitNoWallClockFlag` — `exit_fast` helper exits immediately, proving no wall-clock or idle timeout flag is set; (3) `TestACPTransportWallClockCapReapsChildProcess` (Unix) — `spawn_child` helper forks a sleeping grandchild and writes its PID to a temp file; the test verifies the PID is dead after `killProcessGroup` fires.
- f_004 [medium] resolved internal/app/config.go:399: The wall-clock cap positive-floor acceptance criterion is not verified by a unit test. The suite has request-plumbing checks for default and override values, but no test for `resolveWallClockCap` or config loading with zero/negative `wall_clock_cap`.
  Resolution: Added `TestResolveWallClockCap` in `config_test.go` covering: explicit override used, zero maps to `defaultWallClockCap`, and negative is rejected. Also added `TestReadConfigRejectsNegativeWallClockCap` that loads a config with `wall_clock_cap: -1h` and verifies `readConfig` returns an error naming `wall_clock_cap`.
- f_005 [high] resolved internal/agents/acp_transport_wallclock_test.go:16: The wall-clock transport tests do not exercise the contract's core helper-process scenarios: the helper only sleeps silently, so there is no trickled output/protocol activity keeping the idle watchdog alive, no successful ACP transport attempt that completes before the cap, and no spawned child process checked for reaping.
  Resolution: Same fix as f_003. `TestWallClockCapFiresWhileIdleTimerKeptAlive` directly proves the trickled-activity/wall-clock independence. `TestACPTransportFastExitNoWallClockFlag` covers the normal-exit-before-cap scenario. `TestACPTransportWallClockCapReapsChildProcess` covers the spawned-child reaping scenario.
- f_006 [medium] resolved internal/app/agent_attempt_timeout_test.go:228: The reviewer wall-clock skipped-lens test ignores the review command exit code, so it does not prove the round continues or converges on the remaining successful lens.
  Resolution: In `TestReviewRunWallClockKilledLensIsSkippedNotParseMiss`, captured the return value of `app.Run` and added `if code != 0 { t.Fatalf(...) }`. With one successful lens returning empty findings and the rest wall-clock-killed (skipped), the round produces zero proposals and the loop must converge with exit 0, proving the round continued past the killed lenses.
- f_007 [medium] resolved internal/app/config.go:399: The wall-clock cap resolver's zero/default and negative-rejection behavior is not unit-tested.
  Resolution: Same fix as f_004. `TestResolveWallClockCap` tests zero→default and negative→error for `resolveWallClockCap`. `TestReadConfigRejectsNegativeWallClockCap` tests the config-load rejection path.
- f_008 [medium] resolved internal/app/agent_attempt_transport_test.go:312: The request-propagation tests do not directly cover all ACP-backed attempt paths carrying WallClockCap.
  Resolution: Added `TestClarifyRunPassesDefaultWallClockCap` (clarify stage) and `TestReviewFixRunPassesDefaultWallClockCap` (review fixer stage) in `agent_attempt_transport_test.go`. Combined with the existing execute and review-run tests, all major ACP-backed attempt paths are now verified to carry the built-in `defaultWallClockCap`.
- f_009 [medium] resolved docs/agents.md:324: Public agent docs omit the new wall-clock cap configuration and result semantics.
  Resolution: Added a paragraph in `docs/agents.md` after the idle-timeout section (before the output-channels paragraph) documenting: the absolute wall-clock ceiling applies to all ACP-backed attempt types (executor, reviewer, fixer, clarifier, drafter), the default (2h), the `wall_clock_cap` config key, zero/negative rejection, the distinct `wall_clock_timeout: true` result flag, and the skipped-lens behavior for capped reviewers.
- Proposal summary: pending=0 accepted=9 rejected=0

## Reusable Project Knowledge
- scope: in scope: Add a separate ACP wall-clock cap duration alongside the existing idle Timeout plumbing, with a built-in default larger than the 25m idle default and an explicit override path that does not reinterpret the existing idle --timeout.
- scope: in scope: Start the ACP wall-clock cap once per attempt from the recorded start time; it must not reset on streamed output, protocol callbacks, tool activity, or any idle-watchdog activity tick.
- scope: in scope: When the wall-clock cap fires, cancel the ACP run context so the existing process-group kill/reap path handles the adapter and child agent process tree.
- scope: in scope: Propagate a distinct wall-clock timeout outcome through agents.RunResult, app processResult, per-stage attempt result JSON, and last-result JSON so callers can distinguish it from normal completion, idle timeout, and transport errors.
- scope: in scope: Apply the cap through the shared ACP transport/lifecycle path so executor, reviewer, review fixer, clarifier, contract drafter, and contract reviewer attempts are protected.
- scope: in scope: Surface a wall-clock-capped review lens through the existing skipped_lenses mechanism with a reason that names the wall-clock cap, while continuing the round when at least one lens succeeds.
- scope: in scope: Add deterministic helper/fake-process tests for the transport and review-loop behavior; do not invoke live external agents.
- scope: out of scope: Do not add automatic fallback or retry to claude/opus or any other reliable emitter after a wall-clock cap.
- scope: out of scope: Do not change idle-timeout duration defaults, idle activity/reset behavior, or completed_despite_timeout semantics.
- scope: out of scope: Do not change reviewer panel composition, lens selection, reviewer output parsing, or reviewer findings capture behavior.
- scope: out of scope: Do not add graceful degradation behavior for death, rate-limit, network, or provider errors beyond the wall-clock capped lens handling requested here.
- scope: out of scope: Do not reintroduce any plan-DAG behavior.
- scope: out of scope: Do not add tests that run real ACP agents or require external agent authentication.
- review_resolution: f_001 resolved: The wall-clock timer is armed before the ACP adapter has actually started, so a valid positive cap can expire before the recorded attempt start and be reported as a generic transport start failure instead of wall_clock_timeout.; resolution: Moved `startWallClockTimeout` to after `cmd.Start()` in `acp_transport.go`. The wall-clock timer is now armed only after the adapter process is running, so the ceiling cannot fire before the process exists and be reported as a transport start failure.
- review_resolution: f_002 resolved: The wall-clock timer is armed before the recorded attempt start and before the adapter process is started, so it is not measured from the `started` timestamp as required. With a very small positive configured cap, the context can be canceled before `cmd.Start`; the early return path then bypasses result construction and loses the distinct `wall_clock_timeout` outcome.; resolution: Same move as f_001 fix. The timer arm is now after `started := time.Now().UTC()` and after `cmd.Start()` succeeds. If `cmd.Start()` fails (early return `RunResult{}`), the timer was never armed so there is no risk of a cancelled context producing a misleading wall_clock_timeout outcome on the start-error path.
- review_resolution: f_003 resolved: The required ACP helper tests are incomplete: the new wall-clock transport test only covers a silent non-responsive helper and idle distinction, but does not cover periodic output/protocol activity past the idle window, normal ACP completion within the cap, or process-tree reaping of a spawned child.; resolution: Added three tests to `acp_transport_wallclock_test.go` and `acp_transport_wallclock_unix_test.go`: (1) `TestWallClockCapFiresWhileIdleTimerKeptAlive` — goroutine ticks the idle-activity channel at 20ms intervals keeping the 500ms idle timer alive while the 100ms wall-clock cap fires; (2) `TestACPTransportFastExitNoWallClockFlag` — `exit_fast` helper exits immediately, proving no wall-clock or idle timeout flag is set; (3) `TestACPTransportWallClockCapReapsChildProcess` (Unix) — `spawn_child` helper forks a sleeping grandchild and writes its PID to a temp file; the test verifies the PID is dead after `killProcessGroup` fires.
- review_resolution: f_004 resolved: The wall-clock cap positive-floor acceptance criterion is not verified by a unit test. The suite has request-plumbing checks for default and override values, but no test for `resolveWallClockCap` or config loading with zero/negative `wall_clock_cap`.; resolution: Added `TestResolveWallClockCap` in `config_test.go` covering: explicit override used, zero maps to `defaultWallClockCap`, and negative is rejected. Also added `TestReadConfigRejectsNegativeWallClockCap` that loads a config with `wall_clock_cap: -1h` and verifies `readConfig` returns an error naming `wall_clock_cap`.
- review_resolution: f_005 resolved: The wall-clock transport tests do not exercise the contract's core helper-process scenarios: the helper only sleeps silently, so there is no trickled output/protocol activity keeping the idle watchdog alive, no successful ACP transport attempt that completes before the cap, and no spawned child process checked for reaping.; resolution: Same fix as f_003. `TestWallClockCapFiresWhileIdleTimerKeptAlive` directly proves the trickled-activity/wall-clock independence. `TestACPTransportFastExitNoWallClockFlag` covers the normal-exit-before-cap scenario. `TestACPTransportWallClockCapReapsChildProcess` covers the spawned-child reaping scenario.
- review_resolution: f_006 resolved: The reviewer wall-clock skipped-lens test ignores the review command exit code, so it does not prove the round continues or converges on the remaining successful lens.; resolution: In `TestReviewRunWallClockKilledLensIsSkippedNotParseMiss`, captured the return value of `app.Run` and added `if code != 0 { t.Fatalf(...) }`. With one successful lens returning empty findings and the rest wall-clock-killed (skipped), the round produces zero proposals and the loop must converge with exit 0, proving the round continued past the killed lenses.
- review_resolution: f_007 resolved: The wall-clock cap resolver's zero/default and negative-rejection behavior is not unit-tested.; resolution: Same fix as f_004. `TestResolveWallClockCap` tests zero→default and negative→error for `resolveWallClockCap`. `TestReadConfigRejectsNegativeWallClockCap` tests the config-load rejection path.
- review_resolution: f_008 resolved: The request-propagation tests do not directly cover all ACP-backed attempt paths carrying WallClockCap.; resolution: Added `TestClarifyRunPassesDefaultWallClockCap` (clarify stage) and `TestReviewFixRunPassesDefaultWallClockCap` (review fixer stage) in `agent_attempt_transport_test.go`. Combined with the existing execute and review-run tests, all major ACP-backed attempt paths are now verified to carry the built-in `defaultWallClockCap`.
- review_resolution: f_009 resolved: Public agent docs omit the new wall-clock cap configuration and result semantics.; resolution: Added a paragraph in `docs/agents.md` after the idle-timeout section (before the output-channels paragraph) documenting: the absolute wall-clock ceiling applies to all ACP-backed attempt types (executor, reviewer, fixer, clarifier, drafter), the default (2h), the `wall_clock_cap` config key, zero/negative rejection, the distinct `wall_clock_timeout: true` result flag, and the skipped-lens behavior for capped reviewers.
- review_resolution: proposal p_001 accepted as f_001
- review_resolution: proposal p_002 accepted as f_002
- review_resolution: proposal p_003 accepted as f_003
- review_resolution: proposal p_004 accepted as f_004
- review_resolution: proposal p_005 accepted as f_005
- review_resolution: proposal p_006 accepted as f_006
- review_resolution: proposal p_007 accepted as f_007
- review_resolution: proposal p_008 accepted as f_008
- review_resolution: proposal p_009 accepted as f_009
- validation: go test ./internal/agents ./internal/app passed
- validation: go test ./... passed
- validation: go build ./... passed
- validation: make check passed

## Artifacts
- Contract: contract/contract.json
- Gate report: gate/gate-report.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Proposals: review/proposals.jsonl
- Proposal decisions: review/proposal-decisions.jsonl
