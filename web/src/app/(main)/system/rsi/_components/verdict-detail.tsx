"use client";

import * as React from "react";

import Link from "next/link";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { rsiApi } from "@/lib/rsi-api";
import type { RSIEvidence, RSILogEntry, RSIProposal } from "@/lib/rsi-types";

import { promptHistoryHref, rate, when } from "./rsi-format";

interface Props {
  proposal: RSIProposal;
  evidence: RSIEvidence | undefined;
}

// VerdictDetail is the expanded row: the newest verdict's per-gate rates with
// denominators, paired gained/lost, regressions with their targets, and the
// proposal's adoption log.
export function VerdictDetail({ proposal, evidence }: Props) {
  const [log, setLog] = React.useState<RSILogEntry[] | null>(null);
  const [logError, setLogError] = React.useState("");
  const v = evidence?.verdict;
  const history = promptHistoryHref(proposal);

  // Re-read the log when the row changes status (after an action and the list reload).
  // biome-ignore lint/correctness/useExhaustiveDependencies: status is the trigger, not a value read in the effect
  React.useEffect(() => {
    rsiApi
      .proposal(proposal.id)
      .then((d) => setLog(d.log ?? []))
      .catch((err: unknown) => setLogError(errorText(err)));
  }, [proposal.id, proposal.status]);

  return (
    <div className="grid gap-4 py-2 text-sm">
      <div className="grid gap-1">
        <p>
          <span className="text-muted-foreground">{t("Rationale")}: </span>
          {proposal.rationale}
        </p>
        <p className="text-muted-foreground text-xs">
          {t("Proposed from")}: {(proposal.proposed_from ?? []).join(", ") || "—"}
          {proposal.body_sha && <span className="ml-3 font-mono">sha {proposal.body_sha}</span>}
          {proposal.prompt_version_after !== undefined && (
            <span className="ml-3">
              {t("prompt v{before} → v{after}", {
                before: proposal.prompt_version_before ?? "—",
                after: proposal.prompt_version_after,
              })}
            </span>
          )}
          {history && (
            <Link href={history} className="ml-3 text-primary underline underline-offset-2">
              {t("Prompt history")}
            </Link>
          )}
        </p>
      </div>

      {!v ? (
        <p className="text-muted-foreground">{t("No evidence uploaded yet.")}</p>
      ) : (
        <>
          <p className="text-muted-foreground text-xs">
            {t("Evidence #{id} at {at}. Runs: baseline {ba} attempted / {bf} failed, candidate {ca} / {cf}.", {
              id: evidence.id,
              at: when(evidence.created_at),
              ba: v.runs[0][0],
              bf: v.runs[0][1],
              ca: v.runs[1][0],
              cf: v.runs[1][1],
            })}
          </p>
          {(v.reasons ?? []).length > 0 && (
            <ul className="list-disc pl-5 text-destructive">
              {(v.reasons ?? []).map((r) => (
                <li key={r}>{r}</li>
              ))}
            </ul>
          )}
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Gate")}</TableHead>
                <TableHead>{t("Baseline")}</TableHead>
                <TableHead>{t("Candidate")}</TableHead>
                <TableHead className="text-right">{t("Gained")}</TableHead>
                <TableHead className="text-right">{t("Lost")}</TableHead>
                <TableHead className="text-right">{t("Both")}</TableHead>
                <TableHead className="text-right">{t("Neither")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(v.gates ?? []).map((g) => (
                <TableRow key={g.gate}>
                  <TableCell className="font-mono text-xs">{g.gate}</TableCell>
                  <TableCell>{rate(g.baseline)}</TableCell>
                  <TableCell>{rate(g.candidate)}</TableCell>
                  <TableCell className="text-right text-emerald-600">+{g.gained}</TableCell>
                  <TableCell className="text-right text-red-600">−{g.lost}</TableCell>
                  <TableCell className="text-right">{g.both}</TableCell>
                  <TableCell className="text-right">{g.neither}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <p className="text-xs">
            <span className="text-muted-foreground">{t("Targets improved")}: </span>
            {(v.targets_improved ?? []).join(", ") || "—"}
            <span className="ml-1 text-muted-foreground">
              {t("({n} of {total} targets)", {
                n: (v.targets_improved ?? []).length,
                total: (v.targets ?? []).length,
              })}
            </span>
          </p>
          {(v.regressions ?? []).length > 0 && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Regression: module")}</TableHead>
                  <TableHead>{t("Gate")}</TableHead>
                  <TableHead className="text-right">{t("Gained / lost")}</TableHead>
                  <TableHead>{t("Targets")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(v.regressions ?? []).map((r) => (
                  <TableRow key={`${r.module}/${r.gate}`}>
                    <TableCell className="font-mono text-xs">{r.module}</TableCell>
                    <TableCell className="font-mono text-xs">{r.gate}</TableCell>
                    <TableCell className="text-right">
                      +{r.gained} / −{r.lost}
                    </TableCell>
                    <TableCell className="text-xs">{(r.targets ?? []).join(", ")}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </>
      )}

      <div className="grid gap-1">
        <p className="font-medium text-xs">{t("Adoption log")}</p>
        {logError && <p className="text-destructive text-xs">{logError}</p>}
        <ul className="grid gap-0.5 text-muted-foreground text-xs">
          {(log ?? []).map((e) => (
            <li key={e.id}>
              <span className="font-mono">{when(e.at)}</span> · {e.action} · {e.actor}
              {typeof e.detail?.reason === "string" && e.detail.reason && <span> · {e.detail.reason}</span>}
            </li>
          ))}
          {log !== null && log.length === 0 && <li>—</li>}
        </ul>
      </div>
    </div>
  );
}
