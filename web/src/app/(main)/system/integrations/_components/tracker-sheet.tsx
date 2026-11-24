"use client";

import * as React from "react";

import { toast } from "sonner";

import { ConfigField } from "@/components/config-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { t } from "@/i18n";
import { buildConfig } from "@/lib/config-form";
import { integrationsApi } from "@/lib/integrations-api";
import type { ConfigValues, TicketTracker, TicketTrackerMeta } from "@/lib/types";

import { trackerFields, trackerKindLabel } from "./tracker-fields";

interface TrackerForm {
  name: string;
  kind: string;
  enabled: boolean;
  config: ConfigValues;
}

function formFor(tracker: TicketTracker | null, defaultKind: string): TrackerForm {
  if (!tracker) return { name: "", kind: defaultKind, enabled: true, config: {} };
  // Masked credentials go into the form as-is and are sent back as-is, which keeps the stored values.
  return { name: tracker.name, kind: tracker.kind, enabled: tracker.enabled, config: { ...tracker.config } };
}

// TrackerSheet creates or edits one ticket tracker. A destination change (repo, base URL,
// test id) without re-entering the credentials is refused by the server; its message is shown as-is.
export function TrackerSheet({
  open,
  onOpenChange,
  meta,
  editing,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  meta: TicketTrackerMeta | null;
  editing: TicketTracker | null;
  onSaved: (tracker: TicketTracker) => void;
}) {
  // GitHub first when offered: the common case; the server lists kinds alphabetically.
  const defaultKind = meta?.kinds.find((k) => k.kind === "github")?.kind ?? meta?.kinds[0]?.kind ?? "github";
  const [form, setForm] = React.useState<TrackerForm>(() => formFor(editing, defaultKind));
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    if (open) setForm(formFor(editing, defaultKind));
  }, [open, editing, defaultKind]);

  const kindMeta = meta?.kinds.find((k) => k.kind === form.kind);
  const secrets = new Set(editing?.secret_keys ?? kindMeta?.secret_keys ?? []);
  const fields = trackerFields(form.kind);

  async function save() {
    setSaving(true);
    try {
      const config = buildConfig(fields, form.config);
      const saved = editing
        ? await integrationsApi.updateTracker(editing.id, { name: form.name.trim(), enabled: form.enabled, config })
        : await integrationsApi.createTracker({
            name: form.name.trim(),
            kind: form.kind,
            enabled: form.enabled,
            config,
          });
      toast.success(editing ? t("Saved: {name}", { name: saved.name }) : t("Added: {name}", { name: saved.name }));
      onSaved(saved);
      onOpenChange(false);
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }), { duration: 10000 });
    } finally {
      setSaving(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full data-[side=right]:sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{editing ? editing.name : t("Add ticket tracker")}</SheetTitle>
          <SheetDescription>
            {kindMeta?.destination_keys.length
              ? t("Changing {keys} requires entering the credentials again", {
                  keys: kindMeta.destination_keys.join(", "),
                })
              : trackerKindLabel(form.kind)}
          </SheetDescription>
        </SheetHeader>
        <form
          className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4"
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label>{t("Tracker type")}</Label>
              <Select
                value={form.kind}
                // A different kind has different credential fields; nothing carries over.
                onValueChange={(v) => v && setForm((f) => ({ ...f, kind: v, config: {} }))}
                disabled={!!editing}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(meta?.kinds ?? []).map((k) => (
                    <SelectItem key={k.kind} value={k.kind}>
                      {trackerKindLabel(k.kind)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="tracker-name">{t("Name")}</Label>
              <Input
                id="tracker-name"
                value={form.name}
                placeholder={trackerKindLabel(form.kind)}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              />
            </div>
            {fields.map((def) => (
              <ConfigField
                key={`${form.kind}-${def.key}`}
                def={def}
                value={form.config[def.key]}
                isSecret={secrets.has(def.key)}
                onChange={(v) => setForm((f) => ({ ...f, config: { ...f.config, [def.key]: v } }))}
              />
            ))}
            <div className="flex items-center gap-2 text-sm">
              <Switch
                checked={form.enabled}
                onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
                aria-label={t("Enable")}
              />
              {t("Enable this tracker")}
            </div>
          </div>
          <div className="flex gap-2 pt-2 pb-6">
            <Button type="submit" disabled={saving}>
              {editing ? t("Save") : t("Add")}
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}
