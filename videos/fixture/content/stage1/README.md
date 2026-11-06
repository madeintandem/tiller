# Acme Orders

The order service behind Acme's back office. It serves the order book over a
small JSON API that the warehouse screens and the support desk read from.

## Run it

```sh
npm start          # listens on http://localhost:3000, or $PORT
```

## Test it

```sh
bin/test           # the whole suite
bin/test test/orders.test.js
```

No dependencies to install: it runs on Node 22 or later and uses only the
standard library.

## The API

See [the orders API](.tiller/design/as-is/orders-api.md) for the endpoints and
[the order store](.tiller/design/as-is/order-store.md) for the order shape.
