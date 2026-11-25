"use client";

import * as React from "react";

import {
  BuildingIcon,
  GlobeIcon,
  LayoutTemplateIcon,
  LinkIcon,
  type LucideIcon,
  NetworkIcon,
  RefreshCwIcon,
  SmartphoneIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react";
import { toast } from "sonner";

import { AssetDslSearch } from "@/components/asset-dsl-search";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Asset, Company } from "@/lib/types";
import { cn } from "@/lib/utils";
import { useActiveCompanyID } from "@/stores/company/company-store";

import { DeleteAssetsDialog, DeleteCompanyDialog } from "./_components/asset-delete-dialogs";
import { ASSET_COLS, AssetRow } from "./_components/asset-rows";
import { AssetTable } from "./_components/asset-table";
import { CompaniesTable } from "./_components/companies-table";
import { CompanyDialog } from "./_components/company-dialogs";

const TABS: { key: string; label: string; icon: LucideIcon }[] = [
  { key: "company", label: t("Companies"), icon: BuildingIcon },
  { key: "root_domain", label: t("Root domains"), icon: GlobeIcon },
  { key: "ip", label: "IP", icon: NetworkIcon },
  { key: "subdomain", label: t("Subdomains"), icon: GlobeIcon },
  { key: "app", label: t("Apps"), icon: SmartphoneIcon },
  { key: "service", label: t("Services"), icon: LayoutTemplateIcon },
  { key: "endpoint", label: t("Endpoints"), icon: LinkIcon },
];

export default function AssetsPage() {
  // The sidebar's company switcher narrows the data tabs to that company.
  const companyId = useActiveCompanyID();
  const [rows, setRows] = React.useState<Asset[]>([]);
  const [total, setTotal] = React.useState(0);
  const [companies, setCompanies] = React.useState<Company[]>([]);
  const [counts, setCounts] = React.useState<Record<string, number>>({});
  const [tab, setTab] = React.useState("company");
  const [query, setQuery] = React.useState("");
  const [loaded, setLoaded] = React.useState(false);
  const [loading, setLoading] = React.useState(false);
  const [page, setPage] = React.useState(0);
  const [size, setSize] = React.useState(50);
  const [dslError, setDslError] = React.useState("");
  const [refreshKey, setRefreshKey] = React.useState(0);
  const assetsRequestRef = React.useRef(0);
  const [rowsTab, setRowsTab] = React.useState("");

  // asset selection & delete
  const [selected, setSelected] = React.useState<Set<number>>(new Set());
  const [deleteIds, setDeleteIds] = React.useState<number[]>([]);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  // company delete
  const [companyDeleteTarget, setCompanyDeleteTarget] = React.useState<Company | null>(null);
  const [companyDeleteAssets, setCompanyDeleteAssets] = React.useState(false);
  const [companyDeleting, setCompanyDeleting] = React.useState(false);

  // Companies + per-type counts (tab badges) — loaded on demand, no background polling.
  const loadMeta = React.useCallback(() => {
    api
      .companies()
      .then(setCompanies)
      .catch(() => {
        /* Keep the last successful company snapshot on a transient failure. */
      });
    api
      .assetCounts()
      .then(setCounts)
      .catch(() => {
        /* Keep the last successful counters on a transient failure. */
      });
  }, []);

  // Manual refresh: reload counts/companies and re-fetch the current page.
  const refresh = React.useCallback(() => {
    loadMeta();
    setRefreshKey((k) => k + 1);
    setSelected(new Set());
  }, [loadMeta]);

  const toggleSelect = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = (ids: number[]) => {
    setSelected((prev) => {
      const allSelected = ids.every((id) => prev.has(id));
      const next = new Set(prev);
      if (allSelected) {
        ids.forEach((id) => {
          next.delete(id);
        });
      } else {
        ids.forEach((id) => {
          next.add(id);
        });
      }
      return next;
    });
  };

  const openDelete = (ids: number[]) => {
    setDeleteIds(ids);
    setDeleteOpen(true);
  };

  const confirmDelete = async () => {
    setDeleting(true);
    try {
      const res = await api.deleteAssets(deleteIds);
      toast.success(t("Deleted {count} assets", { count: res.deleted }));
      setSelected(new Set());
      refresh();
    } catch (e) {
      toast.error(t("Delete failed: {error}", { error: String((e as Error)?.message ?? e) }));
    } finally {
      setDeleting(false);
      setDeleteOpen(false);
    }
  };

  const confirmDeleteCompany = async () => {
    if (!companyDeleteTarget) return;
    setCompanyDeleting(true);
    try {
      const res = await api.deleteCompany(companyDeleteTarget.id, companyDeleteAssets);
      const msg =
        companyDeleteAssets && res.assets_deleted > 0
          ? t("Company deleted, along with {count} assets", { count: res.assets_deleted })
          : t("Company deleted");
      toast.success(msg);
      refresh();
    } catch (e) {
      toast.error(t("Delete failed: {error}", { error: String((e as Error)?.message ?? e) }));
    } finally {
      setCompanyDeleting(false);
      setCompanyDeleteTarget(null);
      setCompanyDeleteAssets(false);
    }
  };

  React.useEffect(() => {
    loadMeta();
  }, [loadMeta]);

  // Reset query + page + selection when switching tabs
  // biome-ignore lint/correctness/useExhaustiveDependencies: tab changes intentionally reset tab-local controls.
  React.useEffect(() => {
    setQuery("");
    setDslError("");
    setSelected(new Set());
    setRows([]);
    setRowsTab("");
    setTotal(0);
    setLoaded(false);
  }, [tab]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: these controls intentionally reset server pagination.
  React.useEffect(() => setPage(0), [tab, size, query, companyId]);

  const dslMode = query.trim() !== "";

  // Server-side paginated page loader for the active data tab (company tab excluded).
  // biome-ignore lint/correctness/useExhaustiveDependencies: refreshKey is an explicit manual-reload trigger.
  React.useEffect(() => {
    if (tab === "company") return;
    const request = ++assetsRequestRef.current;
    const dsl = query.trim();
    setLoading(true);
    const offset = page * size;
    const run = async () => {
      try {
        const r = dsl
          ? await api.searchAssets(dsl, tab, size, offset, companyId)
          : await api.assets(tab, size, offset, companyId);
        if (assetsRequestRef.current !== request) return;
        setRows(r.assets);
        setRowsTab(tab);
        setTotal(r.total);
        setDslError("");
      } catch (e) {
        if (assetsRequestRef.current !== request) return;
        setDslError(String((e as Error)?.message ?? e));
        setRows([]);
        setRowsTab(tab);
        setTotal(0);
      } finally {
        if (assetsRequestRef.current === request) {
          setLoading(false);
          setLoaded(true);
        }
      }
    };
    const tid = setTimeout(run, dsl ? 400 : 0);
    return () => {
      clearTimeout(tid);
      if (assetsRequestRef.current === request) assetsRequestRef.current++;
    };
  }, [tab, page, size, query, refreshKey, companyId]);

  React.useEffect(() => {
    if (tab === "company") return;
    const lastPage = Math.max(0, Math.ceil(total / size) - 1);
    if (page > lastPage) setPage(lastPage);
  }, [page, size, tab, total]);

  const companyById = React.useMemo(() => {
    const m = new Map<number, string>();
    for (const c of companies) m.set(c.id, c.name);
    return m;
  }, [companies]);

  const companyName = (id?: number) => (id ? (companyById.get(id) ?? "") : "");

  const tabCounts: Record<string, number> = { ...counts, company: companies.length };
  const totalAssets = Object.values(counts).reduce((a, b) => a + b, 0);

  // rows are already the current server-side page.
  const tabData = (type: string): Asset[] => (rowsTab === type ? rows : []);
  const currentRows = tabData(tab);
  const slice = <T,>(list: T[]) => list;

  const dataTabs = TABS.filter((x) => x.key !== "company");

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
    <div className="flex h-[calc(100vh-6rem)] min-h-0 flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("Assets")}</h1>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted-foreground tabular-nums">
            {t("{count} assets in total", { count: totalAssets })}
          </span>
          {selected.size > 0 && (
            <div className="flex items-center gap-1">
              <Button variant="destructive" size="sm" onClick={() => openDelete(Array.from(selected))}>
                <Trash2Icon className="size-3.5" /> {t("Delete selected ({count})", { count: selected.size })}
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                onClick={() => setSelected(new Set())}
                aria-label={t("Clear selection")}
              >
                <XIcon className="size-4" />
              </Button>
            </div>
          )}
          <Button variant="outline" size="sm" onClick={refresh} disabled={loading}>
            <RefreshCwIcon className={cn("size-4", loading && "animate-spin")} /> {t("Refresh")}
          </Button>
          <CompanyDialog onSaved={refresh} />
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab} className="flex min-h-0 flex-1 flex-col gap-4">
        <div className="overflow-x-auto overflow-y-hidden">
          <TabsList variant="default">
            {TABS.map((x) => (
              <TabsTrigger key={x.key} value={x.key}>
                <x.icon className="size-3.5" />
                {x.label}
                <span className="ml-1 tabular-nums text-muted-foreground">{tabCounts[x.key]}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </div>

        <TabsContent value="company" className="mt-0 flex min-h-0 flex-1 flex-col">
          <CompaniesTable
            companies={companies}
            onSaved={refresh}
            onDelete={(c) => {
              setCompanyDeleteTarget(c);
              setCompanyDeleteAssets(false);
            }}
          />
        </TabsContent>

        {dataTabs.map((x) => (
          <TabsContent key={x.key} value={x.key} className="mt-0 flex min-h-0 flex-1 flex-col gap-2">
            {searchBox}
            <AssetTable
              cols={ASSET_COLS[x.key] ?? []}
              loaded={loaded}
              total={total}
              page={page}
              size={size}
              onSize={setSize}
              onPage={setPage}
              rows={currentRows}
              selected={selected}
              onToggleAll={toggleSelectAll}
            >
              {tabData(x.key).map((a) => (
                <AssetRow
                  key={a.id}
                  a={a}
                  type={x.key}
                  selected={selected.has(a.id)}
                  onToggle={toggleSelect}
                  onDelete={openDelete}
                  companyName={companyName}
                />
              ))}
            </AssetTable>
          </TabsContent>
        ))}
      </Tabs>

      <DeleteAssetsDialog
        open={deleteOpen}
        count={deleteIds.length}
        busy={deleting}
        onOpenChange={setDeleteOpen}
        onConfirm={() => void confirmDelete()}
      />
      <DeleteCompanyDialog
        target={companyDeleteTarget}
        withAssets={companyDeleteAssets}
        busy={companyDeleting}
        onWithAssets={setCompanyDeleteAssets}
        onClose={() => {
          setCompanyDeleteTarget(null);
          setCompanyDeleteAssets(false);
        }}
        onConfirm={() => void confirmDeleteCompany()}
      />
    </div>
  );
}
