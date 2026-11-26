"use client";

import * as React from "react";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { t } from "@/i18n";
import { FINDING_LANES, laneParam } from "@/lib/integrations";
import type { FindingLane } from "@/lib/types";

import { LANE_META } from "./lane-badge";

// useLaneFilter keeps the findings page's lane filter in the URL (?lane=sast), like the
// import_id deep link: it is shareable, applies at once, and never sticks to a later visit.
export function useLaneFilter(): { lane: FindingLane | ""; setLane: (lane: FindingLane | "") => void } {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const lane = laneParam(params.get("lane"));
  const setLane = React.useCallback(
    (next: FindingLane | "") => {
      const q = new URLSearchParams(params.toString());
      if (next) q.set("lane", next);
      else q.delete("lane");
      const qs = q.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname);
    },
    [params, pathname, router],
  );
  return { lane, setLane };
}

const ALL = "all";

// LaneFilterSelect is the lane dropdown of the findings filter bar.
export function LaneFilterSelect({
  lane,
  onChange,
}: {
  lane: FindingLane | "";
  onChange: (lane: FindingLane | "") => void;
}) {
  return (
    <Select value={lane || ALL} onValueChange={(v) => onChange(laneParam(v))}>
      <SelectTrigger size="sm" className="w-32" aria-label={t("Filter by lane")}>
        <SelectValue placeholder={t("Lane")} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>{t("All lanes")}</SelectItem>
        {FINDING_LANES.map((l) => (
          <SelectItem key={l} value={l}>
            {LANE_META[l].label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
