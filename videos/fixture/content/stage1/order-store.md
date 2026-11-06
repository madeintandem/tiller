# Order store

As-is account of the order book, translated from the retired `docs/orders.md`
(its "The order" section) and derived from `src/orders.js` where the doc was
silent.

## What this surface is

An in-memory order book. `createStore(orders)` in `src/orders.js` wraps a
list of orders, by default the six seeded ones, and offers two reads. The API
in `src/server.js` is its only caller.

## Data it owns

Orders, held in a `Map` keyed by id for the life of the process. Nothing
persists; a restart reloads the seed.

| Field      | Type   | Notes                                              |
|------------|--------|----------------------------------------------------|
| `id`       | string | Assigned by the storefront.                        |
| `customer` | string | The ordering account's display name.               |
| `total`    | number | In cents.                                          |
| `status`   | string | One of `pending`, `paid`, `shipped`, `delivered`, `cancelled`. |
| `placedAt` | string | The order date, `YYYY-MM-DD`.                      |

The five statuses are exported as `STATUSES`. The store does not check them:
an order with any other status would load unchanged.

## Behavior to preserve

- `list()` returns every order, sorted by `placedAt`, oldest first.
- `get(id)` returns the order with that id, or `undefined`.
- The store copies each order when it is created, so callers cannot change
  the seed.

## Edges

- Read by `src/server.js`. No other module imports it.
- No I/O of any kind.

## Described paths

- `src/orders.js`
