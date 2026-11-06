// The order book: an in-memory store seeded with Acme's open orders.
// Nothing here persists; a restart reloads the seed.

export const STATUSES = ["pending", "paid", "shipped", "delivered", "cancelled"];

const seed = [
  { id: "1001", customer: "Northwind Traders", total: 12900, status: "shipped", placedAt: "2026-10-02" },
  { id: "1002", customer: "Blue Heron Cafe", total: 4350, status: "pending", placedAt: "2026-10-04" },
  { id: "1003", customer: "Tailspin Toys", total: 78000, status: "paid", placedAt: "2026-10-05" },
  { id: "1004", customer: "Fourth Coffee", total: 2199, status: "delivered", placedAt: "2026-10-07" },
  { id: "1005", customer: "Northwind Traders", total: 15500, status: "shipped", placedAt: "2026-10-09" },
  { id: "1006", customer: "Wide World Importers", total: 9900, status: "cancelled", placedAt: "2026-10-11" },
];

export function createStore(orders = seed) {
  const byId = new Map(orders.map((order) => [order.id, { ...order }]));

  return {
    // Every order, oldest first.
    list() {
      return [...byId.values()].sort((a, b) => a.placedAt.localeCompare(b.placedAt));
    },

    // One order by id, or undefined.
    get(id) {
      return byId.get(id);
    },
  };
}
