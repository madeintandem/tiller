# tiller docs site

Astro + Starlight. See [AGENTS.md](./AGENTS.md) for what is generated and what
is authored here.

```sh
npm ci
npm run dev       # http://localhost:4321/tiller/
npm run build     # dist/
```

`npm run build` first fetches the Pagefind corresponding-source archive with the GitHub CLI (`gh`, authenticated) unless the verified archive is already in `public/`. `npm run dev` does not need it.
