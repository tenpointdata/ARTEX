"use client";

import * as React from "react";

import { DownloadIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Finding } from "@/lib/types";
import { cn } from "@/lib/utils";

import type { FindingFilters } from "./finding-filters";

interface DeepenDialogProps {
  /** The finding to deepen; null = closed. */
  finding: Finding | null;
  onClose: () => void;
  onDone: (finding: Finding) => void;
}

// DeepenDialog queues a high-priority worker intent in the finding's task to push exploitation further.
export function DeepenDialog({ finding, onClose, onDone }: DeepenDialogProps) {
  const [description, setDescription] = React.useState("");
  const [deepening, setDeepening] = React.useState(false);
  const close = () => {
    setDescription("");
    onClose();
  };

  async function submit() {
    if (!finding?.finding_id || !description.trim() || deepening) return;
    setDeepening(true);
    try {
      const result = await api.deepenFinding(finding.finding_id, description.trim());
      toast.success(
        result.queued
          ? t("Deepen intent #{id} has entered the task queue", { id: result.intent_id })
          : t("Created high-priority worker intent #{id}", { id: result.intent_id }),
      );
      onDone(finding);
      close();
    } catch (error) {
      toast.error(t("Submit failed: {error}", { error: (error as Error).message }));
    } finally {
      setDeepening(false);
    }
  }

  return (
    <Dialog
      open={finding !== null}
      onOpenChange={(open) => {
        if (open || deepening) return;
        close();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("Deepen exploitation")}</DialogTitle>
          <DialogDescription className="break-words">
            {t(
              "A priority-10 worker intent will be created in the original task #{id} to run secondary verification based on this vulnerability:",
              { id: finding?.task_id ?? "" },
            )}{" "}
            {finding?.name || finding?.vulnclass || finding?.summary}
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="finding-deepen-description">{t("Exploitation description")}</FieldLabel>
            <Textarea
              id="finding-deepen-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              maxLength={4000}
              placeholder={t(
                "Describe the exploitation path to verify, boundary conditions, targets, or expected evidence",
              )}
              disabled={deepening}
            />
            <FieldDescription className="flex justify-between gap-3">
              <span>{t("The new intent inherits this vulnerability's asset anchors.")}</span>
              <span className="shrink-0 tabular-nums">{description.length} / 4000</span>
            </FieldDescription>
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={close} disabled={deepening}>
            {t("Cancel")}
          </Button>
          <Button onClick={() => void submit()} disabled={deepening || !description.trim()}>
            {deepening && <Spinner data-icon="inline-start" />}
            {t("Create deepen intent")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type ExportScope = "filtered" | "all" | "selected";
type ExportFormat = "md-single" | "md-zip" | "csv" | "json";

interface ExportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  filters: FindingFilters;
  company: string | undefined;
  importId: string;
  assetScope: string | null;
  selectedIds: Set<string>;
  /** The current filter's result count, shown on the "current filter" choice. */
  filteredTotal: number;
}

// ExportDialog exports findings: scope (current filter / all / selected) × format (single md / per-finding md zip / csv / json).
// The scope defaults to "Selected" when rows are checked, else "Current filter".
export function ExportDialog({
  open,
  onOpenChange,
  filters,
  company,
  importId,
  assetScope,
  selectedIds,
  filteredTotal,
}: ExportDialogProps) {
  const [exportScope, setExportScope] = React.useState<ExportScope>("filtered");
  const [exportFormat, setExportFormat] = React.useState<ExportFormat>("md-single");
  const [exporting, setExporting] = React.useState(false);
  const setExportOpen = onOpenChange;
  const hasSelection = selectedIds.size > 0;

  React.useEffect(() => {
    if (open) setExportScope(hasSelection ? "selected" : "filtered");
  }, [open, hasSelection]);

  async function doExport() {
    setExporting(true);
    try {
      await api.exportFindings({
        format: exportFormat,
        scope: exportScope,
        filters: { ...filters, assetScope: assetScope ?? undefined, company, importId },
        ids: [...selectedIds],
      });
      setExportOpen(false);
      toast.success(t("Export download started"));
    } catch (e) {
      toast.error(t("Export failed: {error}", { error: (e as Error).message }));
    } finally {
      setExporting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("Export findings")}</DialogTitle>
          <DialogDescription>
            {t("Choose the export scope and format; the browser downloads the file automatically once generated.")}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-5 py-1">
          <div className="flex flex-col gap-2">
            <span className="text-xs text-muted-foreground">{t("Export scope")}</span>
            <RadioGroup value={exportScope} onValueChange={(v) => setExportScope(v as typeof exportScope)}>
              <label htmlFor="export-scope-filtered" className="flex items-center gap-2 text-sm">
                <RadioGroupItem id="export-scope-filtered" value="filtered" />{" "}
                {t("Export the current filter results ({count} items)", { count: filteredTotal })}
              </label>
              <label htmlFor="export-scope-all" className="flex items-center gap-2 text-sm">
                <RadioGroupItem id="export-scope-all" value="all" /> {t("Export all")}
              </label>
              <label
                htmlFor="export-scope-selected"
                className={cn("flex items-center gap-2 text-sm", selectedIds.size === 0 && "text-muted-foreground")}
              >
                <RadioGroupItem id="export-scope-selected" value="selected" disabled={selectedIds.size === 0} />
                {t("Export the {count} selected items", { count: selectedIds.size })}
              </label>
            </RadioGroup>
          </div>

          <div className="flex flex-col gap-2">
            <span className="text-xs text-muted-foreground">{t("Export format")}</span>
            <RadioGroup value={exportFormat} onValueChange={(v) => setExportFormat(v as typeof exportFormat)}>
              <label htmlFor="export-format-md-single" className="flex items-center gap-2 text-sm">
                <RadioGroupItem id="export-format-md-single" value="md-single" />{" "}
                {t("Markdown summary report (a single .md file)")}
              </label>
              <label htmlFor="export-format-md-zip" className="flex items-center gap-2 text-sm">
                <RadioGroupItem id="export-format-md-zip" value="md-zip" />{" "}
                {t("Markdown per finding (one .md per vulnerability, packaged as .zip)")}
              </label>
              <label htmlFor="export-format-csv" className="flex items-center gap-2 text-sm">
                <RadioGroupItem id="export-format-csv" value="csv" /> {t("CSV table (.csv)")}
              </label>
              <label htmlFor="export-format-json" className="flex items-center gap-2 text-sm">
                <RadioGroupItem id="export-format-json" value="json" /> JSON (.json)
              </label>
            </RadioGroup>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setExportOpen(false)} disabled={exporting}>
            {t("Cancel")}
          </Button>
          <Button
            onClick={() => void doExport()}
            disabled={exporting || (exportScope === "selected" && selectedIds.size === 0)}
          >
            <DownloadIcon /> {exporting ? t("Exporting…") : t("Export")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
