## What and why

<!-- One paragraph: what changes, and the problem or issue it closes. Link the issue. -->

## How it was verified

<!-- The commands you ran and what they showed: make test, make lint, the UI suite, a live drive against a scratch project. Evidence, not adjectives. -->

## Checklist

- [ ] Every commit is signed off (`git commit -s`); see the DCO section of CONTRIBUTING.md
- [ ] Tests added or updated for the behavior that changed
- [ ] Where behavior changed, the spec in `docs/spec/` or the docs that describe it changed in this same PR
- [ ] If `ui/src` changed, `ui/dist` was rebuilt with `make ui` and committed
- [ ] CI is green
