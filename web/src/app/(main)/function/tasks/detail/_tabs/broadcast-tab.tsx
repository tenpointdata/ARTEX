"use client";

import * as React from "react";

import {
  ArrowDownIcon,
  ArrowUpIcon,
  ArrowUpToLineIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  PauseIcon,
  PlayIcon,
  SearchIcon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { Edge, ExploreKind, FindingAsset, TaskNode } from "@/lib/types";
import { cn } from "@/lib/utils";

import { dayFmt, FILTER_KINDS, KIND_META, PAGE_SIZES, POLL_MS } from "./broadcast-meta";
import { BroadcastRow } from "./broadcast-parts";

export function BroadcastTab({ taskId }: { taskId: string }) {
  const [kinds, setKinds] = React.useState<ExploreKind[]>([]);
  const [queryInput, setQueryInput] = React.useState("");
  const [query, setQuery] = React.useState("");
  const [order, setOrder] = React.useState<"asc" | "desc">("desc");
  const [page, setPage] = React.useState(1);
  const [size, setSize] = React.useState(20);
  const [live, setLive] = React.useState(true);

  const [items, setItems] = React.useState<TaskNode[]>([]);
  const [edges, setEdges] = React.useState<Edge[]>([]);
  const [refs, setRefs] = React.useState<Record<string, TaskNode>>({});
  const [assets, setAssets] = React.useState<Record<string, FindingAsset[]>>({});
  const [total, setTotal] = React.useState(0);
  const [loaded, setLoaded] = React.useState(false);
  const [freshIDs, setFreshIDs] = React.useState<Set<string>>(new Set());
  const [pending, setPending] = React.useState(0);
  const [now, setNow] = React.useState(0);

  const seenRef = React.useRef<Set<string>>(new Set());
  const baselineRef = React.useRef<number | null>(null);
  const streamRef = React.useRef("");
  // Only "newest-first page 1" is the real live seat; every other position only updates the unread count,
  // leaving the list alone so content does not shift under pagination/expand.
  const atLive = page === 1 && order === "desc";

  React.useEffect(() => {
    setNow(Date.now());
    const t = setInterval(() => setNow(Date.now()), 30000);
    return () => clearInterval(t);
  }, []);

  // Input debounce: the query fires 300ms after typing stops, and returns to page 1.
  React.useEffect(() => {
    const t = setTimeout(() => {
      setQuery(queryInput);
      setPage(1);
    }, 300);
    return () => clearTimeout(t);
  }, [queryInput]);

  React.useEffect(() => {
    let alive = true;
    let rendered = false; // whether this query has already rendered its content
    // Switching task/filters/sort = switching to a different broadcast stream: clear "new" marks and the unread baseline. Paging is not a stream switch,
    // otherwise there would be no unread count to compute when returning to the latest.
    const stream = `${taskId}|${kinds.join(",")}|${query}|${order}`;
    if (streamRef.current !== stream) {
      streamRef.current = stream;
      seenRef.current = new Set();
      baselineRef.current = null;
      setPending(0);
    }
    const load = () =>
      api
        .explorationNodes(taskId, { page, size, kinds, q: query, order })
        .then((r) => {
          if (!alive) return;
          // The live seat refreshes every round; other positions render only the first time, then polls just update the unread count.
          if (atLive || !rendered) {
            rendered = true;
            setItems(r.items);
            setEdges(r.edges);
            setRefs(r.refs);
            setAssets(r.assets);
          }
          setTotal(r.total);
          if (atLive) {
            const seen = seenRef.current;
            setFreshIDs(seen.size === 0 ? new Set() : new Set(r.items.filter((n) => !seen.has(n.id)).map((n) => n.id)));
            seenRef.current = new Set(r.items.map((n) => n.id));
            baselineRef.current = r.total;
            setPending(0);
          } else {
            const base = baselineRef.current;
            setPending(base === null ? 0 : Math.max(0, r.total - base));
          }
          setLoaded(true);
        })
        .catch(() => {
          // Polling is best-effort: keep the last successful broadcast content and retry automatically next round.
        });
    void load();
    if (!live) {
      return () => {
        alive = false;
      };
    }
    const stopPolling = setVisibleInterval(load, POLL_MS);
    return () => {
      alive = false;
      stopPolling();
    };
  }, [taskId, page, size, kinds, query, order, live, atLive]);

  const toggleKind = (kind: ExploreKind) => {
    setKinds((cur) => (cur.includes(kind) ? cur.filter((k) => k !== kind) : [...cur, kind]));
    setPage(1);
  };

  const backToLive = () => {
    setPage(1);
    setOrder("desc");
    setPending(0);
  };

  // Server-side refs only fill in "neighbors not on this page"; references between same-page nodes must fall back to items,
  // otherwise adjacent broadcasts referencing each other degrade to a bare "Node #id".
  const nodeIndex = React.useMemo(() => {
    const idx: Record<string, TaskNode> = { ...refs };
    for (const n of items) idx[n.id] = n;
    return idx;
  }, [refs, items]);

  const pageCount = Math.max(1, Math.ceil(total / size));
  const start = total === 0 ? 0 : (page - 1) * size + 1;
  const end = (page - 1) * size + items.length;

  // When switching tasks or filters shrinks the count, pull the page number back in bounds.
  React.useEffect(() => {
    if (page > pageCount) setPage(pageCount);
  }, [page, pageCount]);

  // Group by day: the stream breaks at date boundaries, so paging a long task still shows which day things happened.
  const groups: Array<{ day: string; rows: TaskNode[] }> = [];
  for (const node of items) {
    const ts = Date.parse(node.ts);
    const day = Number.isNaN(ts) ? t("Unknown date") : dayFmt.format(ts);
    const last = groups[groups.length - 1];
    if (last && last.day === day) last.rows.push(node);
    else groups.push({ day, rows: [node] });
  }

  return (
    <Card className="overflow-hidden py-0">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 border-b px-4 py-2.5">
        <div className="relative w-full sm:w-64">
          <SearchIcon className="absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={queryInput}
            onChange={(e) => setQueryInput(e.target.value)}
            placeholder={t("Search content / source / node id")}
            className="h-8 pl-8"
            aria-label={t("Search broadcasts")}
          />
        </div>
        <div className="flex flex-wrap items-center gap-1">
          {FILTER_KINDS.map((kind) => {
            const meta = KIND_META[kind];
            const active = kinds.includes(kind);
            return (
              <button
                key={kind}
                type="button"
                onClick={() => toggleKind(kind)}
                aria-pressed={active}
                className={cn(
                  "rounded-md border px-2 py-0.5 text-xs font-medium transition-colors",
                  active ? meta.chip : "border-transparent text-muted-foreground hover:bg-accent",
                )}
              >
                {meta.label}
              </button>
            );
          })}
          {kinds.length > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={() => {
                setKinds([]);
                setPage(1);
              }}
            >
              {t("Clear")}
            </Button>
          )}
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() => {
              setOrder((o) => (o === "desc" ? "asc" : "desc"));
              setPage(1);
            }}
            aria-label={
              order === "desc"
                ? t("Currently newest first; click to switch to oldest first")
                : t("Currently oldest first; click to switch to newest first")
            }
          >
            {order === "desc" ? <ArrowDownIcon /> : <ArrowUpIcon />}
            {order === "desc" ? t("Newest first") : t("Oldest first")}
          </Button>
          <Button
            variant={live ? "outline" : "secondary"}
            size="sm"
            className="h-8"
            onClick={() => setLive((v) => !v)}
            aria-label={live ? t("Pause auto-refresh") : t("Resume auto-refresh")}
          >
            {live ? <PauseIcon /> : <PlayIcon />}
            {live ? t("Auto-refresh") : t("Paused")}
          </Button>
        </div>
      </div>

      {/* Unread hint shown when away from the live seat */}
      {!atLive && pending > 0 && (
        <button
          type="button"
          onClick={backToLive}
          className="flex w-full items-center justify-center gap-1.5 border-b bg-primary/10 py-1.5 text-xs font-medium text-primary hover:bg-primary/15"
        >
          <ArrowUpToLineIcon className="size-3.5" />
          {t("{count} new broadcasts · Back to latest", { count: pending > 99 ? "99+" : pending })}
        </button>
      )}

      <CardContent className="px-4 py-0">
        {!loaded ? (
          <div className="flex flex-col gap-3 py-4">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">
            {query || kinds.length > 0
              ? t("No broadcasts match the filters.")
              : t("This task has not produced any exploration nodes yet.")}
          </p>
        ) : (
          groups.map((group) => (
            <div key={group.day}>
              <div className="py-2 pl-[6.25rem] text-xs font-medium text-muted-foreground">{group.day}</div>
              {group.rows.map((node) => (
                <BroadcastRow
                  key={node.id}
                  node={node}
                  edges={edges}
                  refs={nodeIndex}
                  assets={assets}
                  now={now}
                  fresh={freshIDs.has(node.id)}
                />
              ))}
            </div>
          ))
        )}
      </CardContent>

      <CardFooter className="flex flex-wrap items-center gap-2 border-t px-4 py-2.5 text-xs text-muted-foreground">
        <Select
          value={String(size)}
          onValueChange={(v) => {
            setSize(Number(v));
            setPage(1);
          }}
        >
          <SelectTrigger size="sm" className="h-7 w-24">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {PAGE_SIZES.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {t("{count} / page", { count: n })}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <span className="tabular-nums">
          {start}–{end} / {total}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <Button
            variant="outline"
            size="icon-sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            aria-label={t("Previous page")}
          >
            <ChevronLeftIcon />
          </Button>
          <span className="tabular-nums">
            {page} / {pageCount}
          </span>
          <Button
            variant="outline"
            size="icon-sm"
            disabled={page >= pageCount}
            onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
            aria-label={t("Next page")}
          >
            <ChevronRightIcon />
          </Button>
        </div>
      </CardFooter>
    </Card>
  );
}
