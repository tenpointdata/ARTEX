"use client";

import * as React from "react";

import {
  AlertTriangleIcon,
  HistoryIcon,
  Loader2Icon,
  PencilIcon,
  PlayIcon,
  PlusIcon,
  RefreshCwIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  Trash2Icon,
} from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
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
import { Card } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { buildMatchers } from "@/lib/tamper-hooks";
import type { TamperHook, TrafficExchange } from "@/lib/types";

import { HookDryRunSheet } from "./hook-dry-run-sheet";
import { HookEditorDialog } from "./hook-editor-dialog";
import { type DryRunTarget, type EditorState, editorFromHook, emptyEditor, fmtDateTime } from "./hook-editor-state";
import { HookVersionsSheet } from "./hook-versions-sheet";

export function TamperHooksPanel({ recentExchanges = [] }: { recentExchanges?: TrafficExchange[] }) {
  const canOperate = useCan("operate"); // every hook mutation and dry-run needs operate
  const [hooks, setHooks] = React.useState<TamperHook[]>([]);
  const [enabledByEnv, setEnabledByEnv] = React.useState<boolean>(true);
  const [loading, setLoading] = React.useState(true);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [togglingId, setTogglingId] = React.useState<number | null>(null);

  // Create / Edit dialog state
  const [editor, setEditor] = React.useState<EditorState | null>(null);
  const [saving, setSaving] = React.useState(false);
  const [formError, setFormError] = React.useState<string | null>(null);

  // Delete confirmation
  const [deleteTarget, setDeleteTarget] = React.useState<TamperHook | null>(null);
  const [deleting, setDeleting] = React.useState(false);

  // Versions sheet / dry-run sheet targets
  const [versionsHook, setVersionsHook] = React.useState<TamperHook | null>(null);
  const [dryRunTarget, setDryRunTarget] = React.useState<DryRunTarget | null>(null);

  const loadHooks = React.useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const res = await api.trafficHooks();
      setHooks(res.hooks ?? []);
      setEnabledByEnv(Boolean(res.enabled_by_env));
    } catch (e) {
      setLoadError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void loadHooks();
  }, [loadHooks]);

  const handleToggle = async (hook: TamperHook, next: boolean) => {
    setTogglingId(hook.id);
    try {
      const updated = await api.toggleTrafficHook(hook.id, next);
      setHooks((prev) => prev.map((h) => (h.id === hook.id ? updated : h)));
      toast.success(
        next ? t('Hook "{name}" enabled', { name: hook.name }) : t('Hook "{name}" disabled', { name: hook.name }),
      );
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setTogglingId(null);
    }
  };

  const handleSave = async () => {
    if (!editor) return;
    setFormError(null);
    const priority = Number.parseInt(editor.priority, 10);
    if (Number.isNaN(priority)) {
      setFormError(t("Priority must be an integer"));
      return;
    }
    const payload = {
      name: editor.name.trim(),
      enabled: editor.mode === "create" ? false : editor.enabled,
      phase: editor.phase,
      priority,
      matchers: buildMatchers(editor),
      source: editor.source,
      note: editor.note.trim(),
    };
    setSaving(true);
    try {
      if (editor.mode === "create") {
        await api.createTrafficHook(payload);
        toast.success(t("Hook created (disabled by default)"));
      } else if (editor.id !== undefined) {
        await api.updateTrafficHook(editor.id, payload);
        toast.success(t("Hook updated"));
      }
      setEditor(null);
      await loadHooks();
    } catch (e) {
      setFormError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await api.deleteTrafficHook(deleteTarget.id);
      toast.success(t("Hook deleted"));
      setDeleteTarget(null);
      await loadHooks();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setDeleting(false);
    }
  };

  const openDryRunForHook = (hook: TamperHook) =>
    setDryRunTarget({ label: `${hook.name} (v${hook.version}, ${hook.phase})`, hookId: hook.id });

  const openDryRunForDraft = () => {
    if (!editor) return;
    setDryRunTarget({
      label: t("Draft: {name} ({phase})", { name: editor.name.trim() || t("untitled"), phase: editor.phase }),
      inline: {
        name: editor.name.trim() || "dry-run",
        phase: editor.phase,
        matchers: buildMatchers(editor),
        source: editor.source,
      },
    });
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      {!enabledByEnv && (
        <Alert className="border-amber-500/40 bg-amber-500/10 py-2.5">
          <AlertTriangleIcon className="size-4 text-amber-500" />
          <AlertDescription className="text-xs">
            {t(
              "Live hook execution is disabled (ARTEX_TAMPER_HOOKS_ENABLED is off). You can create, edit, version, and dry-run hooks, but none will run on live proxy traffic until enabled.",
            )}
          </AlertDescription>
        </Alert>
      )}

      {loadError && (
        <Alert variant="destructive" className="py-2">
          <AlertDescription className="text-xs">{loadError}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {t(
            "Operator-only Go-subset tampering hooks run after declarative rewrite rules per phase and fail open on error.",
          )}
        </p>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" className="h-8" onClick={() => void loadHooks()} disabled={loading}>
            <RefreshCwIcon className={`size-3.5 ${loading ? "animate-spin" : ""}`} />
            {t("Refresh")}
          </Button>
          {canOperate && (
            <Button
              size="sm"
              className="h-8"
              onClick={() => {
                setFormError(null);
                setEditor(emptyEditor("request"));
              }}
            >
              <PlusIcon className="size-3.5" />
              {t("New hook")}
            </Button>
          )}
        </div>
      </div>

      <Card className="min-h-0 flex-1 overflow-auto p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("Name")}</TableHead>
              <TableHead className="w-28">{t("Phase")}</TableHead>
              <TableHead className="w-24">{t("Priority")}</TableHead>
              <TableHead className="w-24">{t("Version")}</TableHead>
              <TableHead className="w-24">{t("Enabled")}</TableHead>
              <TableHead className="w-44">{t("Last error")}</TableHead>
              <TableHead className="w-48 text-right">{t("Actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && hooks.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="py-10 text-center text-sm text-muted-foreground">
                  <Loader2Icon className="mx-auto mb-2 size-5 animate-spin" />
                  {t("Loading hooks…")}
                </TableCell>
              </TableRow>
            ) : hooks.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="py-10 text-center text-sm text-muted-foreground">
                  {t("No tampering hooks configured yet.")}
                </TableCell>
              </TableRow>
            ) : (
              hooks.map((hook) => (
                <TableRow key={hook.id}>
                  <TableCell>
                    <div className="font-medium">{hook.name}</div>
                    {hook.note && <div className="line-clamp-1 text-xs text-muted-foreground">{hook.note}</div>}
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className="font-mono text-xs">
                      {hook.phase}
                    </Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs tabular-nums">{hook.priority}</TableCell>
                  <TableCell className="font-mono text-xs tabular-nums">v{hook.version}</TableCell>
                  <TableCell>
                    <Switch
                      checked={hook.enabled}
                      disabled={togglingId === hook.id || !canOperate}
                      aria-label={t("Toggle hook {name}", { name: hook.name })}
                      onCheckedChange={(checked) => void handleToggle(hook, checked)}
                    />
                  </TableCell>
                  <TableCell>
                    {hook.load_error ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge variant="destructive" className="gap-1 font-mono text-xs">
                            <ShieldAlertIcon className="size-3" />
                            {t("load_error")}
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-xs font-mono text-xs">{hook.load_error}</TooltipContent>
                      </Tooltip>
                    ) : hook.last_error ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge variant="destructive" className="gap-1 font-mono text-xs">
                            <AlertTriangleIcon className="size-3" />
                            {hook.last_error.class} ({hook.last_error.count})
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent className="text-xs">
                          {t("Last seen at {time}", { time: fmtDateTime(hook.last_error.at) })}
                        </TooltipContent>
                      </Tooltip>
                    ) : (
                      <Badge variant="secondary" className="gap-1 text-xs text-muted-foreground">
                        <ShieldCheckIcon className="size-3" />
                        {t("None")}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      {canOperate && (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          title={t("Dry-run")}
                          aria-label={t("Dry-run")}
                          onClick={() => openDryRunForHook(hook)}
                        >
                          <PlayIcon className="size-3.5" />
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        title={t("Versions")}
                        aria-label={t("Versions")}
                        onClick={() => setVersionsHook(hook)}
                      >
                        <HistoryIcon className="size-3.5" />
                      </Button>
                      {canOperate && (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          title={t("Edit")}
                          aria-label={t("Edit")}
                          onClick={() => {
                            setFormError(null);
                            setEditor(editorFromHook(hook));
                          }}
                        >
                          <PencilIcon className="size-3.5" />
                        </Button>
                      )}
                      {canOperate && (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          title={t("Delete")}
                          aria-label={t("Delete")}
                          className="text-destructive hover:text-destructive"
                          onClick={() => setDeleteTarget(hook)}
                        >
                          <Trash2Icon className="size-3.5" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>

      <HookEditorDialog
        editor={editor}
        onEditorChange={setEditor}
        formError={formError}
        saving={saving}
        onSave={() => void handleSave()}
        onDryRunDraft={openDryRunForDraft}
        onClose={() => {
          setEditor(null);
          setFormError(null);
        }}
      />

      {/* Delete confirmation */}
      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("Delete tampering hook?")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('This permanently removes "{name}" and its stored version history.', {
                name: deleteTarget?.name ?? "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>{t("Cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void handleDelete();
              }}
              disabled={deleting}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting && <Loader2Icon className="size-3.5 animate-spin" />}
              {t("Delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <HookVersionsSheet hook={versionsHook} onHookChange={setVersionsHook} onRolledBack={loadHooks} />

      <HookDryRunSheet target={dryRunTarget} onClose={() => setDryRunTarget(null)} recentExchanges={recentExchanges} />
    </div>
  );
}
