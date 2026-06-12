# Agent tool-call trace — design notes

Survey-driven design input for recording the tool-call activity of spawned
agents. Sources: the Agent Client Protocol specification and the Go SDK pactum
pins, the source of the two ACP adapters pactum launches, the first-party
telemetry layers of both agent CLIs, and the published practice of harness
trajectory recorders. Ideas are absorbed without attribution; this document is
the reference for the tool-trace backlog item.

## The problem

Each attempt records the agent's streamed *text* (`stdout.log`) plus
exit/usage metadata; tool calls — file reads and writes, shell commands,
searches — are invisible. Three costs:

- **Live blindness.** A tool-heavy run looks frozen for minutes while the
  agent healthily works (the watchdog ticks, the operator sees nothing).
- **No behavioral evidence.** Questions like "did the executor read the whole
  file or a range" or "do agents use the repo's own CLI" are unanswerable —
  measured narration in execute logs is a ~6% proxy at best.
- **Audit gap.** The run record shows *what changed* (gate diff) but not *what
  the agent did* to get there.

## What the survey established

### 1. The data already arrives — pactum discards it

ACP `session/update` notifications with `tool_call` / `tool_call_update`
carry `toolCallId`, `title`, `kind`, `status`, `locations`, `rawInput`, and
`rawOutput`. The pinned Go SDK deserializes all of them, and
`acpClient.SessionUpdate` already receives every notification — it ticks the
idle watchdog and writes only message text, discarding the tool payload. No
new process, protocol, or agent flag is needed: the smallest possible tap is
a write inside the handler pactum already has.

### 2. Both adapters populate the optional fields fully

`rawInput`/`rawOutput` are optional in the spec — sufficiency is adapter
behavior, not a protocol guarantee. Verified for both adapters pactum
launches:

- **claude adapter**: `rawInput` is the full tool input (the literal shell
  command line for execute calls; exact `offset`/`limit` for ranged reads —
  the "ranged read adoption" question becomes directly measurable);
  `rawOutput` is the untruncated tool result; file tools set
  `locations [{path, line}]`.
- **codex adapter**: exec begin/end events map to tool calls carrying the
  full `argv` and `cwd`, classified kinds (read/search/execute), and a
  terminal update with `exit_code`, stdout/stderr, and duration; patch
  events map to `edit` with per-file locations.

### 3. Lifecycle and taxonomy are ready-made

Status lifecycle `pending → in_progress → completed|failed` keyed by
`toolCallId` gives per-call duration and success from transitions alone. The
`kind` taxonomy (ten values: read, edit, delete, move, search, execute,
think, fetch, switch_mode, other) is an open string — record verbatim, never
validate against a closed set.

### 4. Honest limits

- **Coverage is per-event-type and incomplete**: adapter mappings have had
  whole categories missing until explicitly fixed. ACP-derived tool counts
  are *lower bounds*; the trace must say so.
- **Never re-emit accumulated output**: a prior adapter bug re-sent the
  growing output buffer per chunk — O(N²) memory, crashed on large outputs.
  Record deltas or record once at completion; bound what is persisted.

### 5. The redaction model worth copying

The strongest first-party policy: per-tool-call events carry **size and
duration metadata by default, content only on explicit opt-in**, with
individual values truncated (~512 chars) and the whole payload bounded
(~4K). Command lines and tool outputs are where secrets live; sizes are
where the analytics live.

### 6. First-party telemetry is a cross-check, not the tap

The claude CLI's OpenTelemetry exporter (env-enabled, per-tool events with
sizes/duration) works under the adapter but duplicates what ACP already
delivers and requires an OTel pipeline; the codex `--json` event stream is a
parallel JSONL channel pactum does not need. Both remain useful for
verifying trace completeness, not as primary sources.

## Trace design

**Artifact**: `tool-trace.jsonl` beside each attempt's `stdout.log`, written
from the existing `SessionUpdate` handler. Ignored by the workspace
`.gitignore` exactly like `*.log` transcripts — the trace is a local
diagnostic, not part of the committed durable record.

**Per-line fields** (one line per `tool_call` and per `tool_call_update`):

- `ts` — RFC3339 client-arrival time (ACP carries no timestamps).
- `event` — `call` | `update`.
- `tool_call_id`, `kind` (verbatim open string), `title`, `status`.
- `locations` — `[{path, line}]` when present.
- `input_bytes` / `output_bytes` — always recorded (serialized sizes).
- `duration_ms`, `exit_code` — on the terminal update, derived by pairing on
  `tool_call_id`; calls left dangling at turn end close as `abandoned`.
- `raw_input` / `raw_output` — **off by default**; an opt-in env/flag
  records them with per-value truncation and a bounded total per line,
  following the redaction model above.

**Live feed**: one line per call to the live output (`kind title` on start,
status + duration on completion) — this closes the long-standing
"tool-heavy run looks frozen" gap as a side effect.

**Consumers**: the navigation-arc adoption question ("do agents do ranged
reads after outline/skeletons land") becomes a one-liner over
`tool-trace.jsonl`; the isolation arc gains an audit trail of every command
an attempt ran.

## Slicing

1. **Trace + live feed** (one slice): record fields above from
   `SessionUpdate`, emit the live one-liners, gitignore the artifact, tests
   over a scripted ACP stream (call/update pairing, dangling-call closure,
   sizes-not-content default, the never-rebuffer rule).
2. **Opt-in content capture** (small follow-up): the redacted
   `raw_input`/`raw_output` mode for deep debugging.
3. **Adoption metrics** (rides the navigation arc): a small report over
   traces — ranged-read share, repo-CLI usage, per-kind counts — to test
   navigation-arc hypotheses with data instead of narration grep.
