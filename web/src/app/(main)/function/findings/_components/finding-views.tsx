"use client";

import * as React from "react";

import Link from "next/link";

import { ArrowUpRightIcon, ChevronRightIcon } from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import { TablePagination } from "@/components/table-pagination";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { t } from "@/i18n";
import { cn } from "@/lib/utils";

import { AssetTree, type assetPathOf } from "./asset-tree";
import { fmtTime, SEVERITIES } from "./finding-meta";
import { type FindingRowProps, FindingsTable } from "./findings-table";
import { type FindingData, findingGroupKey } from "./use-finding-data";

interface GroupedViewProps {
  data: FindingData;
  rowProps: FindingRowProps;
}

// FindingsGroupedView lists findings in collapsible groups by task; each open group pages on its own.
export function FindingsGroupedView({ data, rowProps }: GroupedViewProps) {
  return (
    <div className="flex flex-col gap-3">
      {data.groups.map((group) => {
        const key = findingGroupKey(group);
        const groupOpen = data.expandedGroups.has(key);
        const state = data.groupFindings[key] ?? {
          items: [],
          total: group.count,
          page: 1,
          pageSize: 10,
          loaded: false,
          loading: false,
        };
        return (
          <Card key={key} className="gap-0 py-0">
            <CardHeader className="px-4 py-3">
              <div className="flex min-w-0 flex-wrap items-center gap-3">
                <button
                  type="button"
                  className="flex min-w-0 flex-1 items-center gap-3 text-left"
                  aria-expanded={groupOpen}
                  onClick={() => data.toggleGroup(key)}
                >
                  <ChevronRightIcon
                    className={cn(
                      "size-4 shrink-0 text-muted-foreground transition-transform",
                      groupOpen && "rotate-90",
                    )}
                  />
                  <div className="flex min-w-0 flex-col gap-1">
                    <CardTitle className="truncate text-sm">
                      {group.task_id === null
                        ? t("Unlinked / task deleted")
                        : group.task_name
                          ? t("{name} (task #{id})", { name: group.task_name, id: group.task_id })
                          : t("Task #{id}", { id: group.task_id })}
                    </CardTitle>
                    <CardDescription className="truncate" title={group.task_description}>
                      {group.task_description || t("Source task unavailable")}
                    </CardDescription>
                  </div>
                </button>
                <div className="flex flex-wrap items-center gap-2">
                  {group.task_status && <StatusBadge domain="task" value={group.task_status} dot />}
                  {SEVERITIES.map((level) => {
                    const count = group[level];
                    if (count === 0) return null;
                    return (
                      <span key={level} className="inline-flex items-center gap-1">
                        <StatusBadge domain="severity" value={level} dot />
                        <span className="text-xs tabular-nums text-muted-foreground">{count}</span>
                      </span>
                    );
                  })}
                  <span className="text-xs tabular-nums text-muted-foreground">{fmtTime(group.last_found_at)}</span>
                  {group.task_id !== null && (
                    <Button size="icon-sm" variant="ghost" asChild>
                      <Link
                        href={`/function/tasks/detail?id=${group.task_id}`}
                        aria-label={t("View task #{id}", { id: group.task_id })}
                      >
                        <ArrowUpRightIcon />
                      </Link>
                    </Button>
                  )}
                </div>
              </div>
            </CardHeader>
            {groupOpen && (
              <CardContent className="px-0">
                {state.loading && !state.loaded ? (
                  <div className="flex min-h-36 items-center justify-center">
                    <Spinner />
                  </div>
                ) : (
                  <>
                    <FindingsTable
                      items={state.items}
                      selectAllLabel={t("Select all on this page in this group")}
                      {...rowProps}
                    />
                    <TablePagination
                      page={state.page}
                      pageSize={state.pageSize}
                      total={state.total}
                      onPageChange={(nextPage) => void data.loadGroup(key, nextPage, state.pageSize)}
                      onPageSizeChange={(nextSize) => void data.loadGroup(key, 1, nextSize)}
                    />
                  </>
                )}
              </CardContent>
            )}
          </Card>
        );
      })}
      {data.groups.length === 0 && (
        <Card>
          <CardContent className="py-12 text-center text-sm text-muted-foreground">
            {t("No matching findings.")}
          </CardContent>
        </Card>
      )}
      <TablePagination
        page={data.page}
        pageSize={data.pageSize}
        total={data.groupTotal}
        onPageChange={data.setPage}
        onPageSizeChange={(nextPageSize) => {
          data.setPageSize(nextPageSize);
          data.setPage(1);
        }}
        pageSizeOptions={[5, 10, 20]}
      />
    </div>
  );
}

interface AssetViewProps {
  data: FindingData;
  assetPath: ReturnType<typeof assetPathOf>;
  /** The shared flat list (table + pagination), narrowed to the picked subtree. */
  list: React.ReactNode;
}

// FindingsAssetView is the asset tree on the left and the findings under the picked subtree on the right.
export function FindingsAssetView({ data, assetPath, list }: AssetViewProps) {
  return (
    <div className="grid min-h-0 items-start gap-4 lg:grid-cols-[20rem_minmax(0,1fr)] xl:grid-cols-[24rem_minmax(0,1fr)]">
      <Card className="gap-0 py-3 lg:sticky lg:top-4">
        <CardContent className="flex flex-col px-3">
          {data.assetTree.loading && !data.assetTree.loaded ? (
            <div className="flex min-h-36 items-center justify-center">
              <Spinner />
            </div>
          ) : (
            <AssetTree
              nodes={data.assetTree.nodes}
              selected={data.assetScope}
              onSelect={data.setAssetScope}
              loading={data.assetTree.loading}
              truncated={data.assetTree.truncated}
              droppedKinds={data.assetTree.droppedKinds}
              findingTotal={data.assetTree.findingTotal}
              onRefresh={() => void data.loadAssetTree()}
            />
          )}
        </CardContent>
      </Card>
      <div className="flex min-w-0 flex-col gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-1 text-sm text-muted-foreground">
          <button
            type="button"
            className={cn("hover:text-foreground", data.assetScope === null && "font-medium text-foreground")}
            onClick={() => data.setAssetScope(null)}
          >
            {t("All assets")}
          </button>
          {assetPath.map((node) => (
            <React.Fragment key={node.key}>
              <ChevronRightIcon className="size-3.5 shrink-0" aria-hidden="true" />
              <button
                type="button"
                className={cn(
                  "max-w-[16rem] truncate hover:text-foreground",
                  node.key === data.assetScope && "font-medium text-foreground",
                )}
                title={node.label}
                onClick={() => data.setAssetScope(node.key)}
              >
                {node.display}
              </button>
            </React.Fragment>
          ))}
          <span className="ml-auto shrink-0 text-xs tabular-nums">
            {t("{count} in total", { count: data.flat.total })}
          </span>
        </div>
        {list}
      </div>
    </div>
  );
}
