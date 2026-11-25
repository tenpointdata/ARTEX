"use client";

import * as React from "react";

import Link from "next/link";

import { ArrowLeftIcon, UploadIcon } from "lucide-react";
import { toast } from "sonner";

import { TablePagination } from "@/components/table-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { intlLocale, t } from "@/i18n";
import { detectImportFormat, IMPORT_FORMATS, importFindingsHref, laneCountEntries } from "@/lib/integrations";
import { integrationsApi } from "@/lib/integrations-api";
import type { ScanImport, ScanImportFormat, ScanImportOutcome } from "@/lib/types";

import { LANE_META } from "../../../function/findings/_components/lane-badge";
import { AnchorAssetPicker } from "./_components/anchor-asset-picker";

const FORMAT_LABEL: Record<ScanImportFormat, string> = {
  "nuclei-jsonl": "Nuclei (JSONL)",
  "nmap-xml": "Nmap (XML)",
  "zap-json": "OWASP ZAP (JSON)",
  "burp-xml": "Burp Suite (XML)",
  sarif: "SARIF 2.1.0 (Semgrep…)",
};

const MAX_IMPORT_BYTES = 32 * 1024 * 1024; // server/scan_import_api.go decodeMaxBytes

function fileHint(tooLarge: boolean, detected: boolean): string {
  if (tooLarge) return t("The file exceeds the 32 MiB import limit");
  return detected ? t("Format detected from the file") : t("Could not detect the format; pick it above");
}

function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(intlLocale);
}

// Scanner imports: upload a scanner's output file; assets get provenance and results become
// unverified POE-0 findings. A re-import never edits or promotes an existing finding.
// Importing needs operate (server/rbac_policy.go); a read-only role sees the history only.
export default function ScanImportsPage() {
  const canImport = useCan("operate");
  const [file, setFile] = React.useState<File | null>(null);
  const [format, setFormat] = React.useState<ScanImportFormat | "">("");
  const [detected, setDetected] = React.useState(false);
  const [taskId, setTaskId] = React.useState("");
  const [anchorIds, setAnchorIds] = React.useState<number[]>([]);
  const [busy, setBusy] = React.useState(false);
  const [outcome, setOutcome] = React.useState<ScanImportOutcome | null>(null);
  const [imports, setImports] = React.useState<ScanImport[]>([]);
  const [total, setTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(20);
  const [loaded, setLoaded] = React.useState(false);
  const inputRef = React.useRef<HTMLInputElement>(null);

  const load = React.useCallback(() => {
    integrationsApi
      .imports(pageSize, (page - 1) * pageSize)
      .then((r) => {
        setImports(r.items);
        setTotal(r.total);
      })
      .catch((e) => toast.error(t("Failed to load imports: {error}", { error: (e as Error).message })))
      .finally(() => setLoaded(true));
  }, [page, pageSize]);
  React.useEffect(load, [load]);

  // Picking a file detects its format, so the usual flow is pick → Import.
  async function pick(f: File | null) {
    setFile(f);
    setOutcome(null);
    if (!f) return;
    const guess = detectImportFormat(f.name, await f.slice(0, 4096).text());
    setDetected(guess !== null);
    if (guess) setFormat(guess);
  }

  const taskNum = taskId.trim() === "" ? undefined : Number(taskId);
  const taskInvalid = taskNum !== undefined && !(Number.isSafeInteger(taskNum) && taskNum > 0);
  // Anchor assets belong to the chosen task, so a change of task drops them (the server answers 400 for the rest).
  const anchorTask = taskNum !== undefined && !taskInvalid ? String(taskNum) : "";
  const tooLarge = (file?.size ?? 0) > MAX_IMPORT_BYTES;

  async function runImport() {
    if (!file || !format || taskInvalid || tooLarge) return;
    setBusy(true);
    try {
      const out = await integrationsApi.importScan(file, format, taskNum, anchorIds);
      setOutcome(out);
      toast.success(
        t("Imported {file}: {assets} assets, {new} new findings", {
          file: file.name,
          assets: out.import.assets,
          new: out.import.findings_new,
        }),
      );
      setFile(null);
      setAnchorIds([]);
      if (inputRef.current) inputRef.current.value = "";
      // The new import is the newest row: show page one (setPage(1) alone does not reload when already there).
      if (page === 1) load();
      else setPage(1);
    } catch (e) {
      toast.error(t("Import failed: {error}", { error: (e as Error).message }), { duration: 10000 });
    } finally {
      setBusy(false);
    }
  }

  const problems = [...(outcome?.asset_errors ?? []), ...(outcome?.finding_errors ?? [])];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="ghost" size="icon-sm" asChild aria-label={t("Back to integrations")}>
          <Link href="/system/integrations">
            <ArrowLeftIcon />
          </Link>
        </Button>
        <div>
          <h1 className="text-xl font-semibold">{t("Scanner imports")}</h1>
          <p className="text-muted-foreground text-sm">
            {t(
              "Imported results are scanner claims: findings start unverified with POE 0, and a re-import never changes an existing finding.",
            )}
          </p>
        </div>
      </div>

      {canImport && (
        <Card>
          <CardContent>
            <form
              className="flex flex-wrap items-end gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                void runImport();
              }}
            >
              <div className="grid gap-2">
                <Label htmlFor="import-file">{t("Scanner file")}</Label>
                <Input
                  id="import-file"
                  ref={inputRef}
                  type="file"
                  className="w-72"
                  accept=".jsonl,.json,.xml,.sarif"
                  onChange={(e) => void pick(e.target.files?.[0] ?? null)}
                />
              </div>
              <div className="grid gap-2">
                <Label>{t("Format")}</Label>
                <Select value={format} onValueChange={(v) => setFormat((v ?? "") as ScanImportFormat | "")}>
                  <SelectTrigger className="w-56" aria-label={t("Format")}>
                    <SelectValue placeholder={t("Select format")} />
                  </SelectTrigger>
                  <SelectContent>
                    {IMPORT_FORMATS.map((f) => (
                      <SelectItem key={f} value={f}>
                        {FORMAT_LABEL[f]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="import-task">{t("Task ID (optional)")}</Label>
                <Input
                  id="import-task"
                  className="w-32"
                  inputMode="numeric"
                  value={taskId}
                  aria-invalid={taskInvalid}
                  onChange={(e) => {
                    setTaskId(e.target.value);
                    setAnchorIds([]);
                  }}
                />
              </div>
              <div className="grid gap-2">
                <Label>{t("Anchor assets (optional)")}</Label>
                <AnchorAssetPicker taskId={anchorTask} value={anchorIds} onChange={setAnchorIds} />
              </div>
              <Button type="submit" disabled={!file || !format || taskInvalid || tooLarge || busy}>
                <UploadIcon /> {busy ? t("Importing…") : t("Import")}
              </Button>
            </form>
            {file && <p className="text-muted-foreground mt-2 text-xs">{fileHint(tooLarge, detected)}</p>}
            {outcome && (
              <div className="mt-3 text-sm">
                <p>
                  {t(
                    "Import #{id}: {records} records, {assets} assets, {new} new findings, {existing} already known, {info} informational",
                    {
                      id: outcome.import.id,
                      records: outcome.import.records,
                      assets: outcome.import.assets,
                      new: outcome.import.findings_new,
                      existing: outcome.import.findings_existing,
                      info: outcome.import.informational,
                    },
                  )}
                </p>
                {outcome.new_findings.length > 0 && (
                  <Link className="text-primary text-sm hover:underline" href={importFindingsHref(outcome.import.id)}>
                    {t("Review the new findings")}
                  </Link>
                )}
                {problems.length > 0 && (
                  <ul className="text-destructive mt-2 list-disc pl-5 text-xs">
                    {problems.slice(0, 10).map((p) => (
                      <li key={p}>{p}</li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <Card className="gap-0">
        <CardHeader>
          <CardTitle className="text-sm">{t("Past imports")}</CardTitle>
        </CardHeader>
        <CardContent className="px-0">
          <ImportsTable imports={imports} loaded={loaded} />
          {total > 0 && (
            <TablePagination
              page={page}
              pageSize={pageSize}
              total={total}
              onPageChange={setPage}
              onPageSizeChange={(size) => {
                setPageSize(size);
                setPage(1);
              }}
              pageSizeOptions={[20, 50, 100]}
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function ImportsTable({ imports, loaded }: { imports: ScanImport[]; loaded: boolean }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>#</TableHead>
          <TableHead>{t("Imported")}</TableHead>
          <TableHead>{t("File")}</TableHead>
          <TableHead>{t("Tool")}</TableHead>
          <TableHead>{t("Task")}</TableHead>
          <TableHead>{t("Fact node")}</TableHead>
          <TableHead className="text-right">{t("Records")}</TableHead>
          <TableHead className="text-right">{t("Assets")}</TableHead>
          <TableHead className="text-right">{t("New findings")}</TableHead>
          <TableHead>{t("Lanes")}</TableHead>
          <TableHead className="text-right">{t("Known")}</TableHead>
          <TableHead className="text-right">{t("Info")}</TableHead>
          <TableHead>{t("By")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {imports.length === 0 && (
          <TableRow>
            <TableCell colSpan={13} className="text-muted-foreground py-8 text-center text-sm">
              {loaded ? t("No imports yet") : t("Loading…")}
            </TableCell>
          </TableRow>
        )}
        {imports.map((im) => (
          <TableRow key={im.id} id={`import-${im.id}`}>
            <TableCell className="font-mono text-xs">{im.id}</TableCell>
            <TableCell className="text-xs whitespace-nowrap">{fmtTime(im.created_at)}</TableCell>
            <TableCell
              className="max-w-56 truncate font-mono text-xs"
              title={`${im.file_name} · sha256 ${im.file_sha256}`}
            >
              {im.file_name || "—"}
              {im.truncated && (
                <Badge variant="outline" className="ml-2">
                  {t("truncated")}
                </Badge>
              )}
            </TableCell>
            <TableCell className="text-sm">
              {im.tool_name || FORMAT_LABEL[im.format] || im.format}
              {im.tool_version && <span className="text-muted-foreground"> {im.tool_version}</span>}
            </TableCell>
            <TableCell className="font-mono text-xs">
              {im.task_id ? <Link href={`/function/tasks/detail?id=${im.task_id}`}>#{im.task_id}</Link> : "—"}
            </TableCell>
            <TableCell className="font-mono text-xs">
              {im.fact_node_id ? (
                im.task_id ? (
                  <Link
                    href={`/function/tasks/detail?id=${im.task_id}`}
                    title={t("Open task #{id}, where this import is recorded as fact node #{node}", {
                      id: im.task_id,
                      node: im.fact_node_id,
                    })}
                  >
                    #{im.fact_node_id}
                  </Link>
                ) : (
                  `#${im.fact_node_id}`
                )
              ) : (
                "—"
              )}
            </TableCell>
            <TableCell className="text-right tabular-nums">{im.records}</TableCell>
            <TableCell className="text-right tabular-nums">{im.assets}</TableCell>
            <TableCell className="text-right tabular-nums">
              {im.findings_new > 0 ? (
                <Link
                  className="text-primary underline underline-offset-2"
                  href={importFindingsHref(im.id)}
                  aria-label={t("Show the {count} new findings of import #{id}", { count: im.findings_new, id: im.id })}
                >
                  {im.findings_new}
                </Link>
              ) : (
                im.findings_new
              )}
            </TableCell>
            <TableCell className="text-xs whitespace-nowrap">
              <LaneCounts importId={im.id} counts={im.lane_counts} />
            </TableCell>
            <TableCell className="text-right tabular-nums">{im.findings_existing}</TableCell>
            <TableCell className="text-right tabular-nums">{im.informational}</TableCell>
            <TableCell className="text-xs">{im.imported_by}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

// LaneCounts renders "SAST 3 · SCA 1", each lane linking to that import's findings in the lane.
function LaneCounts({ importId, counts }: { importId: number; counts: ScanImport["lane_counts"] }) {
  const entries = laneCountEntries(counts);
  if (entries.length === 0) return <span className="text-muted-foreground">—</span>;
  return (
    <>
      {entries.map(({ lane, count }, i) => (
        <React.Fragment key={lane}>
          {i > 0 && <span className="text-muted-foreground"> · </span>}
          <Link className="hover:underline" href={`${importFindingsHref(importId)}&lane=${lane}`}>
            {LANE_META[lane].label} {count}
          </Link>
        </React.Fragment>
      ))}
    </>
  );
}
