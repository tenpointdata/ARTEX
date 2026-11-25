"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";

import { rich } from "./rich";

export type WrapupTabProps = {
  wrapup: string;
  setWrapup: (v: string) => void;
  wrapupDefault: string;
  wrapupTurns: string;
  setWrapupTurns: (v: string) => void;
  wrapupTurnsDefault: number;
  saveWrapup: () => void;
  resetWrapup: () => void;
  ttSupported: boolean;
  ttWrapup: string;
  setTtWrapup: (v: string) => void;
  ttWrapupDefault: string;
  ttTurns: string;
  setTtTurns: (v: string) => void;
  ttTurnsDefault: number;
  saveTaskTimeoutWrapup: () => void;
  resetTaskTimeoutWrapup: () => void;
};

// Wrap-up prompt tab: per-run wrap-up plus the task-timeout wrap-up (worker/planner only).
export function WrapupTab(p: WrapupTabProps) {
  const {
    wrapup,
    setWrapup,
    wrapupDefault,
    wrapupTurns,
    setWrapupTurns,
    wrapupTurnsDefault,
    saveWrapup,
    resetWrapup,
    ttSupported,
    ttWrapup,
    setTtWrapup,
    ttWrapupDefault,
    ttTurns,
    setTtTurns,
    ttTurnsDefault,
    saveTaskTimeoutWrapup,
    resetTaskTimeoutWrapup,
  } = p;
  return (
    <div className="grid gap-3">
      <p className="text-muted-foreground text-xs leading-relaxed">
        {rich(
          t(
            'When this Agent is terminated due to <b>timeout</b> or <b>exhausted turns</b>, the system injects this "wrap-up prompt" for one wrap-up round: It first writes back anything identified but not yet stored, then outputs a one-line summary (to avoid an abrupt ending). Leave empty to use the built-in default.',
          ),
        )}
      </p>
      <div className="flex items-center gap-2">
        <Label className="text-xs">{t("Wrap-up prompt body")}</Label>
        {wrapup.trim() ? (
          <Badge variant="secondary" className="px-1.5 py-0">
            {t("Custom")}
          </Badge>
        ) : (
          <Badge variant="outline" className="px-1.5 py-0">
            {t("Using built-in default")}
          </Badge>
        )}
      </div>
      <Textarea
        className="font-mono text-xs"
        rows={10}
        value={wrapup}
        placeholder={wrapupDefault || t("Leave empty to use the built-in default wrap-up prompt")}
        onChange={(e) => setWrapup(e.target.value)}
      />
      <div className="grid gap-1.5">
        <Label htmlFor="wrapup-turns" className="text-xs">
          {t("Wrap-up turns (max turns for the wrap-up phase itself; 0 = built-in default of {count} turns)", {
            count: wrapupTurnsDefault,
          })}
        </Label>
        <Input
          id="wrapup-turns"
          type="number"
          min={0}
          className="h-8 w-32"
          value={wrapupTurns}
          onChange={(e) => setWrapupTurns(e.target.value)}
        />
      </div>
      <div className="flex gap-2">
        <Button size="sm" onClick={saveWrapup}>
          {t("Save")}
        </Button>
        <Button size="sm" variant="outline" onClick={resetWrapup}>
          {t("Restore default")}
        </Button>
      </div>
      {wrapupDefault && (
        <>
          <Separator />
          <div className="grid gap-1.5">
            <Label className="text-muted-foreground text-xs">{t("Built-in default (read-only, for reference)")}</Label>
            <pre className="text-muted-foreground max-h-40 overflow-y-auto rounded-md border bg-muted/30 p-2 text-xs whitespace-pre-wrap">
              {wrapupDefault}
            </pre>
          </div>
        </>
      )}

      {ttSupported && (
        <>
          <Separator className="my-2" />
          <p className="text-muted-foreground text-xs leading-relaxed">
            {rich(
              t(
                '<b>Task timeout wrap-up</b> (<b>separate</b> from the per-run wrap-up above): injected when the <b>entire task</b> hits its timeout cap and is about to end. Its semantics are often the opposite of per-run (e.g. planner: per-run says "don\'t stop, keep planning", task timeout says "time\'s up, stop and make the final call"). Leave empty to use the built-in default.',
              ),
            )}
          </p>
          <div className="flex items-center gap-2">
            <Label className="text-xs">{t("Task timeout wrap-up prompt body")}</Label>
            {ttWrapup.trim() ? (
              <Badge variant="secondary" className="px-1.5 py-0">
                {t("Custom")}
              </Badge>
            ) : (
              <Badge variant="outline" className="px-1.5 py-0">
                {t("Using built-in default")}
              </Badge>
            )}
          </div>
          <Textarea
            className="font-mono text-xs"
            rows={10}
            value={ttWrapup}
            placeholder={ttWrapupDefault || t("Leave empty to use the built-in default task timeout wrap-up prompt")}
            onChange={(e) => setTtWrapup(e.target.value)}
          />
          <div className="grid gap-1.5">
            <Label htmlFor="tt-turns" className="text-xs">
              {t("Wrap-up turns (0 = built-in default of {count} turns)", { count: ttTurnsDefault })}
            </Label>
            <Input
              id="tt-turns"
              type="number"
              min={0}
              className="h-8 w-32"
              value={ttTurns}
              onChange={(e) => setTtTurns(e.target.value)}
            />
          </div>
          <div className="flex gap-2">
            <Button size="sm" onClick={saveTaskTimeoutWrapup}>
              {t("Save")}
            </Button>
            <Button size="sm" variant="outline" onClick={resetTaskTimeoutWrapup}>
              {t("Restore default")}
            </Button>
          </div>
          {ttWrapupDefault && (
            <div className="grid gap-1.5">
              <Label className="text-muted-foreground text-xs">
                {t("Built-in default (read-only, for reference)")}
              </Label>
              <pre className="text-muted-foreground max-h-40 overflow-y-auto rounded-md border bg-muted/30 p-2 text-xs whitespace-pre-wrap">
                {ttWrapupDefault}
              </pre>
            </div>
          )}
        </>
      )}
    </div>
  );
}
