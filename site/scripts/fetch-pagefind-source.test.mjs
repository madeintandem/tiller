import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { ensureArchive, parseCorrespondingSource } from './fetch-pagefind-source.mjs';

const bytes = Buffer.from('corresponding source bytes');
const source = {
  archive: 'pagefind-9.9.9-corresponding-source.tar.gz',
  release: 'third-party/pagefind-9.9.9-source',
  sha256: createHash('sha256').update(bytes).digest('hex'),
};

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), 'tiller-pagefind-source-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  return directory;
}

function downloader(content, calls) {
  return (requested, directory) => {
    calls.push(requested.release);
    writeFileSync(join(directory, requested.archive), content);
  };
}

test('accepts a manifest that pins the version-named archive, its release, and a sha256', () => {
  assert.deepEqual(parseCorrespondingSource({ pagefindVersion: '9.9.9', correspondingSource: source }, '9.9.9'), source);
});

test('refuses a manifest without a complete pinned corresponding source', () => {
  for (const manifest of [
    { pagefindVersion: '9.9.9' },
    { pagefindVersion: '9.9.9', correspondingSource: { ...source, sha256: 'abc' } },
    { pagefindVersion: '9.9.9', correspondingSource: { ...source, release: '' } },
    { pagefindVersion: '9.9.9', correspondingSource: { ...source, archive: 'pagefind-1.0.0-corresponding-source.tar.gz' } },
    { pagefindVersion: '1.0.0', correspondingSource: source },
  ]) {
    assert.throws(() => parseCorrespondingSource(manifest, '9.9.9'), /lacks correspondingSource/);
  }
});

test('keeps a present archive whose checksum matches without downloading', t => {
  const directory = fixture(t);
  writeFileSync(join(directory, source.archive), bytes);
  const calls = [];
  ensureArchive(directory, source, downloader(bytes, calls));
  assert.deepEqual(calls, []);
});

test('downloads a missing archive and verifies it before placing it', t => {
  const directory = fixture(t);
  const calls = [];
  const placed = ensureArchive(directory, source, downloader(bytes, calls));
  assert.deepEqual(calls, [source.release]);
  assert.equal(placed, join(directory, source.archive));
  assert.deepEqual(readFileSync(placed), bytes);
  assert.deepEqual(readdirSync(directory), [source.archive]);
});

test('replaces a stale archive whose checksum differs', t => {
  const directory = fixture(t);
  writeFileSync(join(directory, source.archive), 'stale');
  const calls = [];
  ensureArchive(directory, source, downloader(bytes, calls));
  assert.deepEqual(calls, [source.release]);
  assert.deepEqual(readFileSync(join(directory, source.archive)), bytes);
});

test('refuses a download whose checksum differs and leaves nothing behind', t => {
  const directory = fixture(t);
  assert.throws(() => ensureArchive(directory, source, downloader(Buffer.from('tampered'), [])), /does not match manifest/);
  assert.deepEqual(readdirSync(directory), []);
});

test('a failed download names the release and both remedies and leaves nothing behind', t => {
  const directory = fixture(t);
  const failing = () => { throw new Error('gh: authentication required'); };
  assert.throws(() => ensureArchive(directory, source, failing),
    error => /third-party\/pagefind-9\.9\.9-source/.test(error.message) &&
      /gh auth login/.test(error.message) && /site\/public/.test(error.message));
  assert.deepEqual(readdirSync(directory), []);
});
