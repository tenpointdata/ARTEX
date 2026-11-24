"use client";

import * as React from "react";

import { RotateCcwIcon, SaveIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Agent, Tool } from "@/lib/types";

// Traffic tools are host tools gated by the global traffic capture switch: bindable, but
// only usable when capture is on. Keep in sync with traffic.SeedToolMetas.
import { applyRows, type ParamRow, TRAFFIC_TOOL_KEYS, toRows } from "./schema-rows";

// ToolEditor is the full edit form for one tool, rendered inside the drawer.
export function ToolEditor({
  tool,
  agents,
  captureOn,
  onSaved,
  onClose,
}: {
  tool: Tool;
  agents: Agent[];
  captureOn: boolean;
  onSaved: () => void;
  onClose: () => void;
}) {
  // traffic tools can't be bound/enabled until the global traffic capture switch is on.
  const trafficGated = TRAFFIC_TOOL_KEYS.has(tool.key) && !captureOn;
  const [description, setDescription] = React.useState(tool.description);
  const [bound, setBound] = React.useState<string[]>(tool.agents);
  const [enabled, setEnabled] = React.useState(tool.enabled);
  const [rows, setRows] = React.useState<ParamRow[]>(() => toRows(tool.schema));
  const [saving, setSaving] = React.useState(false);

  const setRow = (i: number, patch: Partial<ParamRow>) =>
    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  const toggleAgent = (k: string) => setBound((b) => (b.includes(k) ? b.filter((x) => x !== k) : [...b, k]));

  async function save() {
    setSaving(true);
    try {
      await api.saveTool(tool.key, {
        description,
        schema: applyRows(tool.schema, rows),
        agents: bound,
        enabled,
      });
      toast.success(t('Tool "{key}" saved', { key: tool.key }));
      onSaved();
      onClose();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    } finally {
      setSaving(false);
    }
  }
  async function reset() {
    try {
      await api.resetTool(tool.key);
      toast.success(t('Restored "{key}" to the code default', { key: tool.key }));
      onSaved();
      onClose();
    } catch (e) {
      toast.error(t("Restore failed: {error}", { error: (e as Error).message }));
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4">
        {trafficGated && (
          <div className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-muted-foreground text-xs">
            {(() => {
              const [before, after] = t("This tool depends on {capture}.").split("{capture}");
              return (
                <>
                  {before}
                  <b>{t("traffic capture")}</b>
                  {after}
                </>
              );
            })()} {t(
              'Enable traffic capture under "System settings" first before binding it to agents and enabling it.',
            )}
          </div>
        )}
        {/* binding + switch */}
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
          <div className="grid gap-1.5">
            <Label className="text-muted-foreground text-xs">
              {t("Bind agents (controls which agents get this tool)")}
            </Label>
            <div className="flex flex-wrap gap-3">
              {agents.map((ag) => (
                <label key={ag.key} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={bound.includes(ag.key)}
                    disabled={trafficGated}
                    onCheckedChange={() => toggleAgent(ag.key)}
                  />
                  {ag.name}
                  <span className="font-mono text-muted-foreground text-xs">{ag.key}</span>
                </label>
              ))}
              {agents.length === 0 && <span className="text-muted-foreground text-xs">{t("(no agents)")}</span>}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Switch checked={enabled} onCheckedChange={setEnabled} id={`en-${tool.key}`} />
            <Label htmlFor={`en-${tool.key}`} className="text-sm">
              {t("Enable")}
            </Label>
          </div>
        </div>

        {/* description */}
        <div className="grid gap-1.5">
          <Label className="text-muted-foreground text-xs">{t("Tool description (sent to the model)")}</Label>
          <Textarea
            className="font-mono text-xs"
            rows={6}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>

        {/* params */}
        <div className="grid gap-2">
          <Label className="text-muted-foreground text-xs">
            {t("Parameters (name / type / required are read-only; description and default are editable)")}
          </Label>
          {rows.length === 0 && <span className="text-muted-foreground text-xs">{t("(no parameters)")}</span>}
          {rows.map((r, i) => (
            <div
              key={`${r.parentKey ?? ""}.${r.name}`}
              className={
                r.parentKey ? "ml-3 grid gap-2 border-muted border-l-2 py-2 pl-3" : "grid gap-2 rounded-md border p-3"
              }
            >
              <div className="flex flex-wrap items-center gap-2">
                {r.parentKey && <span className="font-mono text-[10px] text-muted-foreground">↳</span>}
                <span className="font-mono text-sm">{r.name}</span>
                <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                  {r.type || "?"}
                </Badge>
                {r.required && (
                  <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                    {t("Required")}
                  </Badge>
                )}
                {r.parentKey && <span className="text-[10px] text-muted-foreground">{t("items sub-field")}</span>}
              </div>
              <div className="grid gap-2">
                <div className="grid gap-1">
                  <Label className="text-[11px] text-muted-foreground">{t("Description")}</Label>
                  <Input
                    className="text-xs"
                    value={r.description}
                    onChange={(e) => setRow(i, { description: e.target.value })}
                  />
                </div>
                <div className="grid gap-1">
                  <Label className="text-[11px] text-muted-foreground">{t("Default")}</Label>
                  <Input
                    className="text-xs"
                    placeholder={r.scalar ? t("(empty = no default)") : t("scalars only")}
                    disabled={!r.scalar}
                    value={r.defaultStr}
                    onChange={(e) => setRow(i, { defaultStr: e.target.value })}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <Separator className="mt-4" />
      <div className="flex flex-wrap gap-2 p-4">
        <Button size="sm" onClick={save} disabled={saving}>
          <SaveIcon /> {t("Save")}
        </Button>
        <Button size="sm" variant="outline" onClick={reset}>
          <RotateCcwIcon /> {t("Restore defaults")}
        </Button>
      </div>
    </div>
  );
}
