#!/usr/bin/env node
import { createWriteStream, existsSync, mkdirSync, renameSync, rmSync, chmodSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { get } from 'node:https';
import { dirname, join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import {
  assetName,
  platformError,
  cachePath,
  downloadUrl,
  isAllowedRedirectHost,
  lookupChecksum,
  sha256ofFile,
} from './lib.mjs';

const PKG = JSON.parse(
  await readFile(join(dirname(fileURLToPath(import.meta.url)), '..', 'package.json'), 'utf8')
);
const version = PKG.version;

function die(msg) {
  process.stderr.write(msg + '\n');
  process.exit(1);
}

// Platform gate — before any cache or network work.
const err = platformError(process.platform, process.arch);
if (err) die(err);

const asset = assetName(process.platform, process.arch);
const binary = cachePath(version, asset);

// Cache hit: the binary was previously verified and atomically installed.
if (!existsSync(binary)) {
  const checksums = JSON.parse(
    await readFile(join(dirname(fileURLToPath(import.meta.url)), '..', 'checksums.json'), 'utf8')
  );

  let expected;
  try {
    expected = lookupChecksum(checksums, asset);
  } catch (e) {
    die(e.message);
  }

  const url = downloadUrl(version, asset);
  const cacheDir = dirname(binary);
  mkdirSync(cacheDir, { recursive: true });
  const tmp = binary + '.tmp';

  await new Promise((resolve, reject) => {
    function request(targetUrl, redirectsLeft) {
      if (redirectsLeft <= 0) {
        reject(new Error(`pactum: too many redirects downloading ${url}`));
        return;
      }
      get(targetUrl, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          const loc = new URL(res.headers.location);
          if (!isAllowedRedirectHost(loc.hostname)) {
            res.destroy();
            reject(new Error(`pactum: unexpected redirect host ${loc.hostname} (from ${url})`));
            return;
          }
          res.destroy();
          request(res.headers.location, redirectsLeft - 1);
          return;
        }
        if (res.statusCode !== 200) {
          res.destroy();
          reject(new Error(`pactum: HTTP ${res.statusCode} downloading ${url}`));
          return;
        }
        const out = createWriteStream(tmp);
        res.pipe(out);
        out.on('finish', resolve);
        out.on('error', reject);
        res.on('error', reject);
      }).on('error', (e) => reject(new Error(`pactum: network error downloading ${url}: ${e.message}`)));
    }
    request(url, 5);
  }).catch((e) => {
    try { rmSync(tmp); } catch { /* ignore */ }
    die(e.message);
  });

  const actual = await sha256ofFile(tmp).catch((e) => {
    try { rmSync(tmp); } catch { /* ignore */ }
    die(`pactum: failed to hash downloaded file: ${e.message}`);
  });

  if (actual !== expected) {
    try { rmSync(tmp); } catch { /* ignore */ }
    die(`pactum: checksum mismatch for ${asset}: expected ${expected}, got ${actual}`);
  }

  chmodSync(tmp, 0o755);
  renameSync(tmp, binary);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
  die(`pactum: failed to launch binary: ${result.error.message}`);
}
process.exit(result.status ?? 1);
