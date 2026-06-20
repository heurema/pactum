# @heurema/pactum

Install the [pactum](https://github.com/heurema/pactum) CLI — a contract-first
coding-agent orchestrator — via npm.

```sh
npm i -g @heurema/pactum    # then: pactum --help
# or, without installing:
npx @heurema/pactum --help
```

On first run the launcher downloads the prebuilt `pactum` binary for your
platform from the matching GitHub Release, verifies its SHA-256 against the
checksums baked into this package, caches it under `~/.cache/pactum/<version>/`,
and execs it. No build toolchain required; subsequent runs use the cached binary.

## Supported platforms

| OS | arch |
| --- | --- |
| macOS | arm64, x64 |
| Linux (glibc) | x64, arm64 |

Windows and Alpine/musl Linux are not supported yet — the launcher exits early
with a clear message rather than failing cryptically. Set `PACTUM_NPM_CACHE` to
override the cache location.

See the [repository](https://github.com/heurema/pactum) for full documentation,
including the contract-first workflow and the agent skill.
