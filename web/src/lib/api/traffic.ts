import { t } from "@/i18n";
import { MOCK } from "@/lib/mock/enabled";
import type {
  Audit,
  TrafficDetail,
  TrafficHARExportMeta,
  TrafficHARImportResult,
  TrafficHost,
  TrafficReplayRequest,
  TrafficReplayResult,
  TrafficResp,
  TrafficStreamDirection,
  TrafficStreamPage,
} from "@/lib/types";

import { ApiError, del, get, getToken, http, post, tq } from "./core";

export const trafficApi = {
  // ---- traffic / audit / report / chat ----
  audit: (task?: string) => get<Audit>(`/audit${tq(task)}`),
  traffic: (
    page = 0,
    size = 100,
    host = "",
    method = "",
    q = "",
    opts: {
      body?: string;
      path?: string;
      status?: string;
      rewritten?: string;
      respMin?: string;
      respMax?: string;
      sort?: string;
      order?: string;
      company?: number | null;
    } = {},
  ) =>
    get<TrafficResp>(
      `/traffic?page=${page}&size=${size}` +
        (host ? `&host=${encodeURIComponent(host)}` : "") +
        (method && method !== "all" ? `&method=${encodeURIComponent(method)}` : "") +
        (q ? `&q=${encodeURIComponent(q)}` : "") +
        (opts.body ? `&body=${encodeURIComponent(opts.body)}` : "") +
        (opts.path ? `&path=${encodeURIComponent(opts.path)}` : "") +
        (opts.status && opts.status !== "all" ? `&status=${encodeURIComponent(opts.status)}` : "") +
        (opts.rewritten && opts.rewritten !== "all" ? `&rewritten=${encodeURIComponent(opts.rewritten)}` : "") +
        (opts.respMin ? `&resp_min=${encodeURIComponent(opts.respMin)}` : "") +
        (opts.respMax ? `&resp_max=${encodeURIComponent(opts.respMax)}` : "") +
        (opts.sort ? `&sort=${encodeURIComponent(opts.sort)}` : "") +
        (opts.order ? `&order=${encodeURIComponent(opts.order)}` : "") +
        (opts.company ? `&company=${opts.company}` : ""),
    ),
  trafficExchange: (id: string) => get<TrafficDetail>(`/traffic/exchange?id=${encodeURIComponent(id)}`),
  // One page of a WebSocket/SSE exchange's messages, in order.
  trafficStream: (id: string, offset = 0, limit = 50, direction: TrafficStreamDirection = "") =>
    get<TrafficStreamPage>(
      `/traffic/stream?exchange=${encodeURIComponent(id)}&offset=${offset}&limit=${limit}${direction ? `&direction=${direction}` : ""}`,
    ),
  // Raw bytes of one stored blob (a full message payload or an oversized body).
  // The JSON-only `get` cannot carry binary, hence the direct fetch.
  trafficBlob: async (hash: string) => {
    if (MOCK) throw new Error(t("Blob download is not available in mock mode"));
    const token = getToken();
    const r = await fetch(`/api/traffic/blob?hash=${encodeURIComponent(hash)}`, {
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    });
    if (!r.ok) throw new Error(t("Blob download failed: {error}", { error: r.status }));
    return new Uint8Array(await r.arrayBuffer());
  },
  // Re-sends an edited request through the proxy (bypasses capture rewrite rules
  // unless req.apply_rules is true). Errors carry the HTTP status (ApiError):
  // 413 body too large, 502 proxy/upstream, 503 capture off, 504 timeout.
  trafficReplay: async (req: TrafficReplayRequest) => {
    if (MOCK) throw new Error(t("Replay is not available in mock mode"));
    return post<TrafficReplayResult>(`/traffic/replay`, {
      ...req,
      apply_rules: Boolean(req.apply_rules),
    });
  },
  // Imports a raw HAR 1.2 JSON document into the traffic store.
  trafficImportHAR: async (rawJSON: string): Promise<TrafficHARImportResult> => {
    if (MOCK) throw new Error(t("HAR import is not available in mock mode"));
    return http<TrafficHARImportResult>(`/traffic/import/har`, {
      method: "POST",
      body: rawJSON,
    });
  },
  // Exports filtered traffic as a HAR 1.2 file download and returns truncation metadata.
  trafficExportHAR: async (
    opts: {
      host?: string;
      method?: string;
      q?: string;
      body?: string;
      path?: string;
      status?: string;
      rewritten?: string;
      respMin?: string;
      respMax?: string;
    } = {},
  ): Promise<TrafficHARExportMeta> => {
    if (MOCK) throw new Error(t("HAR export is not available in mock mode"));
    const qs = new URLSearchParams();
    if (opts.host) qs.set("host", opts.host);
    if (opts.method && opts.method !== "all") qs.set("method", opts.method);
    if (opts.q) qs.set("q", opts.q);
    if (opts.body) qs.set("body", opts.body);
    if (opts.path) qs.set("path", opts.path);
    if (opts.status && opts.status !== "all") qs.set("status", opts.status);
    if (opts.rewritten && opts.rewritten !== "all") qs.set("rewritten", opts.rewritten);
    if (opts.respMin) qs.set("resp_min", opts.respMin);
    if (opts.respMax) qs.set("resp_max", opts.respMax);
    const suffix = qs.toString();
    return downloadHAR(`/api/traffic/export/har${suffix ? `?${suffix}` : ""}`);
  },
  // Exports exactly the selected exchanges as a HAR 1.2 download (audited server-side).
  trafficExportHARByIds: async (ids: string[]): Promise<TrafficHARExportMeta> => {
    if (MOCK) throw new Error(t("HAR export is not available in mock mode"));
    return downloadHAR(`/api/traffic/bulk-export`, { ids });
  },
  trafficHosts: () => get<{ hosts: TrafficHost[] }>(`/traffic/hosts`),
  trafficDeleteHost: (host: string) => del<{ deleted: number }>(`/traffic?host=${encodeURIComponent(host)}`),
  trafficDeleteHosts: (hosts: string[]) => del<{ deleted: number }>(`/traffic/hosts`, { hosts }),
  // Deletes exactly the selected exchanges, not their hosts (audited server-side).
  trafficDeleteIds: (ids: string[]) => post<{ deleted: number }>(`/traffic/bulk-delete`, { ids }),
  // Purges every exchange and compacts the index; `reclaimed` is the bytes of
  // index handed back to the filesystem. Evidence bound to findings is kept.
  trafficDeleteAll: () => del<{ deleted: number; reclaimed: number }>(`/traffic/all`),
};

// Fetches a HAR export (the JSON helpers cannot carry a file download), saves it
// as artex-traffic.har, and returns the X-ARTEX-HAR-* truncation metadata. A
// JSON body makes it a POST (the selection export); none, a GET (the filter export).
async function downloadHAR(url: string, body?: object): Promise<TrafficHARExportMeta> {
  const token = getToken();
  const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};
  const r = await fetch(
    url,
    body
      ? { method: "POST", headers: { ...headers, "Content-Type": "application/json" }, body: JSON.stringify(body) }
      : { headers },
  );
  if (r.status === 401) {
    if (typeof window !== "undefined") {
      window.location.href = "/login";
    }
    throw new Error(t("Unauthorized"));
  }
  if (!r.ok) {
    let msg = String(r.status);
    try {
      const payload = (await r.json()) as { error?: unknown };
      if (typeof payload.error === "string" && payload.error.trim()) msg = payload.error.trim();
    } catch {
      // Keep status fallback.
    }
    throw new ApiError(t("HAR export failed: {error}", { error: msg }), r.status);
  }
  const blob = await r.blob();
  const exported = Number(r.headers.get("X-ARTEX-HAR-Exported") ?? "0") || 0;
  const matched = Number(r.headers.get("X-ARTEX-HAR-Matched") ?? "0") || 0;
  const truncated = r.headers.get("X-ARTEX-HAR-Truncated") === "true";
  const truncatedReason = r.headers.get("X-ARTEX-HAR-Truncated-Reason") ?? undefined;
  const objectUrl = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = objectUrl;
  a.download = "artex-traffic.har";
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(objectUrl);
  return { exported, matched, truncated, truncatedReason };
}
