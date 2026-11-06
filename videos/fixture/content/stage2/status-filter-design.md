# Filter orders by status

## The problem

The warehouse screen only cares about orders that are paid and waiting to
ship, and the support desk mostly looks up shipped ones. Today both pull the
whole order book from `GET /orders` and filter it in the browser. That is
fine at six orders and slow at six thousand, and every client writes the
same filter a little differently.

## What we want

Let a client ask for one status:

    GET /orders?status=shipped

- A known status returns only the orders with that status, oldest first,
  in the same `{ "orders": [...] }` shape.
- No `status` parameter keeps today's behavior: every order.
- A status we don't recognize is a client mistake. Answer `400` with an
  error that says which statuses are valid, rather than quietly returning
  everything or nothing.

## Out of scope

Filtering by customer or date, paging, and more than one status per
request. We can come back for those once the screens use this.
