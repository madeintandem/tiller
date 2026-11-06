# Filter orders by status

## Summary

Add an optional `status` query parameter to `GET /orders` so a client can
ask for one status. Changes existing code: the list route in `src/server.js`
and the list read in `src/orders.js`, both described by the confirmed as-is
docs. Drawn from `.tiller/design/status-filter.md`.

## Behavior

- `GET /orders?status=<status>`, where `<status>` is one of the five in
  `STATUSES` (`src/orders.js:4`), answers `200` with
  `{ "orders": [...] }` holding only the orders with that status, oldest
  first. A status no order has yet answers `200` with an empty list.
- `GET /orders` with no `status` parameter is unchanged: every order, oldest
  first, as `as-is/orders-api.md` describes.
- Any other `status` value, including an empty one, answers `400` with
  `{ "error": "unknown status \"<value>\"; expected one of pending, paid, shipped, delivered, cancelled" }`.
  It never falls back to the full list.
- Matching is exact and case-sensitive: `Shipped` is unknown.
- Other query parameters stay ignored, as today.

## Notes

`GET /orders/:id`, the 404 and 405 answers, and the order shape do not
change. Customer and date filters, paging, and several statuses per request
stay out of scope, as the design says.
