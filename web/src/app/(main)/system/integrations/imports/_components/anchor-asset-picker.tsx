"use client";

import * as React from "react";

import { AnchorIcon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { assetLabel, taskAssetTypeLabel } from "@/lib/task-assets";
import type { Asset } from "@/lib/types";

const PAGE = 200;

// AnchorAssetPicker picks the task assets an import's findings are anchored to (the
// upload's asset_ids). The server only accepts assets already in the task's scope or asset
// set, so the list is that task's assets; without a task there is nothing to pick.
export function AnchorAssetPicker({
  taskId,
  value,
  onChange,
}: {
  taskId: string; // "" = no valid task chosen yet
  value: number[];
  onChange: (ids: number[]) => void;
}) {
  const uid = React.useId();
  const [assets, setAssets] = React.useState<Asset[]>([]);
  const [total, setTotal] = React.useState(0);
  const [loading, setLoading] = React.useState(false);
  const [error, setError] = React.useState("");
  const [filter, setFilter] = React.useState("");

  React.useEffect(() => {
    setAssets([]);
    setTotal(0);
    setError("");
    setFilter("");
    if (!taskId) return;
    let alive = true;
    setLoading(true);
    api
      .taskAssets(taskId, "", PAGE, 0)
      .then((r) => {
        if (!alive) return;
        setAssets(r.assets);
        setTotal(r.total);
      })
      .catch((e) => alive && setError((e as Error).message))
      .finally(() => alive && setLoading(false));
    return () => {
      alive = false;
    };
  }, [taskId]);

  const selected = React.useMemo(() => new Set(value), [value]);
  const needle = filter.trim().toLowerCase();
  const rows = needle
    ? assets.filter((a) => `${assetLabel(a)} ${taskAssetTypeLabel(a.type)}`.toLowerCase().includes(needle))
    : assets;

  function toggle(id: number, checked: boolean) {
    onChange(checked ? [...value, id] : value.filter((v) => v !== id));
  }

  const disabled = !taskId;
  return (
    <Popover>
      <div className="flex items-center gap-1">
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            title={disabled ? t("Choose a task to anchor the findings to its assets") : undefined}
          >
            <AnchorIcon />
            {value.length > 0 ? t("{count} anchor assets", { count: value.length }) : t("Anchor assets")}
          </Button>
        </PopoverTrigger>
        {value.length > 0 && (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("Clear the anchor assets")}
            onClick={() => onChange([])}
          >
            <XIcon />
          </Button>
        )}
      </div>
      <PopoverContent align="start" className="w-96 gap-2 p-2">
        <Input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder={t("Filter assets")}
          aria-label={t("Filter assets")}
        />
        <div className="max-h-64 overflow-y-auto">
          {error ? (
            <p className="text-destructive p-2 text-xs">{t("Failed to load assets: {error}", { error })}</p>
          ) : loading ? (
            <p className="text-muted-foreground p-2 text-xs">{t("Loading…")}</p>
          ) : rows.length === 0 ? (
            <p className="text-muted-foreground p-2 text-xs">{t("No assets")}</p>
          ) : (
            <ul>
              {rows.map((a) => (
                <li key={a.id}>
                  <label
                    htmlFor={`${uid}-asset-${a.id}`}
                    className="hover:bg-muted flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm"
                  >
                    <Checkbox
                      id={`${uid}-asset-${a.id}`}
                      checked={selected.has(a.id)}
                      onCheckedChange={(c) => toggle(a.id, c === true)}
                    />
                    <span className="min-w-0 flex-1 truncate font-mono text-xs" title={assetLabel(a)}>
                      {assetLabel(a)}
                    </span>
                    <span className="text-muted-foreground shrink-0 text-xs">{taskAssetTypeLabel(a.type)}</span>
                  </label>
                </li>
              ))}
            </ul>
          )}
        </div>
        {total > assets.length && (
          <p className="text-muted-foreground px-2 text-xs">
            {t("Showing the first {count} of {total} assets", { count: assets.length, total })}
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}
