"use client";

import type * as React from "react";

import { FileTextIcon, ShieldAlertIcon } from "lucide-react";

import { CopyButton } from "@/components/copy-button";
import { Markdown } from "@/components/markdown";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { TableCell, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { statusMeta } from "@/lib/status";
import type { Finding, Severity } from "@/lib/types";

import { type FindingEdit, type FindingReport, SEVERITIES } from "./finding-meta";

interface FindingExpandedRowProps {
  finding: Finding;
  columnCount: number;
  report: FindingReport | undefined;
  edit: FindingEdit | null;
  onEditChange: React.Dispatch<React.SetStateAction<FindingEdit | null>>;
  saving: boolean;
  onSave: (finding: Finding) => void;
  /** The triage verdict bar, shown first so the verdict is one click away from the evidence. */
  triage?: React.ReactNode;
}

// FindingExpandedRow is a findings-table row's inline detail: triage bar, edit buffer, evidence and the lazily loaded report.
export function FindingExpandedRow({
  finding: f,
  columnCount,
  report: rep,
  edit,
  onEditChange,
  saving,
  onSave,
  triage,
}: FindingExpandedRowProps) {
  return (
    <TableRow className="hover:bg-transparent">
      {/* whitespace-normal overrides TableCell's default nowrap, otherwise the expanded area's text
          is forced onto one line and overflows the cell. */}
      <TableCell colSpan={columnCount} className="bg-muted/30 whitespace-normal">
        <div className="flex flex-col gap-2 px-2 py-1">
          {triage}
          {/* Inline edit: name/category/severity, editable and savable (standalone finding rows only). */}
          {f.finding_id && edit && (
            <div className="flex flex-wrap items-end gap-3 rounded-md border bg-background px-3 py-2.5">
              <div className="flex min-w-[12rem] flex-1 flex-col gap-1">
                <Label className="text-xs text-muted-foreground">{t("Vulnerability")}</Label>
                <Input
                  value={edit.name}
                  onChange={(e) => onEditChange((s) => (s ? { ...s, name: e.target.value } : s))}
                  placeholder={t("Readable title; falls back to category when empty")}
                />
              </div>
              <div className="flex min-w-[10rem] flex-col gap-1">
                <Label className="text-xs text-muted-foreground">{t("Category")}</Label>
                <Input
                  value={edit.vulnclass}
                  onChange={(e) => onEditChange((s) => (s ? { ...s, vulnclass: e.target.value } : s))}
                  placeholder={t("e.g. SQL Injection")}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label className="text-xs text-muted-foreground">{t("Severity")}</Label>
                <Select
                  value={edit.severity}
                  onValueChange={(v) => onEditChange((s) => (s ? { ...s, severity: v as Severity } : s))}
                >
                  <SelectTrigger size="sm" className="w-28">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SEVERITIES.map((sv) => (
                      <SelectItem key={sv} value={sv}>
                        {statusMeta("severity", sv).label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <Button size="sm" disabled={saving} onClick={() => onSave(f)}>
                {saving ? t("Saving…") : t("Save")}
              </Button>
            </div>
          )}
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <ShieldAlertIcon className="size-3.5" />
            {t("Evidence")}
            {f.vulnclass && (
              <span>
                · {t("Type:")}
                <code className="rounded bg-muted px-1.5 py-0.5 font-mono">{f.vulnclass}</code>
              </span>
            )}
            {f.param_id && <code className="rounded bg-muted px-1.5 py-0.5 font-mono">{f.param_id}</code>}
            {f.assets && f.assets.length > 0 && (
              <span className="flex flex-wrap items-center gap-1">
                · {t("Assets:")}
                {f.assets.map((a) => (
                  <code key={a.id} className="rounded bg-muted px-1.5 py-0.5 font-mono" title={a.type}>
                    {a.label}
                  </code>
                ))}
              </span>
            )}
          </div>
          <pre className="overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs whitespace-pre-wrap">
            {f.evidence}
          </pre>

          {/* Detailed report (Markdown): lazily loaded by finding_id on expand, viewable without entering the detail page. */}
          {f.finding_id && (
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                <span className="flex items-center gap-2">
                  <FileTextIcon className="size-3.5" />
                  {t("Detailed report")}
                </span>
                {rep?.status === "done" && rep.text.trim() && (
                  <CopyButton
                    text={rep.text}
                    successMessage={t("Detailed report copied")}
                    variant="ghost"
                    className="h-6 px-2 text-xs"
                  />
                )}
              </div>
              {(() => {
                if (!rep || rep.status === "loading")
                  return <p className="text-xs text-muted-foreground">{t("Loading…")}</p>;
                if (rep.status === "error")
                  return <p className="text-xs text-muted-foreground">{t("Failed to load the report.")}</p>;
                if (!rep.text.trim())
                  return <p className="text-xs text-muted-foreground">{t("No detailed report yet.")}</p>;
                return (
                  // break-words inherits into paragraphs/lists; pre additionally gets
                  // whitespace-pre-wrap so code blocks wrap too — otherwise long code lines/URLs
                  // would stretch the colSpan cell and push the whole table into horizontal scrolling.
                  <div className="min-w-0 break-words rounded-md border bg-background px-3 py-2 [&_pre]:whitespace-pre-wrap">
                    <Markdown text={rep.text} />
                  </div>
                );
              })()}
            </div>
          )}
        </div>
      </TableCell>
    </TableRow>
  );
}
