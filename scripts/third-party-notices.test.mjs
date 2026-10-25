import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { bundledPackageRoots, goToolchain, licenseTexts, packageNotices, pagefindEmittedJavaScriptNotices } from './third-party-notices.mjs';

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), 'tiller-notices-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  return directory;
}

test('bundled closure includes scoped and nested production packages, not application modules', () => {
  const roots = bundledPackageRoots([
    '/repo/ui/src/main.tsx',
    '/repo/ui/node_modules/react/index.js',
    '/repo/ui/node_modules/react/cjs/react.production.js',
    '/repo/ui/node_modules/@scope/package/dist/index.js?commonjs-proxy',
    '\0/repo/ui/node_modules/outer/node_modules/transitive/index.js',
    '\0vite/modulepreload-polyfill.js',
  ]);
  assert.deepEqual([...roots].sort(), [
    '/repo/ui/node_modules/@scope/package',
    '/repo/ui/node_modules/outer/node_modules/transitive',
    '/repo/ui/node_modules/react',
  ]);
});

test('copies full upstream license and NOTICE text without substituting metadata', t => {
  const directory = fixture(t);
  writeFileSync(join(directory, 'package.json'), JSON.stringify({ name: 'upstream', version: '1.2.3', license: 'MIT' }));
  writeFileSync(join(directory, 'LICENSE'), 'Copyright Original Authors\nAll original license terms.\n');
  writeFileSync(join(directory, 'NOTICE'), 'Required upstream attribution.\n');
  const text = packageNotices([directory]);
  assert.match(text, /upstream 1\.2\.3/);
  assert.match(text, /Copyright Original Authors\nAll original license terms\./);
  assert.match(text, /Required upstream attribution\./);
});

test('includes separate licenses shipped in license directories', t => {
  const directory = fixture(t);
  mkdirSync(join(directory, 'LICENSE'));
  writeFileSync(join(directory, 'LICENSE/LICENSE'), 'Main license text.');
  writeFileSync(join(directory, 'LICENSE/LICENSE-vendored'), 'Vendored copyright and license text.');
  const text = licenseTexts(directory);
  assert.match(text, /Main license text\./);
  assert.match(text, /Vendored copyright and license text\./);
});

test('refuses a distribution with SPDX metadata but no actual upstream text', t => {
  const directory = fixture(t);
  writeFileSync(join(directory, 'package.json'), JSON.stringify({ name: 'missing', version: '1.0.0', license: 'MIT' }));
  assert.throws(() => packageNotices([directory]), /No upstream license text/);
});

test('versioned upstream text covers missing Pagefind UI license and every prebundled runtime', t => {
  const directory = fixture(t);
  writeFileSync(join(directory, 'package.json'), JSON.stringify({ name: '@pagefind/default-ui', version: '1.5.2', license: 'MIT' }));
  writeFileSync(join(directory, 'NOTICE'), 'Installed supplemental upstream notice.');
  const text = packageNotices([directory]);
  assert.match(text, /Copyright 2022 Pagefind/);
  assert.match(text, /svelte \(prebundled Pagefind UI runtime\) 4\.2\.20/);
  assert.match(text, /Copyright \(c\) 2016-23/);
  for (const name of ['is-alphabetical', 'is-decimal', 'is-alphanumerical', 'bcp-47']) {
    assert.ok(text.includes(`${name} (prebundled Pagefind UI runtime)`));
  }
  assert.match(text, /Copyright \(c\) 2016 Titus Wormer/);
  assert.match(text, /Permission is hereby granted/);
  assert.match(text, /Installed supplemental upstream notice\./);
});

test('does not reuse vendored upstream text for an unreviewed package version', t => {
  const directory = fixture(t);
  writeFileSync(join(directory, 'package.json'), JSON.stringify({ name: '@pagefind/default-ui', version: '1.5.3', license: 'MIT' }));
  assert.throws(() => packageNotices([directory]), /No upstream license text/);
});

test('Pagefind notices include the full linked WASM licenses and upstream copyright', t => {
  const directory = fixture(t);
  writeFileSync(join(directory, 'package.json'), JSON.stringify({ name: 'pagefind', version: '1.5.2', license: 'MIT' }));
  writeFileSync(join(directory, 'LICENSE'), 'Pagefind package license fixture.');
  const text = packageNotices([directory]);
  assert.match(text, /pagefind_microjson 0\.1\.4/);
  assert.match(text, /Robert Spencer/);
  assert.match(text, /GNU GENERAL PUBLIC LICENSE/);
  assert.match(text, /END OF TERMS AND CONDITIONS/);
  for (const name of ['wasm-bindgen', 'bit-set', 'pagefind_stem', 'minicbor', 'write-json']) {
    assert.ok(text.includes(name), `missing linked WASM dependency ${name}`);
  }
});

const emittedPagefindScripts = [
  'pagefind-component-ui.js', 'pagefind-highlight.js', 'pagefind-modular-ui.js',
  'pagefind-ui.js', 'pagefind-worker.js', 'pagefind.js',
];

test('post-Vite Pagefind assets carry full templating, highlighting, and decompression notices', t => {
  const directory = fixture(t);
  for (const file of emittedPagefindScripts) writeFileSync(join(directory, file), '// emitted asset fixture');
  const text = pagefindEmittedJavaScriptNotices(directory, '1.5.2');
  assert.match(text, /adequate-little-templates \(prebundled Pagefind component UI\) 1\.0\.2/);
  assert.match(text, /Copyright \(c\) 2026 Liam Bigelow/);
  assert.match(text, /mark\.js \(prebundled Pagefind highlighting\) 8\.11\.1/);
  assert.match(text, /Copyright \(c\) 2014–2018 Julian Kühnel/);
  assert.match(text, /Copyright \(c\) 2020 Arjun Barrett/);
  assert.match(text, /Permission is hereby granted/);
  assert.match(text, /IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE/);
  for (const name of ['svelte', 'is-alphabetical', 'is-decimal', 'is-alphanumerical', 'bcp-47']) {
    assert.ok(text.includes(name), `missing emitted UI helper ${name}`);
  }
});

test('unknown or missing emitted Pagefind assets require reviewing the complete closure', t => {
  const directory = fixture(t);
  for (const file of emittedPagefindScripts) writeFileSync(join(directory, file), '// emitted asset fixture');
  writeFileSync(join(directory, 'new-unattributed-helper.js'), '// new emitted asset');
  assert.throws(() => pagefindEmittedJavaScriptNotices(directory, '1.5.2'), /emitted JavaScript closure changed/);
  const missing = fixture(t);
  writeFileSync(join(missing, 'pagefind.js'), '// incomplete emitted closure');
  assert.throws(() => pagefindEmittedJavaScriptNotices(missing, '1.5.2'), /emitted JavaScript closure changed/);
  assert.throws(() => pagefindEmittedJavaScriptNotices(directory, '1.5.3'), /Missing versioned Pagefind JavaScript closure/);
});

test('notices run the toolchain go.mod names, not the contributor\'s local Go', () => {
  assert.equal(goToolchain('module example\n\ngo 1.27.1\n'), 'go1.27.1');
  assert.equal(goToolchain('module example\n\ngo 1.27\n'), 'go1.27.0');
  assert.equal(goToolchain('module example\n\ngo 1.27.1\n\ntoolchain go1.27.3\n'), 'go1.27.3');
  assert.throws(() => goToolchain('module example\n'), /names no go version/);
});
