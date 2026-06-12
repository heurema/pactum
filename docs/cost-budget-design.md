# Token accounting, budget, and estimation — design

Status: **draft for review**. Supersedes the one-paragraph "Budget stop" sketch in
[`loop-architecture-design.md`](loop-architecture-design.md).

> **Budget surface removed (M25.1).** The earlier `review.budget` config block
> (`mode` / `max_tokens`) gated nothing real and has been removed: the default
> config no longer emits it, `readConfig` rejects a leftover `review.budget` key
> with a loud configuration error, and the warn/block budget plumbing in the
> review loop is deleted. Token accounting (the usage ledger and `pactum usage`)
> is untouched. Budget enforcement returns later as a designed feature; this
> document is its home. A budget must be denominated in **effective units**
> (the provider-weighted cost proxy below), not raw tokens — a cache-heavy run
> can read an order of magnitude cheaper than its raw token count suggests.

## Why

pactum orchestrates paid agent CLIs (`codex`, `claude`) in one-shot runs and in an
autonomous review rounds (`review run`). Today it has **zero visibility** into what a task consumes:
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

## Cache economics (verified for the budget feature)

A future budget must price token classes, not count them, and the dominant lever
is the cache. The facts below are verified against current official provider
documentation (cited inline); anything not directly supported by official docs is
labelled an **implementation assumption**. These are the inputs the effective-unit
metric (`pactum usage`) already encodes and the budget feature must account in.

### Per-provider write/read multipliers (relative to fresh input = 1.0×)

| Class | Anthropic | OpenAI / Codex |
|---|---|---|
| Fresh input | 1.0× | 1.0× |
| Cache **write** | **1.25×** (5-minute TTL; 2× for 1-hour) | **free** → priced as fresh (1.0×) |
| Cache **read** | **0.1×** | **0.1×** (cached input is discounted up to ~90%) |
| Output | **5×** (Opus 4.8 is $5/MTok in, $25/MTok out) | 5× (standard ratio) |

- **Anthropic** charges a 1.25× premium to *write* the cache and bills cache
  *reads* at ~0.1× of base input; output is ~5× input at current Opus pricing.
  Source: Anthropic prompt-caching docs (`platform.claude.com/docs/en/build-with-claude/prompt-caching`)
  and pricing (`platform.claude.com/docs/en/pricing`).
- **OpenAI/Codex** charge **no** premium for cache writes ("caching happens
  automatically, with no … extra cost"), so a write prices as fresh input; cached
  reads are discounted by up to ~90% (≈0.1×). Source: OpenAI prompt-caching guide
  (`developers.openai.com/api/docs/guides/prompt-caching`).

These ratios live as named constants (`anthropicCacheWriteMultiplier`,
`openAICacheWriteMultiplier`, `effectiveCacheReadMultiplier`,
`effectiveOutputMultiplier`) in `internal/app/usage.go`.

### Cache scoping and routing

- **Anthropic.** Caches are scoped to the **organization** (and, within it, the
  workspace); a cache entry is keyed by the exact prompt prefix and is
  model-scoped. Source: Anthropic prompt-caching docs. *Implementation
  assumption:* for headless Claude Code the **effective** cache scope is
  machine + working directory — the run's prompt prefix embeds the repo/working
  directory, so two runs in different directories never share a prefix and thus
  never share a cache entry, even within one org.
- **OpenAI/Codex.** Caches are **not shared between organizations**; requests are
  routed to a machine by a hash of the prompt's first ~256 tokens, so the KV
  cache is effectively **machine-local**, and `prompt_cache_key` increases routing
  stickiness (same prefix → same engine → higher hit rate). Source: OpenAI
  prompt-caching guide. Minimum cacheable prefix is 1024 tokens.

### The concurrent cold-start write race (panel fan-out)

A cache entry becomes readable only **after the first response that wrote it
begins streaming**. So when the review panel fans N reviewers out concurrently
over an identical prompt prefix, all N start before any cache exists and each pays
the full cold-write price — N redundant writes, zero reads. Source: Anthropic
prompt-caching docs (concurrent-request timing).

The savings model is a **staggered launch**: send one request, await its first
streamed token (the cache is now warm), then fire the remaining N−1, which read
the prefix at 0.1× instead of writing it at 1.0–1.25×. For a five-lens, five-member
panel the cold-start writes dominate the prefix cost, so staggering is the single
largest lever.

**Implemented (M25.2) for `review run`.** Each review round groups its reviewer
lens attempts by the resolved `(engine, model, effort)` — across the whole
panel, independent of registry name, since the model and effort are part of the
effective cache key. A Claude group with more than one attempt launches exactly
one lead attempt and holds the rest; the held attempts release the moment the
lead streams its first visible output (the cache is now warm), or immediately if
the lead finishes without producing any, or after a 60-second hold so a silent
lead can never serialize the panel. Codex groups launch unchanged — codex sets a
per-thread `prompt_cache_key` and OpenAI charges no write premium, so there is no
benefit and no cost. The stagger is built-in default behavior with no config
knob (like the lens fan-out itself); it prints one held line and one released
line to the live output, and it only reorders launches — artifact schemas and
paths, attempt ID ordering, request prompt references, and proposal semantics
match the unstaggered path (timing fields and usage values naturally differ —
that is the point). The first-visible-output signal is transport-agnostic: over ACP it is the
first non-empty agent message chunk written to `stdout.log`, over the CLI the
first non-empty stdout or stderr write. See
[`agents.md`](agents.md#review-plan-vs-run). The budget feature can model
expected spend with vs. without it.

### Rule: budgets are denominated in effective units

Because cache reads are ~0.1× and writes 1.0–1.25×, a raw-token budget would
misprice a cache-heavy run by an order of magnitude. The budget feature must cap
**effective units** — `fresh×1.0 + write×{1.25 anthropic | 1.0 openai} + read×0.1
+ output×5` — the same per-provider proxy `pactum usage` reports today.

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
  cross-run aggregate (by run / stage / agent / model) is implemented as
  `pactum usage --all` (M13.0); the per-day trend series is a deferred follow-up.
  `--all` leads with the workspace summary, sorts the `by_run` rows by captured
  total tokens descending, and takes `--top N` to cap that run list (totals and
  the other breakdowns still aggregate every run).
- **"Tokens for this task"** = fold the run's `usage.jsonl` (sum each class, count
  calls). `pactum usage [run_id]` reports the per-run total + by-stage / by-agent /
  by-attempt breakdowns; `status` carries the per-run total. Both report the cache
  **hit rate** (cache_read vs input, JSON field `cache_read_ratio`) — caching is the
  biggest cost lever in the loop.
- **Effective units (M25.1).** Every total and breakdown also reports
  `effective_units`: the provider-weighted cost proxy (fresh×1.0 +
  write×{1.25 anthropic | 1.0 openai} + read×0.1 + output×5) from *Cache economics*
  above. It is computed on demand from the ledger and is **not** persisted in
  `UsageRecord`. Unsupported providers keep their raw token totals but report
  `effective_units: 0` and increment `effective_units_unavailable_calls`.
- **Uncaptured is unknown, not zero.** A `captured=false` record is unknown usage:
  it counts as a call (and toward `uncaptured_calls`) but contributes no tokens or
  effective units, and renders as "usage not reported by the agent" rather than a
  misleading zero row. A `captured=true` record with zero tokens stays a real zero.

## Forward layers (designed-for, deferred)

These read the same records; no schema change.

- **Cost ($):** a versioned per-model, **per-token-class** price table (normal input,
  output, cache-write, cache-read, reasoning priced separately) → fill the `Cost`
  block. Prefer an agent-reported cost where trustworthy; otherwise compute. (claude's
  headless `-p` becomes cleanly $-denominated once Anthropic's separate Agent-SDK
  credit takes effect.)
- **Budget stop (designed, not yet implemented):** an earlier slice shipped a
  token-`max_tokens` stop wired to `review.budget.{mode, max_tokens}`, but that
  surface gated nothing useful in practice and was removed (M25.1) — see the note
  at the top of this document. When it returns it must cap **effective units**, not
  raw tokens (see Cache economics above), and the primitive is **post-call
  accumulation of the run's captured usage**: at the start of rounds after the
  first, when the accumulated effective units reach the configured ceiling, a
  `block` mode terminates the loop and records the cap and the spent units; a
  `warn` mode records a warning and continues. A single round may overshoot by its
  own calls because there is no pre-call estimation gate. (`max_usd` was removed
  earlier as dead; a USD ceiling can return with the cost layer.)
- **Estimation:** input is countable before a call (provider count-tokens endpoint /
  local tokenizer / `chars/4`); output is not (per-stage historical ratios from our own
  ledger + `max_tokens` ceiling); a loop total must model quadratic context growth and
  is reported as a **range**, never a point. Surface used / remaining / projected and a
  context-window gauge.

## Phased plan

Slice 4 once shipped ahead of the cost slice but was rolled back (M25.1) because
the `review.budget` surface it exposed enforced nothing useful; it returns later
denominated in effective units.

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
4. **Slice 4 — budget stop (designed; earlier token-only version rolled back in
   M25.1).** Cap **effective units** per run; accumulate captured usage + terminal
   stop; `warn`/`block`; compose with the loop. Post-call only; no dollar
   enforcement.
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
