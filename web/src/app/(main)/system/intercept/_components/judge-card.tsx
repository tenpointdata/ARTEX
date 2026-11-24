"use client";

import * as React from "react";

import { BotIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useCan } from "@/hooks/use-session";
import { intlLocale, t } from "@/i18n";
import { api } from "@/lib/api";
import type { JudgeConfig, JudgeDayUsage, JudgeUsage, LLMProfile } from "@/lib/types";

import { Field, tRich } from "./shared";

// fmtTokens compacts token counts (1.2k / 3.4M) for approval usage stats.
function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "k";
  return String(n);
}

// JudgeStat is one stat tile (label + value).
function JudgeStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-muted/20 px-3 py-2">
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className="mt-0.5 text-lg font-semibold tabular-nums">{value}</div>
    </div>
  );
}

// JudgeSparkbars draws a mini bar chart of daily consumption (input+output) over the last N days with plain divs, avoiding a chart library dependency.
function JudgeSparkbars({ daily }: { daily: JudgeDayUsage[] }) {
  const max = Math.max(1, ...daily.map((d) => d.input_tokens + d.output_tokens));
  return (
    <div className="flex h-16 items-end gap-0.5">
      {daily.map((d) => {
        const total = d.input_tokens + d.output_tokens;
        const h = Math.max(2, Math.round((total / max) * 100));
        return (
          <div
            key={d.date}
            title={t("{date} · {calls} calls · {tokens} tokens", {
              date: d.date,
              calls: d.calls,
              tokens: fmtTokens(total),
            })}
            className="min-w-[2px] flex-1 rounded-sm bg-violet-500/60 hover:bg-violet-500"
            style={{ height: `${h}%` }}
          />
        );
      })}
    </div>
  );
}

// ---- LLM fallback judge card ----

const FOLLOW_ACTIVE = "0"; // profile_id 0 = follow the active/default profile

const defaultJudge = (): JudgeConfig => ({
  enabled: false,
  mode: "closed",
  profile_id: 0,
  prompt: "",
  timeout_seconds: 15,
  fail_action: "ask",
  ask_timeout_seconds: 300,
  ask_timeout_action: "deny",
});

export function JudgeCard() {
  // PUT /api/intercept/judge is admin; other roles see the configuration and usage read-only.
  const canAdmin = useCan("admin");
  const [cfg, setCfg] = React.useState<JudgeConfig>(defaultJudge());
  const [profiles, setProfiles] = React.useState<LLMProfile[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [usage, setUsage] = React.useState<JudgeUsage | null>(null);

  // Approval usage stats: failures must not break the config page; fetched only when enabled.
  const loadUsage = React.useCallback(async () => {
    try {
      setUsage(await api.interceptJudgeUsage(30));
    } catch {
      // Ignore: stats being unavailable must not affect config editing
    }
  }, []);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [j, ps] = await Promise.all([api.interceptGetJudgeConfig(), api.llmProfiles()]);
      setCfg(j);
      setProfiles(ps);
    } catch (e) {
      toast.error(t("Failed to load model fallback config: {error}", { error: (e as Error).message }));
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    load();
  }, [load]);

  // Once enabled (including when the first load reads the switch as true), fetch approval usage stats.
  React.useEffect(() => {
    if (cfg.enabled) loadUsage();
  }, [cfg.enabled, loadUsage]);

  function patch(p: Partial<JudgeConfig>) {
    setCfg((c) => ({ ...c, ...p }));
  }

  async function save() {
    setSaving(true);
    try {
      await api.interceptSetJudgeConfig(cfg);
      toast.success(t("Model fallback config saved"));
      await load(); // read back: if the prompt was cleared, the built-in template is refilled
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    } finally {
      setSaving(false);
    }
  }

  async function restorePrompt() {
    // Clear the prompt and save → the server then returns the full built-in template, refilled into the input.
    setSaving(true);
    try {
      await api.interceptSetJudgeConfig({ ...cfg, prompt: "" });
      const j = await api.interceptGetJudgeConfig();
      setCfg(j);
      toast.success(t("Built-in default template restored"));
    } catch (e) {
      toast.error(t("Restore failed: {error}", { error: (e as Error).message }));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      {/* Enable switch — standalone highlighted bar */}
      <div
        className={`flex items-center justify-between gap-3 rounded-lg border px-4 py-3 ${
          cfg.enabled ? "border-violet-400/50 bg-violet-50/40 dark:bg-violet-950/20" : "bg-muted/40"
        }`}
      >
        <div className="flex items-center gap-2.5">
          <BotIcon className={`h-5 w-5 shrink-0 ${cfg.enabled ? "text-violet-600" : "text-muted-foreground"}`} />
          <div>
            <p className="text-sm font-semibold leading-tight">{t("Model fallback approval")}</p>
            <p className="text-xs text-muted-foreground mt-0.5">
              {tRich(
                "Only commands inside the {scope} that {norule} are semantically judged by the model (allow / escalate to human / block)",
                {
                  scope: <span className="font-medium text-foreground">{t("interception scope")}</span>,
                  norule: <span className="font-medium text-foreground">{t("match no interception rule")}</span>,
                },
              )}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <span className="text-xs text-muted-foreground">{cfg.enabled ? t("Enabled") : t("Disabled")}</span>
          <Switch
            checked={cfg.enabled}
            disabled={loading || !canAdmin}
            onCheckedChange={(v) => patch({ enabled: v })}
          />
        </div>
      </div>

      {/* Approval token usage stats (global cumulative, independent of per-model configs) */}
      {cfg.enabled && usage && (
        <Card>
          <CardContent className="p-4">
            <div className="mb-3 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">{t("Approval token consumption")}</p>
                <p className="text-xs text-muted-foreground">
                  {t(
                    "Cumulative model fallback approval usage, metered separately (worker=judge), not mixed with per-model config stats",
                  )}
                </p>
              </div>
              <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={loadUsage}>
                {t("Refresh")}
              </Button>
            </div>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
              <JudgeStat label={t("Approval calls")} value={usage.calls.toLocaleString(intlLocale)} />
              <JudgeStat label={t("Input tokens")} value={fmtTokens(usage.input_tokens)} />
              <JudgeStat label={t("Output tokens")} value={fmtTokens(usage.output_tokens)} />
              <JudgeStat label={t("Cache read")} value={fmtTokens(usage.cache_read_tokens)} />
              <JudgeStat label={t("Cache write")} value={fmtTokens(usage.cache_write_tokens)} />
            </div>
            {usage.daily.length > 0 && (
              <div className="mt-4">
                <p className="mb-2 text-[10px] uppercase tracking-wider text-muted-foreground">
                  {t("Daily consumption over the last 30 days (input + output)")}
                </p>
                <JudgeSparkbars daily={usage.daily} />
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {cfg.enabled && (
        <fieldset disabled={!canAdmin} className="grid min-w-0 gap-4 lg:grid-cols-5">
          {/* Left: prompt editor (expanded inline, main area) */}
          <Card className="lg:col-span-3">
            <CardContent className="flex h-full flex-col gap-2 p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">{t("Approval prompt")}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("The model decides ALLOW / ASK / DENY based on this; directly editable")}
                  </p>
                </div>
                {canAdmin && (
                  <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={restorePrompt} disabled={saving}>
                    {t("Restore default template")}
                  </Button>
                )}
              </div>
              <Textarea
                className="min-h-[22rem] flex-1 resize-none font-mono text-xs leading-relaxed"
                value={cfg.prompt}
                readOnly={!canAdmin}
                onChange={(e) => patch({ prompt: e.target.value })}
                placeholder={t("Leave empty to use the built-in template")}
                spellCheck={false}
              />
              <p className="text-right text-[11px] text-muted-foreground">
                {t("{count} chars", { count: cfg.prompt.length })}
              </p>
            </CardContent>
          </Card>

          {/* Right: decision parameters (settings column) */}
          <Card className="lg:col-span-2">
            <CardContent className="space-y-5 p-4">
              <div className="space-y-4">
                <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  {t("Judge model and policy")}
                </p>
                <Field label={t("Approval model")}>
                  <Select value={String(cfg.profile_id || 0)} onValueChange={(v) => patch({ profile_id: Number(v) })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={FOLLOW_ACTIVE}>{t("Follow active profile")}</SelectItem>
                      {profiles.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name} ({p.model})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field label={t("Judge mode")}>
                  <Select value={cfg.mode ?? "closed"} onValueChange={(v) => patch({ mode: v as JudgeConfig["mode"] })}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="closed">
                        {t("Closed questions (code decides; prompt is the operator policy; failure asks)")}
                      </SelectItem>
                      <SelectItem value="freeform">{t("Free-form prompt (model decides)")}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
                <Field label={t("Model judge timeout (seconds)")}>
                  <Input
                    type="number"
                    min={1}
                    value={cfg.timeout_seconds}
                    onChange={(e) => {
                      const n = parseInt(e.target.value, 10);
                      if (n > 0) patch({ timeout_seconds: n });
                    }}
                  />
                </Field>
                <Field label={t("When the model fails (error / timeout / unparsable)")}>
                  <Select
                    value={cfg.fail_action}
                    onValueChange={(v) => patch({ fail_action: v as JudgeConfig["fail_action"] })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="allow">{t("Allow")}</SelectItem>
                      <SelectItem value="ask">{t("Escalate to human approval")}</SelectItem>
                      <SelectItem value="deny">{t("Block")}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              </div>

              <Separator />

              <div className="space-y-4">
                <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  {t('Human approval (when the model decides "ask")')}
                </p>
                <Field label={t("Approval wait timeout (seconds)")}>
                  <Input
                    type="number"
                    min={5}
                    value={cfg.ask_timeout_seconds}
                    onChange={(e) => {
                      const n = parseInt(e.target.value, 10);
                      if (n > 0) patch({ ask_timeout_seconds: n });
                    }}
                  />
                </Field>
                <Field label={t("Default action after timeout")}>
                  <Select
                    value={cfg.ask_timeout_action}
                    onValueChange={(v) => patch({ ask_timeout_action: v as JudgeConfig["ask_timeout_action"] })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="deny">{t("Block")}</SelectItem>
                      <SelectItem value="allow">{t("Allow")}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              </div>
            </CardContent>
          </Card>
        </fieldset>
      )}

      {canAdmin && (
        <div className="flex justify-end">
          <Button size="sm" onClick={save} disabled={saving || loading}>
            {saving ? t("Saving…") : t("Save config")}
          </Button>
        </div>
      )}
    </div>
  );
}

// ---- page ----
