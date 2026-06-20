# Installing Pactum via npm

The `@heurema/pactum` npm package provides a prebuilt binary launcher for macOS
and Linux. On first run it downloads the matching pactum binary from the project's
GitHub Release, verifies its checksum, caches it locally, and execs it.

## Quick install

```sh
npm i -g @heurema/pactum
pactum --help
```

Or without a global install:

```sh
npx @heurema/pactum --help
```

## Supported platforms

| OS    | Architecture | Notes                     |
|-------|--------------|---------------------------|
| macOS | arm64        | Apple Silicon             |
| macOS | x64          | Intel                     |
| Linux | amd64        | glibc (Ubuntu, Debian, …) |
| Linux | arm64        | glibc (Ubuntu, Debian, …) |

### Windows

Windows binaries are not published yet. The launcher exits immediately with a
clear message:

```
pactum: Windows binaries are not published yet; use WSL2 (Ubuntu/Debian) or build from source: https://github.com/heurema/pactum
```

Use WSL2 with an Ubuntu or Debian image, or [build from source](install.md).

### Alpine / musl Linux

Only glibc Linux binaries are shipped. On Alpine or other musl-based
distributions the launcher exits with:

```
pactum: only glibc Linux binaries are published; Alpine/musl is not yet supported — use a glibc image (Ubuntu/Debian) or build from source: https://github.com/heurema/pactum
```

Use a glibc image (Ubuntu/Debian) or [build from source](install.md).

## Binary cache

The downloaded binary is cached at:

```
~/.cache/pactum/<version>/<asset-name>
```

Override the cache root with the `PACTUM_NPM_CACHE` environment variable:

```sh
PACTUM_NPM_CACHE=/opt/pactum-cache npx @heurema/pactum --help
```

The binary is downloaded and checksum-verified once per version; subsequent
invocations use the cached copy without any network requests.

## Alternative: GitHub Release tarball

The GitHub Release at `https://github.com/heurema/pactum/releases` also
provides bare binaries for each platform. Download the asset matching your
platform, verify its SHA-256 against the release's checksum manifest, mark it
executable, and place it on your `PATH`:

```sh
# Example for Linux amd64:
curl -fLO https://github.com/heurema/pactum/releases/download/v<version>/pactum-linux-amd64
sha256sum -c <(echo "<sha256>  pactum-linux-amd64")
chmod 755 pactum-linux-amd64
mv pactum-linux-amd64 ~/.local/bin/pactum
```

For building from source, see [install.md](install.md).
