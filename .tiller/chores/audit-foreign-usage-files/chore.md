# Audit managed projects for usage files captured from other projects

Imported from [GitHub issue 15](https://github.com/madeintandem/tiller/issues/15).

## What

Check each managed project that has usage capture turned on for committed `.tiller/usage/` files that belong to another project's sessions, and remove the foreign ones by hand in an ordinary commit.

A project needs the check when either condition held before it ran v0.24.0:

- It has a sibling path that collides once separators and dots are flattened, such as `/p/app.foo`, `/p/app-foo`, and `/p/app/foo`. Harness log folders flatten the path, and capture admitted every session in the shared folder.
- It sits below its repository root, such as `/repo/apps/a`. Capture treated the repository root as a capture root and took in sibling projects.

For each such project, compare every usage file's session against the project's own sessions, by the working directory the session's log records.

## Why

v0.24.0 stops new capture from crossing those boundaries, and it deliberately deletes and rewrites nothing already captured. A project that met either condition can still hold committed files for sessions that were never its own. What is exposed is metadata: session ids, times, models, and token counts. Transcripts are never captured.

## Done when

- Every managed project with `usage.capture: true` is listed with one of three results: neither condition applies, checked and clean, or checked with the foreign files removed in a named commit.
- A decision is recorded on whether the docs gain a short note telling other users how to run the same check.
