"use client";

import { SendIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import type { NotificationChannel, NotificationMeta } from "@/lib/types";

import { CHANNEL_FIELDS, type ChannelForm, KIND_LABEL, SEVERITY_OPTIONS } from "./channel-fields";
import { ConfigField } from "./channel-form";

export interface ChannelSheetProps {
  open: boolean;
  setOpen: (open: boolean) => void;
  editing: NotificationChannel | null;
  form: ChannelForm;
  setF: (patch: Partial<ChannelForm>) => void;
  setCfg: (key: string, value: unknown) => void;
  meta: NotificationMeta | null;
  saving: boolean;
  testing: boolean;
  saveForm: () => void;
  testChannel: () => void;
}

export function ChannelSheet({
  open,
  setOpen,
  editing,
  form,
  setF,
  setCfg,
  meta,
  saving,
  testing,
  saveForm,
  testChannel,
}: ChannelSheetProps) {
  const canAdmin = useCan("admin"); // channel changes and test sends are admin (server/rbac_policy.go)
  const fields = CHANNEL_FIELDS[form.kind] ?? [];
  const secretKeys = new Set(meta?.kinds.find((k) => k.kind === form.kind)?.secret_keys ?? []);
  const defaultRate = meta?.kinds.find((k) => k.kind === form.kind)?.default_rate_per_min ?? 0;

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent side="right" className="w-full data-[side=right]:sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{editing ? editing.name : t("Add notification channel")}</SheetTitle>
          <SheetDescription>
            {KIND_LABEL[form.kind] ?? form.kind}
            {" · "}
            {defaultRate > 0 ? t("default rate limit {rate}/min", { rate: defaultRate }) : t("no rate limit")}
          </SheetDescription>
        </SheetHeader>

        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4">
          <fieldset disabled={!canAdmin} className="grid min-w-0 gap-4 py-4">
            <div className="grid gap-2">
              <Label>{t("Channel type")}</Label>
              <Select
                value={form.kind}
                onValueChange={(v) => {
                  // Switching type means switching to a different set of credential fields; the old config cannot be merged in.
                  setF({ kind: v, config: {} });
                }}
                disabled={!!editing}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(meta?.kinds ?? []).map((k) => (
                    <SelectItem key={k.kind} value={k.kind}>
                      {KIND_LABEL[k.kind] ?? k.kind}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {editing && (
                <p className="text-muted-foreground text-xs">
                  {t(
                    "The channel type cannot be changed — a different type means a different set of credentials; create a new channel instead.",
                  )}
                </p>
              )}
            </div>

            <div className="grid gap-2">
              <Label htmlFor="n-name">{t("Channel name")}</Label>
              <Input
                id="n-name"
                placeholder={t("Incident response group / Daily broadcast group")}
                value={form.name}
                onChange={(e) => setF({ name: e.target.value })}
              />
            </div>

            {fields.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t(
                  "The form for this channel type is not defined yet (the frontend is missing a CHANNEL_FIELDS entry); add it and retry.",
                )}
              </p>
            ) : (
              fields.map((d) => (
                <ConfigField
                  key={d.key}
                  def={d}
                  value={form.config[d.key]}
                  isSecret={secretKeys.has(d.key)}
                  onChange={(v) => setCfg(d.key, v)}
                />
              ))
            )}

            <div className="grid gap-2">
              <Label>{t("Delivery timing")}</Label>
              <Select value={form.mode} onValueChange={(v) => setF({ mode: v as "realtime" | "digest" })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="realtime">{t("Realtime · one message per finding")}</SelectItem>
                  <SelectItem value="digest">{t("Digest · merged into one message per interval")}</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-muted-foreground text-xs">
                {t(
                  'For "realtime for High, digest for the rest", create two channels: one realtime with a High threshold, one digest with no severity limit.',
                )}
              </p>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="n-rate">{t("Rate limit (messages/min)")}</Label>
              <Input
                id="n-rate"
                type="number"
                min={0}
                placeholder={defaultRate > 0 ? String(defaultRate) : t("0 = unlimited")}
                value={form.ratePerMin}
                onChange={(e) => setF({ ratePerMin: e.target.value })}
              />
              <p className="text-muted-foreground text-xs">
                {t(
                  "Leave empty to use the channel default; 0 means no limit. Going over the limit never drops messages, only delays them.",
                )}
              </p>
            </div>

            <div className="border-t pt-4">
              <p className="mb-3 text-sm font-medium">{t("Filter rules (empty = no filtering)")}</p>
              <div className="grid gap-4">
                <div className="grid gap-2">
                  <Label>{t("Minimum severity")}</Label>
                  <Select
                    value={form.minSeverity || "all"}
                    onValueChange={(v) => setF({ minSeverity: v === "all" ? "" : v })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {SEVERITY_OPTIONS.map((o) => (
                        <SelectItem key={o.value || "all"} value={o.value || "all"}>
                          {o.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="n-inc">{t("Only these vulnerability types")}</Label>
                  <Textarea
                    id="n-inc"
                    placeholder={"SQL injection\ncommand execution"}
                    value={form.includeText}
                    onChange={(e) => setF({ includeText: e.target.value })}
                  />
                  <p className="text-muted-foreground text-xs">
                    {t("One keyword per line, matched case-insensitively as a substring. Leave empty for all types.")}
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="n-exc">{t("Exclude these vulnerability types")}</Label>
                  <Textarea
                    id="n-exc"
                    placeholder={"Information disclosure"}
                    value={form.excludeText}
                    onChange={(e) => setF({ excludeText: e.target.value })}
                  />
                  <p className="text-muted-foreground text-xs">
                    {t("Exclusion beats inclusion: an item matching both is excluded.")}
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="n-tasks">{t("Restrict to task IDs")}</Label>
                  <Input
                    id="n-tasks"
                    placeholder="1, 2, 3"
                    value={form.taskIDsText}
                    onChange={(e) => setF({ taskIDsText: e.target.value })}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="n-assets">{t("Restrict to asset IDs")}</Label>
                  <Input
                    id="n-assets"
                    placeholder="10, 11"
                    value={form.assetIDsText}
                    onChange={(e) => setF({ assetIDsText: e.target.value })}
                  />
                  <p className="text-muted-foreground text-xs">
                    {t("Empty task/asset = no restriction; when filled, the finding must intersect them.")}
                  </p>
                </div>
                <div className="flex items-center gap-2 text-sm">
                  <Switch
                    checked={form.onStatusChange}
                    onCheckedChange={(v) => setF({ onStatusChange: v })}
                    aria-label={t("Include status changes")}
                  />
                  {t("Also push when a finding's status changes (realtime mode only)")}
                </div>
              </div>
            </div>

            <div className="flex items-center gap-2 text-sm">
              <Switch checked={form.enabled} onCheckedChange={(v) => setF({ enabled: v })} aria-label={t("Enable")} />
              {t("Enable this channel")}
            </div>
          </fieldset>

          {canAdmin && (
            <div className="flex gap-2 pt-2 pb-6">
              <Button onClick={saveForm} disabled={saving}>
                {editing ? t("Save") : t("Add")}
              </Button>
              {editing && (
                <Button variant="outline" onClick={testChannel} disabled={testing}>
                  <SendIcon /> {t("Send test message")}
                </Button>
              )}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
