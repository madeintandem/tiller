# Console test races the file page's scroll to its line

## What

Make the test "The file page while the page rereads > keeps the file, does not scroll to its line again, then updates it in place" in `ui/src/components/__tests__/reread-report-file.test.tsx` deterministic. At line 118 it reads `scrolls.mock.calls.length` as soon as `findByText("The line itself")` resolves and asserts at line 119 that the count is above zero. The text can render before the effect that scrolls to `#L3` has run, so the count can still be zero. The test should wait for the first scroll (for example with `waitFor`) before taking the baseline it later compares against, and keep asserting that the reread does not scroll again.

## Why

It failed once in CI on the v1.0.0 stamp commit `4cce86e`, in the "Console tests and ui/dist freshness" job ([run 43745942783](https://github.com/madeintandem/tiller/actions/runs/43745942783)), with `AssertionError: expected 0 to be greater than 0` at line 119. That commit touched no console file, the same test passed 15 times in a row locally, and a rerun of the job passed. A flaky gate on main trains people to rerun red CI instead of reading it.

## Done when

- The test waits for the initial scroll before recording the baseline count, and still fails if the reread scrolls to the line a second time.
- The test passes 50 consecutive local runs (`npx vitest run src/components/__tests__/reread-report-file.test.tsx` in `ui/`).
- No other assertion in the file is weakened, and no product code under `ui/src` changes unless the scroll timing itself is shown to be a defect.
