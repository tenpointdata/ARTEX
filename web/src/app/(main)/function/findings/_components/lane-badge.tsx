import { t } from "@/i18n";
import { codeLocation } from "@/lib/integrations";
import { type Tone, toneClasses } from "@/lib/status";
import type { FindingLane, FindingSource } from "@/lib/types";
import { cn } from "@/lib/utils";

export const LANE_META: Record<FindingLane, { label: string; tone: Tone }> = {
  runtime: { label: t("Runtime"), tone: "neutral" },
  sast: { label: t("SAST"), tone: "blue" },
  sca: { label: t("SCA"), tone: "violet" },
};

// LaneBadge shows the lane of a scanner-imported finding. Findings without a source are
// ARTEX's own runtime work, so they show nothing and the badge stays a signal for imports.
export function LaneBadge({ lane }: { lane?: FindingLane }) {
  const meta = lane ? LANE_META[lane] : undefined;
  if (!meta) return null;
  return (
    <span
      className={cn(
        "inline-flex w-fit items-center whitespace-nowrap rounded-md border px-2 py-0.5 font-medium text-xs",
        toneClasses[meta.tone],
      )}
      title={t("Finding lane")}
    >
      {meta.label}
    </span>
  );
}

// CodeLocation is the truncated "path:line" of a code-lane finding; the full text is its tooltip.
export function CodeLocation({
  source,
  className,
}: {
  source?: Pick<FindingSource, "location" | "line">;
  className?: string;
}) {
  const text = codeLocation(source);
  if (!text) return null;
  return (
    <code className={cn("block max-w-full truncate font-mono text-muted-foreground text-xs", className)} title={text}>
      {text}
    </code>
  );
}
