"use client";

import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { t } from "@/i18n";
import { TRIAGE_VERDICTS, type TriageNav } from "@/lib/triage";
import type { Finding, FindingStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

import { findingStatusLabel } from "./finding-meta";

interface TriageBarProps {
  finding: Finding;
  busy?: boolean;
  onVerdict: (status: FindingStatus) => void;
  onNav: (nav: TriageNav) => void;
  hasPrev: boolean;
  hasNext: boolean;
}

// TriageBar is the open finding's one-click verdicts (each with its key) plus skip controls.
export function TriageBar({ finding, busy, onVerdict, onNav, hasPrev, hasNext }: TriageBarProps) {
  return (
    <div
      className="flex flex-wrap items-center gap-1.5 rounded-md border bg-background px-3 py-2"
      role="toolbar"
      aria-label={t("Triage this finding")}
    >
      <span className="mr-1 text-xs font-medium text-muted-foreground">{t("Verdict")}</span>
      {TRIAGE_VERDICTS.map((v) => {
        const current = finding.status === v.status;
        return (
          <Button
            key={v.status}
            size="sm"
            variant={current ? "secondary" : "outline"}
            className={cn("h-7 gap-1.5", current && "pointer-events-none")}
            disabled={busy}
            aria-pressed={current}
            aria-keyshortcuts={v.key}
            onClick={() => onVerdict(v.status)}
          >
            {findingStatusLabel(v.status)}
            <Kbd>{v.key.toUpperCase()}</Kbd>
          </Button>
        );
      })}
      <div className="ml-auto flex items-center gap-1">
        <Button
          size="sm"
          variant="ghost"
          className="h-7 gap-1.5"
          disabled={!hasPrev}
          aria-keyshortcuts="k"
          onClick={() => onNav("prev")}
        >
          <ChevronUpIcon data-icon="inline-start" />
          {t("Previous")}
          <Kbd>K</Kbd>
        </Button>
        <Button
          size="sm"
          variant="ghost"
          className="h-7 gap-1.5"
          disabled={!hasNext}
          aria-keyshortcuts="j"
          onClick={() => onNav("next")}
        >
          <ChevronDownIcon data-icon="inline-start" />
          {t("Skip")}
          <Kbd>J</Kbd>
        </Button>
      </div>
    </div>
  );
}
