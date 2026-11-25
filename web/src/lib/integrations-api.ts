import { http } from "@/lib/api";
import { chunk, PUSH_BATCH } from "@/lib/integrations";
import type {
  FindingIntegrations,
  ScanImport,
  ScanImportFormat,
  ScanImportOutcome,
  TicketPushResult,
  TicketSyncResult,
  TicketTracker,
  TicketTrackerInput,
  TicketTrackerMeta,
} from "@/lib/types";

// Client for the E2 integration routes (docs/integrations.md). Kept apart from
// lib/api.ts, which is already the size of several modules.

const json = (method: string, body: unknown): RequestInit => ({ method, body: JSON.stringify(body) });

export const integrationsApi = {
  meta: () => http<TicketTrackerMeta>("/integrations/meta"),
  trackers: () => http<{ trackers: TicketTracker[] }>("/integrations/trackers").then((r) => r.trackers ?? []),
  createTracker: (input: TicketTrackerInput) => http<TicketTracker>("/integrations/trackers", json("POST", input)),
  // PATCH: only the fields given change; a masked config value sent back keeps the stored one.
  updateTracker: (id: number, input: TicketTrackerInput) =>
    http<TicketTracker>(`/integrations/trackers/${id}`, json("PATCH", input)),
  deleteTracker: (id: number) => http<{ deleted: number }>(`/integrations/trackers/${id}`, { method: "DELETE" }),
  // push sends the ids in batches of the server's cap and joins the per-finding results.
  push: async (trackerId: number, findingIds: number[]): Promise<TicketPushResult[]> => {
    const out: TicketPushResult[] = [];
    for (const batch of chunk(findingIds, PUSH_BATCH)) {
      const r = await http<{ results: TicketPushResult[] }>(
        `/integrations/trackers/${trackerId}/push`,
        json("POST", { finding_ids: batch }),
      );
      out.push(...(r.results ?? []));
    }
    return out;
  },
  sync: (trackerId: number) =>
    http<{ results: TicketSyncResult[] }>(`/integrations/trackers/${trackerId}/sync`, { method: "POST" }).then(
      (r) => r.results ?? [],
    ),
  // One page of past imports, newest first, with the total for the pager.
  imports: (limit = 100, offset = 0) =>
    http<{ imports: ScanImport[] | null; total: number }>(`/integrations/imports?limit=${limit}&offset=${offset}`).then(
      (r) => ({ items: r.imports ?? [], total: r.total ?? 0 }),
    ),
  // importScan posts the raw file as the body; format and provenance travel in the query.
  importScan: (file: File, format: ScanImportFormat, taskId?: number) => {
    const q = new URLSearchParams({ format, file_name: file.name });
    if (taskId) q.set("task_id", String(taskId));
    return http<ScanImportOutcome>(`/integrations/imports?${q.toString()}`, {
      method: "POST",
      body: file,
      headers: { "Content-Type": "application/octet-stream" },
    });
  },
  findingIntegrations: (findingId: string) =>
    http<FindingIntegrations>(`/exploration/findings/${encodeURIComponent(findingId)}/integrations`),
};
