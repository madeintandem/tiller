# Code review: feature orders/status-filter ({{FEATURE_ID}}) "Filter orders by status"

Reviewed {{REVIEW_DATE}} by jc. A derived review, uncommitted until a disposition.
Scope: orders/status-filter/filter-by-status ({{TASK_ID}}), in_review, unmerged.
Pins: integration main at {{MAIN_SHA}}; tiller/orders/status-filter/filter-by-status at {{BRANCH_SHA}}.
Contract read: feature.md, feature.eval.md (C1, C2); no preserve criteria.

## Summary

| #  | Severity | Finding                                           | Found in | Where |
|----|----------|---------------------------------------------------|----------|-------|
| B1 | blocker  | Unknown status returns every order (violates C2)  | orders/status-filter/filter-by-status ({{TASK_ID}}), in_review, unmerged | src/orders.js:22 |

1 bug (1 blocker)

## Bugs

### Blocker

#### B1 Unknown status returns every order instead of a 400

Severity: blocker. Confidence: traced. Found in: orders/status-filter/filter-by-status ({{TASK_ID}}), in_review, unmerged.
Anchors: `src/server.js:19` (`url.searchParams.get("status") ?? undefined` passes any value through), `src/orders.js:22` (`if (!STATUSES.includes(status)) { return all; }`). Violates: C2.
**Reproduction** Start the server and request `GET /orders?status=lost`. The route hands `"lost"` to `store.list()`, which finds it is not in `STATUSES` and returns the full list.
**Expected** `400` with `{ "error": "unknown status \"lost\"; expected one of pending, paid, shipped, delivered, cancelled" }`, as feature.md and C2 say. A typo in a client's filter should fail loudly.
**Actual** `200` with all six orders, which a client cannot tell apart from a real result. `?status=Shipped` and `?status=` do the same. `test/status-filter.test.js` has no unknown-status case, though task.md's Tests section lists two.
**Fixed when** an unknown, miscased, or empty status answers `400` with that error, the store no longer falls back to the full list, and `test/status-filter.test.js` covers all three.

## Limitations

None.
