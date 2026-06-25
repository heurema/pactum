# Contract Review: Scope fidelity

You are reviewing a software change contract through the **scope-fidelity** lens.

Review the contract fields below using only your assigned lens checklist.
Do not flag issues that belong to other lenses.

## Contract

**Goal**: Add pactum's own, self-contained npm distribution launcher so users can install the pactum CLI via `npm i -g @heurema/pactum` / `npx @heurema/pactum`, WITHOUT coupling to any external/forked toolchain. This slice is the distribution MECHANISM only (the launcher package + hermetic tests + docs); the GitHub-Actions release wiring that actually publishes binaries + the npm package is a SEPARATE follow-up slice and is OUT OF SCOPE here.

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

**Scope in**:
  - Create a new top-level `npm/` package for `@heurema/pactum` with a dependency-free Node ESM launcher, package metadata, checksum manifest, and hermetic node:test coverage.
  - Implement the launcher as a thin `npm/bin/pactum.mjs` entrypoint backed by testable pure helpers, including platform mapping, platform rejection, cache path resolution, checksum lookup, download verification, atomic cache install, and child process execution.
  - Add npm install documentation in `docs/install-npm.md` and link to it from the existing install documentation.

**Scope out**:
  - Do not change `.github/workflows/release.yml` or add npm publishing, checksum generation, or bare-binary upload release automation.
  - Do not publish to npm, perform live GitHub Release downloads, or add any validation that depends on a published release asset.
  - Do not add optionalDependencies, platform-specific npm packages, postinstall scripts, Homebrew, Docker, Windows support, musl/Alpine support, signed provenance, or Go source changes.

**Acceptance criteria**:
  - npm/package.json declares `@heurema/pactum`, `type: module`, `bin.pactum: bin/pactum.mjs`, placeholder version `0.0.0`, `files` limited to `["bin", "checksums.json"]`, Node engine `>=18`, `repository` and `homepage` pointing at `github.com/heurema/pactum`, no `dependencies` key (or an explicitly empty dependencies object), and no `prepare`, `preinstall`, or `postinstall` entries in `scripts`; a gate command asserts each of these fields explicitly rather than only checking JSON parse.
  - npm/checksums.json is valid JSON and defines all four asset keys (`pactum-darwin-arm64`, `pactum-darwin-amd64`, `pactum-linux-amd64`, `pactum-linux-arm64`); missing or empty checksum values cause a loud nonzero failure rather than skipped verification; a gate command asserts all four keys are present.
  - The helper module is named exactly `npm/bin/lib.mjs` and exports the pure functions imported by both `npm/bin/pactum.mjs` and `npm/bin/pactum.test.mjs`; the validation command `node --check npm/bin/lib.mjs` is the definitive filename requirement and supersedes any 'or similar' phrasing in the scope description.
  - The launcher maps only darwin/linux arm64/x64 to bare binary asset names using Go arch names (Node `x64` -> Go `amd64`), rejects Windows, musl/Alpine Linux, and unsupported platform/arch combinations before any network or cache work, and reports all expected failures as single-line messages on stderr without stack traces.
  - The launcher reads its own package version from `npm/package.json`, resolves the cache base directory as `PACTUM_NPM_CACHE` -> `$XDG_CACHE_HOME/pactum` -> `~/.cache/pactum`, and caches the binary at `<base>/<version>/<assetName>`. A file present at the versioned cache path is treated as proof of prior verified download, established by a prior successful atomic rename from a verified temp file; the launcher does NOT re-hash the cached binary on every exec and makes no network requests on a cache hit. If `spawnSync` fails for any reason (missing file, permission error, etc.), the launcher exits nonzero with a clear single-line error on stderr without a stack trace.
  - On cache miss, the launcher constructs the pinned URL `https://github.com/heurema/pactum/releases/download/v<version>/<assetName>`, follows redirects only to hosts with the suffix `.githubusercontent.com` (the GitHub release asset CDN), rejects any redirect to any other host with a clear single-line nonzero error on stderr (without a stack trace) before consuming the body, streams the download to a temp file, verifies SHA256 against the value from `checksums.json`, deletes the temp file on mismatch and exits nonzero with a single-line error on stderr (without a stack trace), and on success sets `chmod 0o755` and atomically renames the temp file into the versioned cache path; any network failure (connection refused, timeout, unexpected HTTP status) produces a single-line error on stderr (without a stack trace) that includes the attempted download URL.
  - The launcher execs the cached binary with `spawnSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' })` and exits with the child's `status` code; if `spawnSync` returns an `error` property, the launcher exits nonzero with a clear single-line message on stderr without a stack trace.
  - npm/bin/pactum.test.mjs uses `node:test` hermetically (no real network, no live downloads) and covers all of the following: (a) platform/arch -> asset-name mapping including `x64`->`amd64` translation and all four supported targets; (b) platform gate rejects `win32` (any arch), simulated musl/Alpine, and unsupported arch combinations, each nonzero with a non-empty single-line message, injected via a testable helper or env override; (c) SHA256 verification: correct digest passes, wrong digest fails; (d) cache-path construction is version-scoped and honors `PACTUM_NPM_CACHE`; (e) missing or empty checksum entry -> loud nonzero failure; (f) redirect host-allowlist predicate: a pure exported helper returns true only for hosts with suffix `.githubusercontent.com` and false for all other hosts including `github.com` itself and arbitrary URLs; (g) pinned URL construction: given a version string and asset name, the generated URL equals exactly `https://github.com/heurema/pactum/releases/download/v<version>/<assetName>`; (h) cache-hit no-download: when the versioned cache file already exists at the expected path, the download code path is not invoked; (i) checksum mismatch temp-file cleanup: a failed SHA256 verification causes the temp file to be deleted before the launcher exits; (j) spawn exit-code propagation: a stubbed `spawnSync` returning `{ status: N }` causes the process to exit with code N; (k) successful cache-miss install: after a verified download with the correct SHA256, the implementation records that chmod 0o755 was called on the temp file and that the temp file was atomically renamed into the versioned cache path; (l) platform gate ordering: when `win32`, musl/Alpine, or an unsupported arch is detected, neither the cache-path helper nor the download helper is invoked; (m) spawn error property: a stubbed `spawnSync` returning `{ status: null, error: new Error('ENOENT') }` causes the launcher to exit nonzero with a single-line message on stderr and without printing a stack trace; (n) error message format: checksum mismatch, redirect-host rejection, and network failure each produce a single-line message on stderr without a stack trace, and the network-failure message includes the attempted download URL.
  - docs/install-npm.md documents global npm install (`npm i -g @heurema/pactum`), npx usage, supported OS/arch matrix (macOS arm64/x64, Linux glibc amd64/arm64), unsupported Windows and Alpine/musl behavior with a description of the error output the user will see, cache location (`~/.cache/pactum/<version>/`) and the `PACTUM_NPM_CACHE` override, and the GitHub Release tarball as the manual alternative channel; existing install documentation links to this page without claiming release automation is implemented in this slice. Two gate commands enforce this: one asserts the file exists and contains all of the required phrases (`npm i -g @heurema/pactum`, npx usage, a Windows mention, an Alpine or musl mention as a separate match, `PACTUM_NPM_CACHE`, `~/.cache/pactum`, and the GitHub Release URL pattern); the other asserts that at least one file in `docs/` other than `install-npm.md` itself links to `install-npm.md`.

**Validation commands**:
  - node --check npm/bin/pactum.mjs
  - node --check npm/bin/lib.mjs
  - node --test npm/
  - node -e "const p=JSON.parse(require('fs').readFileSync('npm/package.json','utf8')); const a=(c,m)=>{if(!c)throw new Error(m)}; a(p.name==='@heurema/pactum','name must be @heurema/pactum'); a(p.type==='module','type must be module'); a(p.bin&&p.bin.pactum==='bin/pactum.mjs','bin.pactum must be bin/pactum.mjs'); a(p.version==='0.0.0','version must be 0.0.0'); a(Array.isArray(p.files)&&p.files.length===2&&p.files.includes('bin')&&p.files.includes('checksums.json'),'files must be exactly [bin, checksums.json]'); a(p.engines&&p.engines.node&&/^>=18/.test(p.engines.node),'engines.node must be >=18'); a(p.repository&&(typeof p.repository==='string'?p.repository:(p.repository.url||'')).includes('github.com/heurema/pactum'),'repository must point at github.com/heurema/pactum'); a(p.homepage&&p.homepage.includes('github.com/heurema/pactum'),'homepage must point at github.com/heurema/pactum'); a(!p.dependencies||!Object.keys(p.dependencies).length,'must have no runtime dependencies'); a(!p.scripts||(p.scripts.prepare==null&&p.scripts.preinstall==null&&p.scripts.postinstall==null),'must have no prepare/preinstall/postinstall scripts');"
  - node -e "const c=JSON.parse(require('fs').readFileSync('npm/checksums.json','utf8')); ['pactum-darwin-arm64','pactum-darwin-amd64','pactum-linux-amd64','pactum-linux-arm64'].forEach(k=>{if(!Object.hasOwn(c,k))throw new Error('missing checksums.json key: '+k);});"
  - make check
  - test -f docs/install-npm.md && grep -q 'npm i -g @heurema/pactum' docs/install-npm.md && grep -q 'npx @heurema/pactum' docs/install-npm.md && grep -q 'Windows' docs/install-npm.md && grep -qE 'Alpine|musl' docs/install-npm.md && grep -q 'PACTUM_NPM_CACHE' docs/install-npm.md && grep -q '\.cache/pactum' docs/install-npm.md && grep -q 'github.com/heurema/pactum/releases' docs/install-npm.md
  - grep -rl 'install-npm' docs/ | grep -qv 'install-npm.md'

**Assumptions**:
  - Node 18 or newer is available for local validation.
  - The follow-up release slice will overwrite the placeholder version and checksum values and publish the bare binary assets expected by this launcher.
  - Because JSON does not allow comments, any release-overwrite note for `checksums.json` must remain JSON-valid, for example via documentation or a JSON string field accepted by the implementation.
  - The local Go toolchain (`go`, `make`) is available for running `make check`, and the `make check` target passes on the base branch before this slice is applied; any pre-existing Go test failures are unrelated to this slice and must be resolved independently before the gate is run.

## Lens: Scope fidelity

Checklist:
- Is scope.in coherent with and proportionate to the goal?
- Is scope.out coherent and not contradictory with scope.in?
- Is the scope neither over-broad nor under-broad for the stated goal?

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
