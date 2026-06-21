# Review Fixer Context

## Run
- Run id: run_20260620_175320
- Run status: contract_approved

## Approved contract
- Goal: Add a transport retry/error classifier and apply automatic retries to READ-ONLY agent stages only. This is v1: write-stage (write-enabled execute/fixer) retry is explicitly OUT of scope and deferred to a later slice, because retrying after a possible write risks duplicate writes.

First understand the actual transport error shapes: read internal/agents/acp_transport.go and internal/agents/runner.go to see what errors agentTransport().Run actually returns (ACP adapter subprocess failures, JSON-RPC errors, network/timeout/context-cancellation, and model-API errors such as rate-limit/quota that the adapter forwards as text). Classify THOSE real shapes, not a hypothetical HTTP client.

Create internal/agents/retry.go with a TransportErrorClass type (fields: Retryable bool, Kind string, Reason string) and a function ClassifyTransportError(error) TransportErrorClass. The classifier must: walk the FULL error chain (errors.Unwrap / errors.As), not just the top-level message; classify as transient/retryable = transport drops, network errors, timeouts and context-deadline-exceeded, rate-limit/429/overloaded, and 5xx; classify as permanent/non-retryable = quota exhausted, auth/permission failures (401/403/insufficient_quota), and clear client errors; when both a transient and a permanent signal appear, give transient/5xx precedence only when it is the genuine outermost cause (do not let an incidentally-mentioned nested 4xx flip a real 5xx); be careful not to match incidental numbers in unrelated text. Also treat an empty/no-output transport result as retryable up to a small cap.

Apply retries in runAgentAttemptLifecycle (internal/app/agent_attempt.go) around the single agentTransport().Run call — NOT inside the loop engine (internal/loop). Retry ONLY when the stage is read-only (RunRequest.ReadOnly == true) AND the error classifies retryable (or the result was empty), using jittered exponential backoff with a small max-attempts cap. For write-enabled stages do NOT retry — keep current behavior exactly. Record each retry decision as a durable JSONL artifact line under the attempt directory (schema pactum.agent_retry_decision.v1alpha1 with fields: stage, attempt_id, kind, reason, retryable, read_only, attempt_number, delay_ms).

Scope: new internal/agents/retry.go (with tests) and internal/app/agent_attempt.go (with tests). Do NOT touch acp_transport.go write-boundary/permission handling, the gate, the loop engine, provider/transport selection, or add any write-stage retry. Acceptance: ClassifyTransportError has table-driven tests covering full-chain unwrap, transient cases (network/timeout/rate-limit/5xx), permanent cases (quota/auth), transient-over-incidental-nested-4xx precedence, and incidental-number non-matching; runAgentAttemptLifecycle retries a transient failure on a read-only stage and then succeeds, does NOT retry a permanent failure, does NOT retry any failure on a write-enabled stage, and writes retry-decision artifacts; make check passes.
- In scope:
  - Add internal/agents/retry.go with TransportErrorClass {Retryable, Kind, Reason} and ClassifyTransportError(error) implemented against the real ACP transport/runner error shapes.
  - Add internal/agents classifier tests for wrapped chains, errors.As-compatible network/timeout errors, context deadline errors, ACP/JSON-RPC/adapter text errors, rate-limit/429/overloaded, 5xx, quota/auth/401/403/insufficient_quota, clear client errors, incidental-number non-matches, and mixed transient/permanent precedence.
  - Update runAgentAttemptLifecycle in internal/app/agent_attempt.go to retry only around the single a.agentTransport().Run call, only when cfg.ReadOnly is true and the transport failure or empty/no-output result is retryable.
  - Use bounded jittered exponential backoff for read-only retries while keeping tests deterministic and fast.
  - Append durable retry decision JSONL records under the attempt directory with schema pactum.agent_retry_decision.v1alpha1 and fields stage, attempt_id, kind, reason, retryable, read_only, attempt_number, and delay_ms.
  - Add internal/app tests using fake transports for read-only transient retry success, read-only permanent no-retry, write-enabled no-retry, empty/no-output retry cap, and retry-decision artifact contents.
- Out of scope:
  - Changing the contract goal or answering clarification questions.
  - Retrying write-enabled stages, including execute runs and review-fix or contract-fixer stages.
  - Moving retry logic into internal/loop or changing loop-engine retry/corrective behavior.
  - Changing ACP write-boundary or permission enforcement, adapter command selection, provider/transport registry selection, gate behavior, prompt building, review proposal parsing, or usage accounting.
  - Behavioral changes to internal/agents/acp_transport.go or internal/agents/runner.go beyond preserving compatibility with the new classifier.
- Acceptance criteria:
  - ClassifyTransportError returns Retryable=true with stable non-empty Kind and Reason for transport drops, network errors, net.Error timeouts, context.DeadlineExceeded, rate-limit/429/overloaded text (without an accompanying insufficient_quota signal in the same error), and genuine outermost 5xx signals.
  - ClassifyTransportError returns Retryable=false with stable non-empty Kind and Reason for quota exhausted/insufficient_quota (including HTTP 429 responses whose body carries an insufficient_quota signal), auth or permission failures including 401/403, and clear non-retryable client errors.
  - ClassifyTransportError returns Retryable=false with Kind='unknown' and a non-empty Reason for any error that does not match a known transient or permanent pattern, providing a conservative default that prevents unrecognized errors from triggering retries.
  - Classifier tests prove the full error chain is inspected via errors.Unwrap/errors.As, incidental numbers in unrelated text are ignored, an outer genuine 5xx is not flipped by an incidental or nested 4xx, genuine permanent outer causes remain non-retryable, and an unrecognized error returns Retryable=false with Kind='unknown'.
  - Classifier tests include a case where the same error contains both a 429/rate-limit signal and an insufficient_quota signal, and verify that ClassifyTransportError returns Retryable=false (permanent wins).
  - For a read-only lifecycle attempt whose first transport call fails with a retryable transient error and second call succeeds, the fake transport is called twice and result/last-result artifacts reflect the successful call.
  - For a read-only lifecycle attempt with a permanent transport error, the fake transport is called once and existing failure reporting behavior is preserved.
  - For a write-enabled lifecycle attempt with a retryable transient transport error, the fake transport is called once, no retry is scheduled, and existing write-stage failure behavior is preserved.
  - For an empty/no-output transport result on a read-only stage, retries stop after at most three total transport calls (the initial call plus at most two retries) and cannot loop indefinitely; if all calls return empty results the final empty result is returned as-is.
  - When a read-only transport call returns context.Canceled and RunResult.TimedOut is true (idle timeout signaled by the runner canceling the context after the idle deadline), the lifecycle treats the attempt as retryable regardless of what ClassifyTransportError returns for the context.Canceled error alone.
  - When context.Canceled arrives with RunResult.TimedOut false and without a transport-drop or provider transient signal that ClassifyTransportError independently marks Retryable=true, the lifecycle does NOT retry.
  - RunResult.WallClockTimeout true or RunResult.CompletedDespiteTimeout true suppresses any retry on any stage including read-only stages; the result is returned as-is even if the error or empty-output heuristic would otherwise indicate retryable.
  - Retry-decision JSONL artifact lines are valid JSON, live under the corresponding attempt directory, use schema pactum.agent_retry_decision.v1alpha1, include the required fields with no absolute local paths, and record both retry and final no-retry decisions with delay_ms set to 0 when no retry is scheduled.
  - Existing ACP read-only write/permission enforcement, execute attempt, review attempt, and timeout tests continue to pass.
- Validation commands:
  - go test ./internal/agents ./internal/app
  - make check

## Current review findings
- Summary: findings=7 open=7 resolved=0 blocking_open=6
- Blocking findings (fix or rebut these — emit exactly one fix-outcome for each):
  - f_001 severity=medium category=correctness blocking=true status=open: Retry-decision artifact append failures are silently ignored, so a read-only retry can occur without the required durable JSONL record.
    location: internal/app/agent_attempt.go:332
  - f_002 severity=medium category=correctness blocking=true status=open: The classifier gives any 5xx token precedence over auth/client errors in ACP internal-error text, so permanent outer auth/client failures that mention an incidental 5xx are retried.
    location: internal/agents/retry.go:121
  - f_003 severity=medium category=scope blocking=true status=open: Retry backoff is exponential but not jittered.
    location: internal/app/agent_attempt.go:98
  - f_004 severity=medium category=validation blocking=true status=open: The retry-specific tests do not verify that CompletedDespiteTimeout suppresses retries on a read-only stage.
    location: internal/app/agent_attempt.go:175
  - f_005 severity=medium category=quality blocking=true status=open: The lifecycle tests for context-cancellation retry behavior do not use context.Canceled, so they do not cover the required real error shape.
    location: internal/app/agent_attempt_retry_test.go:329
  - f_006 severity=medium category=quality blocking=true status=open: Retry-decision artifact write failures are silently ignored.
    location: internal/app/agent_attempt.go:332
- Advisory (non-blocking) findings (context only — do NOT edit code and do NOT emit outcomes for them):
  - f_007 severity=medium category=quality blocking=false status=open: Document the new read-only transport retry behavior and retry-decision artifact; the agent docs still describe reviewer rounds as costing exactly five subprocess runs per reviewer.
    location: docs/agents.md:527

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
