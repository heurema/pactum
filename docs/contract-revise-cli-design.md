# Contract revise CLI — design notes

How an operator edits a contract between `accept` and `approve` when the operator
is an **LLM agent**, not a human typing. Distilled from a three-voice design
panel (codex gpt-5.5 + two opus, distinct lenses); ideas are absorbed without
attribution. This is the reference for the declarative-revise backlog item.

## The problem

`contract revise` today is `--goal` plus `--add-in-scope` / `--add-acceptance` /
`--add-validation` / … — **add-only, no remove**. That is a flag explosion and,
worse, it is structurally incomplete: when a drafter writes a bad `validation`
command there is no way to remove it via the CLI, so the operator hand-edits
`contract.json` — which changed the contract hash and **orphaned an already-run
execution attempt by SHA**, forcing a re-execute. A `--remove-X` per field would
double the surface and removal-by-exact-string-match is brittle.

## The operator is an agent — design for that

The flow is human → agent (an LLM driving the pactum CLI via a skill) → pactum.
Consequences that drive every decision below:

- **No `$EDITOR`.** The operator reads/writes JSON and reacts to machine-readable
  output; it cannot drive an interactive editor.
- **It does not faithfully round-trip a whole document.** An LLM re-serializing
  the full contract will silently re-word `goal`, reorder a list, or drop a
  field — and that becomes a *successful* write, not an error. So no design may
  depend on whole-document fidelity.
- **It reuses stale reads and retries blind.** It may act on a `show` from
  earlier in the conversation, and it retries on timeout without knowing whether
  the first call landed (the environment's own note: a timed-out attempt may have
  completed the work). So lost-update and double-apply are the default hazards.

The agent's scarce resource is not expressiveness — it is **unambiguous,
verifiable, atomic, idempotent state transitions**.

## The edit model: partial-replace

`contract revise <run> --from -|<file>` reads a **partial** contract document.
Every field *present* in the document replaces that field's value wholesale;
fields *absent* are untouched. Key-presence (not value-truthiness) is the signal.

- Whole-document replace is rejected: it forces the LLM to reproduce every field,
  making "silently dropped/reworded field" the default failure.
- JSON-Patch (`{op,path,value}`) is rejected: LLMs are unreliable at JSON-Pointer
  paths and array indices; wrong altitude for a contract with ~8 known fields.
- Partial-replace bounds the blast radius to the fields the agent *names*; an
  omitted field is provably untouched because it is never round-tripped.

Null/empty: a key **absent** = untouched; a list present as `[]` = cleared;
`null` for a non-nullable scalar (e.g. `goal`) is **rejected**, not treated as
clear (LLMs conflate "" / null / absent — make the distinction explicit).

## List semantics: replace wholesale

A present list field (`validation.commands`, `acceptance_criteria`, `scope.in`,
…) **replaces the whole list**. One rule: *the list you send is the list you
get*. This gives removal, reordering, and in-place edit for free — and removal is
exactly the gap that motivated this. Merge/append is rejected: it re-creates the
easy-add/no-remove asymmetry. Submitted order is preserved; duplicates are
**stable-deduped (keep first)** so the re-hash is reproducible; dropped
duplicates are **reported in the result** (hidden normalization changes what gets
approved).

## Concurrency is the default, not a flag

The version check is **on by default and carried inside the document**, not an
opt-in `--if-version` flag (a safety flag can be forgotten — silently losing the
protection). `show --json` returns a `version`; the agent echoes it back as
`base_version` in the `--from` document:

```jsonc
// pactum contract show <run> --json
{ "version": "sha256:a1b2…", "contract": { … } }

// the agent sends back base_version + only the changed fields
{ "base_version": "sha256:a1b2…",
  "contract": { "validation": { "commands": ["make check"] } } }
```

pactum applies the partial-replace **only if** `base_version` equals the current
version; a missing or stale `base_version` is an **atomic reject** (nothing
written, `contract_unchanged: true`). This makes "read before you write"
unskippable, turns the agent's two hazards (stale read, blind retry) into a clean
*reject → re-read → retry*, and gives exactly-once on retries. The rare deliberate
unconditional write is the opt-in escape hatch `--force`.

## `version` is the content hash

`version` is the sha256 of the normalized contract content — **the same hash the
contract already uses at `approve`**. Pre-approval it is exposed as `version`; at
approval it becomes `contract_sha256`. One mechanism, no separate revision-counter
state to persist, and **no-op idempotency falls out for free**: re-submitting the
same content yields the same version, so pactum reports `changed: false` and does
not re-hash or reset approval. (`version`, not `id`: `run_id` is the run's stable
identity; the contract content needs a token that *changes per edit* — a version.)

## Approval reset is flag-gated

Revise is legal on an `accepted` or `approved` contract. A content-changing revise
re-hashes; on an `approved` contract that resets it to `accepted` and drops the
pin — which invalidates a pinned artifact and can orphan execution lineage (the
original bug). Because that side effect is expensive and irreversible, it requires
an **explicit `--allow-approval-reset`**; without the flag, a revise that would
reset approval is rejected. Loud after-the-fact reporting is not enough for an
agent operator. A no-op revise never resets approval. Orphaned attempts remain
queryable by their old hash; the result reports `attempts_orphaned: N`.

This keeps the polarity right — **safe is the default, dangerous needs a flag:**

| operation | default | to allow |
| --- | --- | --- |
| version (stale-write) check | on | — |
| bypass the version check (unconditional write) | rejected | `--force` |
| reset an existing approval | rejected | `--allow-approval-reset` |

The common path carries no flags at all.

## Error feedback: one shot, all at once, field-addressed

Invalid input returns a single structured JSON error, non-zero exit, with **all**
problems at once (not first-fail — else the agent burns a round-trip per error):

```jsonc
{ "ok": false, "contract_unchanged": true,
  "issues": [
    { "field": "validation.commands[1]", "code": "EMPTY_REQUIRED", "message": "…", "value": "" },
    { "field": "scop", "code": "UNKNOWN_FIELD", "message": "unknown field; did you mean 'scope'?" }
  ] }
```

- **Reject unknown fields** (hard error, with a "did you mean"): silent-ignore is
  the worst failure — a typo'd key becomes a no-op the agent reads as success.
- `contract_unchanged: true` on reject tells the agent the write was atomic and it
  can retry without re-checking state.
- A valid no-op revise is exit 0 with `changed: false`.

## Drop the old flags — no deprecation

pactum has no users, so there is no compat obligation: `--add-*` and `--goal` are
**removed outright**, not deprecated. Keeping them "briefly" would also contradict
the core decision (two ways to edit one field makes the agent mix paradigms and
churn the hash) — removing them is the *completion* of that decision, not a
violation. The only in-repo consumers to update in the same change: the internal
`contract accept` path (it applies the drafter proposal through the same field
mechanism — switch it to partial-replace; on an empty skeleton, replace == the
proposal), the dogfood skill/agent instructions, and the tests.

## The CLI surface

```sh
pactum contract show <run> --json                 # → { version, contract }
pactum contract revise <run> --from -|<file>       # base_version in the document; stale ⇒ reject
                          [--force]                 # bypass the version check (rare, deliberate)
                          [--allow-approval-reset]  # permit resetting an existing approval (rare)
pactum contract approve <run>
```

`revise` returns, on success:
`{ ok, contract, base_version, new_version, changed, deduped, approval_reset?, previous_approval_hash?, attempts_orphaned? }`,
and on failure the structured `issues[]` above.

## First slice (minimal coherent cut)

1. `contract show --json` returns the editable contract plus a `version` token
   (sha256 of the normalized contract content).
2. `contract revise <run> --from -|<file>` — partial-replace, strict schema,
   unknown-field rejection, stable dedupe (report dropped), all-errors JSON.
3. The version check is on by default (`base_version` in the document); stale or
   missing ⇒ atomic reject.
4. No-op idempotence: identical content does not re-hash or reset approval.
5. Approved-contract handling: a content-changing revise is rejected unless
   `--allow-approval-reset`.
6. **Remove** `--add-*` and `--goal`; switch the internal `accept` path to
   partial-replace; update tests and the skill instructions.

Deferred: `--dry-run` with a field-level diff, richer diffs, and the `--force`
bypass (add once the core path is stable; until then a missing `base_version` is
simply rejected).

## What this gets right that is easy to get wrong

- **Do not trust the agent to round-trip JSON.** Partial-replace exists so that an
  omitted field is never re-serialized and therefore never mangled.
- **The version token is part of the feature, not an implementation detail.**
  Without it (and the contract is only hashed at approve today), partial-replace
  still lets a stale agent cleanly overwrite newer state. It ships in slice 1.
- **Surface normalization.** Dedupe/sanitize must be reported in the result —
  hidden normalization silently changes what gets approved.
