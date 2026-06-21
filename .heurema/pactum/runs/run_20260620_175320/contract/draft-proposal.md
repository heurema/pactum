# Contract Draft Proposal

## Status
- Run id: run_20260620_175320
- Status: accepted
- Source: drafter_attempt
- Drafter attempt: drafter_attempt_001
- Drafter: codex
- Accepted by: manual
- Accepted at: 2026-06-20T17:56:23Z

## In scope
- Add internal/agents/retry.go with TransportErrorClass {Retryable, Kind, Reason} and ClassifyTransportError(error) implemented against the real ACP transport/runner error shapes.
- Add internal/agents classifier tests for wrapped chains, errors.As-compatible network/timeout errors, context deadline errors, ACP/JSON-RPC/adapter text errors, rate-limit/429/overloaded, 5xx, quota/auth/401/403/insufficient_quota, clear client errors, incidental-number non-matches, and mixed transient/permanent precedence.
- Update runAgentAttemptLifecycle in internal/app/agent_attempt.go to retry only around the single a.agentTransport().Run call, only when cfg.ReadOnly is true and the transport failure or empty/no-output result is retryable.
- Use bounded jittered exponential backoff for read-only retries while keeping tests deterministic and fast.
- Append durable retry decision JSONL records under the attempt directory with schema pactum.agent_retry_decision.v1alpha1 and fields stage, attempt_id, kind, reason, retryable, read_only, attempt_number, and delay_ms.
- Add internal/app tests using fake transports for read-only transient retry success, read-only permanent no-retry, write-enabled no-retry, empty/no-output retry cap, and retry-decision artifact contents.

## Out of scope
- Changing the contract goal or answering clarification questions.
- Retrying write-enabled stages, including execute runs and review-fix or contract-fixer stages.
- Moving retry logic into internal/loop or changing loop-engine retry/corrective behavior.
- Changing ACP write-boundary or permission enforcement, adapter command selection, provider/transport registry selection, gate behavior, prompt building, review proposal parsing, or usage accounting.
- Behavioral changes to internal/agents/acp_transport.go or internal/agents/runner.go beyond preserving compatibility with the new classifier.

## Acceptance criteria
- ClassifyTransportError returns Retryable=true with stable non-empty Kind and Reason for transport drops, network errors, net.Error timeouts, context.DeadlineExceeded, rate-limit/429/overloaded text, and genuine outermost 5xx signals.
- ClassifyTransportError returns Retryable=false with stable non-empty Kind and Reason for quota exhausted/insufficient_quota, auth or permission failures including 401/403, and clear non-retryable client errors.
- Classifier tests prove the full error chain is inspected, incidental numbers in unrelated text are ignored, an outer genuine 5xx is not flipped by an incidental or nested 4xx, and genuine permanent outer causes remain non-retryable.
- For a read-only lifecycle attempt whose first transport call fails with a retryable transient error and second call succeeds, the fake transport is called twice and result/last-result artifacts reflect the successful call.
- For a read-only lifecycle attempt with a permanent transport error, the fake transport is called once and existing failure reporting behavior is preserved.
- For a write-enabled lifecycle attempt with a retryable transient transport error, the fake transport is called once, no retry is scheduled, and existing write-stage failure behavior is preserved.
- For an empty/no-output transport result on a read-only stage, retries stop after at most three total transport calls and cannot loop indefinitely.
- Retry-decision JSONL artifact lines are valid JSON, live under the corresponding attempt directory, use schema pactum.agent_retry_decision.v1alpha1, include the required fields with no absolute local paths, and record both retry and final no-retry decisions with delay_ms set to 0 when no retry is scheduled.
- Existing ACP read-only write/permission enforcement, execute attempt, review attempt, and timeout tests continue to pass.

## Validation commands
- go test ./internal/agents ./internal/app
- make check

## Assumptions
- The retry-decision artifact filename will be retry-decisions.jsonl in each attempt directory unless an existing repository convention requires a different name.
- The v1 max-attempts cap is three total transport calls per read-only lifecycle attempt: the initial call plus at most two retries.
- attempt_number is a one-based transport-call number within the same Pactum attempt directory, not a new Pactum attempt ID.
- A retry decision record is written for every evaluated failed or empty/no-output transport result, including decisions not to retry.
- Plain context.Canceled without a timeout/deadline, transport-drop, or provider transient signal is not independently considered retryable because it may represent intentional cancellation.

