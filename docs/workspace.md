# Workspace layout

Pactum stores everything for a repository under a single workspace directory:

```
.heurema/pactum/
```

`pactum init` creates it. The workspace holds the project map, the per-run
artifacts, the accepted project memory, the event ledger, and scratch space.

## Layout

```
.heurema/pactum/
├── manifest.json            # workspace manifest (schema, tool version, map run)
├── config.yaml              # workspace configuration
├── .gitignore               # ignores the generated areas below
├── map/                     # generated project map + search index
│   ├── repo-map.md
│   ├── llms.txt
│   ├── files.jsonl
│   ├── code-items.jsonl
│   ├── hashes.jsonl
│   ├── search.sqlite
│   └── manifest.json
├── runs/                    # one directory per run
│   └── <run_id>/
│       ├── run.json
│       ├── task.md
│       ├── context/         # repo + memory context for the run
│       ├── clarify/         # clarification questions/answers/decisions
│       ├── contract/        # contract, prompt, prompt manifest, approval
│       ├── gate/            # gate report + validation command output
│       ├── execute/         # captured agent execution attempts
│       ├── review/          # manual review + reviewer attempts/proposals
│       └── ledger/          # per-run event ledger
├── memory/                  # accepted project memory
│   ├── items.jsonl
│   ├── project-memory.md
│   └── refreshes.jsonl
├── ledger/                  # workspace event + usage ledger
├── cache/                   # scratch cache
└── tmp/                     # scratch temp
```

## Generated vs durable artifacts

The workspace mixes two kinds of artifacts.

**Generated artifacts** are reproducible from your repository and configuration,
or are large/scratch output. They can be deleted and rebuilt and are not worth
tracking in version control. The `.gitignore` that `pactum init` writes inside
the workspace ignores exactly these areas:

```
locks/
map/
ledger/
cache/
tmp/
runs/*/ledger/
runs/*/execute/
runs/*/review/
```

- `map/` — rebuilt at any time with `pactum map refresh` (includes the binary
  `search.sqlite` index).
- `ledger/` and `runs/*/ledger/` — append-only event logs.
- `cache/`, `tmp/`, `locks/` — scratch and coordination state.
- `runs/*/execute/` — captured agent stdout/stderr and attempt records, which
  can be large and machine-specific.
- `runs/*/review/` — reviewer attempts and review working files.

**Durable artifacts** are the contract-first record of a run and the accepted
project memory. These are small, deterministic, and meaningful to a human:

- `manifest.json` and `config.yaml`.
- Per run: `run.json`, `task.md`, and the `context/`, `clarify/`, `contract/`,
  `gate/`, and `memory/` directories.
- Project memory: `memory/items.jsonl`, `memory/project-memory.md`, and
  `memory/refreshes.jsonl`.

## Why not to blindly commit `.heurema/`

Do not run `git add .heurema/` from the repository root expecting the whole tree
to be safe to commit. The workspace deliberately contains generated, binary, and
scratch content (the SQLite search index, caches, agent logs) that is large,
path- or machine-specific, regenerable, and noisy in diffs.

The workspace ships its own `.gitignore` (the list above), but that file only
applies to paths **inside** `.heurema/pactum/` — it does not stop you from adding
the generated areas if you force them, and it does not decide for you whether the
workspace should be tracked at all.

You have two reasonable choices:

- **Track nothing.** Add `.heurema/` to your repository's root `.gitignore` and
  treat the workspace as local-only state.
- **Track the durable record.** Leave the workspace `.gitignore` in place and
  commit the durable artifacts listed above when your team wants traceability —
  the approved contract, clarifications, gate report, review outcome, and
  accepted project memory for a run are a useful audit trail. Commit
  deliberately and review what you are adding; let the workspace `.gitignore`
  keep the generated areas out.
