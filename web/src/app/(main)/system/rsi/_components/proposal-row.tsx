"use client";

import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import type { RSIEvidence, RSIProposal } from "@/lib/rsi-types";

import type { RSIAction } from "./action-dialog";
import { moduleOf, when } from "./rsi-format";
import { VerdictDetail } from "./verdict-detail";

export const PROPOSAL_COLUMNS = 8;

interface Props {
  proposal: RSIProposal;
  evidence: RSIEvidence | undefined;
  loopOn: boolean;
  canAct: boolean;
  expanded: boolean;
  onToggle: () => void;
  onAction: (kind: RSIAction) => void;
}

// VerdictCell summarizes the newest verdict: pass/fail, net paired change and regressions.
function VerdictCell({ evidence }: { evidence: RSIEvidence | undefined }) {
  if (!evidence) return <span className="text-muted-foreground">—</span>;
  const v = evidence.verdict;
  const gained = (v.gates ?? []).reduce((s, g) => s + g.gained, 0);
  const lost = (v.gates ?? []).reduce((s, g) => s + g.lost, 0);
  const regressions = (v.regressions ?? []).length;
  return (
    <span className="flex flex-wrap items-center gap-2 text-xs">
      <StatusBadge domain="rsi" value={evidence.passed ? "passed" : "failed"} />
      <span className="text-muted-foreground">#{evidence.id}</span>
      <span>
        <span className="text-emerald-600">+{gained}</span> / <span className="text-red-600">−{lost}</span>
      </span>
      {regressions > 0 && <span className="text-red-600">{t("{n} regressions", { n: regressions })}</span>}
    </span>
  );
}

// ProposalRow is one proposal with its inline actions. Adopt shows only when the
// newest evidence passed and the loop is on; rollback only for an adopted one.
export function ProposalRow({ proposal: p, evidence, loopOn, canAct, expanded, onToggle, onAction }: Props) {
  const canAdopt = loopOn && p.status === "passed" && evidence?.passed === true;
  const canReject = p.status === "proposed" || p.status === "passed" || p.status === "failed";
  const canRollback = p.status === "adopted";
  const Chevron = expanded ? ChevronDownIcon : ChevronRightIcon;
  return (
    <>
      <TableRow className="cursor-pointer" onClick={onToggle} aria-expanded={expanded}>
        <TableCell className="font-mono text-xs">
          <span className="flex items-center gap-1">
            <Chevron className="size-3.5 text-muted-foreground" />#{p.id}
          </span>
        </TableCell>
        <TableCell>
          <StatusBadge domain="rsi" value={p.status} />
        </TableCell>
        <TableCell className="font-mono text-xs">{moduleOf(p)}</TableCell>
        <TableCell className="text-right">{p.round}</TableCell>
        <TableCell>
          <VerdictCell evidence={evidence} />
        </TableCell>
        <TableCell className="text-xs">{p.created_by}</TableCell>
        <TableCell className="whitespace-nowrap text-xs">{when(p.updated_at)}</TableCell>
        <TableCell className="text-right" onClick={(e) => e.stopPropagation()}>
          {canAct && (
            <span className="inline-flex gap-1">
              {canAdopt && (
                <Button size="sm" onClick={() => onAction("adopt")}>
                  {t("Adopt")}
                </Button>
              )}
              {canRollback && (
                <Button size="sm" variant="destructive" onClick={() => onAction("rollback")}>
                  {t("Roll back")}
                </Button>
              )}
              {canReject && (
                <Button size="sm" variant="outline" onClick={() => onAction("reject")}>
                  {t("Reject")}
                </Button>
              )}
            </span>
          )}
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow className="hover:bg-transparent">
          <TableCell colSpan={PROPOSAL_COLUMNS} className="whitespace-normal bg-muted/30">
            <VerdictDetail proposal={p} evidence={evidence} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}
