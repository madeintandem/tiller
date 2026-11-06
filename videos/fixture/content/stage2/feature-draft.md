# Filter orders by status

## Summary

Add an optional `status` query parameter to `GET /orders` so a client can
ask for one status. Changes existing code: the list route in `src/server.js`
and the list read in `src/orders.js`, both described by the confirmed as-is
docs.

## Behavior

To be specified: a known status, no status, and an unknown status, per
`.tiller/design/status-filter.md`.

## Notes

The exact error body for an unknown status is left to specification.
