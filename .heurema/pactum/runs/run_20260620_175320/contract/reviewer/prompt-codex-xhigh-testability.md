# Contract Review: Testability

You are reviewing a software change contract through the **acceptance-testability** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Add a transport retry/error classifier and apply automatic retries to READ-ONLY agent stages only. This is v1: write-stage (write-enabled execute/fixer) retry is explicitly OUT of scope and deferred to a later slice, because retrying after a possible write risks duplicate writes.

First understand the actual transport error shapes: read internal/agents/acp_transport.go and internal/agents/runner.go to see what errors agentTransport().Run actually returns (ACP adapter subprocess failures, JSON-RPC errors, network/timeout/context-cancellation, and model-API errors such as rate-limit/quota that the adapter forwards as text). Classify THOSE real shapes, not a hypothetical HTTP client.

Create internal/agents/retry.go with a TransportErrorClass type (fields: Retryable bool, Kind string, Reason string) and a function ClassifyTransportError(error) TransportErrorClass. The classifier must: walk the FULL error chain (errors.Unwrap / errors.As), not just the top-level message; classify as transient/retryable = transport drops, network errors, timeouts and context-deadline-exceeded, rate-limit/429/overloaded, and 5xx; classify as permanent/non-retryable = quota exhausted, auth/permission failures (401/403/insufficient_quota), and clear client errors; when both a transient and a permanent signal appear, give transient/5xx precedence only when it is the genuine outermost cause (do not let an incidentally-mentioned nested 4xx flip a real 5xx); be careful not to match incidental numbers in unrelated text. Also treat an empty/no-output transport result as retryable up to a small cap.

Apply retries in runAgentAttemptLifecycle (internal/app/agent_attempt.go) around the single agentTransport().Run call — NOT inside the loop engine (internal/loop). Retry ONLY when the stage is read-only (RunRequest.ReadOnly == true) AND the error classifies retryable (or the result was empty), using jittered exponential backoff with a small max-attempts cap. For write-enabled stages do NOT retry — keep current behavior exactly. Record each retry decision as a durable JSONL artifact line under the attempt directory (schema pactum.agent_retry_decision.v1alpha1 with fields: stage, attempt_id, kind, reason, retryable, read_only, attempt_number, delay_ms).

Scope: new internal/agents/retry.go (with tests) and internal/app/agent_attempt.go (with tests). Do NOT touch acp_transport.go write-boundary/permission handling, the gate, the loop engine, provider/transport selection, or add any write-stage retry. Acceptance: ClassifyTransportError has table-driven tests covering full-chain unwrap, transient cases (network/timeout/rate-limit/5xx), permanent cases (quota/auth), transient-over-incidental-nested-4xx precedence, and incidental-number non-matching; runAgentAttemptLifecycle retries a transient failure on a read-only stage and then succeeds, does NOT retry a permanent failure, does NOT retry any failure on a write-enabled stage, and writes retry-decision artifacts; make check passes.

**Scope in**:
  - Add internal/agents/retry.go with TransportErrorClass {Retryable, Kind, Reason} and ClassifyTransportError(error) implemented against the real ACP transport/runner error shapes.
  - Add internal/agents classifier tests for wrapped chains, errors.As-compatible network/timeout errors, context deadline errors, ACP/JSON-RPC/adapter text errors, rate-limit/429/overloaded, 5xx, quota/auth/401/403/insufficient_quota, clear client errors, incidental-number non-matches, and mixed transient/permanent precedence.
  - Update runAgentAttemptLifecycle in internal/app/agent_attempt.go to retry only around the single a.agentTransport().Run call, only when cfg.ReadOnly is true and the transport failure or empty/no-output result is retryable.
  - Use bounded jittered exponential backoff for read-only retries while keeping tests deterministic and fast.
  - Append durable retry decision JSONL records under the attempt directory with schema pactum.agent_retry_decision.v1alpha1 and fields stage, attempt_id, kind, reason, retryable, read_only, attempt_number, and delay_ms.
  - Add internal/app tests using fake transports for read-only transient retry success, read-only permanent no-retry, write-enabled no-retry, empty/no-output retry cap, and retry-decision artifact contents.

**Scope out**:
  - Changing the contract goal or answering clarification questions.
  - Retrying write-enabled stages, including execute runs and review-fix or contract-fixer stages.
  - Moving retry logic into internal/loop or changing loop-engine retry/corrective behavior.
  - Changing ACP write-boundary or permission enforcement, adapter command selection, provider/transport registry selection, gate behavior, prompt building, review proposal parsing, or usage accounting.
  - Behavioral changes to internal/agents/acp_transport.go or internal/agents/runner.go beyond preserving compatibility with the new classifier.

**Acceptance criteria**:
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

**Validation commands**:
  - go test ./internal/agents ./internal/app
  - make check

**Assumptions**:
  - The retry-decision artifact filename is retry-decisions.jsonl in each attempt directory unless an existing repository convention requires a different name.
  - The v1 max-attempts cap is three total transport calls per read-only lifecycle attempt: the initial call plus at most two retries.
  - attempt_number is a one-based transport-call number within the same Pactum attempt directory, not a new Pactum attempt ID.
  - A retry decision record is written for every evaluated failed or empty/no-output transport result, including decisions not to retry.
  - Plain context.Canceled without a coincident RunResult.TimedOut=true, transport-drop, or provider transient signal is not independently considered retryable because it may represent intentional cancellation. When context.Canceled coincides with RunResult.TimedOut=true, the lifecycle treats it as a retryable idle timeout rather than intentional cancellation.
  - An empty/no-output transport result means the transport Run call returns a nil error and RunResult.StdoutPath either does not exist or its content consists entirely of whitespace (zero non-whitespace bytes after trimming). A non-nil error is always routed through ClassifyTransportError rather than the empty-result path; a non-zero exit code alone does not constitute an empty result. A legitimately-empty response (e.g., a read-only stage that genuinely produces no text) is retried up to the max-attempts cap and returned as-is if all attempts remain empty.
  - When a 429 response carries an insufficient_quota body signal, the permanent/non-retryable classification wins over the transient 429 rate-limit classification. A plain 429 or overloaded signal without an insufficient_quota body signal remains transient/retryable.

## Lens: Testability

Checklist:
- Is each acceptance criterion backed by or expressible as a runnable validation command (not just prose)?
- Are any criteria purely prose with no machine-checkable outcome?

## Output

State your analysis in prose. If you find issues, also include a structured block:

```json
{
  "schema": "pactum.reviewer_findings.v1alpha1",
  "findings": [
    {
      "message": "Describe the contract issue clearly.",
      "severity": "medium",
      "category": "quality",
      "blocking": true,
      "evidence": "Quote or cite the contract field that shows the issue."
    }
  ]
}
```

Rules:
- Use severity: low, medium, high, critical.
- Use category: correctness, scope, quality, validation, process, other.
- Omit file and line (not applicable for contract review).
- Set blocking=true for defects that should block approval: gaps that make the contract unexecutable or ungatable.
- Set blocking=false for advisory issues.
- If no issues, say so clearly. Do not include an empty findings block.
