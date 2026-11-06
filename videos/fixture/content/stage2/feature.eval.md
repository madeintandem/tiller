---
node: {{FEATURE_ID}}
criteria:
  - id: C1
    statement: GET /orders?status=<status> returns only the orders with that status, oldest first
    polarity: introduce
    binding: { type: test, ref: "test/status-filter.test.js" }
    required: true
  - id: C2
    statement: An unknown status answers 400 with an error naming the valid statuses
    polarity: introduce
    binding: { type: test, ref: "test/status-filter.test.js" }
    required: true
---

# Acceptance

Both criteria run `bin/test test/status-filter.test.js`, the tests the
filter-by-status task adds. `bin/test` passes its arguments to `node --test`,
so the ref selects that one file. The unchanged list and lookup behavior is
already covered by `test/orders.test.js`, which the tests gate runs on every
check.
