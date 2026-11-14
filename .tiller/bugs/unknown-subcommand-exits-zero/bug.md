# An unknown word under a command group prints help and exits 0

Imported from [GitHub issue 17](https://github.com/madeintandem/tiller/issues/17).

## Reproduction

Observed with the v0.24.0 release binary on 2026-11-14, in a fresh project (`git init`, then `tiller init --name Acme --key acme --kind greenfield`).

Run any of these and read the exit code:

```
tiller add bogus --json
tiller add bogus
tiller claim bogus X --json
tiller completion bogus --json
```

## Expected

An unrecognized subcommand under a command group is misuse. `docs/spec/cli-surface.md`, "Output and exit codes", says every `--json` output, success or failure, is the same envelope, and that misuse exits `2` with an `error` block. An unknown top-level command already behaves this way: `tiller bogus --json` prints one error envelope with reason `unknown_command` and exits `2`.

## Actual

Each command above prints the group's help page on stdout, prints nothing on stderr, and exits `0`, with or without `--json`. Cobra raises no error for an unknown word under a group that has no run function, so the parser-stage rendering added in v0.24.0 never runs. A script or an agent loop reads the result as success, and under `--json` it receives a help page where the contract promises an envelope.

## Fixed when

- `tiller add bogus`, `tiller claim bogus X`, and `tiller completion bogus` exit `2`, print one `tiller: <message>` line on stderr, and print nothing on stdout.
- The same commands with `--json` print exactly one error envelope on stdout and exit `2`.
- `docs/spec/cli-surface.md`, "Output and exit codes", names the reason used (`unknown_command` or `bad_args`).
- A group invoked with no subcommand, and `--help`, still print help and exit `0`.
