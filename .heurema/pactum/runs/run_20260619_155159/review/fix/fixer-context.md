# Review Fixer Context

## Run
- Run id: run_20260619_155159
- Run status: contract_approved

## Approved contract
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

## Current review findings
- Summary: findings=9 open=9 resolved=0 blocking_open=9
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: The wall-clock timer is armed before the ACP adapter has actually started, so a valid positive cap can expire before the recorded attempt start and be reported as a generic transport start failure instead of wall_clock_timeout.
    location: internal/agents/acp_transport.go:82
  - f_002 severity=medium category=correctness blocking=true status=open: The wall-clock timer is armed before the recorded attempt start and before the adapter process is started, so it is not measured from the `started` timestamp as required. With a very small positive configured cap, the context can be canceled before `cmd.Start`; the early return path then bypasses result construction and loses the distinct `wall_clock_timeout` outcome.
    location: internal/agents/acp_transport.go:82
  - f_003 severity=medium category=validation blocking=true status=open: The required ACP helper tests are incomplete: the new wall-clock transport test only covers a silent non-responsive helper and idle distinction, but does not cover periodic output/protocol activity past the idle window, normal ACP completion within the cap, or process-tree reaping of a spawned child.
    location: internal/agents/acp_transport_wallclock_test.go:73
  - f_004 severity=medium category=validation blocking=true status=open: The wall-clock cap positive-floor acceptance criterion is not verified by a unit test. The suite has request-plumbing checks for default and override values, but no test for `resolveWallClockCap` or config loading with zero/negative `wall_clock_cap`.
    location: internal/app/config.go:399
  - f_005 severity=high category=quality blocking=true status=open: The wall-clock transport tests do not exercise the contract's core helper-process scenarios: the helper only sleeps silently, so there is no trickled output/protocol activity keeping the idle watchdog alive, no successful ACP transport attempt that completes before the cap, and no spawned child process checked for reaping.
    location: internal/agents/acp_transport_wallclock_test.go:16
  - f_006 severity=medium category=quality blocking=true status=open: The reviewer wall-clock skipped-lens test ignores the review command exit code, so it does not prove the round continues or converges on the remaining successful lens.
    location: internal/app/agent_attempt_timeout_test.go:228
  - f_007 severity=medium category=quality blocking=true status=open: The wall-clock cap resolver's zero/default and negative-rejection behavior is not unit-tested.
    location: internal/app/config.go:399
  - f_008 severity=medium category=quality blocking=true status=open: The request-propagation tests do not directly cover all ACP-backed attempt paths carrying WallClockCap.
    location: internal/app/agent_attempt_transport_test.go:312
  - f_009 severity=medium category=quality blocking=true status=open: Public agent docs omit the new wall-clock cap configuration and result semantics.
    location: docs/agents.md:324
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - none

## Artifacts
- Contract: contract/contract.json
- Review: review/review.json
- Findings: review/findings.jsonl
- Resolutions: review/resolutions.jsonl
- Gate report: gate/gate-report.json
- Execution result: execute/last-result.json

## Fixer guidance
- Source files are the source of truth.
- Use `pactum search "<term>"` and inspect current source files before relying on this context.
- For each current review finding, trace the finding to the code.
- If a finding is valid, fix it in place within the approved contract scope.
- If a finding is a false positive, leave code unchanged for that finding and explain the rebuttal in your final output.
- Do not approve the review or mutate review findings/resolutions/proposals.
- Do not modify generated `.heurema` artifacts.
