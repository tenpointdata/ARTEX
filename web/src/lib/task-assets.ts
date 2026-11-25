import { t } from "@/i18n";
import type { Asset, NewAssetType } from "@/lib/types";

const ASSET_TYPE_LABELS: Record<NewAssetType, string> = {
  app: t("App"),
  endpoint: t("Endpoint"),
  ip: "IP",
  root_domain: t("Root domain"),
  service: t("Service"),
  subdomain: t("Subdomain"),
};

const TASK_ASSET_SOURCE_LABELS: Record<string, string> = {
  agent: t("Agent-discovered"),
  anchor: t("Blackboard anchor"),
  api: t("Assets API"),
  company: t("Company association"),
  legacy: t("Legacy association"),
  manual: t("Manually added"),
  system: t("System association"),
  task: t("Task initialization"),
};

export function taskAssetTypeLabel(type: NewAssetType): string {
  return ASSET_TYPE_LABELS[type];
}

export function taskAssetSourceLabel(source: string): string {
  return TASK_ASSET_SOURCE_LABELS[source] ?? source;
}

export function firstText(values: Array<string | undefined>, fallback: string): string {
  return values.find((value) => value?.trim()) ?? fallback;
}

// assetLabel is the one-line name of an asset: its domain, IP, URL or host:port, else #id.
export function assetLabel(asset: Asset): string {
  switch (asset.type) {
    case "root_domain":
    case "subdomain":
      return firstText([asset.domain], `#${asset.id}`);
    case "ip":
      return firstText([asset.ip], `#${asset.id}`);
    case "app":
      return firstText([asset.app_name, asset.url], `#${asset.id}`);
    case "service": {
      const host = firstText([asset.ip, asset.domain], "");
      const address = [host, asset.port].filter((value) => value !== undefined && value !== "").join(":");
      return firstText([asset.url, address, asset.service_name], `#${asset.id}`);
    }
    case "endpoint":
      return firstText([[asset.method, asset.url].filter(Boolean).join(" ")], `#${asset.id}`);
  }
}
