import * as React from "react";

import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { t } from "@/i18n";
import type { InterceptAction } from "@/lib/types";

// fmtTokens compacts token counts (1.2k / 3.4M) for approval usage stats.
export function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "k";
  return String(n);
}

// tRich translates a sentence whose {placeholders} are rendered as React nodes (e.g. emphasised spans or code).
export function tRich(key: string, nodes: Record<string, React.ReactNode>): React.ReactNode[] {
  return t(key)
    .split(/(\{\w+\})/)
    .map((part, i) => {
      const name = /^\{(\w+)\}$/.exec(part)?.[1];
      // biome-ignore lint/suspicious/noArrayIndexKey: static sentence fragments
      return name && name in nodes ? <React.Fragment key={i}>{nodes[name]}</React.Fragment> : part;
    });
}

// An argv pattern (JSON array of tokens) shown as a prefix: ["nmap"] -> "nmap *".
export function argvLabel(pattern: string): string {
  try {
    const tokens: unknown = JSON.parse(pattern);
    if (Array.isArray(tokens)) return `${tokens.join(" ")} *`;
  } catch {
    // not a JSON array: show the raw pattern
  }
  return pattern;
}

// ---- small components ----

export function ActionBadge({ action }: { action: InterceptAction }) {
  if (action === "allow") return <Badge variant="secondary">{t("Allow")}</Badge>;
  if (action === "deny") return <Badge variant="destructive">{t("Block")}</Badge>;
  return (
    <Badge variant="outline" className="border-amber-400 text-amber-600">
      {t("Ask")}
    </Badge>
  );
}

export function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs font-medium text-muted-foreground uppercase tracking-wide">{label}</Label>
      {children}
    </div>
  );
}
