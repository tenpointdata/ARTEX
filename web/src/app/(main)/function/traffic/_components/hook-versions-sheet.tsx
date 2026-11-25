"use client";

import * as React from "react";

import { Code2Icon, Loader2Icon, RotateCcwIcon } from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { TamperHook, TamperHookVersion } from "@/lib/types";

import { fmtDateTime } from "./hook-editor-state";

interface HookVersionsSheetProps {
  /** The hook whose history is shown; null closes the sheet. */
  hook: TamperHook | null;
  onHookChange: (hook: TamperHook | null) => void;
  /** Called after a rollback so the hook list can refresh. */
  onRolledBack: () => Promise<void>;
}

// Version history of a hook (up to 20 revisions) with source preview and rollback.
export function HookVersionsSheet({ hook: versionsHook, onHookChange, onRolledBack }: HookVersionsSheetProps) {
  const [versions, setVersions] = React.useState<TamperHookVersion[]>([]);
  const [versionsLoading, setVersionsLoading] = React.useState(false);
  const [expandedVersion, setExpandedVersion] = React.useState<number | null>(null);
  const [rollbackTarget, setRollbackTarget] = React.useState<TamperHookVersion | null>(null);
  const [rollingBack, setRollingBack] = React.useState(false);
  const openId = versionsHook?.id;

  // Load the history whenever a different hook is opened.
  React.useEffect(() => {
    if (openId === undefined) return;
    let alive = true;
    setVersions([]);
    setExpandedVersion(null);
    setVersionsLoading(true);
    api
      .trafficHookVersions(openId)
      .then((res) => alive && setVersions(res.versions ?? []))
      .catch((e: Error) => alive && toast.error(e.message))
      .finally(() => alive && setVersionsLoading(false));
    return () => {
      alive = false;
    };
  }, [openId]);

  const handleRollback = async () => {
    if (!versionsHook || !rollbackTarget) return;
    setRollingBack(true);
    try {
      const updated = await api.rollbackTrafficHook(versionsHook.id, rollbackTarget.version);
      toast.success(
        t("Rolled back to v{version} (now v{current})", { version: rollbackTarget.version, current: updated.version }),
      );
      setRollbackTarget(null);
      onHookChange(updated);
      await onRolledBack();
      const res = await api.trafficHookVersions(updated.id);
      setVersions(res.versions ?? []);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setRollingBack(false);
    }
  };

  return (
    <>
      <Sheet open={versionsHook !== null} onOpenChange={(open) => !open && onHookChange(null)}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
          <SheetHeader>
            <SheetTitle>{t("Versions: {name}", { name: versionsHook?.name ?? "" })}</SheetTitle>
            <SheetDescription>
              {t("Up to 20 newest revisions are retained. Rolling back creates a new monotonic version.")}
            </SheetDescription>
          </SheetHeader>

          <div className="mt-4">
            {versionsLoading ? (
              <div className="py-10 text-center text-sm text-muted-foreground">
                <Loader2Icon className="mx-auto mb-2 size-5 animate-spin" />
                {t("Loading versions…")}
              </div>
            ) : versions.length === 0 ? (
              <div className="py-8 text-center text-sm text-muted-foreground">{t("No stored versions.")}</div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-24">{t("Version")}</TableHead>
                    <TableHead className="w-24">{t("Phase")}</TableHead>
                    <TableHead>{t("Priority")}</TableHead>
                    <TableHead className="w-36">{t("Time")}</TableHead>
                    <TableHead className="w-44 text-right">{t("Actions")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {versions.map((v) => {
                    const isCurrent = versionsHook?.version === v.version;
                    const isExpanded = expandedVersion === v.version;
                    return (
                      <React.Fragment key={v.id}>
                        <TableRow>
                          <TableCell>
                            <div className="flex items-center gap-1.5">
                              <Badge variant={isCurrent ? "default" : "outline"} className="font-mono text-xs">
                                v{v.version}
                              </Badge>
                              {isCurrent && <Badge variant="secondary">{t("Current")}</Badge>}
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className="font-mono text-xs">
                              {v.phase}
                            </Badge>
                          </TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground">
                            {t("prio {n}", { n: v.priority })} · {v.source_sha256.slice(0, 12)}
                            {v.note && <div className="font-sans">{v.note}</div>}
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground">{fmtDateTime(v.created_at)}</TableCell>
                          <TableCell className="text-right">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-7 px-2 text-xs"
                                onClick={() => setExpandedVersion(isExpanded ? null : v.version)}
                              >
                                <Code2Icon className="size-3.5" />
                                {isExpanded ? t("Hide source") : t("View source")}
                              </Button>
                              {!isCurrent && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="h-7 px-2.5 text-xs"
                                  onClick={() => setRollbackTarget(v)}
                                >
                                  <RotateCcwIcon className="size-3.5" />
                                  {t("Rollback to v{version}", { version: v.version })}
                                </Button>
                              )}
                            </div>
                          </TableCell>
                        </TableRow>
                        {isExpanded && (
                          <TableRow>
                            <TableCell colSpan={5}>
                              <pre className="max-h-72 overflow-auto rounded-md border bg-muted/40 p-2.5 font-mono text-xs">
                                {v.source}
                              </pre>
                            </TableCell>
                          </TableRow>
                        )}
                      </React.Fragment>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </div>
        </SheetContent>
      </Sheet>

      <AlertDialog open={rollbackTarget !== null} onOpenChange={(open) => !open && setRollbackTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("Rollback hook to v{version}?", { version: rollbackTarget?.version ?? 0 })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('This restores the matchers and source from v{version} as a new version on "{name}".', {
                version: rollbackTarget?.version ?? 0,
                name: versionsHook?.name ?? "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={rollingBack}>{t("Cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void handleRollback();
              }}
              disabled={rollingBack}
            >
              {rollingBack && <Loader2Icon className="size-3.5 animate-spin" />}
              {t("Rollback")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
