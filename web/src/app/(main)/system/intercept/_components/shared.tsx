import * as React from "react";

import { Label } from "@/components/ui/label";
import { t } from "@/i18n";

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

export function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs font-medium text-muted-foreground uppercase tracking-wide">{label}</Label>
      {children}
    </div>
  );
}
