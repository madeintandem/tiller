# Orders API

As-is account of the HTTP API, translated from the retired `docs/orders.md`
(its Endpoints section) and reconciled against `src/server.js`.

## What this surface is

The read-only JSON API over the order book. `createServer(store)` in
`src/server.js` builds a plain `node:http` server around an order store;
run directly, it listens on `$PORT`, or 3000. Orders are created upstream by
the storefront and loaded into the order book when the service starts. The
README names its current users: the warehouse screens and the support desk.

## Data it owns

None. Every read goes through the order store (see `order-store.md`).

## Behavior to preserve

- `GET /orders` answers `200` with `{ "orders": [...] }`: every order, oldest
  first. The query string is ignored today, so `GET /orders?anything` returns
  the full list.
- `GET /orders/:id` answers `200` with `{ "order": { ... } }`. An id with no
  order answers `404` with `{ "error": "order <id> not found" }`.
- Any other path answers `404` with `{ "error": "not found" }`.
- Any method other than `GET` answers `405`. The code also sends
  `{ "error": "method not allowed" }`, which the old doc did not mention.
- Every response is JSON with `content-type: application/json`.

## Edges

- Calls `createStore().list()` and `.get(id)` from `src/orders.js`.
- `test/orders.test.js` starts the server on a free port and covers each rule
  above.
- No authentication, no outbound calls.

## Described paths

- `src/server.js`
