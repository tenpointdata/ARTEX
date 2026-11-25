"use client";

import * as React from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { rsiApi } from "@/lib/rsi-api";
import type { RSIProposal, RSIProposalResult } from "@/lib/rsi-types";
import { statusMeta } from "@/lib/status";

import { moduleOf } from "./rsi-format";

export type RSIAction = "adopt" | "reject" | "rollback";

// PendingAction is the dialog's input: what to do to which proposal, and for
// adopt, the evidence row the adoption cites (the newest, which passed).
export interface PendingAction {
  kind: RSIAction;
  proposal: RSIProposal;
  evidenceId?: number;
}

interface Props {
  action: PendingAction | null;
  onClose: () => void;
  onDone: (p: RSIProposal) => void;
}

interface ActionCopy {
  title: string;
  confirm: string;
  field: string;
  describe: (module: string, evidenceId: number | undefined) => string;
  run: (id: number, value: string, evidenceId: number | undefined) => Promise<RSIProposalResult>;
}

const actions: Record<RSIAction, ActionCopy> = {
  adopt: {
    title: t("Adopt proposal #{id}"),
    confirm: t("Adopt"),
    field: t("Owner password"),
    describe: (module, ev) =>
      t("Makes {module} live, citing evidence #{ev}. The server re-checks the verdict and the live prompt.", {
        module,
        ev: ev ?? "—",
      }),
    run: (id, password, ev) => rsiApi.adopt(id, ev ?? 0, password),
  },
  reject: {
    title: t("Reject proposal #{id}"),
    confirm: t("Reject"),
    field: t("Reason (optional)"),
    describe: () => t("Closes the proposal without adopting it."),
    run: (id, reason) => rsiApi.reject(id, reason.trim()),
  },
  rollback: {
    title: t("Roll back proposal #{id}"),
    confirm: t("Roll back"),
    field: t("Reason"),
    describe: (module) => t("Restores what {module} ran before the adoption. Open even with the loop off.", { module }),
    run: (id, reason) => rsiApi.rollback(id, reason.trim()),
  },
};

// ActionDialog is the one confirm step for adopt (owner password), reject and
// rollback (a reason for the log). The second click is its submit.
export function ActionDialog({ action, onClose, onDone }: Props) {
  const [value, setValue] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState("");

  // biome-ignore lint/correctness/useExhaustiveDependencies: action is the trigger, not a value read in the effect
  React.useEffect(() => {
    setValue("");
    setError("");
  }, [action]);

  if (!action) return null;
  const { kind, proposal } = action;
  const c = actions[kind];
  const isAdopt = kind === "adopt";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!action) return;
    setBusy(true);
    setError("");
    try {
      const id = action.proposal.id;
      const res = await actions[action.kind].run(id, value, action.evidenceId);
      toast.success(t("Proposal #{id} is now {status}", { id, status: statusMeta("rsi", res.proposal.status).label }));
      onDone(res.proposal);
      onClose();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{c.title.replace("{id}", String(proposal.id))}</DialogTitle>
            <DialogDescription>{c.describe(moduleOf(proposal), action.evidenceId)}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="rsi-action-value">{c.field}</Label>
            <Input
              id="rsi-action-value"
              type={isAdopt ? "password" : "text"}
              autoComplete={isAdopt ? "current-password" : "off"}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              required={kind !== "reject"}
              autoFocus
            />
          </div>
          {error && (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("Cancel")}
            </Button>
            <Button type="submit" variant={isAdopt ? "default" : "destructive"} disabled={busy}>
              {c.confirm}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
