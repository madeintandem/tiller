# Order filtering

## Intent

Let the warehouse screen and the support desk ask the orders API for the
orders they need instead of filtering the whole order book in the browser.
Drawn from `.tiller/design/status-filter.md`.

## Scope

One increment: filtering `GET /orders` by a single status. Filtering by
customer or date, paging, and multi-status requests are out of scope in the
design and are not planned here.

## Definition of done

`GET /orders?status=<status>` is live on main with its acceptance contract
green, and the existing endpoints behave as the as-is baseline describes.
