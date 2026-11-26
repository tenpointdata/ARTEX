import type {
  ConfigValues,
  FindingLane,
  FindingSource,
  ScanImportFormat,
  TicketPushResult,
  TicketSyncAction,
  TicketSyncResult,
  TicketTracker,
} from "./types";

// Pure helpers for the E2 integrations UI (ticket trackers, scanner imports).
// No UI or i18n imports, so they run under plain node in the unit tests.

export const IMPORT_FORMATS: ScanImportFormat[] = ["nuclei-jsonl", "nmap-xml", "zap-json", "burp-xml", "sarif"];

// PUSH_BATCH mirrors the server's cap of 200 finding ids per push request.
export const PUSH_BATCH = 200;

// The server-side option every tracker config may carry (server/ticket_api.go trackerOptionKeys).
export const INCLUDE_EVIDENCE = "include_evidence";

// Tracker kinds that accept signed push updates (ticketing.Webhooks), and the
// route they post to (server/ticket_webhook.go ticketWebhookPrefix).
export const WEBHOOK_KINDS: ReadonlySet<string> = new Set(["github", "jira"]);
export const WEBHOOK_SECRET = "webhook_secret";

export function webhookURL(origin: string, trackerId: number): string {
  return `${origin.replace(/\/+$/, "")}/api/hooks/trackers/${trackerId}`;
}

export function evidenceEnabled(config: ConfigValues): boolean {
  return config[INCLUDE_EVIDENCE] === true;
}

// detectImportFormat guesses a scanner file's format from its name and first
// bytes, so the common case needs no format pick. null means "ask the operator".
export function detectImportFormat(fileName: string, head: string): ScanImportFormat | null {
  const name = fileName.toLowerCase();
  const text = head.trimStart().slice(0, 4096);
  if (name.endsWith(".sarif") || name.endsWith(".sarif.json")) return "sarif";
  if (name.endsWith(".jsonl")) return "nuclei-jsonl";
  if (text.startsWith("<")) {
    if (/<nmaprun[\s>]/.test(text)) return "nmap-xml";
    if (/burpVersion|<issues[\s>]/.test(text)) return "burp-xml";
    return null;
  }
  if (text.startsWith("{") || text.startsWith("[")) {
    if (/"\$schema"\s*:\s*"[^"]*sarif/i.test(text) || /"runs"\s*:\s*\[/.test(text)) return "sarif";
    if (/"@programName"\s*:\s*"ZAP"|"@version"[\s\S]*"site"\s*:/.test(text)) return "zap-json";
    if (/"template-id"\s*:|"templateID"\s*:/.test(text)) return "nuclei-jsonl";
  }
  return null;
}

// pushableFindingIds turns the findings table's string ids into the numeric,
// de-duplicated ids the push API takes; anything else is dropped.
export function pushableFindingIds(ids: Iterable<string>): number[] {
  const out = new Set<number>();
  for (const id of ids) {
    const n = Number(id);
    if (Number.isSafeInteger(n) && n > 0) out.add(n);
  }
  return [...out];
}

export function chunk<T>(items: T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) out.push(items.slice(i, i + size));
  return out;
}

export interface PushSummary {
  created: number;
  updated: number;
  failed: number;
  // The first few "#id: error" lines, for the toast.
  errors: string[];
}

export function summarizePush(results: TicketPushResult[], maxErrors = 3): PushSummary {
  const s: PushSummary = { created: 0, updated: 0, failed: 0, errors: [] };
  for (const r of results) {
    if (r.error) {
      s.failed++;
      if (s.errors.length < maxErrors) s.errors.push(`#${r.finding_id}: ${r.error}`);
    } else if (r.created) {
      s.created++;
    } else {
      s.updated++;
    }
  }
  return s;
}

export function summarizeSync(results: TicketSyncResult[]): Record<TicketSyncAction, number> {
  const s: Record<TicketSyncAction, number> = { unchanged: 0, pulled: 0, pushed: 0, error: 0 };
  for (const r of results) s[r.action] = (s[r.action] ?? 0) + 1;
  return s;
}

function configText(config: ConfigValues, key: string): string {
  const v = config[key];
  if (v === null || v === undefined) return "";
  return typeof v === "string" ? v.trim() : String(v);
}

function urlHost(raw: string): string {
  try {
    return new URL(raw).host;
  } catch {
    return raw;
  }
}

// Per-kind "where tickets go"; a kind not listed shows its base URL's host.
const DESTINATION: Record<string, (c: ConfigValues) => string> = {
  github: (c) => {
    const base = configText(c, "base_url");
    return base ? `${urlHost(base)}/${configText(c, "repo")}` : configText(c, "repo");
  },
  jira: (c) => [urlHost(configText(c, "base_url")), configText(c, "project_key")].filter(Boolean).join(" · "),
  defectdojo: (c) => {
    const test = configText(c, "test_id");
    return [urlHost(configText(c, "base_url")), test && `test ${test}`].filter(Boolean).join(" · ");
  },
};

// trackerDestination is the one-line "where tickets go" for the trackers table.
export function trackerDestination(tr: Pick<TicketTracker, "kind" | "config">): string {
  const fn = DESTINATION[tr.kind];
  return fn ? fn(tr.config) : urlHost(configText(tr.config, "base_url"));
}

// importIdParam normalizes an import_id query value: a positive integer as a
// string, or "" (no import filter) for anything else.
export function importIdParam(raw: string | null | undefined): string {
  const v = (raw ?? "").trim();
  return /^[1-9]\d{0,17}$/.test(v) ? v : "";
}

// importFindingsHref links to the findings one scanner import created.
export function importFindingsHref(importId: number | string): string {
  return `/function/findings?import_id=${encodeURIComponent(String(importId))}`;
}

export const FINDING_LANES: FindingLane[] = ["runtime", "sast", "sca"];

// laneParam normalizes a lane query value: a known lane, or "" (every lane).
export function laneParam(raw: string | null | undefined): FindingLane | "" {
  const v = (raw ?? "").trim();
  return FINDING_LANES.find((lane) => lane === v) ?? "";
}

// codeLocation renders a code finding's "path:line"; "" when there is no path.
export function codeLocation(source: Pick<FindingSource, "location" | "line"> | null | undefined): string {
  const path = source?.location?.trim();
  if (!path) return "";
  return source?.line && source.line > 0 ? `${path}:${source.line}` : path;
}

// laneCountEntries lists an import's non-zero per-lane finding counts in lane order.
export function laneCountEntries(
  counts: Partial<Record<FindingLane, number>> | null | undefined,
): { lane: FindingLane; count: number }[] {
  return FINDING_LANES.flatMap((lane) => {
    const count = counts?.[lane] ?? 0;
    return count > 0 ? [{ lane, count }] : [];
  });
}

// anchorAssetIdsParam is the `asset_ids` upload query value, or "" when none are picked.
export function anchorAssetIdsParam(ids: readonly number[]): string {
  return [...new Set(ids.filter((id) => Number.isSafeInteger(id) && id > 0))].join(",");
}
