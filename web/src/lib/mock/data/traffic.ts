import type { TrafficDetail, TrafficHost, TrafficResp, TrafficStreamMessage } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

const exchanges = [
  ["x-1", "www.acme.com", "GET", "https://www.acme.com/", 200, "text/html", 48213],
  ["x-2", "www.acme.com", "GET", "https://www.acme.com/search?q=test", 200, "text/html", 5120],
  ["x-3", "www.acme.com", "GET", "https://www.acme.com/.git/HEAD", 200, "text/plain", 23],
  ["x-4", "admin.acme.com", "GET", "https://admin.acme.com/login", 200, "text/html", 12044],
  ["x-5", "admin.acme.com", "POST", "https://admin.acme.com/login", 302, "text/html", 0],
  ["x-6", "admin.acme.com", "GET", "https://admin.acme.com/dashboard", 200, "text/html", 33120],
  ["x-7", "admin.acme.com", "GET", "https://admin.acme.com/api/users?page=1", 200, "application/json", 8842],
  ["x-8", "api.acme.com", "GET", "https://api.acme.com/v1/orders?id=1001", 200, "application/json", 640],
  ["x-9", "api.acme.com", "GET", "https://api.acme.com/v1/orders?id=1002", 200, "application/json", 655],
  ["x-10", "api.acme.com", "GET", "https://api.acme.com/v1/users/me", 200, "application/json", 312],
  ["x-11", "api.acme.com", "GET", "https://api.acme.com/v1/admin", 401, "application/json", 45],
  ["x-12", "shop.acme.com", "GET", "https://shop.acme.com/checkout", 200, "text/html", 21990],
  ["x-13", "shop.acme.com", "POST", "https://shop.acme.com/pay", 200, "application/json", 128],
  ["x-14", "www.acme.com", "GET", "https://www.acme.com/robots.txt", 200, "text/plain", 88],
  ["x-15", "shop.acme.com", "POST", "https://shop.acme.com/api/import", 500, "application/json", 2048],
  ["x-16", "shop.acme.com", "POST", "https://shop.acme.com/api/import", 200, "application/json", 0],
  ["x-17", "10.10.10.20", "GET", "http://10.10.10.20:8080/script", 200, "text/html", 14200],
  ["x-18", "10.10.10.20", "POST", "http://10.10.10.20:8080/scriptText", 200, "text/plain", 86],
  ["x-19", "10.10.10.10", "POST", "https://10.10.10.10:445/psexec", 200, "application/octet-stream", 0],
  ["x-ws", "api.acme.com", "GET", "wss://api.acme.com/live", 101, "websocket", 3600],
  ["x-sse", "api.acme.com", "GET", "https://api.acme.com/events", 200, "text/event-stream", 90],
] as const;

export const traffic: TrafficResp = {
  enabled: true,
  proxy: ":8788",
  count: exchanges.length,
  total: exchanges.length,
  page: 0,
  size: 100,
  exchanges: exchanges.map(([id, host, method, url, status, ct, len], i) => ({
    id,
    host,
    method,
    url,
    status,
    content_type: ct,
    resp_len: len,
    ts: T(`2026-11-14T0${1 + (i % 3)}:${String(10 + i).padStart(2, "0")}:00Z`),
  })),
};

// Distinct hosts with counts, derived from the exchanges above (target picker).
const hostCounts: Record<string, number> = {};
for (const [, host] of exchanges) hostCounts[host] = (hostCounts[host] ?? 0) + 1;
export const trafficHosts: TrafficHost[] = Object.entries(hostCounts).map(([host, count]) => ({ host, count }));

export const trafficDetail: TrafficDetail = {
  req: `GET /v1/orders?id=1002 HTTP/1.1
Host: api.acme.com
Authorization: Bearer eyJhbGciOiJIUzI1NiJ9...
User-Agent: ARTEX-worker/1.0
Accept: application/json

`,
  resp: `HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 655

{
  "order_id": 1002,
  "user_id": 77,
  "amount": 1299.00,
  "address": "Chaoyang, Beijing ****",
  "phone": "138****6021",
  "items": [{ "sku": "A-100", "qty": 1 }]
}`,
};

// More than one page in either direction, with original seqs and a recording
// limit warning. The SSE sample also lets the viewer exercise an empty c2s page.
export const trafficStreamMessages: Record<string, TrafficStreamMessage[]> = {
  "x-ws": [
    ...Array.from({ length: 120 }, (_, index): TrafficStreamMessage => {
      const direction = index % 2 === 0 ? "c2s" : "s2c";
      const payload = JSON.stringify({ action: direction === "c2s" ? "subscribe" : "update", order: index + 1 });
      return {
        seq: index + 1,
        ts: 1794618000 + index,
        direction,
        kind: 1,
        type: "text",
        size: payload.length,
        payload,
      };
    }),
    {
      seq: 121,
      ts: 1794618120,
      direction: "meta",
      kind: 0,
      type: "meta",
      size: 46,
      payload: '{"dropped_messages":3,"dropped_bytes":1024000}',
    },
  ],
  "x-sse": Array.from({ length: 3 }, (_, index): TrafficStreamMessage => {
    const payload = JSON.stringify({ status: "running", progress: (index + 1) * 10 });
    return {
      seq: index + 1,
      ts: 1794618000 + index,
      direction: "s2c",
      kind: 0,
      type: "event",
      event: "progress",
      event_id: String(index + 1),
      size: payload.length,
      payload,
    };
  }),
};

export const trafficStreamDetails: Record<string, TrafficDetail> = {
  "x-ws": {
    req: "GET /live HTTP/1.1\nHost: api.acme.com\nConnection: Upgrade\nUpgrade: websocket\n\n",
    resp: "HTTP/1.1 101 Switching Protocols\nConnection: Upgrade\nUpgrade: websocket\n\n",
    stream_kind: "websocket",
    stream_count: trafficStreamMessages["x-ws"].length,
  },
  "x-sse": {
    req: "GET /events HTTP/1.1\nHost: api.acme.com\nAccept: text/event-stream\n\n",
    resp: "HTTP/1.1 200 OK\nContent-Type: text/event-stream\n\n",
    stream_kind: "sse",
    stream_count: trafficStreamMessages["x-sse"].length,
  },
};

// ── Security / Audit ─────────────────────────────────────────────────────────
