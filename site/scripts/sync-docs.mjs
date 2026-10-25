// sync-docs.mjs copies the repo's own markdown into the site's content
// collection, so the docs site renders the same files the repo carries
// instead of a second copy that could drift. It runs before `astro dev` and
// `astro build` (see package.json). Output is gitignored.
//
// For each source it: takes the first `# ` line as the page title (and drops
// it, Starlight renders the title), adds frontmatter (title, editUrl back to
// the source on GitHub, sidebar order for the specs), and rewrites links:
// sibling `./x.md` links among the specs become `../x/`, and any other
// relative `.md` or path link becomes a GitHub URL, since those files are not
// on the site.
import { mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '..', '..');
const out = resolve(here, '..', 'src', 'content', 'docs');
const GH = 'https://github.com/madeintandem/tiller';

const specOrder = ['artifact-schema', 'cli-surface', 'skill-set', 'ui-surface', 'distribution'];
const specLabel = {
  'artifact-schema': 'Artifact schema',
  'cli-surface': 'CLI surface',
  'skill-set': 'Skill set',
  'ui-surface': 'Console (UI surface)',
  distribution: 'Distribution and adoption',
};

function titleAndBody(md) {
  const lines = md.split('\n');
  const i = lines.findIndex((l) => /^# /.test(l));
  if (i < 0) return { title: null, body: md };
  const title = lines[i].replace(/^# /, '').trim();
  lines.splice(i, 1);
  return { title, body: lines.join('\n').replace(/^\s*\n/, '') };
}

function rewriteLinks(body, { srcRepoPath, siblingSpecs }) {
  const srcDir = dirname(srcRepoPath);
  return body.replace(/\]\(([^)\s]+)\)/g, (m, target) => {
    if (/^(https?:|mailto:|#)/.test(target)) return m;
    const [path, hash = ''] = target.split('#');
    const anchor = hash ? `#${hash}` : '';
    // ./x.md among the specs: a sibling page on the site.
    const sib = path.match(/^\.\/([a-z0-9-]+)\.md$/);
    if (sib && siblingSpecs && siblingSpecs.includes(sib[1])) return `](../${sib[1]}/${anchor})`;
    // Anything else relative: point at the file on GitHub.
    const repoRel = resolve('/', srcDir, path).slice(1);
    return `](${GH}/blob/main/${repoRel}${anchor})`;
  });
}

function emit(target, { title, body, extraFrontmatter = [] }) {
  const fm = ['---', `title: ${JSON.stringify(title)}`, ...extraFrontmatter, '---', ''].join('\n');
  mkdirSync(dirname(target), { recursive: true });
  writeFileSync(target, fm + body);
}

// Specs.
const specDir = join(repo, 'docs', 'spec');
rmSync(join(out, 'spec'), { recursive: true, force: true });
const specs = readdirSync(specDir).filter((f) => f.endsWith('.md')).map((f) => f.replace(/\.md$/, ''));
for (const name of specs) {
  const srcRepoPath = `docs/spec/${name}.md`;
  const { title, body } = titleAndBody(readFileSync(join(repo, srcRepoPath), 'utf8'));
  const order = specOrder.indexOf(name);
  emit(join(out, 'spec', `${name}.md`), {
    title: title ?? name,
    body: rewriteLinks(body, { srcRepoPath, siblingSpecs: specs }),
    extraFrontmatter: [
      `editUrl: ${GH}/edit/main/${srcRepoPath}`,
      `sidebar: { label: ${JSON.stringify(specLabel[name] ?? title ?? name)}, order: ${order < 0 ? 99 : order + 1} }`,
    ],
  });
}

// Single pages lifted from the repo root and docs/.
const singles = [
  { src: 'CONTRIBUTING.md', dest: 'contributing.md', title: 'Contributing' },
  { src: 'docs/releasing.md', dest: 'releasing.md' },
  { src: 'docs/tiller-commands.md', dest: 'cli.md', title: 'CLI reference' },
];
for (const s of singles) {
  const { title, body } = titleAndBody(readFileSync(join(repo, s.src), 'utf8'));
  emit(join(out, s.dest), {
    title: s.title ?? title ?? s.dest,
    body: rewriteLinks(body, { srcRepoPath: s.src }),
    extraFrontmatter: [`editUrl: ${GH}/edit/main/${s.src}`],
  });
}
console.log(`sync-docs: ${specs.length} specs + ${singles.length} pages -> ${out}`);
