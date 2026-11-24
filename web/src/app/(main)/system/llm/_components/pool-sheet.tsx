"use client";

import * as React from "react";

import { RotateCcwIcon, ZapIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import { setVisibleInterval } from "@/lib/polling";
import type { LLMPoolMember, LLMPoolStatus, LLMProfile } from "@/lib/types";
import { cn } from "@/lib/utils";

import { rich } from "@/lib/rich-text";

function cooldownText(secs: number) {
  if (secs <= 0) return "";
  if (secs < 60) return t("{n}s", { n: secs });
  return t("{n}min", { n: Math.ceil(secs / 60) });
}

// The "healthy or not" badge a profile shows on its card. A profile without a Key cannot send any request at all — say that before the breaker;
// the exception is Ollama, a keyless local server that IS callable without a key. The other states come from the pool's breaker
// records (with polling off no new records appear, so "healthy" = no known failure).
export type Health = { label: string; cls: string; hint?: string };
export function healthOf(p: LLMProfile, m?: LLMPoolMember): Health {
  if (!p.api_key_hint && p.format !== "ollama") {
    return {
      label: t("No Key configured"),
      cls: "border-muted-foreground/40 text-muted-foreground",
      hint: t("No API Key set; cannot be called"),
    };
  }
  if (m?.state === "tripped") {
    return {
      label:
        m.cooldown_secs > 0 ? t("Tripped · {cooldown}", { cooldown: cooldownText(m.cooldown_secs) }) : t("Tripped"),
      cls: "border-destructive/50 text-destructive",
      hint: m.last_error,
    };
  }
  if (m?.state === "degraded") {
    return {
      label: t("Degraded · failed {count} times", { count: m.fails }),
      cls: "border-amber-500/50 text-amber-600 dark:text-amber-400",
      hint: m.last_error,
    };
  }
  return { label: t("Healthy"), cls: "border-emerald-500/50 text-emerald-600 dark:text-emerald-400" };
}

// ─────────────────────────────────────────────────────────────────────────────
// Pool settings drawer
// ─────────────────────────────────────────────────────────────────────────────

export function PoolSheet({
  open,
  onOpenChange,
  pool,
  onReload,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  pool: LLMPoolStatus | null;
  onReload: () => Promise<void>;
}) {
  // Pool settings and recovery are PUT /api/settings and POST /api/llm/... (admin).
  const canAdmin = useCan("admin");
  const [busy, setBusy] = React.useState(false);

  // The cooldown countdown is the remaining seconds computed by the backend — fetched on a timer only while the drawer is open and some profile is unhealthy, to keep it ticking.
  React.useEffect(() => {
    if (!open || !pool?.enabled || !pool.chain.some((m) => m.state !== "ok")) return;
    const stopPolling = setVisibleInterval(() => void onReload(), 10_000);
    return stopPolling;
  }, [open, pool, onReload]);

  async function toggle(patch: { llm_pool_enabled?: boolean; llm_pool_bind_fallback?: boolean }) {
    if (busy) return;
    setBusy(true);
    try {
      await api.setSettings(patch);
      await onReload();
      if (patch.llm_pool_enabled !== undefined) {
        toast.success(patch.llm_pool_enabled ? t("LLM pool enabled") : t("LLM pool disabled"));
      } else {
        toast.success(t("Fallback settings updated"));
      }
    } catch (e) {
      toast.error(t("Failed to apply settings: {error}", { error: errorText(e) }));
    } finally {
      setBusy(false);
    }
  }

  async function recover(id?: string) {
    try {
      await api.resetLLMPool(id);
      await onReload();
      toast.success(id ? t("Profile recovered") : t("All profiles recovered"));
    } catch (e) {
      toast.error(t("Recovery failed: {error}", { error: errorText(e) }));
    }
  }

  const enabled = pool?.enabled ?? false;
  const chain = pool?.chain ?? [];
  // Members taking part in the pool (excluding those flagged "excluded from pool"), in the exact order the backend tries them.
  const inChain = chain.filter((m) => m.active || !m.excluded);
  const tripped = chain.filter((m) => m.state === "tripped");

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex flex-col gap-0 p-0 data-[side=right]:sm:max-w-lg">
        <SheetHeader className="px-4">
          <SheetTitle className="flex items-center gap-2">
            <ZapIcon className="size-4" /> {t("LLM Pool · Failover")}
          </SheetTitle>
          <SheetDescription>
            {rich(
              t(
                "When enabled, an Agent <b>without a pinned model</b> automatically fails over to the next config when the current one is unusable (insufficient balance / dead Key / rate limiting / service error).",
              ),
            )}
          </SheetDescription>
        </SheetHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 pb-6">
          <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
            <div className="grid gap-0.5">
              <Label className="text-sm">{t("Enable pool")}</Label>
              <p className="text-muted-foreground text-xs">
                {t("Off by default. When off, only the active profile is ever used and a failure stays a failure.")}
              </p>
            </div>
            <Switch
              checked={enabled}
              disabled={busy || !canAdmin}
              onCheckedChange={(v) => void toggle({ llm_pool_enabled: v })}
              aria-label={t("LLM pool toggle")}
            />
          </div>

          {enabled && (
            <>
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
                <div className="grid gap-0.5">
                  <Label className="text-sm">{t("Also fall back when a pinned config fails")}</Label>
                  <p className="text-muted-foreground text-xs">
                    {t(
                      "Off by default: when an Agent or task pins a config, only that config is used and a failure stays a failure (no silent switch to another model). When enabled, a failing pinned config also falls back to the pool chain below.",
                    )}
                  </p>
                </div>
                <Switch
                  checked={pool?.bind_fallback ?? false}
                  disabled={busy || !canAdmin}
                  onCheckedChange={(v) => void toggle({ llm_pool_bind_fallback: v })}
                  aria-label={t("Bound-config failure fallback toggle")}
                />
              </div>

              <Separator />

              <div className="grid gap-2">
                <div className="flex items-center justify-between">
                  <Label className="text-sm">{t("Pool order")}</Label>
                  {canAdmin && tripped.length > 0 && (
                    <Button size="sm" variant="ghost" onClick={() => void recover()}>
                      <RotateCcwIcon /> {t("Recover all")}
                    </Button>
                  )}
                </div>
                {inChain.length < 2 && (
                  <p className="text-muted-foreground text-xs">
                    {t(
                      "Only {count} usable profile(s) right now, so the pool has no effect — at least 2 usable profiles (API Key set, or Ollama) with pool participation are required.",
                      { count: inChain.length },
                    )}
                  </p>
                )}
                {chain.map((m) => {
                  const excluded = m.excluded && !m.active;
                  const order = excluded ? null : inChain.findIndex((x) => x.profile_id === m.profile_id) + 1;
                  return (
                    <div
                      key={m.profile_id}
                      className={cn(
                        "grid gap-1 rounded-lg border p-2.5 text-sm",
                        excluded && "opacity-55",
                        m.state === "tripped" && "border-destructive/40",
                      )}
                    >
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="w-5 shrink-0 text-center font-mono text-muted-foreground text-xs">
                          {order ?? "—"}
                        </span>
                        <span className="font-medium">{m.name}</span>
                        {m.active && (
                          <Badge variant="outline" className="border-amber-400/50 text-amber-500">
                            {t("Active")}
                          </Badge>
                        )}
                        {excluded && <Badge variant="outline">{t("Excluded from pool")}</Badge>}
                        <div className="ml-auto flex items-center gap-2">
                          {m.state === "tripped" && m.cooldown_secs > 0 && (
                            <span className="text-muted-foreground text-xs">
                              {t("Cooldown {time}", { time: cooldownText(m.cooldown_secs) })}
                            </span>
                          )}
                          {m.state === "degraded" && (
                            <span className="text-muted-foreground text-xs">
                              {t("{count} consecutive failures", { count: m.fails })}
                            </span>
                          )}
                          {canAdmin && m.state !== "ok" && (
                            <Button
                              size="icon"
                              variant="ghost"
                              className="size-7"
                              aria-label={t("Recover now")}
                              title={t("Recover now: clears the breaker so the next call retries this profile")}
                              onClick={() => void recover(m.profile_id)}
                            >
                              <RotateCcwIcon className="size-3.5" />
                            </Button>
                          )}
                        </div>
                      </div>
                      <div className="flex flex-wrap items-center gap-x-3 pl-7 text-muted-foreground text-xs">
                        <code className="truncate font-mono">{m.model}</code>
                        {!m.active && <span>{t("Priority {priority}", { priority: m.priority })}</span>}
                      </div>
                      {m.last_error && (
                        <p className="truncate pl-7 font-mono text-muted-foreground text-xs" title={m.last_error}>
                          {m.last_error}
                        </p>
                      )}
                    </div>
                  );
                })}
                {chain.length === 0 && (
                  <div className="rounded-lg border border-dashed p-4 text-center text-muted-foreground text-sm">
                    {t("No profiles yet")}
                  </div>
                )}
              </div>

              <div className="rounded-lg border border-dashed p-3 text-muted-foreground text-xs leading-relaxed">
                {t(
                  "The active profile is always first; the rest follow priority from high to low (set per profile). After a failure a profile enters a cooldown (60s → 5min → 30min), is skipped while cooling down, and switches back automatically once recovered. Profiles whose context window cannot fit the current request are skipped. Agents and tasks with a pinned model do not join the pool by default.",
                )}
              </div>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
