# A failed --design import leaves a half-built scaffold that a rerun cannot finish

## Reproduction

Run with the v1.0.1 release binary on 2026-11-15, in an empty directory after `git init`, with a
design folder that lists normally but holds one file that cannot be read:

```
mkdir spec && echo "# a" > spec/a.md && echo "# b" > spec/b.md && chmod 000 spec/b.md
mkdir proj && cd proj && git init -q
tiller init --key demo --kind greenfield --name Demo --design ../spec
```

It was first seen on macOS with a design folder under an iCloud-synced Desktop. The three files
there were placeholders whose contents were not on disk (`ls -lO` shows the `dataless` flag), and
the read failed with `resource deadlock avoided`. The `chmod 000` file above reaches the same code
path without iCloud.

## Expected

`docs/spec/cli-surface.md`, in the paragraph on `--design <dir>`, says missing, non-directory, and
empty sources "refuse before scaffold writes", and that an init rerun over an existing manifest
"imports nothing". Together these mean the first scaffold is the only chance to import, so an init
that cannot complete its import should leave the directory as it found it, and the developer
should be able to correct the source and run the same command again. A source file that cannot be
read is a problem with the input, and the refusal should say so in the terms the other `--design`
refusals use (`invalid_design`), naming the file and what to do.

## Actual

Init exits `2` and prints only the raw operating system error:

```
tiller: open /.../spec/b.md: permission denied
```

On the iCloud folder the line was
`tiller: read /Users/.../headway-spec/headway-v1-spec.md: resource deadlock avoided`, which gives
no hint that the file has not been downloaded. With `--json` the envelope carries
`"reason": "error"` rather than a named reason.

By then init has already written state:

- `.tiller/tiller.yml` exists, and its `inputs.imported_from` records the source path and time as
  if the import had happened.
- The skeleton directories exist.
- `.tiller/design/` holds whatever was copied before the failure (`a.md` in the reproduction,
  nothing in the iCloud case).
- `.tiller/orientation.md` is not written.

Fixing the source and running the same command again exits `0` and prints "Design import skipped:
.tiller/ was already initialized, and --design imports only on the init that scaffolds. Copy the
docs under .tiller/design/ by hand." The project is left with a manifest that claims an import, a
partial or empty `design/`, and no command that completes it. The only way forward is to delete
`.tiller/` and start over, which nothing tells the developer.

In `internal/cli/init.go`, `scaffold` writes the manifest and the skeleton before it calls
`importDocs`, and `resolveDesignSource` checks only that the source is a non-empty directory that
does not overlap `.tiller/design/`. It does not read the files.

## Fixed when

- In the reproduction above, init exits non-zero and the project directory contains no `.tiller/`
  afterwards, so `git status` shows nothing new.
- After `chmod 644 ../spec/b.md`, the same init command succeeds, reports two imported design
  files, and `.tiller/design/` holds both.
- The refusal names the unreadable file, carries the reason `invalid_design` in the `--json`
  envelope, and says what to do. For a file whose contents are not on local disk it tells the
  developer to download the file first.
- A regression test covers an unreadable file in the design source and asserts that no manifest,
  skeleton, or partial copy remains.
- `docs/spec/cli-surface.md` states that an unreadable source file refuses before scaffold writes,
  alongside the missing, non-directory, and empty cases.
