import { createHash } from 'node:crypto';
import { createReadStream, existsSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { pipeline } from 'node:stream/promises';

// Maps Node platform/arch to Go OS/arch naming used in release asset names.
const PLATFORM_MAP = {
  'darwin/arm64': 'pactum-darwin-arm64',
  'darwin/x64':   'pactum-darwin-amd64',
  'linux/arm64':  'pactum-linux-arm64',
  'linux/x64':    'pactum-linux-amd64',
};

// Returns the release asset name for the current platform, or null if unsupported.
export function assetName(platform, arch) {
  return PLATFORM_MAP[`${platform}/${arch}`] ?? null;
}

// Returns true if the current Linux process is running on musl (Alpine or similar).
export function detectMusl() {
  if (existsSync('/etc/alpine-release')) return true;
  try {
    const report = process.report?.getReport?.();
    if (report?.header) {
      const h = report.header;
      // glibc sets glibcVersionRuntime; musl leaves it absent.
      if (!h.glibcVersionRuntime && !h.glibcVersionCompiler) return true;
    }
  } catch {
    // ignore — if we can't tell, assume glibc
  }
  return false;
}

// Returns a non-null error string if the platform should be rejected before any
// work, or null if supported. Accepts overrides for testability.
export function platformError(platform, arch, isMusl) {
  if (isMusl === undefined) isMusl = platform === 'linux' ? detectMusl() : false;
  if (platform === 'win32') {
    return 'pactum: Windows binaries are not published yet; use WSL2 (Ubuntu/Debian) or build from source: https://github.com/heurema/pactum';
  }
  if (platform === 'linux' && isMusl) {
    return 'pactum: only glibc Linux binaries are published; Alpine/musl is not yet supported — use a glibc image (Ubuntu/Debian) or build from source: https://github.com/heurema/pactum';
  }
  if (!assetName(platform, arch)) {
    return `pactum: unsupported platform: ${platform}/${arch}`;
  }
  return null;
}

// Returns the cache base directory, respecting env overrides.
export function cacheBase() {
  if (process.env.PACTUM_NPM_CACHE) return process.env.PACTUM_NPM_CACHE;
  if (process.env.XDG_CACHE_HOME) return join(process.env.XDG_CACHE_HOME, 'pactum');
  return join(homedir(), '.cache', 'pactum');
}

// Returns the versioned cache file path for a given asset.
export function cachePath(version, asset) {
  return join(cacheBase(), version, asset);
}

// Returns the pinned GitHub Release download URL.
export function downloadUrl(version, asset) {
  return `https://github.com/heurema/pactum/releases/download/v${version}/${asset}`;
}

// Returns true only for GitHub release asset CDN hosts (suffix .githubusercontent.com).
export function isAllowedRedirectHost(host) {
  return host.endsWith('.githubusercontent.com');
}

// Returns the expected SHA-256 hex for an asset from the checksums object,
// or throws with a loud message if the entry is missing or empty.
export function lookupChecksum(checksums, asset) {
  if (!Object.hasOwn(checksums, asset) || !checksums[asset]) {
    throw new Error(`pactum: no published checksum for ${asset}; this build was not released`);
  }
  return checksums[asset];
}

// Computes SHA-256 of a Buffer and returns the hex digest.
export function sha256ofBuffer(buf) {
  return createHash('sha256').update(buf).digest('hex');
}

// Streams a file and returns its SHA-256 hex digest.
export async function sha256ofFile(filePath) {
  const hash = createHash('sha256');
  await pipeline(createReadStream(filePath), hash);
  return hash.digest('hex');
}
