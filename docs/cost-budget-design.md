# Token accounting, budget, and estimation — design

Status: **draft for review**. Supersedes the one-paragraph "Budget stop" sketch in
[`loop-architecture-design.md`](loop-architecture-design.md) and fills in the
`budget` config block that today is declared but unenforced.

## Why

pactum orchestrates paid agent CLIs (`codex`, `claude`) in one-shot runs and in an
autonomous review loop. Today it has **zero visibility** into what a task consumes:
`ledger/usage.jsonl` is written empty, `ledger/cost.json` is hardcoded zeros,
`budget.max_usd` is never read, and `status` prints `estimated cost: $0.00` as a
constant.

The pressure is real and immediate. On this machine both CLIs are authenticated via
**subscriptions** (Claude Max 20×, ChatGPT), not API keys — so each pactum run draws
from the *same* metered quota as interactive use (5-hour rolling + weekly windows).
The token economy (usage tiers, weekly limits, per-token pricing, a separate monthly
Agent-SDK credit for headless `claude -p`) is the direction the ecosystem is moving.
We want to answer, first, **"how many tokens did this task cost?"** and later **"how
many am I willing to spend, and roughly how many will it take?"**

## Principles

1. **Tokens are the unit; cost ($) is a derived view.** Both CLIs report tokens
   exactly per run; dollars require a price table we'd have to maintain, and on a
   subscription dollars aren't even the billed unit. The durable source of truth is
   raw token counts; cost is recomputed from a versioned price table later.
2. **Capture is best-effort and non-fatal.** A usage parse miss records "unknown"
   and a warning — it must **never** fail a run. Observability cannot break work.
3. **Rich, normalized schema now; thin display now.** Even though we only show a
   total at first, we persist the full provider-normalized split (input / output /
   cache / reasoning) plus the raw blob, so cost, budget, and estimation bolt on
   without a migration.
4. **Accumulate post-call (truth); forecast as a range (estimate).** Spend-so-far is
   summed from real usage. Anything forward-looking (next-call input, loop total) is
   a range, never a false-precision number.

## What is and isn't measurable (subscription reality)

| | Measurable headless? | Notes |
|---|---|---|
| **Tokens per run** | **Yes** — reliable | `codex exec --json` and `claude -p --output-format json` both report it |
| Cost in $ | Derived only | claude self-reports `total_cost_usd` but it is a client-side **estimate** (often 0/null under subscription); codex reports no cost. Compute from a price table |
| Remaining quota % | **No** (headless) | Only interactive `/usage` (claude) / `/status` (codex) or web dashboards expose remaining 5h/weekly fraction. Token totals ÷ a guessed plan cap is an estimate only |

So the honest, solid deliverable is **token accounting per task**. Budgets are
enforced in **tokens** (exact). Cost and quota-fraction are best-effort overlays.

## The record schema (the foundation)

One `UsageRecord` per agent subprocess call, append-only JSONL. Field names mirror the
OpenTelemetry GenAI semantic conventions (`gen_ai.usage.*`) so the ledger can later be
exported as OTel spans/metrics with a rename, and so the normalized meaning is a known
standard rather than ad-hoc.

```go
// UsageRecord is one agent subprocess call's token usage, normalized across
// providers. Tokens are the unit; Cost is a derived view added later.
type UsageRecord struct {
	SchemaVersion int    `json:"schema_version"` // 1
	RecordID      string `json:"record_id"`
	DedupKey      string `json:"dedup_key,omitempty"` // sha256(provider response/message id)

	// Run / stage context
	RunID     string `json:"run_id"`
	AttemptID string `json:"attempt_id"`
	Stage     string `json:"stage"`      // execute | review | fix | clarify | draft
	CreatedAt string `json:"created_at"` // RFC3339

	// OTel-aligned dimensions
	Provider      string `json:"provider"` // codex | anthropic  (discriminator — never drop)
	Agent         string `json:"agent"`    // codex | claude  (pactum builtin name)
	RequestModel  string `json:"request_model,omitempty"`
	ResponseModel string `json:"response_model,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"` // pinned CLI version (drift attribution)

	// Normalized token counts — OTel INCLUSIVE convention:
	//   InputTokens includes cache; OutputTokens includes reasoning.
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"` // always materialized (Anthropic has none)

	// Token-class detail (subset view; needed for the later per-class cost layer;
	// NOT guaranteed to sum to the parent).
	CacheReadTokens     int64 `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int64 `json:"cache_creation_tokens,omitempty"`
	ReasoningTokens     int64 `json:"reasoning_tokens,omitempty"`

	// Capture provenance
	Captured bool            `json:"captured"`      // false => parse miss; counts are 0/partial
	Raw      json.RawMessage `json:"raw,omitempty"` // verbatim provider usage blob — never lose data

	// Forward-compat cost layer (nil now; additive later)
	Cost *UsageCost `json:"cost,omitempty"` // { usd, price_source, priced_at }
}
```

### Why each non-obvious field

- **`Provider` discriminator + `Raw` blob.** The two providers use *opposite*
  conventions, so a normalized number is only interpretable with provenance, and the
  raw blob future-proofs against new fields and lets a later cost layer price token
  classes we didn't normalize.
- **OTel inclusive convention.** We canonicalize `InputTokens` to *include* cache and
  `OutputTokens` to *include* reasoning, and keep the cache/reasoning sub-counts
  separately for pricing. (The sub-counts are a view; they need not sum to the parent.)
- **`TotalTokens` always materialized.** Anthropic provides no total; compute it once
  at ingest so no downstream consumer re-derives it (and gets the cache rule wrong).
- **`DedupKey`.** Providers re-emit usage across streaming events and parallel tool
  calls share a message id; dedup by the provider response/message id to avoid
  double-counting.
- **`Captured`.** Distinguishes a real zero from a parse miss (best-effort principle).
- **`Cost` nil now.** Purely additive path to the $ layer; no schema change later.

### Per-provider normalization (the cache trap)

The single highest-risk detail: **codex/OpenAI `input` already includes cached tokens
(subset); Anthropic `input` excludes cache (additive).** Normalize at ingest:

**codex** (`turn.completed.usage`: `input_tokens`, `cached_input_tokens`,
`output_tokens`, `reasoning_output_tokens`; `cached ⊆ input`; usage is **cumulative
per session** → take the *final* `turn.completed`):
```
InputTokens         = input_tokens                         // already inclusive
CacheReadTokens     = cached_input_tokens
CacheCreationTokens = 0                                     // codex reports no writes
ReasoningTokens     = reasoning_output_tokens               // optional (newer versions)
OutputTokens        = output_tokens + reasoning_output_tokens // codex output EXCLUDES reasoning
TotalTokens         = InputTokens + OutputTokens
```

**claude** (`result.usage`: `input_tokens`, `output_tokens`,
`cache_creation_input_tokens`, `cache_read_input_tokens`; input **excludes** cache):
```
CacheReadTokens     = cache_read_input_tokens
CacheCreationTokens = cache_creation_input_tokens
InputTokens         = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
ReasoningTokens     = thinking tokens if present else 0
OutputTokens        = output_tokens                        // already inclusive of thinking
TotalTokens         = InputTokens + OutputTokens
```

## Capturing usage

### Where

Parse at the **runner** boundary, record once in the **shared lifecycle** — both
already exist:

1. `internal/agents/runner.go` already captures each subprocess's stdout/stderr. Add a
   **per-agent usage parser** (a strategy on the agent descriptor) that runs on the
   captured output after the process exits and fills a new `RunResult.Usage` field
   (best-effort; on miss, `Captured=false`).
2. `internal/app/agent_attempt.go`'s `runAgentAttemptLifecycle` (the M11.2 dedup that
   all five commands share) records the `UsageRecord` to the ledger in exactly one
   place — so execute / review / fix / clarify / draft are all covered for free.

### The `--json` coupling (sequencing)

Getting the rich, structured split requires `--json` (codex) / `--output-format json`
(claude). That changes stdout, and the impact splits by **stage**, not agent:

- **Write stages (execute, fix):** pactum does not parse the agent's stdout for results
  (the result is the file diff), so switching to json output is low-risk. **Start here.**
- **Read stages (review, clarify, draft):** these parse fenced-JSON findings/questions
  out of the text; json mode wraps that in an envelope (`.result`), so their parser
  must be adapted. **Follow-up slice.**

### Robust parsing (best-effort)

- codex: read JSONL line-by-line, skip unknown/garbled lines, take the **last**
  `turn.completed.usage` (cumulative = session total). Fallback: scrape stderr
  `tokens used\s+([\d,]+)` (total only, version-unstable) → record total, `Captured`
  still true but no split. On total failure: `Captured=false`.
- claude: decode the single `result` object (pointer/omitempty fields → missing =
  zero, not error); ignore unknown fields; on JSON failure fall back to text mode and
  `Captured=false`.
- Pin and store the CLI version (`AgentVersion`) so format drift is attributable.

## Ledger and surfacing

- **Per-run, append-only:** `runs/<id>/ledger/usage.jsonl` (one record per agent call).
  This is already in the committed set under the M11.9 VCS policy (the durable record).
- **Global rollup (derived, rebuildable):** scan run files to aggregate by
  run / stage / agent / model / day. Never the source of truth. The static
  cross-run aggregate (by run / stage / agent / model + cache-read ratio) is
  implemented as `pactum usage --all` (M13.0); the per-day trend series is a
  deferred follow-up.
- **"Tokens for this task"** = fold the run's `usage.jsonl` (sum each class, count
  calls). Replace the hardcoded zeros in `status` with the live per-run total + a
  by-stage / by-agent breakdown, and add a `pactum usage [run_id]` command. Show the
  cache-hit ratio (cache_read vs input) — caching is the biggest cost lever in the loop.

## Forward layers (designed-for, deferred)

These read the same records; no schema change.

- **Cost ($):** a versioned per-model, **per-token-class** price table (normal input,
  output, cache-write, cache-read, reasoning priced separately) → fill the `Cost`
  block. Prefer an agent-reported cost where trustworthy; otherwise compute. (claude's
  headless `-p` becomes cleanly $-denominated once Anthropic's separate Agent-SDK
  credit takes effect.)
- **Budget stop (implemented for the review loop):** primitive is **`max_tokens` per
  run** (exact, no price table). Truth = post-call accumulation of the run's
  **captured** usage records; uncaptured records do not count toward the stop. At the
  start of rounds after the first, when the captured total reaches the configured
  ceiling, `mode: block` terminates the loop with **`budget_exceeded`** and records
  the mode, `max_tokens`, and captured token total in `review/loop-summary.json`.
  `mode: warn` records a budget warning and continues. A single round may overshoot
  by its own calls because there is no pre-call estimation gate in this slice.
  (`max_usd` was later removed from the config as dead — the M16.0 config redesign
  keeps only the enforced `review.budget.{mode, max_tokens}`; a USD ceiling can
  return with the cost layer.)
- **Estimation:** input is countable before a call (provider count-tokens endpoint /
  local tokenizer / `chars/4`); output is not (per-stage historical ratios from our own
  ledger + `max_tokens` ceiling); a loop total must model quadratic context growth and
  is reported as a **range**, never a point. Surface used / remaining / projected and a
  context-window gauge.

## Phased plan

Slice 4 was implemented ahead of the cost slice because token-native enforcement
does not need pricing data.

1. **Slice 1 — token accounting + visibility (this milestone).** `UsageRecord` schema +
   per-agent parser in the runner (`--json` for write stages: execute, fix) +
   `RunResult.Usage` + record in the shared lifecycle → `usage.jsonl`; live per-run
   total + breakdown in `status` and a `pactum usage` command. Best-effort/non-fatal.
   No cost, no budget, no estimation.
2. **Slice 2 — read-stage capture (implemented).** Reviewer / clarify / draft run
   with structured output too; their parsers unwrap the agent message text before
   reading fenced JSON, so read-stage usage is captured in full-run totals.
3. **Slice 3 — cost ($) overlay.** Versioned per-class price table → `Cost` block;
   `pactum usage` shows $ alongside tokens (flagged estimate under subscription).
4. **Slice 4 — budget stop (implemented).** `max_tokens` per run; accumulate
   captured usage + terminal `budget_exceeded`; `mode` warn/block; compose with the
   loop. Post-call only; no dollar enforcement.
5. **Slice 5 — estimation.** Pre-call input count + historical output ratios →
   used/remaining/projected range; context-window gauge.

## Risks / open questions

- **Format drift.** Both CLIs change usage output across versions (codex
  `reasoning_output_tokens` is recent; the stderr "tokens used" line is unstable).
  Mitigation: best-effort parsing, `AgentVersion` stamping, the raw blob.
- **Read-stage coupling.** Slice 2 touches the findings parser — the riskiest part;
  isolate and test it. Until then, full-run totals exclude reviewer/clarify/draft.
- **Cumulative vs per-call.** codex usage is cumulative per session; one `codex exec`
  is one session, so the final event is the call's total. Multiple attempts/rounds are
  summed across records. Never sum streaming deltas.
- **Subscription cost meaning.** `total_cost_usd` is an estimate and may be 0/null
  under Max; never treat any CLI's self-reported cost as billing truth.
- **Quota fraction is not authoritative headless.** If we ever show "% of plan used",
  label it an estimate (tokens ÷ a guessed cap), or shell out to interactive surfaces.

## References

- OpenTelemetry GenAI semantic conventions (token attribute names):
  `opentelemetry.io/docs/specs/semconv/gen-ai/`
- Anthropic Messages API usage object (no `total_tokens`; input excludes cache):
  `platform.claude.com/docs/en/api/messages`
- Claude Code headless / agent-sdk cost tracking (`--output-format json` usage,
  `total_cost_usd` is an estimate, dedup by message id):
  `code.claude.com/docs/en/headless`, `code.claude.com/docs/en/agent-sdk/cost-tracking`
- OpenAI usage objects (cached tokens are a subset of input; reasoning tokens):
  `developers.openai.com/api/docs/guides/prompt-caching`
- codex non-interactive `--json` output (`turn.completed.usage`, cumulative):
  `developers.openai.com/codex/noninteractive`
- Anthropic token-counting endpoint (pre-call input count):
  `platform.claude.com/docs/en/build-with-claude/token-counting`
