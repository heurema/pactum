# Task

Add pactum's own, self-contained npm distribution launcher so users can install the pactum CLI via `npm i -g @heurema/pactum` / `npx @heurema/pactum`, WITHOUT coupling to any external/forked toolchain. This slice is the distribution MECHANISM only (the launcher package + hermetic tests + docs); the GitHub-Actions release wiring that actually publishes binaries + the npm package is a SEPARATE follow-up slice and is OUT OF SCOPE here.

Design (decided by a Claude+Codex+Gemini council): a SINGLE npm package whose `bin` is a thin Node launcher that, on first run, lazily downloads the matching prebuilt pactum binary from the project's existing GitHub Release, verifies it against a checksum manifest BAKED INTO the npm package (so the npm registry — not the GitHub release — is the root of trust), caches it per-version, and execs it. No `optionalDependencies`, no platform sub-packages, no postinstall script (lazy first-run download survives `--ignore-scripts` and is immune to npm's optional-deps lockfile-pruning bug).

In scope — create a new top-level `npm/` package directory in the pactum repo:

1. `npm/package.json`:
   - `name`: `@heurema/pactum`, `type`: `module`, `bin`: `{ "pactum": "bin/pactum.mjs" }`.
   - `version`: a placeholder (e.g. `0.0.0`) — the real version is stamped from the release tag by the future release job; do not hardcode a real version.
   - `files`: `["bin", "checksums.json"]`. `engines.node`: a sane floor (e.g. `>=18`).
   - NO runtime dependencies (pure Node stdlib only: `node:os`, `node:fs`, `node:path`, `node:https`, `node:crypto`, `node:child_process`, `node:zlib` if needed). `description`, `license`, `repository`, `homepage` pointing at github.com/heurema/pactum.

2. `npm/bin/pactum.mjs` — the launcher (Node ESM, `#!/usr/bin/env node`):
   - PLATFORM MAP: map `process.platform`/`process.arch` to the release asset name using GO naming for the arch (Node `x64` -> Go `amd64`, Node `arm64` -> `arm64`). Supported targets ONLY: `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`. Asset name pattern: `pactum-<goos>-<goarch>` (a BARE binary, not the .tar.gz — so the launcher never needs a cross-platform archive extractor).
   - PLATFORM GATE (run BEFORE any network or cache work; fail loud, early, nonzero, actionable):
     * `win32` (any arch): print that Windows binaries are not published yet and exit nonzero (do NOT fall through to a cryptic ENOENT). (Windows is a deliberate future target.)
     * Linux on musl/Alpine: detect (e.g. existence of `/etc/alpine-release`, or `process.report.getReport().header.glibcVersion` being absent) and print that pactum currently ships glibc-only Linux binaries — use a glibc image (Ubuntu/Debian) — then exit nonzero.
     * Any other unsupported platform/arch: a clear "unsupported platform: <platform>/<arch>" error, nonzero.
   - CACHE: resolve a per-version cache path. Base dir = `process.env.PACTUM_NPM_CACHE` || (`$XDG_CACHE_HOME`/pactum || `~/.cache/pactum`); cached binary at `<base>/<version>/<assetName>` (read `version` from the package's own package.json so cache is version-scoped). If a verified binary already exists there, use it (NEVER re-download).
   - DOWNLOAD (only when cache miss): fetch the bare binary from a PINNED, versioned URL: `https://github.com/heurema/pactum/releases/download/v<version>/<assetName>`. Follow GitHub's release-asset redirect to its asset CDN (`*.githubusercontent.com`) but REJECT redirects to any other host. Stream to a temp file, then verify sha256 against the expected value from the baked `checksums.json`; on mismatch, delete the temp file and fail loud (do not exec an unverified binary). On success, `chmod 0o755` and ATOMICALLY rename into the cache path.
   - CHECKSUM SOURCE: read expected sha256 from `npm/checksums.json` shipped inside the package (keyed by asset name, e.g. `{ "pactum-darwin-arm64": "<sha256>", ... }`). For THIS slice (no release wiring yet) create a `npm/checksums.json` with the four asset keys and placeholder/empty values plus a clear comment-doc that the release job overwrites it; the launcher must handle a missing/empty checksum by failing loud ("no published checksum for <asset>; this build was not released") rather than skipping verification.
   - EXEC: `spawnSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' })`; exit with the child's status (or a clear error if spawn fails).
   - Errors must be human-readable single-line messages on stderr with a nonzero exit; no stack-trace dumps for expected failure modes (unsupported platform, checksum mismatch, network failure with the URL shown).

3. `npm/bin/pactum.test.mjs` (node:test, hermetic — NO network, NO real downloads):
   - platform/arch -> asset-name mapping incl. the `x64`->`amd64` translation and all four supported targets.
   - the gate rejects `win32` and a simulated musl/Alpine and an unsupported arch, each nonzero with a message (inject the detection via a testable helper / env override rather than mutating the real OS).
   - sha256 verification: a known buffer verifies against its correct digest and is rejected for a wrong digest.
   - cache-path construction is version-scoped and honors `PACTUM_NPM_CACHE`.
   - missing/empty checksum entry -> loud failure (no silent skip).
   Structure the launcher so these are unit-testable (export pure helpers from a `npm/bin/lib.mjs` or similar that both `pactum.mjs` and the test import), keeping `pactum.mjs` as a thin entry.

4. Docs: add a concise `docs/install-npm.md` (and a pointer from the existing install docs): the one-command path `npm i -g @heurema/pactum` (then `pactum ...`) and `npx @heurema/pactum ...`; the supported matrix (macOS arm64/x64, Linux amd64/arm64 glibc); explicitly note Windows and Alpine/musl are NOT yet supported and how it fails; note the binary is cached under `~/.cache/pactum/<version>/` and the `PACTUM_NPM_CACHE` override; note the GitHub-Release tarball remains the manual/alternative channel. English only; no references to the codex-acp fork or any external project.

Out of scope (do NOT do here): the release.yml changes that publish bare-binary assets + generate/​bake the real `checksums.json` + the `publish-npm` job (separate slice; this slice only DEFINES the asset-name + URL + checksums.json format the release job must satisfy); `optionalDependencies`/platform sub-packages (the future "variant A" upgrade); Windows or musl/Alpine binaries; signed provenance/SLSA; Homebrew; changing the existing `.tar.gz` GitHub-Release packaging; any Go source changes (pactum's binary, including its go:embedded skill, is unchanged).

Tests / validation (all must pass in the gate; node is available): `node --check npm/bin/pactum.mjs`; `node --test npm/` (the hermetic launcher tests); a JSON-validity check of `npm/package.json` and `npm/checksums.json` (e.g. `node -e "JSON.parse(require('fs').readFileSync('npm/package.json'))"`); `make check` (the Go suite must remain green — no Go files change). Note for the contract: the launcher's REAL end-to-end download is intentionally NOT gate-tested (it requires a published release); it is verified by a later live smoke-test, and the gate covers the logic hermetically.

Generated: 2026-06-20T13:37:22Z
