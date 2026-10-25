import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, renameSync, rmSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

// The Pagefind corresponding-source archive is about 30MB and changes with every
// Pagefind bump, so it is a GitHub Release asset, not a tracked file. Each site
// build places it in public/ (gitignored), and Pages serves it beside the
// GPL-covered WASM. Bytes that do not match the SHA-256 pinned in the versioned
// license manifest are refused.

function sha256(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex');
}

export function parseCorrespondingSource(manifest, version) {
  const archive = `pagefind-${version}-corresponding-source.tar.gz`;
  const source = manifest.correspondingSource;
  if (manifest.pagefindVersion !== version || !source || source.archive !== archive ||
      typeof source.release !== 'string' || !source.release ||
      !/^[0-9a-f]{64}$/.test(source.sha256 ?? '')) {
    throw new Error(`third_party/pagefind-wasm-${version}/manifest.json lacks correspondingSource ` +
      `{ archive: "${archive}", release, sha256 }`);
  }
  return source;
}

export function ensureArchive(directory, source, download) {
  const destination = join(directory, source.archive);
  if (existsSync(destination) && sha256(destination) === source.sha256) return destination;
  // Stage beside the destination so the final rename never crosses devices.
  const staging = mkdtempSync(join(directory, '.pagefind-source-'));
  try {
    try {
      download(source, staging);
    } catch (error) {
      throw new Error(`Could not download ${source.archive} from release ${source.release} ` +
        `(${error.message}). Run \`gh auth login\`, or place the verified archive at site/public/${source.archive}.`);
    }
    const fetched = join(staging, source.archive);
    const actual = existsSync(fetched) ? sha256(fetched) : 'nothing';
    if (actual !== source.sha256) {
      throw new Error(`${source.archive} from release ${source.release} has sha256 ${actual}, ` +
        `which does not match manifest ${source.sha256}`);
    }
    renameSync(fetched, destination);
  } finally {
    rmSync(staging, { recursive: true, force: true });
  }
  return destination;
}

export function downloadRelease(source, directory) {
  execFileSync('gh', ['release', 'download', source.release, '--repo', 'madeintandem/tiller',
    '--pattern', source.archive, '--dir', directory], { stdio: ['ignore', 'inherit', 'pipe'] });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const { version } = JSON.parse(readFileSync(join(root, 'site/node_modules/pagefind/package.json'), 'utf8'));
  const manifest = JSON.parse(readFileSync(join(root, 'third_party', `pagefind-wasm-${version}`, 'manifest.json'), 'utf8'));
  const placed = ensureArchive(join(root, 'site/public'), parseCorrespondingSource(manifest, version), downloadRelease);
  console.log(`fetch-pagefind-source: ${placed} verified`);
}
