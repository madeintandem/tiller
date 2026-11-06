import http from "node:http";
import { pathToFileURL } from "node:url";
import { createStore } from "./orders.js";

function send(res, status, body) {
  res.writeHead(status, { "content-type": "application/json" });
  res.end(JSON.stringify(body));
}

export function createServer(store = createStore()) {
  return http.createServer((req, res) => {
    const url = new URL(req.url, "http://localhost");

    if (req.method !== "GET") {
      return send(res, 405, { error: "method not allowed" });
    }

    if (url.pathname === "/orders") {
      return send(res, 200, { orders: store.list() });
    }

    const match = url.pathname.match(/^\/orders\/([^/]+)$/);
    if (match) {
      const order = store.get(match[1]);
      if (!order) {
        return send(res, 404, { error: `order ${match[1]} not found` });
      }
      return send(res, 200, { order });
    }

    return send(res, 404, { error: "not found" });
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const port = Number(process.env.PORT ?? 3000);
  createServer().listen(port, () => {
    console.log(`Acme Orders listening on http://localhost:${port}`);
  });
}
