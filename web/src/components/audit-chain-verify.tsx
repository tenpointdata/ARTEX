"use client";

import { CircleCheckIcon, ShieldAlertIcon } from "lucide-react";

import { CopyButton } from "@/components/copy-button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { t } from "@/i18n";
import type { AuditVerification } from "@/lib/governance-types";

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="font-medium tabular-nums">{value}</dd>
    </div>
  );
}

// AuditVerifyResult shows the outcome of GET /intercept/history/chain/verify.
// onInspectRow narrows the chain table to the entries of one intercept row.
export function AuditVerifyResult({
  result,
  onInspectRow,
}: {
  result: AuditVerification;
  onInspectRow: (rowId: number) => void;
}) {
  const mismatched = result.rows_mismatched ?? [];
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      {result.ok ? <CircleCheckIcon /> : <ShieldAlertIcon />}
      <AlertTitle className="flex items-center gap-2">
        {result.ok ? t("Chain verified") : t("Chain broken")}
        <Badge variant={result.ok ? "default" : "destructive"}>{result.ok ? t("OK") : t("Broken")}</Badge>
      </AlertTitle>
      <AlertDescription className="flex min-w-0 flex-col gap-3">
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label={t("Entries")} value={result.entries} />
          <Stat label={t("Head seq")} value={result.head_seq} />
          <Stat label={t("Rows checked")} value={result.rows_checked} />
          <Stat label={t("Rows unchained")} value={result.rows_unchained} />
        </dl>
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-muted-foreground text-xs">{t("Head hash")}</span>
          <div className="flex min-w-0 items-center gap-2">
            <code className="min-w-0 break-all font-mono text-xs">{result.head_hash || "—"}</code>
            <CopyButton text={result.head_hash} size="xs" label={t("Copy")} />
          </div>
          <p className="text-muted-foreground text-xs">
            {t(
              "Record the head seq and head hash outside the database, for example in the engagement report. A full rewrite of the chain can only be caught by comparing against that copy.",
            )}
          </p>
        </div>
        {result.ok ? null : (
          <div className="flex min-w-0 flex-col gap-2">
            {result.broken_at || result.problem ? (
              <p className="text-sm">
                {result.broken_at ? t("Broken at seq {seq}", { seq: result.broken_at }) : null}
                {result.broken_at && result.problem ? ": " : null}
                {result.problem}
              </p>
            ) : null}
            {mismatched.length ? (
              <div className="flex min-w-0 flex-col gap-1">
                <span className="text-xs">{t("Intercept rows that differ from their newest chain entry")}</span>
                <div className="flex flex-wrap gap-1">
                  {mismatched.map((id) => (
                    <Button
                      key={id}
                      type="button"
                      size="xs"
                      variant="outline"
                      title={t("Show the chain entries of this row")}
                      onClick={() => onInspectRow(id)}
                    >
                      #{id}
                    </Button>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        )}
      </AlertDescription>
    </Alert>
  );
}
