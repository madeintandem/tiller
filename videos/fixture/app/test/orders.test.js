import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "../src/server.js";

// Start the API on a free port and return a request helper for it.
async function startApi(t) {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, resolve));
  t.after(() => server.close());
  const base = `http://localhost:${server.address().port}`;
  return async (path, init) => {
    const res = await fetch(base + path, init);
    return { status: res.status, body: await res.json() };
  };
}

test("GET /orders lists every order, oldest first", async (t) => {
  const request = await startApi(t);
  const { status, body } = await request("/orders");
  assert.equal(status, 200);
  assert.deepEqual(
    body.orders.map((o) => o.id),
    ["1001", "1002", "1003", "1004", "1005", "1006"],
  );
});

test("GET /orders/:id returns one order", async (t) => {
  const request = await startApi(t);
  const { status, body } = await request("/orders/1003");
  assert.equal(status, 200);
  assert.equal(body.order.customer, "Tailspin Toys");
});

test("GET /orders/:id answers 404 for an unknown order", async (t) => {
  const request = await startApi(t);
  const { status, body } = await request("/orders/9999");
  assert.equal(status, 404);
  assert.equal(body.error, "order 9999 not found");
});

test("anything but GET answers 405", async (t) => {
  const request = await startApi(t);
  const { status } = await request("/orders", { method: "POST" });
  assert.equal(status, 405);
});
