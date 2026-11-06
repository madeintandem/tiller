# Filter the order list by status

## Approach

Read `status` from the query string in the `/orders` route
(`src/server.js:18`). When it is absent, list everything as today. When it
is one of `STATUSES`, pass it to the store and return the matching orders.
Otherwise answer `400` with the error body the feature spec gives, before
touching the store. In `src/orders.js`, give `list()` an optional
`{ status }` filter applied after the existing sort, so the order stays
oldest first.

## Reuses

- `STATUSES` (`src/orders.js:4`) as the list of valid statuses and for the
  error message.
- `send()` (`src/server.js:5`) for the `400` response.
- The existing sort in `list()` (`src/orders.js:21`).
- Searched `src/` and `test/` for any query-string parsing: none found; the
  route already builds a `URL` (`src/server.js:12`), so `url.searchParams`
  is the reader.

## Surfaces

- `src/server.js`
- `src/orders.js`
- `test/status-filter.test.js` (new)

## Tests

Add `test/status-filter.test.js`, started the way `test/orders.test.js`
starts the server:

- `?status=shipped` returns orders 1001 and 1005, in that order (C1).
- `?status=pending` returns only 1002 (C1).
- `?status=lost` answers `400` with the error naming the five statuses (C2).
- `?status=Shipped` answers `400` (C2).

Run `bin/test test/status-filter.test.js` and confirm it runs only that file.

## Edge cases

- An empty `status=` is unknown, not absent: `searchParams.get` returns `""`.
- A known status that matches no order still answers `200` with an empty
  list. Every status has a seeded order today, so the store test covers it
  with its own orders.
- Repeated `status` parameters: the first one wins, as `searchParams.get`
  does.
