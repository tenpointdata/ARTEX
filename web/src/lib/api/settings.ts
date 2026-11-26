import { t } from "@/i18n";
import type { ChatMention } from "@/lib/chat-mentions";
import { MOCK } from "@/lib/mock/enabled";
import type {
  ChatAttachment,
  NoiseStatus,
  NotificationChannel,
  NotificationDelivery,
  NotificationFilter,
  NotificationMeta,
  Settings,
  ReportBranding,
  TamperDryRunRequest,
  TamperDryRunResult,
  TamperHook,
  TamperHookInput,
  TamperHooksListResp,
  TamperHookVersion,
} from "@/lib/types";

import { arr, del, get, getToken, http, MAX_CHAT_UPLOAD_BYTES, mockReport, patch, post, put, tq } from "./core";

export const settingsApi = {
  // ---- capture-proxy constrained Go tampering hooks (P4.2, admin-only) ----
  trafficHooks: () => get<TamperHooksListResp>(`/traffic/hooks`),
  createTrafficHook: (body: TamperHookInput) => post<TamperHook>(`/traffic/hooks`, { ...body, enabled: false }),
  updateTrafficHook: (id: number, body: TamperHookInput) => put<TamperHook>(`/traffic/hooks/${id}`, body),
  deleteTrafficHook: (id: number) => del<{ deleted: boolean }>(`/traffic/hooks/${id}`),
  toggleTrafficHook: (id: number, enabled: boolean) => post<TamperHook>(`/traffic/hooks/${id}/toggle`, { enabled }),
  trafficHookVersions: (id: number) => get<{ versions: TamperHookVersion[] }>(`/traffic/hooks/${id}/versions`),
  rollbackTrafficHook: (id: number, version: number) => post<TamperHook>(`/traffic/hooks/${id}/rollback`, { version }),
  dryRunTrafficHook: (body: TamperDryRunRequest) => post<TamperDryRunResult>(`/traffic/hooks/dry-run`, body),

  reportBranding: () => get<ReportBranding>(`/settings/report-branding`),
  setReportBranding: (branding: ReportBranding) => put<ReportBranding>(`/settings/report-branding`, branding),

  // ---- app settings (runtime toggles) ----
  settings: () => get<Settings>(`/settings`),
  setSettings: (patch: Partial<Settings>) => put<Settings>(`/settings`, patch),
  noiseStatus: () => get<import("@/lib/types").NoiseStatus>(`/settings/noise`),
  // Run a real "test" search with the given (or saved) config to verify it works.
  testWebSearch: (patch: {
    web_search_backend?: string;
    web_search_proxy?: string;
    brave_search_api_key?: string;
    tavily_search_api_key?: string;
    keenable_search_api_key?: string;
  }) => post<{ ok: boolean; error?: string; count?: number; backend?: string }>(`/settings/web-search/test`, patch),

  // ---- Vulnerability IM push ----
  // Channels are multi-instance resources (several bots of the same kind, each with its own
  // filter rules), so they form their own group instead of the flat settings key/value store.
  notifyMeta: () => get<NotificationMeta>(`/notify/meta`),
  notifyChannels: () => get<{ channels: NotificationChannel[] }>(`/notify/channels`).then((r) => arr(r.channels)),
  notifyCreateChannel: (payload: {
    name: string;
    kind: string;
    enabled?: boolean;
    mode?: string;
    config: Record<string, unknown>;
    filter?: NotificationFilter;
    rate_per_min?: number;
  }) => post<{ id: number }>(`/notify/channels`, payload),
  // PATCH semantics: submit only the fields to change. Passing a masked config value back as-is means "keep the current value".
  notifyUpdateChannel: (
    id: number,
    payload: {
      name?: string;
      kind?: string;
      enabled?: boolean;
      mode?: string;
      config?: Record<string, unknown>;
      filter?: NotificationFilter;
      rate_per_min?: number;
    },
  ) => patch<{ id: number }>(`/notify/channels/${id}`, payload),
  notifyDeleteChannel: (id: number) => del<{ ok: boolean }>(`/notify/channels/${id}`),
  // Send a test message synchronously; on failure the backend returns the channel's raw error for configuration troubleshooting.
  notifyTestChannel: (id: number) => post<{ ok: boolean; latency_ms: number }>(`/notify/channels/${id}/test`),
  notifyDeliveries: (q: { channelId?: number; state?: string; page?: number; pageSize?: number } = {}) => {
    const p = new URLSearchParams();
    if (q.channelId) p.set("channel_id", String(q.channelId));
    if (q.state) p.set("state", q.state);
    p.set("page", String(q.page ?? 1));
    p.set("page_size", String(q.pageSize ?? 50));
    return get<{ deliveries: NotificationDelivery[]; total: number; page: number; page_size: number }>(
      `/notify/deliveries?${p.toString()}`,
    ).then((r) => ({ ...r, deliveries: arr(r.deliveries) }));
  },
  notifyRetryDelivery: (id: number) => post<{ ok: boolean }>(`/notify/deliveries/${id}/retry`),
  report: async (task?: string) => {
    if (MOCK) return mockReport(task);
    const token = getToken();
    const r = await fetch(`/api/report${tq(task)}`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
    if (!r.ok) throw new Error(t("Report failed: {error}", { error: r.status }));
    return r.text();
  },
  chatMentions: (kind: string, query: string, signal?: AbortSignal, cursor = "") =>
    http<{ items: ChatMention[]; next_cursor?: string }>(
      `/chat/mentions?${new URLSearchParams({ kind, q: query, cursor })}`,
      { signal },
    ),
  chat: (message: string, task?: string, attachments?: ChatAttachment[], seg?: number) =>
    post<{ reply: string; mode: string }>(`/chat${tq(task)}`, { message, attachments, seg }),
  chatStatus: (taskId: string) => get<{ running: boolean }>(`/tasks/${taskId}/chat/status`),
  // Option 1, file upload: lands in the session/task working directory uploads/, returning a relative path the agent can Read.
  chatUpload: async (scope: "task" | "session" | "staging", id: string, files: File[]) => {
    const totalSize = files.reduce((total, file) => total + file.size, 0);
    if (totalSize > MAX_CHAT_UPLOAD_BYTES) {
      throw new Error(t("Files exceed the 512 MiB upload limit (combined size)."));
    }
    if (MOCK)
      return {
        attachments: files.map((f) => ({
          name: f.name,
          path: `uploads/${f.name}`,
          size: f.size,
          abs: `/mock/${f.name}`,
        })),
      };
    const fd = new FormData();
    for (const f of files) fd.append("file", f);
    const token = getToken();
    const r = await fetch(`/api/chat/upload?scope=${scope}&id=${encodeURIComponent(id)}`, {
      method: "POST",
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      body: fd,
    });
    if (!r.ok) throw new Error(t("Upload failed: {error}", { error: `${r.status} ${await r.text()}` }));
    return r.json() as Promise<{ attachments: ChatAttachment[] }>;
  },
  stopChat: (taskId: string) => post<{ status: string }>(`/tasks/${taskId}/chat/stop`, {}),
  gc: (ttl = 86400) => post<{ removed: number }>(`/gc?ttl=${ttl}`, {}),
};
