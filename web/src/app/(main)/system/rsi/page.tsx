"use client";

import * as React from "react";

import Link from "next/link";

import { RefreshCwIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Table, TableBody, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { rsiApi } from "@/lib/rsi-api";
import type { RSIEvidence, RSILoopStatus, RSIProposal } from "@/lib/rsi-types";

import { ActionDialog, type PendingAction, type RSIAction } from "./_components/action-dialog";
import { LoopStatus } from "./_components/loop-status";
import { ProposalRow } from "./_components/proposal-row";

// RSI benchmark gate (RSI.md, server/rsi_loop.go): proposals the loop made, the
// server's verdict over their held-out evidence, and the owner's adopt / reject /
// rollback. Adoption still needs the owner password; the page only carries it.
export default function RSIPage() {
  const canAct = useCan("operate");
  const [status, setStatus] = React.useState<RSILoopStatus | null>(null);
  const [proposals, setProposals] = React.useState<RSIProposal[] | null>(null);
  const [latest, setLatest] = React.useState<Record<string, RSIEvidence>>({});
  const [loadError, setLoadError] = React.useState("");
  const [expanded, setExpanded] = React.useState<number | null>(null);
  const [pending, setPending] = React.useState<PendingAction | null>(null);

  const load = React.useCallback(async () => {
    setLoadError("");
    try {
      const [st, list] = await Promise.all([rsiApi.status(), rsiApi.proposals()]);
      setStatus(st);
      setProposals(list.proposals ?? []);
      setLatest(list.latest_evidence ?? {});
    } catch (err) {
      setLoadError(errorText(err));
      setProposals((p) => p ?? []);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  const act = (p: RSIProposal, kind: RSIAction) =>
    setPending({ kind, proposal: p, evidenceId: latest[String(p.id)]?.id });

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="font-semibold text-xl">{t("Self-improvement gate")}</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            {t(
              "Prompt and lesson edits proposed from live tasks, judged on held-out benchmarks. Adopting one needs a passing verdict and the owner password; rollback is always open.",
            )}{" "}
            <Link href="/system/agents" className="text-primary underline underline-offset-2">
              {t("Agent prompt history")}
            </Link>
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load()}>
          <RefreshCwIcon />
          {t("Refresh")}
        </Button>
      </div>
      <LoopStatus status={status} />
      {loadError && (
        <p role="alert" className="text-destructive text-sm">
          {t("Failed to load proposals: {error}", { error: loadError })}
        </p>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("Proposal")}</TableHead>
            <TableHead>{t("Status")}</TableHead>
            <TableHead>{t("Module")}</TableHead>
            <TableHead className="text-right">{t("Round")}</TableHead>
            <TableHead>{t("Newest verdict")}</TableHead>
            <TableHead>{t("Created by")}</TableHead>
            <TableHead>{t("Updated")}</TableHead>
            <TableHead className="text-right">{t("Actions")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(proposals ?? []).map((p) => (
            <ProposalRow
              key={p.id}
              proposal={p}
              evidence={latest[String(p.id)]}
              loopOn={status?.loop === "on"}
              canAct={canAct}
              expanded={expanded === p.id}
              onToggle={() => setExpanded((x) => (x === p.id ? null : p.id))}
              onAction={(kind) => act(p, kind)}
            />
          ))}
        </TableBody>
      </Table>
      {proposals !== null && proposals.length === 0 && !loadError && (
        <p className="py-6 text-center text-muted-foreground text-sm">{t("No proposals yet.")}</p>
      )}
      <ActionDialog action={pending} onClose={() => setPending(null)} onDone={() => void load()} />
    </div>
  );
}
