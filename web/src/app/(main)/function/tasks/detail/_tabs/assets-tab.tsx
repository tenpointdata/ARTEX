"use client";

import * as React from "react";

import { KeyRoundIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { AssetDslSearch } from "@/components/asset-dsl-search";
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
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { TableCell, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import { assetLabel } from "@/lib/task-assets";
import type { Asset, NewAssetType } from "@/lib/types";
import { cn } from "@/lib/utils";

import { AddTaskAssetsSheet } from "./add-task-assets-sheet";
import { AssetCard, Chips, fmtBytes, MethodBadge, SourceCell, statusTone, TABS } from "./assets-parts";

export function AssetsTab({ taskId }: { taskId: string }) {
  const canOperate = useCan("operate");
  const [rows, setRows] = React.useState<Asset[]>([]);
  const [total, setTotal] = React.useState(0);
  const [counts, setCounts] = React.useState<Record<string, number>>({});
  const [loaded, setLoaded] = React.useState(false);
  const [tab, setTab] = React.useState<NewAssetType>("root_domain");
  const [page, setPage] = React.useState(0);
  const [size, setSize] = React.useState(50);
  const [query, setQuery] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const [dslError, setDslError] = React.useState("");
  const [refreshKey, setRefreshKey] = React.useState(0);
  const [addOpen, setAddOpen] = React.useState(false);
  const [removeTarget, setRemoveTarget] = React.useState<Asset | null>(null);
  const [removing, setRemoving] = React.useState(false);
  const assetsRequestRef = React.useRef(0);
  const dslMode = query.trim() !== "";

  React.useEffect(() => {
    setPage(0);
    setRows([]);
    setLoaded(false);
  }, []);

  // Switching type tabs resets the search (DSL fields differ per type; matches
  // the global asset view).
  // biome-ignore lint/correctness/useExhaustiveDependencies: tab changes intentionally reset tab-local controls.
  React.useEffect(() => {
    setQuery("");
    setDslError("");
  }, [tab]);

  // Query / tab / page-size changes restart server pagination.
  // biome-ignore lint/correctness/useExhaustiveDependencies: these controls intentionally reset server pagination.
  React.useEffect(() => setPage(0), [tab, size, query]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: refreshKey is an explicit manual-reload trigger.
  React.useEffect(() => {
    let active = true;
    const dsl = query.trim();
    const request = ++assetsRequestRef.current;
    const load = async () => {
      setLoading(true);
      try {
        // The search always stays within this task's assets: taskAssets and
        // searchTaskAssets both pin task_id server-side.
        const [current, nextCounts] = await Promise.all([
          dsl
            ? api.searchTaskAssets(taskId, dsl, tab, size, page * size)
            : api.taskAssets(taskId, tab, size, page * size),
          api.assetCounts(taskId),
        ]);
        if (!active || assetsRequestRef.current !== request) return;
        setRows(current.assets);
        setTotal(current.total);
        setCounts(nextCounts ?? {});
        setDslError("");
      } catch (reason) {
        if (!active || assetsRequestRef.current !== request) return;
        const message = String((reason as Error)?.message ?? reason);
        if (dsl) {
          setDslError(message);
          setRows([]);
          setTotal(0);
        } else {
          toast.error(t("Failed to load task assets: {error}", { error: message }));
        }
      } finally {
        if (active && assetsRequestRef.current === request) {
          setLoading(false);
          setLoaded(true);
        }
      }
    };
    // Debounce DSL keystrokes; plain (re)loads and polling run immediately.
    const debounce = setTimeout(() => void load(), dsl ? 400 : 0);
    const stopPolling = setVisibleInterval(() => void load(), 10_000);
    return () => {
      active = false;
      clearTimeout(debounce);
      stopPolling();
    };
  }, [page, query, refreshKey, size, tab, taskId]);

  React.useEffect(() => {
    const maxPage = Math.max(0, Math.ceil(total / size) - 1);
    if (page > maxPage) setPage(maxPage);
  }, [page, size, total]);

  const refresh = React.useCallback(() => {
    setLoaded(false);
    setRefreshKey((current) => current + 1);
  }, []);

  const remove = async () => {
    if (!removeTarget) return;
    setRemoving(true);
    try {
      await api.detachTaskAsset(taskId, removeTarget.id);
      toast.success(t("Removed {label} from the current task", { label: assetLabel(removeTarget) }));
      setRemoveTarget(null);
      refresh();
    } catch (reason) {
      toast.error(t("Failed to remove: {error}", { error: String((reason as Error)?.message ?? reason) }));
    } finally {
      setRemoving(false);
    }
  };

  const removeButton = (asset: Asset) =>
    canOperate && (
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setRemoveTarget(asset)}
        aria-label={t("Remove asset {label} from the task", { label: assetLabel(asset) })}
        title={t("Remove from task")}
      >
        <Trash2Icon />
      </Button>
    );

  const commonCardProps = { loaded, onPage: setPage, onSize: setSize, page, size, total };
  const totalAll = TABS.reduce((sum, item) => sum + (counts[item.key] ?? 0), 0);

  const searchBox = (
    <AssetDslSearch
      query={query}
      onChange={setQuery}
      loading={loading}
      error={dslError}
      count={dslMode ? total : undefined}
    />
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-medium text-sm">{t("Test assets")}</h2>
          <p className="text-muted-foreground text-xs">
            {t("This task is linked to {count} assets in total", { count: totalAll })}
          </p>
        </div>
        {canOperate && (
          <Button size="sm" onClick={() => setAddOpen(true)}>
            <PlusIcon data-icon="inline-start" />
            {t("Add test assets")}
          </Button>
        )}
      </div>

      <Tabs
        value={tab}
        onValueChange={(value) => setTab(value as NewAssetType)}
        className="flex min-h-0 flex-1 flex-col gap-2"
      >
        <div className="overflow-x-auto overflow-y-hidden">
          <TabsList className="w-max">
            {TABS.map((item) => (
              <TabsTrigger key={item.key} value={item.key}>
                <item.icon data-icon="inline-start" />
                {item.label}
                <span className="text-muted-foreground tabular-nums">{counts[item.key] ?? 0}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </div>

        {searchBox}

        <TabsContent value="root_domain" className="mt-0 flex min-h-0 flex-1 flex-col">
          <AssetCard cols={[t("Domain"), t("ICP record"), t("Source"), t("Actions")]} {...commonCardProps}>
            {rows.map((asset) => (
              <TableRow key={asset.id}>
                <TableCell className="font-medium font-mono text-xs">{asset.domain}</TableCell>
                <TableCell className="text-xs">{asset.icp || "—"}</TableCell>
                <TableCell>
                  <SourceCell asset={asset} />
                </TableCell>
                <TableCell className="w-16">{removeButton(asset)}</TableCell>
              </TableRow>
            ))}
          </AssetCard>
        </TabsContent>

        <TabsContent value="ip" className="mt-0 flex min-h-0 flex-1 flex-col">
          <AssetCard
            cols={["IP", t("C segment"), t("Bound domains"), t("Open ports"), t("Source"), t("Actions")]}
            {...commonCardProps}
          >
            {rows.map((asset) => (
              <TableRow key={asset.id}>
                <TableCell className="font-medium font-mono text-xs">{asset.ip}</TableCell>
                <TableCell className="font-mono text-xs">{asset.c_segment || "—"}</TableCell>
                <TableCell>
                  <Chips items={asset.bound_domains ?? []} mono />
                </TableCell>
                <TableCell>
                  <Chips
                    items={(asset.open_ports ?? []).map((item) =>
                      item.service ? `${item.port}/${item.service}` : String(item.port),
                    )}
                    mono
                  />
                </TableCell>
                <TableCell>
                  <SourceCell asset={asset} />
                </TableCell>
                <TableCell className="w-16">{removeButton(asset)}</TableCell>
              </TableRow>
            ))}
          </AssetCard>
        </TabsContent>

        <TabsContent value="subdomain" className="mt-0 flex min-h-0 flex-1 flex-col">
          <AssetCard
            cols={[t("Domain"), t("Root domain"), t("Record type"), t("Record value"), t("Source"), t("Actions")]}
            {...commonCardProps}
          >
            {rows.map((asset) => (
              <TableRow key={asset.id}>
                <TableCell className="font-medium font-mono text-xs">{asset.domain}</TableCell>
                <TableCell className="font-mono text-xs">{asset.root_domain || "—"}</TableCell>
                <TableCell className="text-xs">{asset.record_type || "—"}</TableCell>
                <TableCell className="max-w-xs truncate font-mono text-xs">
                  {(Array.isArray(asset.record_value) ? asset.record_value.join(", ") : asset.record_value) || "—"}
                </TableCell>
                <TableCell>
                  <SourceCell asset={asset} />
                </TableCell>
                <TableCell className="w-16">{removeButton(asset)}</TableCell>
              </TableRow>
            ))}
          </AssetCard>
        </TabsContent>

        <TabsContent value="app" className="mt-0 flex min-h-0 flex-1 flex-col">
          <AssetCard
            cols={[t("App"), t("Address"), t("Category"), t("Title"), t("Fingerprints"), t("Source"), t("Actions")]}
            {...commonCardProps}
          >
            {rows.map((asset) => (
              <TableRow key={asset.id}>
                <TableCell className="max-w-48 truncate font-medium text-xs">{asset.app_name || "—"}</TableCell>
                <TableCell className="max-w-xs truncate font-mono text-xs" title={asset.url}>
                  {asset.url || "—"}
                </TableCell>
                <TableCell className="text-xs">{asset.category || "—"}</TableCell>
                <TableCell className="max-w-48 truncate text-xs">{asset.page_title || "—"}</TableCell>
                <TableCell>
                  <Chips items={asset.technologies ?? []} />
                </TableCell>
                <TableCell>
                  <SourceCell asset={asset} />
                </TableCell>
                <TableCell className="w-16">{removeButton(asset)}</TableCell>
              </TableRow>
            ))}
          </AssetCard>
        </TabsContent>

        <TabsContent value="service" className="mt-0 flex min-h-0 flex-1 flex-col">
          <AssetCard
            cols={[
              t("Address / service"),
              t("Status code"),
              t("Title"),
              t("Response size"),
              t("Fingerprints"),
              t("Auth"),
              t("Source"),
              t("Actions"),
            ]}
            {...commonCardProps}
          >
            {rows.map((asset) => {
              const isHttp = asset.service_type === "http";
              const address = isHttp
                ? asset.url || ""
                : asset.service_name || [asset.ip || asset.domain, asset.port].filter(Boolean).join(":");
              return (
                <TableRow key={asset.id}>
                  <TableCell className="max-w-xs truncate font-mono text-xs" title={address}>
                    {address || "—"}
                  </TableCell>
                  <TableCell>
                    {asset.status_code != null ? (
                      <span
                        className={cn("font-mono font-semibold text-xs tabular-nums", statusTone(asset.status_code))}
                      >
                        {asset.status_code}
                      </span>
                    ) : (
                      "—"
                    )}
                  </TableCell>
                  <TableCell className="max-w-48 truncate text-xs">{asset.page_title || "—"}</TableCell>
                  <TableCell className="text-xs tabular-nums">{fmtBytes(asset.content_length)}</TableCell>
                  <TableCell>
                    <Chips items={asset.technologies ?? []} />
                  </TableCell>
                  <TableCell>
                    {(asset.auth ?? []).length === 0 ? (
                      <span className="text-muted-foreground text-xs">—</span>
                    ) : (
                      (asset.auth ?? []).map((authItem) => {
                        const item = authItem as Record<string, string>;
                        return (
                          <span
                            key={`${asset.id}-${item.type ?? ""}-${item.username ?? ""}-${JSON.stringify(item)}`}
                            className="inline-flex items-center gap-1 text-[11px]"
                          >
                            <KeyRoundIcon className="size-3 text-muted-foreground" />
                            <span className="font-mono">{item.type || item.username || t("Auth")}</span>
                          </span>
                        );
                      })
                    )}
                  </TableCell>
                  <TableCell>
                    <SourceCell asset={asset} />
                  </TableCell>
                  <TableCell className="w-16">{removeButton(asset)}</TableCell>
                </TableRow>
              );
            })}
          </AssetCard>
        </TabsContent>

        <TabsContent value="endpoint" className="mt-0 flex min-h-0 flex-1 flex-col">
          <AssetCard cols={[t("Method"), t("Full URL"), t("Params"), t("Source"), t("Actions")]} {...commonCardProps}>
            {rows.map((asset) => (
              <TableRow key={asset.id}>
                <TableCell className="w-16">
                  <MethodBadge method={asset.method || ""} />
                </TableCell>
                <TableCell className="max-w-sm truncate font-mono text-xs" title={asset.url}>
                  {asset.url || "—"}
                </TableCell>
                <TableCell>
                  <Chips
                    items={(asset.params ?? []).map((param) => {
                      const item = param as Record<string, string>;
                      return item.name ? `${item.name}(${item.location || item.in || "?"})` : "?";
                    })}
                    mono
                  />
                </TableCell>
                <TableCell>
                  <SourceCell asset={asset} />
                </TableCell>
                <TableCell className="w-16">{removeButton(asset)}</TableCell>
              </TableRow>
            ))}
          </AssetCard>
        </TabsContent>
      </Tabs>

      <AddTaskAssetsSheet onAttached={refresh} onOpenChange={setAddOpen} open={addOpen} taskId={taskId} />

      <AlertDialog open={Boolean(removeTarget)} onOpenChange={(open) => !open && !removing && setRemoveTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("Remove from this task?")}</AlertDialogTitle>
            <AlertDialogDescription className="[overflow-wrap:anywhere]">
              {removeTarget
                ? `${t('Remove "{label}" from this task\'s test assets.', { label: assetLabel(removeTarget) })} `
                : ""}
              {t("Global assets, linked traffic, and historical blackboard anchors are kept.")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={removing}>{t("Cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={removing}
              onClick={(event) => {
                event.preventDefault();
                void remove();
              }}
            >
              {removing ? <Spinner data-icon="inline-start" /> : <Trash2Icon data-icon="inline-start" />}
              {removing ? t("Removing") : t("Remove")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
