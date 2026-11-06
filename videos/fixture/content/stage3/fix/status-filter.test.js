import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "../src/server.js";
import { createStore } from "../src/orders.js";

// Start the API on a free port and return a request helper for it.
async function startApi(t) {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, resolve));
  t.after(() => server.close());
  const base = `http://localhost:${server.address().port}`;
  return async (path) => {
    const res = await fetch(base + path);
    return { status: res.status, body: await res.json() };
  };
}

test("?status=shipped returns only shipped orders, oldest first", async (t) => {
  const request = await startApi(t);
  const { status, body } = await request("/orders?status=shipped");
  assert.equal(status, 200);
  assert.deepEqual(body.orders.map((o) => o.id), ["1001", "1005"]);
});

test("?status=pending returns only pending orders", async (t) => {
  const request = await startApi(t);
  const { body } = await request("/orders?status=pending");
  assert.deepEqual(body.orders.map((o) => o.id), ["1002"]);
});

test("a known status with no orders lists nothing", () => {
  const store = createStore([
    { id: "1", customer: "A", total: 1, status: "shipped", placedAt: "2026-10-01" },
  ]);
  assert.deepEqual(store.list({ status: "paid" }), []);
});

for (const value of ["lost", "Shipped", ""]) {
  test(`?status=${JSON.stringify(value)} answers 400 naming the valid statuses`, async (t) => {
    const request = await startApi(t);
    const { status, body } = await request(`/orders?status=${encodeURIComponent(value)}`);
    assert.equal(status, 400);
    assert.equal(
      body.error,
      `unknown status "${value}"; expected one of pending, paid, shipped, delivered, cancelled`,
    );
  });
}
