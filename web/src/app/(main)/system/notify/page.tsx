"use client";

import * as React from "react";

import { BellIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { NotificationChannel, NotificationFilter, NotificationMeta } from "@/lib/types";

import {
  CHANNEL_FIELDS,
  type ChannelForm,
  emptyForm,
  KIND_LABEL,
  parseIDs,
  parseKeywords,
  parseKV,
} from "./_components/channel-fields";
import { FilterSummary } from "./_components/channel-form";
import { ChannelSheet } from "./_components/channel-sheet";
import { DeliveryList } from "./_components/delivery-list";
import {
  ASSETS_LIMIT_MAX,
  GlobalSettingsCard,
  limitInvalid,
  SUMMARY_LIMIT_MAX,
} from "./_components/global-settings-card";
import { formatBacklog, StatTile } from "./_components/stat-tile";
// Upper bounds of the two message render limits. They mirror the backend, which answers 400 outside [0, max].

// This page only orchestrates: loading data, holding form state, calling the API.
// Field definitions and parsing live in _components/channel-fields.ts, controls and the filter summary in
// _components/channel-form.tsx, and the delivery log in _components/delivery-list.tsx —
// they are split out because each stays readable on its own, whereas crammed into one file this page neared 1100 lines.
export default function NotifyPage() {
  const [meta, setMeta] = React.useState<NotificationMeta | null>(null);
  const [channels, setChannels] = React.useState<NotificationChannel[]>([]);
  const [tab, setTab] = React.useState<"channels" | "deliveries">("channels");

  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<NotificationChannel | null>(null);
  const [form, setForm] = React.useState<ChannelForm>(emptyForm("dingtalk"));
  const [saving, setSaving] = React.useState(false);
  const [testing, setTesting] = React.useState(false);

  const [globalSaving, setGlobalSaving] = React.useState(false);
  const [baseURL, setBaseURL] = React.useState("");
  const [digestMin, setDigestMin] = React.useState("");
  const [summaryMax, setSummaryMax] = React.useState("");
  const [assetsMax, setAssetsMax] = React.useState("");

  const load = React.useCallback(() => {
    api
      .notifyMeta()
      .then((m) => {
        setMeta(m);
        setBaseURL(m.public_base_url);
        setDigestMin(m.digest_interval_min);
      })
      .catch((e) => toast.error(t("Failed to load notification settings: {error}", { error: (e as Error).message })));
    // The two render limits are not part of /notify/meta; they are only exposed through /api/settings.
    api
      .settings()
      .then((s) => {
        setSummaryMax(String(s.notify_summary_max_runes ?? ""));
        setAssetsMax(String(s.notify_assets_max_shown ?? ""));
      })
      .catch((e) => toast.error(t("Failed to load message limits: {error}", { error: (e as Error).message })));
    // Channel-list load failures must be reported: failing silently would render as "no channels at all",
    // and users would think their config was lost — scarier than a straight-up error.
    api
      .notifyChannels()
      .then(setChannels)
      .catch((e) => toast.error(t("Failed to load channels: {error}", { error: (e as Error).message })));
  }, []);
  React.useEffect(() => {
    load();
  }, [load]);

  function setF(patch: Partial<ChannelForm>) {
    setForm((f) => ({ ...f, ...patch }));
  }
  function setCfg(key: string, value: unknown) {
    setForm((f) => ({ ...f, config: { ...f.config, [key]: value } }));
  }

  function openAdd() {
    setEditing(null);
    setForm(emptyForm(meta?.kinds[0]?.kind ?? "dingtalk"));
    setOpen(true);
  }

  function openEdit(ch: NotificationChannel) {
    setEditing(ch);
    // filter is a Go struct on the backend and always serializes as an object (never null), so no fallback is needed.
    const f = ch.filter;
    setForm({
      name: ch.name,
      kind: ch.kind,
      mode: ch.mode,
      enabled: ch.enabled,
      ratePerMin: String(ch.rate_per_min),
      // Credentials in the backend-echoed config are masked values; put them into the form as-is and send them back as-is on submit,
      // which tells the backend to keep the stored values.
      config: { ...ch.config },
      minSeverity: f.min_severity ?? "",
      includeText: (f.vulnclass_include ?? []).join("\n"),
      excludeText: (f.vulnclass_exclude ?? []).join("\n"),
      taskIDsText: (f.task_ids ?? []).join(","),
      assetIDsText: (f.asset_ids ?? []).join(","),
      onStatusChange: f.on_status_change ?? false,
    });
    setOpen(true);
  }

  // buildConfig turns form state into the channel config.
  //
  // One rule, two kinds of values:
  //   - masked values ("__masked__...") are sent back verbatim → the backend reads this as "field unchanged, keep the stored value"
  //   - everything else is submitted as user input, where an empty string means "clear this field"
  //
  // Credential fields get no special treatment (no "skip when empty") because that would make it **impossible** to clear
  // a mis-set key — no action in the UI could express "I want this gone". Under the current rule,
  // emptying the input equals clearing the field: one unambiguous, user-controlled semantic.
  // Masked values never appear in inputs (see ConfigField), so "text in the box" always means
  // "the user typed it".
  function buildConfig(): Record<string, unknown> {
    const defs = CHANNEL_FIELDS[form.kind] ?? [];
    const out: Record<string, unknown> = {};
    for (const d of defs) {
      const raw = form.config[d.key];
      if (d.kind === "switch") {
        out[d.key] = raw === true;
        continue;
      }
      if (typeof raw === "string" && raw.startsWith("__masked__")) {
        out[d.key] = raw;
        continue;
      }
      if (d.kind === "number") {
        const n = Number(raw);
        out[d.key] = Number.isFinite(n) && n > 0 ? n : 0;
        continue;
      }
      if (d.kind === "kv") {
        out[d.key] = parseKV(String(raw ?? ""));
        continue;
      }
      if (d.kind === "list") {
        out[d.key] = String(raw ?? "")
          .split(/[\s,]+/)
          .map((s) => s.trim())
          .filter(Boolean);
        continue;
      }
      out[d.key] = String(raw ?? "").trim();
    }
    return out;
  }

  function buildFilter(): NotificationFilter {
    return {
      min_severity: form.minSeverity || undefined,
      vulnclass_include: parseKeywords(form.includeText),
      vulnclass_exclude: parseKeywords(form.excludeText),
      task_ids: parseIDs(form.taskIDsText),
      asset_ids: parseIDs(form.assetIDsText),
      on_status_change: form.onStatusChange,
    };
  }

  async function saveForm() {
    if (!form.name.trim()) {
      toast.error(t("Please fill in a channel name"));
      return;
    }
    setSaving(true);
    try {
      const payload = {
        name: form.name.trim(),
        kind: form.kind,
        mode: form.mode,
        enabled: form.enabled,
        config: buildConfig(),
        filter: buildFilter(),
        rate_per_min: form.ratePerMin.trim() === "" ? undefined : Number(form.ratePerMin),
      };
      if (editing) {
        await api.notifyUpdateChannel(editing.id, payload);
        toast.success(t("Saved"));
        setOpen(false);
      } else {
        await api.notifyCreateChannel(payload);
        toast.success(t("Channel added"));
        setOpen(false);
      }
      load();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    } finally {
      setSaving(false);
    }
  }

  async function testChannel() {
    if (!editing) return;
    setTesting(true);
    try {
      const r = await api.notifyTestChannel(editing.id);
      toast.success(t("Test message sent ({ms} ms); check the group to confirm", { ms: r.latency_ms }));
    } catch (e) {
      // The backend relays the raw error returned by the channel verbatim — the only clue for debugging the config — so show it as-is.
      toast.error(t("Test failed: {error}", { error: (e as Error).message }), { duration: 12000 });
    } finally {
      setTesting(false);
    }
  }

  async function removeChannel(ch: NotificationChannel) {
    try {
      await api.notifyDeleteChannel(ch.id);
      toast.success(t("Deleted: {name}", { name: ch.name }));
      setOpen(false);
      load();
    } catch (e) {
      toast.error(t("Delete failed: {error}", { error: (e as Error).message }));
    }
  }

  async function toggleEnabled(ch: NotificationChannel) {
    try {
      await api.notifyUpdateChannel(ch.id, { enabled: !ch.enabled });
      load();
    } catch (e) {
      toast.error(t("Operation failed: {error}", { error: (e as Error).message }));
    }
  }

  async function toggleGlobal(on: boolean) {
    setGlobalSaving(true);
    try {
      await api.setSettings({ notify_enabled: on });
      setMeta((m) => (m ? { ...m, enabled: on } : m));
      toast.success(on ? t("Push enabled") : t("Push paused"));
    } catch (e) {
      toast.error(t("Operation failed: {error}", { error: (e as Error).message }));
    } finally {
      setGlobalSaving(false);
    }
  }

  async function saveGlobal() {
    // Checked before anything is sent: the backend applies the fields of one PUT in order,
    // so a 400 on a limit would leave the URL and digest interval already saved.
    if (limitInvalid(summaryMax, SUMMARY_LIMIT_MAX)) {
      toast.error(t("Summary length must be a whole number between 0 and {max}", { max: SUMMARY_LIMIT_MAX }));
      return;
    }
    if (limitInvalid(assetsMax, ASSETS_LIMIT_MAX)) {
      toast.error(t("Assets listed must be a whole number between 0 and {max}", { max: ASSETS_LIMIT_MAX }));
      return;
    }
    setGlobalSaving(true);
    try {
      const patch: Record<string, unknown> = { notify_public_base_url: baseURL.trim() };
      const n = Number(digestMin);
      if (Number.isFinite(n) && n > 0) patch.notify_digest_interval_min = n;
      if (summaryMax.trim() !== "") patch.notify_summary_max_runes = Number(summaryMax);
      if (assetsMax.trim() !== "") patch.notify_assets_max_shown = Number(assetsMax);
      await api.setSettings(patch);
      toast.success(t("Saved"));
      load();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    } finally {
      setGlobalSaving(false);
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("Notifications")}</h1>
          <p className="text-muted-foreground text-sm">
            {t(
              "Push findings to DingTalk / Feishu / WeCom and other channels · each channel has its own delivery timing and filter rules",
            )}
          </p>
        </div>
        {meta && (
          // A div, not a label: the Switch carries its own aria-label, and wrapping it in a label
          // would associate it with no native control while making the text look like it should toggle.
          <div className="flex shrink-0 items-center gap-2 text-sm">
            <span className="text-muted-foreground">{t("Master switch")}</span>
            <Switch
              checked={meta.enabled}
              disabled={globalSaving}
              onCheckedChange={toggleGlobal}
              aria-label={t("Push master switch")}
            />
          </div>
        )}
      </div>

      {meta && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          <StatTile
            label={t("Channels")}
            value={`${meta.stats.channels_on} / ${meta.stats.channels}`}
            hint={t("On / total")}
          />
          <StatTile label={t("Sent today")} value={String(meta.stats.sent_today)} />
          <StatTile label={t("Pending")} value={String(meta.stats.pending)} />
          <StatTile
            label={t("Failed")}
            value={String(meta.stats.failed)}
            tone={meta.stats.failed > 0 ? "red" : undefined}
          />
          <StatTile
            label={t("Oldest backlog")}
            value={formatBacklog(meta.stats.backlog_age_ms)}
            // Backlog age is far more useful than backlog count: 3 backlogged items could mean 3 seconds or 3 hours.
            hint={meta.stats.backlog_age_ms > 5 * 60_000 ? t("Push may be stuck") : undefined}
            tone={meta.stats.backlog_age_ms > 5 * 60_000 ? "red" : undefined}
          />
        </div>
      )}

      <GlobalSettingsCard
        baseURL={baseURL}
        setBaseURL={setBaseURL}
        digestMin={digestMin}
        setDigestMin={setDigestMin}
        summaryMax={summaryMax}
        setSummaryMax={setSummaryMax}
        assetsMax={assetsMax}
        setAssetsMax={setAssetsMax}
        globalSaving={globalSaving}
        saveGlobal={saveGlobal}
      />

      <Tabs value={tab} onValueChange={(v) => setTab(v as "channels" | "deliveries")} className="flex flex-col gap-4">
        <TabsList>
          <TabsTrigger value="channels">{t("Channels")}</TabsTrigger>
          <TabsTrigger value="deliveries">{t("Delivery log")}</TabsTrigger>
        </TabsList>

        <TabsContent value="channels">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <button
              type="button"
              onClick={openAdd}
              className="text-foreground/70 border-foreground/70 hover:bg-muted/60 hover:shadow-sm flex min-h-[130px] flex-col items-center justify-center gap-2 rounded-xl border border-dashed transition"
            >
              <PlusIcon className="size-6" />
              <span className="text-sm">{t("Add channel")}</span>
            </button>

            {channels.map((ch) => (
              <Card
                key={ch.id}
                onClick={() => openEdit(ch)}
                className="hover:border-primary/60 cursor-pointer gap-3 transition hover:shadow-sm"
              >
                <CardHeader>
                  <div className="flex items-center gap-2">
                    <BellIcon className="text-muted-foreground size-4 shrink-0" />
                    <CardTitle className="truncate text-base">{ch.name}</CardTitle>
                    {/* The whole card is clickable (opens the editor), so these two controls must each swallow the bubbling click,
                        otherwise toggling/deleting would also trigger editing. stopPropagation hangs off the controls themselves
                        rather than a wrapping div: a div wrapper would create a static element that "looks interactive but has no
                        role", tripping a11y warnings and making no semantic sense either. */}
                    <div className="ml-auto flex items-center gap-2">
                      <Switch
                        checked={ch.enabled}
                        onCheckedChange={() => toggleEnabled(ch)}
                        onClick={(e) => e.stopPropagation()}
                        aria-label={t("Enable")}
                      />
                      <Button
                        size="icon"
                        variant="outline"
                        aria-label={t("Delete")}
                        onClick={(e) => {
                          e.stopPropagation();
                          // void explicitly discards the Promise: removeChannel catches and toasts on its own,
                          // so no await is needed here (onClick is not async).
                          void removeChannel(ch);
                        }}
                      >
                        <Trash2Icon className="text-destructive" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="grid gap-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">{KIND_LABEL[ch.kind] ?? ch.kind}</Badge>
                    <Badge variant="outline">{ch.mode === "digest" ? t("Digest") : t("Realtime")}</Badge>
                    {!ch.enabled && <Badge variant="outline">{t("Disabled")}</Badge>}
                  </div>
                  <FilterSummary filter={ch.filter} />
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        <TabsContent value="deliveries">
          <DeliveryList channels={channels} />
        </TabsContent>
      </Tabs>

      <ChannelSheet
        open={open}
        setOpen={setOpen}
        editing={editing}
        form={form}
        setF={setF}
        setCfg={setCfg}
        meta={meta}
        saving={saving}
        testing={testing}
        saveForm={saveForm}
        testChannel={testChannel}
      />
    </div>
  );
}
