# Orders API

The orders API is read-only. Orders are created upstream by the storefront and
loaded into the order book when the service starts.

## Endpoints

### GET /orders

Returns every order, oldest first:

```json
{ "orders": [ { "id": "1001", "customer": "Northwind Traders", "total": 12900, "status": "shipped", "placedAt": "2026-10-02" } ] }
```

### GET /orders/:id

Returns one order as `{ "order": { ... } }`. An id with no order answers
`404` with `{ "error": "order <id> not found" }`.

Any other path answers `404` with `{ "error": "not found" }`, and any method
other than `GET` answers `405`.

## The order

| Field      | Type   | Notes                                              |
|------------|--------|----------------------------------------------------|
| `id`       | string | Assigned by the storefront.                        |
| `customer` | string | The ordering account's display name.               |
| `total`    | number | In cents.                                          |
| `status`   | string | One of `pending`, `paid`, `shipped`, `delivered`, `cancelled`. |
| `placedAt` | string | The order date, `YYYY-MM-DD`.                      |
