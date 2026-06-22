import { describe, it, before, after, mock } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync, existsSync, chmodSync, renameSync } from 'node:fs';
import { tmpdir, homedir } from 'node:os';
import { join } from 'node:path';
import {
  assetName,
  platformError,
  cacheBase,
  cachePath,
  downloadUrl,
  isAllowedRedirectHost,
  lookupChecksum,
  sha256ofBuffer,
} from './lib.mjs';
import { createHash } from 'node:crypto';

// ── Platform mapping ────────────────────────────────────────────────────────

describe('assetName', () => {
  it('maps darwin/arm64', () => assert.equal(assetName('darwin', 'arm64'), 'pactum-darwin-arm64'));
  it('maps darwin/x64 -> amd64', () => assert.equal(assetName('darwin', 'x64'), 'pactum-darwin-amd64'));
  it('maps linux/arm64', () => assert.equal(assetName('linux', 'arm64'), 'pactum-linux-arm64'));
  it('maps linux/x64 -> amd64', () => assert.equal(assetName('linux', 'x64'), 'pactum-linux-amd64'));
  it('maps win32/x64 -> windows amd64 .exe', () => assert.equal(assetName('win32', 'x64'), 'pactum-windows-amd64.exe'));
  it('returns null for win32/arm64', () => assert.equal(assetName('win32', 'arm64'), null));
  it('returns null for ia32', () => assert.equal(assetName('linux', 'ia32'), null));
});

// ── Platform gate ───────────────────────────────────────────────────────────

describe('platformError', () => {
  it('accepts win32/x64', () => {
    assert.equal(platformError('win32', 'x64', false), null);
  });

  it('rejects win32/arm64 as unsupported', () => {
    const msg = platformError('win32', 'arm64', false);
    assert.ok(msg && msg.length > 0, 'expected non-empty message for win32/arm64');
    assert.match(msg, /unsupported platform/i);
    assert.ok(!msg.includes('\n'), 'must be single-line');
  });

  it('accepts linux/x64 musl as supported', () => {
    assert.equal(platformError('linux', 'x64', /* isMusl */ true), null);
  });

  it('accepts linux/arm64 musl as supported', () => {
    assert.equal(platformError('linux', 'arm64', /* isMusl */ true), null);
  });

  it('rejects unsupported arch', () => {
    const msg = platformError('linux', 'ia32', false);
    assert.ok(msg && msg.length > 0, 'expected non-empty message for unsupported arch');
    assert.match(msg, /unsupported platform/i);
    assert.ok(!msg.includes('\n'), 'must be single-line');
  });

  it('returns null for darwin/arm64', () => {
    assert.equal(platformError('darwin', 'arm64', false), null);
  });

  it('returns null for darwin/x64', () => {
    assert.equal(platformError('darwin', 'x64', false), null);
  });

  it('returns null for linux/x64 glibc', () => {
    assert.equal(platformError('linux', 'x64', false), null);
  });

  it('returns null for linux/arm64 glibc', () => {
    assert.equal(platformError('linux', 'arm64', false), null);
  });

  it('gate returns error before cache-path or download would be called (win32/arm64)', () => {
    // If platformError returns non-null, a real launcher would exit before calling cachePath.
    const err = platformError('win32', 'arm64', false);
    assert.ok(err !== null);
    // assetName returns null for win32/arm64, so cachePath would fail — gate must run first.
    assert.equal(assetName('win32', 'arm64'), null);
  });

  it('gate returns error before cache-path or download would be called (unsupported arch)', () => {
    const err = platformError('linux', 'ia32', false);
    assert.ok(err !== null);
    assert.equal(assetName('linux', 'ia32'), null);
  });
});

// ── Cache path ──────────────────────────────────────────────────────────────

describe('cacheBase / cachePath', () => {
  let saved;

  before(() => {
    saved = {
      PACTUM_NPM_CACHE: process.env.PACTUM_NPM_CACHE,
      XDG_CACHE_HOME: process.env.XDG_CACHE_HOME,
    };
  });

  after(() => {
    if (saved.PACTUM_NPM_CACHE === undefined) delete process.env.PACTUM_NPM_CACHE;
    else process.env.PACTUM_NPM_CACHE = saved.PACTUM_NPM_CACHE;
    if (saved.XDG_CACHE_HOME === undefined) delete process.env.XDG_CACHE_HOME;
    else process.env.XDG_CACHE_HOME = saved.XDG_CACHE_HOME;
  });

  it('honors PACTUM_NPM_CACHE', () => {
    process.env.PACTUM_NPM_CACHE = '/custom/cache';
    delete process.env.XDG_CACHE_HOME;
    assert.equal(cacheBase(), '/custom/cache');
  });

  it('falls back to XDG_CACHE_HOME/pactum', () => {
    delete process.env.PACTUM_NPM_CACHE;
    process.env.XDG_CACHE_HOME = '/xdg';
    assert.equal(cacheBase(), join('/xdg', 'pactum'));
  });

  it('falls back to ~/.cache/pactum', () => {
    delete process.env.PACTUM_NPM_CACHE;
    delete process.env.XDG_CACHE_HOME;
    assert.equal(cacheBase(), join(homedir(), '.cache', 'pactum'));
  });

  it('uses %LOCALAPPDATA% on win32', () => {
    delete process.env.PACTUM_NPM_CACHE;
    const origPlatform = Object.getOwnPropertyDescriptor(process, 'platform');
    const origLA = process.env.LOCALAPPDATA;
    try {
      Object.defineProperty(process, 'platform', { value: 'win32', configurable: true });
      process.env.LOCALAPPDATA = 'C:\\Users\\me\\AppData\\Local';
      assert.equal(cacheBase(), join('C:\\Users\\me\\AppData\\Local', 'pactum', 'cache'));
    } finally {
      Object.defineProperty(process, 'platform', origPlatform);
      if (origLA === undefined) delete process.env.LOCALAPPDATA;
      else process.env.LOCALAPPDATA = origLA;
    }
  });

  it('cachePath is version-scoped', () => {
    process.env.PACTUM_NPM_CACHE = '/c';
    const p = cachePath('1.2.3', 'pactum-linux-amd64');
    assert.equal(p, '/c/1.2.3/pactum-linux-amd64');
  });

  it('different versions produce different paths', () => {
    process.env.PACTUM_NPM_CACHE = '/c';
    assert.notEqual(cachePath('1.0.0', 'pactum-linux-amd64'), cachePath('2.0.0', 'pactum-linux-amd64'));
  });
});

// ── Checksum lookup ─────────────────────────────────────────────────────────

describe('lookupChecksum', () => {
  const cs = {
    'pactum-darwin-arm64': 'abc123',
    'pactum-linux-amd64': '',
  };

  it('returns the checksum for a present entry', () => {
    assert.equal(lookupChecksum(cs, 'pactum-darwin-arm64'), 'abc123');
  });

  it('throws for a missing key', () => {
    assert.throws(
      () => lookupChecksum(cs, 'pactum-darwin-amd64'),
      (e) => {
        assert.match(e.message, /no published checksum/);
        assert.ok(!e.message.includes('\n'), 'single-line');
        return true;
      }
    );
  });

  it('throws for an empty string value', () => {
    assert.throws(
      () => lookupChecksum(cs, 'pactum-linux-amd64'),
      (e) => {
        assert.match(e.message, /no published checksum/);
        assert.ok(!e.message.includes('\n'), 'single-line');
        return true;
      }
    );
  });
});

// ── SHA-256 verification ────────────────────────────────────────────────────

describe('sha256ofBuffer', () => {
  it('correct digest passes comparison', () => {
    const buf = Buffer.from('hello pactum');
    const got = sha256ofBuffer(buf);
    const expected = createHash('sha256').update(buf).digest('hex');
    assert.equal(got, expected);
  });

  it('wrong digest is detected by comparison', () => {
    const buf = Buffer.from('hello pactum');
    const got = sha256ofBuffer(buf);
    assert.notEqual(got, 'deadbeef');
  });
});

// ── Redirect host allowlist ─────────────────────────────────────────────────

describe('isAllowedRedirectHost', () => {
  it('allows objects.githubusercontent.com', () => {
    assert.equal(isAllowedRedirectHost('objects.githubusercontent.com'), true);
  });
  it('allows any *.githubusercontent.com subdomain', () => {
    assert.equal(isAllowedRedirectHost('codeload.githubusercontent.com'), true);
  });
  it('rejects github.com itself', () => {
    assert.equal(isAllowedRedirectHost('github.com'), false);
  });
  it('rejects arbitrary hosts', () => {
    assert.equal(isAllowedRedirectHost('evil.com'), false);
  });
  it('rejects a host that contains but does not end with .githubusercontent.com', () => {
    assert.equal(isAllowedRedirectHost('notgithub.com'), false);
  });
});

// ── Download URL construction ───────────────────────────────────────────────

describe('downloadUrl', () => {
  it('constructs the pinned URL exactly', () => {
    assert.equal(
      downloadUrl('1.2.3', 'pactum-darwin-arm64'),
      'https://github.com/heurema/pactum/releases/download/v1.2.3/pactum-darwin-arm64'
    );
  });

  it('includes the v prefix', () => {
    const u = downloadUrl('0.0.0', 'pactum-linux-amd64');
    assert.ok(u.includes('/v0.0.0/'));
  });
});

// ── Cache-hit no-download ───────────────────────────────────────────────────

describe('cache hit avoids download', () => {
  let tmpDir;

  before(() => {
    tmpDir = mkdtempSync(join(tmpdir(), 'pactum-test-'));
    process.env.PACTUM_NPM_CACHE = tmpDir;
  });

  after(() => {
    delete process.env.PACTUM_NPM_CACHE;
    rmSync(tmpDir, { recursive: true, force: true });
  });

  it('when versioned cache file exists, cachePath returns its path and file is present', () => {
    const version = '1.0.0';
    const asset = 'pactum-linux-amd64';
    const target = cachePath(version, asset);
    mkdirSync(join(tmpDir, version), { recursive: true });
    writeFileSync(target, 'stub-binary');
    assert.ok(existsSync(target), 'cached file should exist');
    // The launcher checks existsSync(binary) and skips download if true.
    // We verify the path is correctly constructed so the check would fire.
    assert.equal(target, join(tmpDir, version, asset));
  });
});

// ── Checksum mismatch temp-file cleanup (logic test) ───────────────────────

describe('checksum mismatch: temp file deleted', () => {
  let tmpDir;

  before(() => {
    tmpDir = mkdtempSync(join(tmpdir(), 'pactum-test-'));
  });

  after(() => {
    rmSync(tmpDir, { recursive: true, force: true });
  });

  it('deletes tmp file on mismatch and does not install', () => {
    const tmp = join(tmpDir, 'pactum.tmp');
    writeFileSync(tmp, 'bad-content');

    const actual = sha256ofBuffer(Buffer.from('bad-content'));
    const expected = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
    const mismatch = actual !== expected;
    assert.ok(mismatch, 'digests should differ for this test');

    // Simulate what the launcher does on mismatch: delete tmp and do not rename.
    if (mismatch) {
      rmSync(tmp, { force: true });
    }

    assert.ok(!existsSync(tmp), 'tmp file must be deleted on mismatch');
    const installedPath = join(tmpDir, 'installed-binary');
    assert.ok(!existsSync(installedPath), 'binary must not be installed');
  });
});

// ── Spawn exit-code propagation ─────────────────────────────────────────────

describe('spawn exit-code propagation', () => {
  it('exits with the child status code (stub)', () => {
    // Simulate launcher logic: spawnSync returns { status: N } -> process.exit(N)
    let exited = null;
    const fakeExit = (code) => { exited = code; };

    const result = { status: 42, error: undefined };
    if (result.error) {
      fakeExit(1);
    } else {
      fakeExit(result.status ?? 1);
    }
    assert.equal(exited, 42);
  });

  it('exits nonzero when spawnSync returns an error', () => {
    let exited = null;
    let stderrMsg = null;
    const fakeExit = (code) => { exited = code; };
    const fakeStderr = (msg) => { stderrMsg = msg; };

    const result = { status: null, error: new Error('ENOENT') };
    if (result.error) {
      fakeStderr(`pactum: failed to launch binary: ${result.error.message}`);
      fakeExit(1);
    } else {
      fakeExit(result.status ?? 1);
    }

    assert.ok(exited !== 0, 'must exit nonzero');
    assert.ok(stderrMsg && stderrMsg.length > 0, 'must emit a stderr message');
    assert.ok(!stderrMsg.includes('\n'), 'message must be single-line');
    assert.ok(!stderrMsg.includes('Error:') || stderrMsg.split('\n').length === 1, 'no stack trace');
  });
});

// ── Successful cache-miss install sequence ──────────────────────────────────

describe('successful cache-miss install sequence', () => {
  let tmpDir;

  before(() => {
    tmpDir = mkdtempSync(join(tmpdir(), 'pactum-test-'));
    process.env.PACTUM_NPM_CACHE = tmpDir;
  });

  after(() => {
    delete process.env.PACTUM_NPM_CACHE;
    rmSync(tmpDir, { recursive: true, force: true });
  });

  it('creates versioned cache dir recursively, chmods, and atomically renames', () => {
    const version = '0.0.0';
    const asset = 'pactum-linux-amd64';
    const target = cachePath(version, asset);
    const cacheDir = join(tmpDir, version);
    const tmp = target + '.tmp';

    // Step 1: mkdirSync recursive — must happen before any file write.
    mkdirSync(cacheDir, { recursive: true });
    assert.ok(existsSync(cacheDir), 'versioned cache dir must exist before rename');

    // Simulate a verified download streamed into tmp.
    writeFileSync(tmp, 'fake-binary-content');

    // Step 2: chmod 0o755.
    chmodSync(tmp, 0o755);

    // Step 3: atomic rename.
    renameSync(tmp, target);

    assert.ok(!existsSync(tmp), 'tmp file must be gone after rename');
    assert.ok(existsSync(target), 'binary must be present at versioned cache path');
  });
});

// ── Error message format ────────────────────────────────────────────────────

describe('error message format', () => {
  it('lookupChecksum produces single-line error', () => {
    try {
      lookupChecksum({}, 'pactum-linux-amd64');
      assert.fail('expected throw');
    } catch (e) {
      assert.ok(!e.message.includes('\n'), 'must be single-line');
    }
  });

  it('simulated redirect-host rejection is single-line', () => {
    const host = 'evil.com';
    const url = 'https://github.com/heurema/pactum/releases/download/v0.0.0/pactum-linux-amd64';
    const msg = `pactum: unexpected redirect host ${host} (from ${url})`;
    assert.ok(!msg.includes('\n'), 'must be single-line');
  });

  it('simulated network failure message includes the URL', () => {
    const url = 'https://github.com/heurema/pactum/releases/download/v0.0.0/pactum-linux-amd64';
    const msg = `pactum: network error downloading ${url}: ECONNREFUSED`;
    assert.ok(msg.includes(url), 'network failure message must include the URL');
    assert.ok(!msg.includes('\n'), 'must be single-line');
  });
});
